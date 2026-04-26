package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"stock-backtester/analysis"
	"stock-backtester/data"
	"stock-backtester/engine"
	"stock-backtester/output"
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
	oddLot := flag.Bool("odd-lot", false, "allow odd-lot trading (TW: 1-share lots instead of 1000)")
	flag.Parse()

	if err := run(*symbol, *startStr, *endStr, *stratName, *short, *long,
		*rsiPeriod, *rsiLow, *rsiHigh, *cash, *outRoot, *oddLot); err != nil {
		log.Fatalf("backtest failed: %v", err)
	}
}

func run(symbol, startStr, endStr, stratName string, short, long, rsiPeriod int,
	rsiLow, rsiHigh, cash float64, outRoot string, oddLot bool) error {

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

	outDir := filepath.Join(outRoot, fmt.Sprintf("%s_%s_%s",
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
	fmt.Printf("  Output         : %s\n", outDir)
}
