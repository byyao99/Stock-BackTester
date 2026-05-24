package papertrading

import (
	"fmt"
	"io"
	"os"
	"time"

	"stock-backtester/data"
	"stock-backtester/engine"
	"stock-backtester/strategy"
)

// settledMinAge is the minimum age (now - latestBar.Time) for a bar to count
// as "session closed". 6 hours covers normal TW (4.5h session) and US (6.5h
// session) days with margin: a US bar at session open + 6h means the regular
// session has just ended.
const settledMinAge = 6 * time.Hour

// Options configures one daily tick.
type Options struct {
	Symbol      string
	Strategy    strategy.Strategy
	StratParams StratParams
	Cash        float64
	Market      data.Market
	AccountFile string
	Notifier    Notifier

	// Output sink for the daily report. Defaults to os.Stdout.
	Out io.Writer

	// Test hooks. Nil falls back to time.Now and data.FetchLatest.
	Clock   func() time.Time
	Fetcher func(symbol string, days int) ([]data.Bar, error)
}

// RunDailyUpdate executes one paper-trading tick: load account, fetch latest
// bars, fill any pending order at the new bar's open, mark-to-market, decide
// today's signal, persist, notify, and print the daily report.
func RunDailyUpdate(opts Options) error {
	if opts.Symbol == "" {
		return fmt.Errorf("Options.Symbol is required")
	}
	if opts.Strategy == nil {
		return fmt.Errorf("Options.Strategy is required")
	}
	if opts.AccountFile == "" {
		return fmt.Errorf("Options.AccountFile is required")
	}
	if opts.Notifier == nil {
		opts.Notifier = ConsoleNotifier{}
	}
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	now := time.Now
	if opts.Clock != nil {
		now = opts.Clock
	}
	fetcher := data.FetchLatest
	if opts.Fetcher != nil {
		fetcher = opts.Fetcher
	}

	acc, err := LoadAccount(opts.AccountFile)
	if err != nil {
		return err
	}
	if acc != nil {
		if err := acc.VerifyIdentity(opts.Symbol, opts.StratParams); err != nil {
			return err
		}
	} else {
		acc = NewAccount(opts.Symbol, opts.Market.Name, opts.StratParams, opts.Cash, now().UTC())
	}

	days := opts.Strategy.MinBars() * 2
	if days < 180 {
		days = 180
	}
	bars, err := fetcher(opts.Symbol, days)
	if err != nil {
		return fmt.Errorf("fetch latest: %w", err)
	}
	if len(bars) < opts.Strategy.MinBars() {
		return fmt.Errorf("fetched only %d bars, strategy %s needs %d",
			len(bars), opts.Strategy.Name(), opts.Strategy.MinBars())
	}
	latestBar := bars[len(bars)-1]
	latestDate := truncDay(latestBar.Time)

	// In-session guard: bar must be old enough to be considered settled.
	if now().UTC().Sub(latestBar.Time) < settledMinAge {
		fmt.Fprintf(opts.Out, "Latest bar %s appears in-session (age < %s); skipping update.\n",
			latestBar.Time.Format(reportDateLayout), settledMinAge)
		fmt.Fprintln(opts.Out, FormatDailyReport(acc, latestBar))
		return nil
	}

	// Idempotency: same trading date already processed.
	if !acc.LastProcessedDate.IsZero() && !latestDate.After(acc.LastProcessedDate) {
		fmt.Fprintf(opts.Out, "Already processed through %s; no new bar.\n",
			acc.LastProcessedDate.Format(reportDateLayout))
		fmt.Fprintln(opts.Out, FormatDailyReport(acc, latestBar))
		return nil
	}

	// t+1 open execution of yesterday's pending signal.
	if acc.Pending != nil && latestDate.After(truncDay(acc.Pending.DecidedOn)) {
		if tr := ExecutePending(acc, latestBar, opts.Market); tr != nil {
			_ = opts.Notifier.Notify(Event{
				Symbol: opts.Symbol,
				Date:   latestBar.Time,
				Kind:   EventTradeFilled,
				Side:   tr.Side,
				Price:  tr.Price,
				Shares: tr.Shares,
			})
		}
		acc.Pending = nil
	}

	// Mark-to-market on the new bar's close.
	posValue := float64(acc.Shares) * latestBar.Close
	acc.DailyEquity = append(acc.DailyEquity, engine.EquityPoint{
		Time:          latestBar.Time,
		Cash:          acc.Cash,
		Shares:        acc.Shares,
		PositionValue: posValue,
		Total:         acc.Cash + posValue,
	})

	// t-close decision for today.
	signals := opts.Strategy.GenerateSignals(bars)
	today := signals[len(signals)-1]
	switch {
	case today == strategy.SignalBuy && acc.Shares == 0:
		acc.Pending = &PendingSignal{
			DecidedOn: latestDate,
			Side:      engine.SideBuy,
			RefClose:  latestBar.Close,
		}
		_ = opts.Notifier.Notify(Event{
			Symbol: opts.Symbol, Date: latestBar.Time, Kind: EventSignalQueued,
			Side: engine.SideBuy, Price: latestBar.Close,
		})
	case today == strategy.SignalSell && acc.Shares > 0:
		acc.Pending = &PendingSignal{
			DecidedOn: latestDate,
			Side:      engine.SideSell,
			RefClose:  latestBar.Close,
		}
		_ = opts.Notifier.Notify(Event{
			Symbol: opts.Symbol, Date: latestBar.Time, Kind: EventSignalQueued,
			Side: engine.SideSell, Price: latestBar.Close,
		})
	default:
		acc.Pending = nil
	}

	acc.LastProcessedDate = latestDate
	acc.UpdatedAt = now().UTC()

	if err := SaveAccount(opts.AccountFile, acc); err != nil {
		return err
	}
	fmt.Fprintln(opts.Out, FormatDailyReport(acc, latestBar))
	return nil
}

func truncDay(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}
