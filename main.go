package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"stock-backtester/analysis"
	"stock-backtester/data"
	"stock-backtester/engine"
	"stock-backtester/output"
	papertrading "stock-backtester/paper_trading"
	"stock-backtester/strategy"
)

const dateLayout = "2006-01-02"

func main() {
	symbol := flag.String("symbol", "", "ticker symbol (e.g. 2330.TW or AAPL)")
	startStr := flag.String("start", "", "start date YYYY-MM-DD")
	endStr := flag.String("end", "", "end date YYYY-MM-DD")
	stratName := flag.String("strategy", "", "strategy name: sma | rsi")
	short := flag.Int("short", 20, "SMA short period")
	long := flag.Int("long", 60, "SMA long period")
	rsiPeriod := flag.Int("rsi-period", 14, "RSI period")
	rsiLow := flag.Float64("rsi-low", 30, "RSI oversold threshold")
	rsiHigh := flag.Float64("rsi-high", 70, "RSI overbought threshold")
	cash := flag.Float64("cash", 1_000_000, "initial cash")
	outRoot := flag.String("out", "results", "output root directory")
	noOutput := flag.Bool("no-output", false, "skip writing trades.csv / equity_curve.csv / summary.json (terminal only)")
	oddLot := flag.Bool("odd-lot", false, "allow odd-lot trading (TW: 1-share lots instead of 1000)")
	mode := flag.String("mode", "backtest", "execution mode: backtest | paper | holdcheck")
	accountFile := flag.String("account-file", "", "paper-mode account JSON path (default paper_accounts/{symbol}_{strategy}.json)")
	flag.Parse()

	switch strings.ToLower(*mode) {
	case "backtest":
		if err := run(*symbol, *startStr, *endStr, *stratName, *short, *long,
			*rsiPeriod, *rsiLow, *rsiHigh, *cash, *outRoot, *oddLot, *noOutput); err != nil {
			log.Fatalf("backtest failed: %v", err)
		}
	case "paper":
		if err := runPaper(*symbol, *stratName, *short, *long,
			*rsiPeriod, *rsiLow, *rsiHigh, *cash, *accountFile, *oddLot); err != nil {
			log.Fatalf("paper trading failed: %v", err)
		}
	case "holdcheck":
		if err := runHoldCheck(*symbol, *startStr, *endStr, *cash, *oddLot); err != nil {
			log.Fatalf("hold check failed: %v", err)
		}
	default:
		log.Fatalf("unknown --mode %q (want backtest | paper | holdcheck)", *mode)
	}
}

func run(symbol, startStr, endStr, stratName string, short, long, rsiPeriod int,
	rsiLow, rsiHigh, cash float64, outRoot string, oddLot, noOutput bool) error {

	if symbol == "" || startStr == "" || endStr == "" || stratName == "" {
		flag.Usage()
		return fmt.Errorf("--symbol, --start, --end, --strategy are required")
	}
	start, err := time.Parse(dateLayout, startStr)
	if err != nil {
		return fmt.Errorf("invalid --start: %w", err)
	}
	end, err := time.Parse(dateLayout, endStr)
	if err != nil {
		return fmt.Errorf("invalid --end: %w", err)
	}
	if !end.After(start) {
		return fmt.Errorf("--end must be after --start")
	}

	strat, err := buildStrategy(stratName, short, long, rsiPeriod, rsiLow, rsiHigh)
	if err != nil {
		return err
	}

	market := data.MarketOf(symbol)
	if oddLot && market.LotSize > 1 {
		market.LotSize = 1
		market.Name += "-oddlot"
	}
	fmt.Printf("Loading %s [%s → %s] (market=%s)\n", symbol, startStr, endStr, market.Name)
	bars, fromCache, err := data.LoadOrFetch(symbol, start, end)
	if err != nil {
		return err
	}
	source := "yahoo"
	if fromCache {
		source = "cache"
	}
	fmt.Printf("Got %d bars from %s\n", len(bars), source)

	if min := strat.MinBars(); len(bars) < min {
		return fmt.Errorf("strategy %s needs at least %d bars to produce an executable signal, but the date range only has %d (extend --start / --end)",
			strat.Name(), min, len(bars))
	}

	signals := strat.GenerateSignals(bars)
	cfg := engine.Config{InitialCash: cash, Market: market}
	res := engine.Run(bars, signals, cfg)
	fmt.Printf("Executed %d trades\n", len(res.Trades))
	if len(res.Trades) == 0 {
		fmt.Println("Note: 0 trades. Likely causes:")
		fmt.Println("  - no crossover/threshold event in this date range (try extending --start)")
		fmt.Println("  - cash too small to buy 1 lot at the prices in this period (try larger --cash)")
		fmt.Println("  - strategy parameters too strict (try different --short/--long or --rsi-* values)")
	}

	metrics := analysis.Compute(res.EquityCurve, res.Trades, cash)
	bhReturn := engine.BuyHold(bars, cash, market)
	var alpha *float64
	if bhReturn != nil {
		a := metrics.TotalReturn - *bhReturn
		alpha = &a
	}
	summary := analysis.Summary{
		Symbol:         symbol,
		Strategy:       strat.Name(),
		Market:         market.Name,
		Start:          bars[0].Time,
		End:            bars[len(bars)-1].Time,
		TradingDays:    len(bars),
		InitialCash:    cash,
		FinalEquity:    metrics.FinalEquity,
		TotalReturn:    metrics.TotalReturn,
		AnnualReturn:   metrics.AnnualReturn,
		MaxDrawdown:    metrics.MaxDrawdown,
		SharpeRatio:    metrics.SharpeRatio,
		WinRate:        metrics.WinRate,
		ProfitFactor:   metrics.ProfitFactor,
		TradeCount:     len(res.Trades),
		RoundTripCount: metrics.RoundTripCount,
		BuyHoldReturn:  bhReturn,
		Alpha:          alpha,
	}

	outDir := ""
	if !noOutput {
		outDir = filepath.Join(outRoot, fmt.Sprintf("%s_%s_%s",
			safeName(symbol), strat.Name(), time.Now().Format("20060102_150405")))
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return err
		}
		if err := output.WriteTrades(filepath.Join(outDir, "trades.csv"), res.Trades); err != nil {
			return err
		}
		if err := output.WriteEquity(filepath.Join(outDir, "equity_curve.csv"), res.EquityCurve); err != nil {
			return err
		}
		if err := output.WriteSummary(filepath.Join(outDir, "summary.json"), summary); err != nil {
			return err
		}
	}

	printSummary(summary, outDir)
	printCurrentMarket(symbol, bars[len(bars)-1], strat)
	return nil
}

func printCurrentMarket(symbol string, backtestEnd data.Bar, strat strategy.Strategy) {
	fmt.Println()
	fmt.Println("===== Current Market =====")

	days := strat.MinBars() * 2
	if days < 180 {
		days = 180
	}
	latest, err := data.FetchLatest(symbol, days)
	if err != nil {
		fmt.Printf("  (could not fetch live data: %v)\n", err)
		return
	}
	last := latest[len(latest)-1]

	fmt.Printf("  Last Close     : %.2f (%s)\n", last.Close, last.Time.Format(dateLayout))

	delta := last.Close - backtestEnd.Close
	pct := delta / backtestEnd.Close * 100
	fmt.Printf("  vs Backtest End: %+.2f%% (was %.2f on %s)\n",
		pct, backtestEnd.Close, backtestEnd.Time.Format(dateLayout))

	if len(latest) < strat.MinBars() {
		fmt.Printf("  Latest Signal  : n/a (only %d bars fetched, need %d)\n",
			len(latest), strat.MinBars())
		return
	}
	sigs := strat.GenerateSignals(latest)
	latestSig := sigs[len(sigs)-1]
	fmt.Printf("  Latest Signal  : %s (decided after %s close)\n",
		latestSig.String(), last.Time.Format(dateLayout))

	if latestSig == strategy.SignalHold {
		for i := len(sigs) - 1; i >= 0; i-- {
			if sigs[i] != strategy.SignalHold {
				fmt.Printf("  Last Crossover : %s on %s (%d trading days ago)\n",
					sigs[i].String(), latest[i].Time.Format(dateLayout), len(sigs)-1-i)
				break
			}
		}
	}

	fmt.Println("  Note: forward signal only — not validated on data after backtest --end.")
}

func runPaper(symbol, stratName string, short, long, rsiPeriod int,
	rsiLow, rsiHigh, cash float64, accountFile string, oddLot bool) error {

	if symbol == "" || stratName == "" {
		return fmt.Errorf("--symbol and --strategy are required for paper mode")
	}
	strat, err := buildStrategy(stratName, short, long, rsiPeriod, rsiLow, rsiHigh)
	if err != nil {
		return err
	}
	market := data.MarketOf(symbol)
	if oddLot && market.LotSize > 1 {
		market.LotSize = 1
		market.Name += "-oddlot"
	}
	if accountFile == "" {
		accountFile = papertrading.DefaultAccountPath("paper_accounts", symbol, strat.Name())
	}
	params := paramsFromFlags(stratName, short, long, rsiPeriod, rsiLow, rsiHigh, oddLot)

	fmt.Printf("Paper trading %s (market=%s, strategy=%s, account=%s)\n",
		symbol, market.Name, strat.Name(), accountFile)

	return papertrading.RunDailyUpdate(papertrading.Options{
		Symbol:      symbol,
		Strategy:    strat,
		StratParams: params,
		Cash:        cash,
		Market:      market,
		AccountFile: accountFile,
		Notifier:    papertrading.ConsoleNotifier{},
	})
}

// runHoldCheck answers "if I bought on --start and held to --end (default
// today), how much would I have?" using the same lot/fee/tax rules as the
// backtest engine. Writes nothing to disk — terminal output only.
func runHoldCheck(symbol, startStr, endStr string, cash float64, oddLot bool) error {
	if symbol == "" || startStr == "" {
		return fmt.Errorf("--symbol and --start are required for holdcheck mode")
	}
	start, err := time.Parse(dateLayout, startStr)
	if err != nil {
		return fmt.Errorf("invalid --start: %w", err)
	}
	end := time.Now().UTC().AddDate(0, 0, 1)
	if endStr != "" {
		end, err = time.Parse(dateLayout, endStr)
		if err != nil {
			return fmt.Errorf("invalid --end: %w", err)
		}
	}
	if !end.After(start) {
		return fmt.Errorf("--end must be after --start")
	}

	market := data.MarketOf(symbol)
	if oddLot && market.LotSize > 1 {
		market.LotSize = 1
		market.Name += "-oddlot"
	}

	// Fetch fresh (skip cache) — holdcheck queries usually end at "today" and
	// shouldn't litter data/cache/ with a new file per day.
	bars, err := data.Fetch(symbol, start, end)
	if err != nil {
		return err
	}
	if len(bars) == 0 {
		return fmt.Errorf("no bars returned for %s in [%s, %s]", symbol, startStr, endStr)
	}

	entry, exit := bars[0], bars[len(bars)-1]
	shares := engine.MaxBuyShares(cash, entry.Open, market)
	if shares == 0 {
		return fmt.Errorf("initial cash %.2f insufficient for 1 lot at entry %.2f (lot size %d)",
			cash, entry.Open, market.LotSize)
	}
	_, buyFee, buyTotal := engine.BuyCost(entry.Open, shares, market)
	leftover := cash - buyTotal

	mtmEquity := leftover + float64(shares)*exit.Close
	mtmReturn := mtmEquity/cash - 1

	_, sellFee, sellTax, sellNet := engine.SellProceeds(exit.Close, shares, market)
	realizedEquity := leftover + sellNet
	realizedReturn := realizedEquity/cash - 1

	days := len(bars)
	years := float64(days) / 252.0
	annualized := math.Pow(1+realizedReturn, 1.0/years) - 1

	fmt.Println()
	fmt.Println("===== Buy & Hold Check =====")
	fmt.Printf("  Symbol         : %s (%s)\n", symbol, market.Name)
	fmt.Printf("  Period         : %s → %s (%d trading days, ~%.2f years)\n",
		entry.Time.Format(dateLayout), exit.Time.Format(dateLayout), days, years)
	fmt.Printf("  Initial Cash   : %.2f %s\n", cash, market.Currency)
	fmt.Printf("  Entry          : %d shares @ %.2f (cost %.2f incl. fee %.2f)\n",
		shares, entry.Open, buyTotal, buyFee)
	fmt.Printf("  Leftover Cash  : %.2f\n", leftover)
	fmt.Printf("  Last Close     : %.2f (%s)\n", exit.Close, exit.Time.Format(dateLayout))
	fmt.Printf("  Mark-to-Market : %.2f (%+.2f%%)\n", mtmEquity, mtmReturn*100)
	fmt.Printf("  If Sold Today  : %.2f net (%+.2f%%, sell fee %.2f + tax %.2f)\n",
		realizedEquity, realizedReturn*100, sellFee, sellTax)
	fmt.Printf("  Annualized     : %+.2f%% (realized basis)\n", annualized*100)
	return nil
}

func paramsFromFlags(name string, short, long, rsiPeriod int, rsiLow, rsiHigh float64, oddLot bool) papertrading.StratParams {
	p := papertrading.StratParams{Name: strings.ToLower(name), OddLot: oddLot}
	switch p.Name {
	case "sma":
		p.Short = short
		p.Long = long
	case "rsi":
		p.RSIPeriod = rsiPeriod
		p.RSILow = rsiLow
		p.RSIHigh = rsiHigh
	}
	return p
}

func buildStrategy(name string, short, long, rsiPeriod int, rsiLow, rsiHigh float64) (strategy.Strategy, error) {
	switch strings.ToLower(name) {
	case "sma":
		if short <= 0 || long <= 0 || short >= long {
			return nil, fmt.Errorf("--short (%d) must be < --long (%d)", short, long)
		}
		return strategy.NewSMACrossover(short, long), nil
	case "rsi":
		if rsiPeriod <= 0 || rsiLow <= 0 || rsiHigh <= rsiLow || rsiHigh >= 100 {
			return nil, fmt.Errorf("invalid RSI params: period=%d low=%.1f high=%.1f", rsiPeriod, rsiLow, rsiHigh)
		}
		return strategy.NewRSI(rsiPeriod, rsiLow, rsiHigh), nil
	default:
		return nil, fmt.Errorf("unknown strategy %q (want sma|rsi)", name)
	}
}

func safeName(s string) string {
	r := strings.NewReplacer("/", "_", " ", "_", ".", "_")
	return r.Replace(s)
}

func printSummary(s analysis.Summary, outDir string) {
	fmt.Println()
	fmt.Println("===== Backtest Summary =====")
	fmt.Printf("  Symbol         : %s (%s)\n", s.Symbol, s.Market)
	fmt.Printf("  Strategy       : %s\n", s.Strategy)
	fmt.Printf("  Period         : %s → %s (%d trading days)\n",
		s.Start.Format(dateLayout), s.End.Format(dateLayout), s.TradingDays)
	fmt.Printf("  Initial Cash   : %.2f\n", s.InitialCash)
	fmt.Printf("  Final Equity   : %.2f\n", s.FinalEquity)
	fmt.Printf("  Total Return   : %.2f%%\n", s.TotalReturn*100)
	fmt.Printf("  Annual Return  : %.2f%%\n", s.AnnualReturn*100)
	if s.BuyHoldReturn != nil {
		verdict := "BEAT"
		if *s.Alpha < 0 {
			verdict = "LOST TO"
		} else if *s.Alpha == 0 {
			verdict = "TIED"
		}
		fmt.Printf("  Buy & Hold     : %.2f%% (strategy %s B&H by %+.2f%%)\n",
			*s.BuyHoldReturn*100, verdict, *s.Alpha*100)
	} else {
		fmt.Printf("  Buy & Hold     : n/a (cash insufficient for 1 lot at entry)\n")
	}
	fmt.Printf("  Max Drawdown   : %.2f%%\n", s.MaxDrawdown*100)
	fmt.Printf("  Sharpe Ratio   : %.3f\n", s.SharpeRatio)
	fmt.Printf("  Win Rate       : %.2f%% (%d round trips)\n", s.WinRate*100, s.RoundTripCount)
	if s.ProfitFactor != nil {
		fmt.Printf("  Profit Factor  : %.3f\n", *s.ProfitFactor)
	} else {
		fmt.Printf("  Profit Factor  : n/a (no completed losing trades)\n")
	}
	fmt.Printf("  Trade Count    : %d\n", s.TradeCount)
	if outDir != "" {
		fmt.Printf("  Output         : %s\n", outDir)
	} else {
		fmt.Printf("  Output         : (skipped, --no-output)\n")
	}
}
