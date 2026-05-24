package papertrading

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"stock-backtester/data"
	"stock-backtester/engine"
	"stock-backtester/strategy"
)

// fakeStrategy emits a fixed signal as the last element of its output and
// HOLD elsewhere. Used to drive RunDailyUpdate deterministically.
type fakeStrategy struct {
	name       string
	minBars    int
	lastSignal strategy.Signal
}

func (f *fakeStrategy) Name() string { return f.name }
func (f *fakeStrategy) MinBars() int { return f.minBars }
func (f *fakeStrategy) GenerateSignals(bars []data.Bar) []strategy.Signal {
	out := make([]strategy.Signal, len(bars))
	if len(bars) > 0 {
		out[len(out)-1] = f.lastSignal
	}
	return out
}

func makeBars(n int, start time.Time, openPrice, closePrice float64) []data.Bar {
	out := make([]data.Bar, n)
	for i := 0; i < n; i++ {
		out[i] = data.Bar{
			Time:  start.AddDate(0, 0, i),
			Open:  openPrice,
			High:  closePrice + 1,
			Low:   openPrice - 1,
			Close: closePrice,
		}
	}
	return out
}

func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

func fixedFetcher(bars []data.Bar) func(symbol string, days int) ([]data.Bar, error) {
	return func(symbol string, days int) ([]data.Bar, error) { return bars, nil }
}

func baseOpts(t *testing.T, lastSignal strategy.Signal, bars []data.Bar, now time.Time) Options {
	t.Helper()
	dir := t.TempDir()
	return Options{
		Symbol:      "AAPL",
		Strategy:    &fakeStrategy{name: "sma_20_60", minBars: 5, lastSignal: lastSignal},
		StratParams: StratParams{Name: "sma", Short: 20, Long: 60},
		Cash:        10_000,
		Market:      data.MarketOf("AAPL"),
		AccountFile: filepath.Join(dir, "acc.json"),
		Notifier:    ConsoleNotifier{Out: &bytes.Buffer{}},
		Out:         &bytes.Buffer{},
		Clock:       fixedClock(now),
		Fetcher:     fixedFetcher(bars),
	}
}

func TestRunDailyUpdateBuySignalFirstRun(t *testing.T) {
	bars := makeBars(10, time.Date(2026, 4, 13, 13, 30, 0, 0, time.UTC), 100, 102)
	now := bars[len(bars)-1].Time.Add(8 * time.Hour)

	opts := baseOpts(t, strategy.SignalBuy, bars, now)
	if err := RunDailyUpdate(opts); err != nil {
		t.Fatalf("RunDailyUpdate: %v", err)
	}

	acc, err := LoadAccount(opts.AccountFile)
	if err != nil {
		t.Fatal(err)
	}
	if acc == nil {
		t.Fatal("account not persisted")
	}
	if acc.Pending == nil || acc.Pending.Side != engine.SideBuy {
		t.Errorf("expected pending BUY, got %+v", acc.Pending)
	}
	if len(acc.Trades) != 0 {
		t.Errorf("no trades expected on first run, got %d", len(acc.Trades))
	}
	if len(acc.DailyEquity) != 1 {
		t.Errorf("expected 1 EquityPoint, got %d", len(acc.DailyEquity))
	}
	if acc.Cash != 10_000 || acc.Shares != 0 {
		t.Errorf("position changed unexpectedly: cash=%v shares=%d", acc.Cash, acc.Shares)
	}
	if !acc.LastProcessedDate.Equal(truncDay(bars[len(bars)-1].Time)) {
		t.Errorf("LastProcessedDate: got %v want %v", acc.LastProcessedDate, truncDay(bars[len(bars)-1].Time))
	}
}

func TestRunDailyUpdateIdempotent(t *testing.T) {
	bars := makeBars(10, time.Date(2026, 4, 13, 13, 30, 0, 0, time.UTC), 100, 102)
	now := bars[len(bars)-1].Time.Add(8 * time.Hour)

	opts := baseOpts(t, strategy.SignalBuy, bars, now)
	if err := RunDailyUpdate(opts); err != nil {
		t.Fatal(err)
	}
	if err := RunDailyUpdate(opts); err != nil {
		t.Fatalf("second run: %v", err)
	}
	acc, err := LoadAccount(opts.AccountFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(acc.DailyEquity) != 1 {
		t.Errorf("EquityPoint duplicated: got %d want 1", len(acc.DailyEquity))
	}
	if len(acc.Trades) != 0 {
		t.Errorf("Trades unexpectedly appeared: %d", len(acc.Trades))
	}
}

func TestRunDailyUpdateNextDayFillsPending(t *testing.T) {
	day1 := time.Date(2026, 4, 13, 13, 30, 0, 0, time.UTC)
	bars1 := makeBars(10, day1, 100, 102)
	now1 := bars1[len(bars1)-1].Time.Add(8 * time.Hour)
	opts := baseOpts(t, strategy.SignalBuy, bars1, now1)

	if err := RunDailyUpdate(opts); err != nil {
		t.Fatal(err)
	}

	bars2 := append([]data.Bar{}, bars1...)
	bars2 = append(bars2, data.Bar{
		Time: day1.AddDate(0, 0, 10), Open: 105, High: 106, Low: 104, Close: 107,
	})
	now2 := bars2[len(bars2)-1].Time.Add(8 * time.Hour)
	opts.Strategy = &fakeStrategy{name: "sma_20_60", minBars: 5, lastSignal: strategy.SignalHold}
	opts.Fetcher = fixedFetcher(bars2)
	opts.Clock = fixedClock(now2)

	if err := RunDailyUpdate(opts); err != nil {
		t.Fatal(err)
	}

	acc, err := LoadAccount(opts.AccountFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(acc.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(acc.Trades))
	}
	tr := acc.Trades[0]
	if tr.Side != engine.SideBuy || tr.Price != 105 {
		t.Errorf("trade detail: got %+v", tr)
	}
	if acc.Pending != nil {
		t.Errorf("pending not cleared: %+v", acc.Pending)
	}
	if acc.Shares == 0 {
		t.Errorf("expected long position after BUY fill")
	}
	if len(acc.DailyEquity) != 2 {
		t.Errorf("expected 2 EquityPoints, got %d", len(acc.DailyEquity))
	}
}

func TestRunDailyUpdateRepeatBuyWhenLongIsNoOp(t *testing.T) {
	bars := makeBars(10, time.Date(2026, 4, 13, 13, 30, 0, 0, time.UTC), 100, 102)
	now := bars[len(bars)-1].Time.Add(8 * time.Hour)

	opts := baseOpts(t, strategy.SignalBuy, bars, now)
	dir := filepath.Dir(opts.AccountFile)
	preset := NewAccount("AAPL", "US", StratParams{Name: "sma", Short: 20, Long: 60}, 10_000, now)
	preset.Cash = 5_000
	preset.Shares = 50
	preset.AvgCost = 100
	if err := SaveAccount(filepath.Join(dir, "acc.json"), preset); err != nil {
		t.Fatal(err)
	}

	if err := RunDailyUpdate(opts); err != nil {
		t.Fatal(err)
	}
	acc, err := LoadAccount(opts.AccountFile)
	if err != nil {
		t.Fatal(err)
	}
	if acc.Pending != nil {
		t.Errorf("BUY signal while long must not queue, got %+v", acc.Pending)
	}
	if acc.Shares != 50 {
		t.Errorf("position changed: got %d want 50", acc.Shares)
	}
}

func TestRunDailyUpdateInSessionGuard(t *testing.T) {
	bars := makeBars(10, time.Date(2026, 4, 13, 13, 30, 0, 0, time.UTC), 100, 102)
	now := bars[len(bars)-1].Time.Add(2 * time.Hour) // < settledMinAge

	var stdout bytes.Buffer
	opts := baseOpts(t, strategy.SignalBuy, bars, now)
	opts.Out = &stdout

	if err := RunDailyUpdate(opts); err != nil {
		t.Fatal(err)
	}
	acc, err := LoadAccount(opts.AccountFile)
	if err != nil {
		t.Fatal(err)
	}
	if acc != nil {
		t.Errorf("account should not be persisted on in-session skip, got %+v", acc)
	}
	if !strings.Contains(stdout.String(), "in-session") {
		t.Errorf("expected in-session message, got: %s", stdout.String())
	}
}

func TestRunDailyUpdateStrategyMismatchFails(t *testing.T) {
	bars := makeBars(10, time.Date(2026, 4, 13, 13, 30, 0, 0, time.UTC), 100, 102)
	now := bars[len(bars)-1].Time.Add(8 * time.Hour)
	opts := baseOpts(t, strategy.SignalHold, bars, now)

	if err := RunDailyUpdate(opts); err != nil {
		t.Fatal(err)
	}
	opts.StratParams = StratParams{Name: "sma", Short: 10, Long: 40}
	err := RunDailyUpdate(opts)
	if err == nil {
		t.Fatalf("expected mismatch error")
	}
	if !strings.Contains(err.Error(), "strategy") {
		t.Errorf("error should mention strategy mismatch, got: %v", err)
	}
}

func TestRunDailyUpdateSellWhenFlatNoPending(t *testing.T) {
	bars := makeBars(10, time.Date(2026, 4, 13, 13, 30, 0, 0, time.UTC), 100, 102)
	now := bars[len(bars)-1].Time.Add(8 * time.Hour)

	opts := baseOpts(t, strategy.SignalSell, bars, now)
	if err := RunDailyUpdate(opts); err != nil {
		t.Fatal(err)
	}
	acc, err := LoadAccount(opts.AccountFile)
	if err != nil {
		t.Fatal(err)
	}
	if acc.Pending != nil {
		t.Errorf("SELL while flat must not queue, got %+v", acc.Pending)
	}
}
