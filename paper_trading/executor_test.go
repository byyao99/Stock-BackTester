package papertrading

import (
	"math"
	"testing"
	"time"

	"stock-backtester/data"
	"stock-backtester/engine"
)

var usMarket = data.MarketOf("AAPL")
var twMarket = data.MarketOf("2330.TW")

func bar(t time.Time, open, high, low, close float64) data.Bar {
	return data.Bar{Time: t, Open: open, High: high, Low: low, Close: close, Volume: 0}
}

func TestExecutePendingBuyUS(t *testing.T) {
	acc := NewAccount("AAPL", "US", StratParams{Name: "sma", Short: 20, Long: 60}, 10_000, time.Now())
	acc.Pending = &PendingSignal{
		DecidedOn: time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC),
		Side:      engine.SideBuy,
		RefClose:  100,
	}
	exec := bar(time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC), 102, 105, 101, 104)

	tr := ExecutePending(acc, exec, usMarket)
	if tr == nil {
		t.Fatalf("expected a Trade, got nil")
	}

	wantQty := engine.MaxBuyShares(10_000, 102, usMarket)
	if tr.Shares != wantQty {
		t.Errorf("shares: got %d want %d", tr.Shares, wantQty)
	}
	if tr.Price != 102 {
		t.Errorf("exec price: got %v want 102", tr.Price)
	}
	_, wantFee, wantTotal := engine.BuyCost(102, wantQty, usMarket)
	if math.Abs(tr.Fee-wantFee) > 1e-9 {
		t.Errorf("fee: got %v want %v", tr.Fee, wantFee)
	}
	wantCash := 10_000 - wantTotal
	if math.Abs(acc.Cash-wantCash) > 1e-9 {
		t.Errorf("cash after: got %v want %v", acc.Cash, wantCash)
	}
	if acc.Shares != wantQty {
		t.Errorf("acc.Shares: got %d want %d", acc.Shares, wantQty)
	}
	wantAvg := wantTotal / float64(wantQty)
	if math.Abs(acc.AvgCost-wantAvg) > 1e-9 {
		t.Errorf("avg cost: got %v want %v", acc.AvgCost, wantAvg)
	}
	if len(acc.Trades) != 1 {
		t.Errorf("Trades length: got %d want 1", len(acc.Trades))
	}
}

func TestExecutePendingBuyAlreadyLongIsNoOp(t *testing.T) {
	acc := NewAccount("AAPL", "US", StratParams{Name: "sma", Short: 20, Long: 60}, 10_000, time.Now())
	acc.Shares = 50
	acc.AvgCost = 100
	acc.Cash = 5_000
	acc.Pending = &PendingSignal{Side: engine.SideBuy}
	exec := bar(time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC), 102, 105, 101, 104)

	if tr := ExecutePending(acc, exec, usMarket); tr != nil {
		t.Errorf("expected nil trade when already long, got %+v", tr)
	}
	if acc.Shares != 50 || acc.Cash != 5_000 {
		t.Errorf("state mutated despite no-op: shares=%d cash=%v", acc.Shares, acc.Cash)
	}
}

func TestExecutePendingSellTW(t *testing.T) {
	acc := NewAccount("2330.TW", "TW", StratParams{Name: "sma", Short: 20, Long: 60}, 1_000_000, time.Now())
	acc.Cash = 100_000
	acc.Shares = 2000
	acc.AvgCost = 587.50
	acc.Pending = &PendingSignal{Side: engine.SideSell}
	exec := bar(time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC), 612, 615, 610, 613)

	tr := ExecutePending(acc, exec, twMarket)
	if tr == nil {
		t.Fatalf("expected a Trade")
	}
	if tr.Shares != 2000 {
		t.Errorf("shares: got %d want 2000", tr.Shares)
	}
	_, wantFee, wantTax, wantNet := engine.SellProceeds(612, 2000, twMarket)
	if math.Abs(tr.Fee-wantFee) > 1e-9 || math.Abs(tr.Tax-wantTax) > 1e-9 {
		t.Errorf("fee/tax: got fee=%v tax=%v want fee=%v tax=%v", tr.Fee, tr.Tax, wantFee, wantTax)
	}
	wantCash := 100_000 + wantNet
	if math.Abs(acc.Cash-wantCash) > 1e-9 {
		t.Errorf("cash after: got %v want %v", acc.Cash, wantCash)
	}
	if acc.Shares != 0 || acc.AvgCost != 0 {
		t.Errorf("position not cleared after sell: shares=%d avg=%v", acc.Shares, acc.AvgCost)
	}
}

func TestExecutePendingSellWhenFlatIsNoOp(t *testing.T) {
	acc := NewAccount("AAPL", "US", StratParams{Name: "sma", Short: 20, Long: 60}, 10_000, time.Now())
	acc.Pending = &PendingSignal{Side: engine.SideSell}
	exec := bar(time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC), 102, 105, 101, 104)

	if tr := ExecutePending(acc, exec, usMarket); tr != nil {
		t.Errorf("expected nil trade when flat, got %+v", tr)
	}
	if acc.Cash != 10_000 || acc.Shares != 0 {
		t.Errorf("state mutated: cash=%v shares=%d", acc.Cash, acc.Shares)
	}
}

func TestExecutePendingNilPending(t *testing.T) {
	acc := NewAccount("AAPL", "US", StratParams{Name: "sma", Short: 20, Long: 60}, 10_000, time.Now())
	exec := bar(time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC), 102, 105, 101, 104)
	if tr := ExecutePending(acc, exec, usMarket); tr != nil {
		t.Errorf("expected nil for no pending, got %+v", tr)
	}
}

func TestExecutePendingInsufficientCash(t *testing.T) {
	acc := NewAccount("2330.TW", "TW", StratParams{Name: "sma", Short: 20, Long: 60}, 100, time.Now())
	acc.Pending = &PendingSignal{Side: engine.SideBuy}
	exec := bar(time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC), 612, 615, 610, 613)

	if tr := ExecutePending(acc, exec, twMarket); tr != nil {
		t.Errorf("expected nil trade when cash < 1 lot, got %+v", tr)
	}
	if acc.Shares != 0 || acc.Cash != 100 {
		t.Errorf("state mutated despite no-fill: shares=%d cash=%v", acc.Shares, acc.Cash)
	}
}
