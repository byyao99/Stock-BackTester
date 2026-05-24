package papertrading

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"stock-backtester/data"
	"stock-backtester/engine"
)

func TestFormatDailyReportFlat(t *testing.T) {
	acc := NewAccount("AAPL", "US", StratParams{Name: "sma", Short: 20, Long: 60}, 10_000, time.Now())
	acc.DailyEquity = []engine.EquityPoint{
		{Time: time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC), Total: 10_000},
		{Time: time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC), Total: 10_000},
	}
	latest := data.Bar{Time: time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC), Close: 100}

	out := FormatDailyReport(acc, latest)
	for _, want := range []string{
		"Paper Trading Daily Report",
		"AAPL (US)",
		"sma_20_60",
		"0 (flat)",
		"Total Assets   : 10,000.00",
		"Pending Signal : none",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in report, got:\n%s", want, out)
		}
	}
}

func TestFormatDailyReportLongWithPending(t *testing.T) {
	acc := NewAccount("2330.TW", "TW", StratParams{Name: "sma", Short: 20, Long: 60}, 1_000_000, time.Now())
	acc.Cash = 412_300
	acc.Shares = 2000
	acc.AvgCost = 587.50
	acc.DailyEquity = []engine.EquityPoint{
		{Time: time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC), Total: 1_611_800},
		{Time: time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC), Total: 1_636_300},
	}
	acc.Pending = &PendingSignal{
		DecidedOn: time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC),
		Side:      engine.SideSell,
		RefClose:  612,
	}
	latest := data.Bar{Time: time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC), Close: 612}

	out := FormatDailyReport(acc, latest)
	for _, want := range []string{
		"2,000 @ avg 587.50",
		"position 1,224,000.00",
		"Total Assets   : 1,636,300.00",
		"Pending Signal : SELL decided 2026-04-25 close",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in report, got:\n%s", want, out)
		}
	}
}

func TestConsoleNotifier(t *testing.T) {
	var buf bytes.Buffer
	n := ConsoleNotifier{Out: &buf}

	if err := n.Notify(Event{
		Symbol: "AAPL", Date: time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC),
		Kind: EventTradeFilled, Side: engine.SideBuy, Price: 102.0, Shares: 50,
	}); err != nil {
		t.Fatal(err)
	}
	if err := n.Notify(Event{
		Symbol: "AAPL", Date: time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC),
		Kind: EventSignalQueued, Side: engine.SideSell, Price: 104.0,
	}); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	for _, want := range []string{
		"AAPL TRADE_FILLED BUY filled 50 @ 102.00 on 2026-04-25",
		"AAPL SIGNAL_QUEUED SELL queued (ref close 104.00) on 2026-04-25",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output, got:\n%s", want, got)
		}
	}
}

func TestThousands(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"}, {5, "5"}, {1000, "1,000"}, {1_234_567, "1,234,567"}, {-2_500, "-2,500"},
	}
	for _, c := range cases {
		if got := thousands(c.in); got != c.want {
			t.Errorf("thousands(%d): got %q want %q", c.in, got, c.want)
		}
	}
}
