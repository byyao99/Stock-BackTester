package papertrading

import (
	"path/filepath"
	"testing"
	"time"

	"stock-backtester/engine"
)

func TestAccountJSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "acc.json")

	created := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	acc := NewAccount("2330.TW", "TW", StratParams{Name: "sma", Short: 20, Long: 60}, 1_000_000, created)
	acc.Cash = 412_300
	acc.Shares = 2000
	acc.AvgCost = 587.50
	acc.LastProcessedDate = time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	acc.Pending = &PendingSignal{
		DecidedOn: time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC),
		Side:      engine.SideSell,
		RefClose:  612.0,
	}
	acc.Trades = []engine.Trade{
		{Time: time.Date(2026, 4, 1, 1, 0, 0, 0, time.UTC), Side: engine.SideBuy, Price: 587.5, Shares: 1000, Fee: 837.19, CashAfter: 411_675.31},
		{Time: time.Date(2026, 4, 10, 1, 0, 0, 0, time.UTC), Side: engine.SideBuy, Price: 587.5, Shares: 1000, Fee: 837.19, CashAfter: 0},
	}
	acc.DailyEquity = []engine.EquityPoint{
		{Time: time.Date(2026, 4, 23, 0, 0, 0, 0, time.UTC), Cash: 412_300, Shares: 2000, PositionValue: 1_200_000, Total: 1_612_300},
		{Time: time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC), Cash: 412_300, Shares: 2000, PositionValue: 1_220_000, Total: 1_632_300},
		{Time: time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC), Cash: 412_300, Shares: 2000, PositionValue: 1_224_000, Total: 1_636_300},
	}

	if err := SaveAccount(path, acc); err != nil {
		t.Fatalf("SaveAccount: %v", err)
	}
	loaded, err := LoadAccount(path)
	if err != nil {
		t.Fatalf("LoadAccount: %v", err)
	}
	if loaded == nil {
		t.Fatalf("LoadAccount returned nil for existing file")
	}

	if loaded.Symbol != acc.Symbol || loaded.Market != acc.Market || loaded.SchemaVersion != acc.SchemaVersion {
		t.Errorf("identity fields drifted: got %+v want %+v", loaded, acc)
	}
	if loaded.Cash != acc.Cash || loaded.Shares != acc.Shares || loaded.AvgCost != acc.AvgCost {
		t.Errorf("position drifted: cash=%v shares=%v avg=%v", loaded.Cash, loaded.Shares, loaded.AvgCost)
	}
	if !loaded.LastProcessedDate.Equal(acc.LastProcessedDate) {
		t.Errorf("LastProcessedDate: got %v want %v", loaded.LastProcessedDate, acc.LastProcessedDate)
	}
	if loaded.Pending == nil {
		t.Fatalf("Pending lost in round-trip")
	}
	if loaded.Pending.Side != acc.Pending.Side || loaded.Pending.RefClose != acc.Pending.RefClose ||
		!loaded.Pending.DecidedOn.Equal(acc.Pending.DecidedOn) {
		t.Errorf("Pending drifted: got %+v want %+v", loaded.Pending, acc.Pending)
	}
	if len(loaded.Trades) != len(acc.Trades) {
		t.Fatalf("Trades length: got %d want %d", len(loaded.Trades), len(acc.Trades))
	}
	for i, want := range acc.Trades {
		got := loaded.Trades[i]
		if got.Side != want.Side || got.Price != want.Price || got.Shares != want.Shares || !got.Time.Equal(want.Time) {
			t.Errorf("Trades[%d] drifted: got %+v want %+v", i, got, want)
		}
	}
	if len(loaded.DailyEquity) != len(acc.DailyEquity) {
		t.Fatalf("DailyEquity length: got %d want %d", len(loaded.DailyEquity), len(acc.DailyEquity))
	}
	for i, want := range acc.DailyEquity {
		got := loaded.DailyEquity[i]
		if got.Cash != want.Cash || got.Shares != want.Shares || got.Total != want.Total || !got.Time.Equal(want.Time) {
			t.Errorf("DailyEquity[%d] drifted: got %+v want %+v", i, got, want)
		}
	}
}

func TestLoadAccountMissingReturnsNil(t *testing.T) {
	dir := t.TempDir()
	got, err := LoadAccount(filepath.Join(dir, "nope.json"))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil account, got %+v", got)
	}
}

func TestVerifyIdentity(t *testing.T) {
	acc := NewAccount("2330.TW", "TW", StratParams{Name: "sma", Short: 20, Long: 60}, 1_000_000, time.Now())

	if err := acc.VerifyIdentity("2330.TW", StratParams{Name: "sma", Short: 20, Long: 60}); err != nil {
		t.Errorf("matching params should pass, got %v", err)
	}
	if err := acc.VerifyIdentity("AAPL", StratParams{Name: "sma", Short: 20, Long: 60}); err == nil {
		t.Errorf("symbol mismatch should fail")
	}
	if err := acc.VerifyIdentity("2330.TW", StratParams{Name: "sma", Short: 10, Long: 40}); err == nil {
		t.Errorf("strategy param mismatch should fail")
	}
	if err := acc.VerifyIdentity("2330.TW", StratParams{Name: "rsi", RSIPeriod: 14, RSILow: 30, RSIHigh: 70}); err == nil {
		t.Errorf("strategy name mismatch should fail")
	}
}

func TestDefaultAccountPath(t *testing.T) {
	got := DefaultAccountPath("paper_accounts", "2330.TW", "sma_20_60")
	want := filepath.Join("paper_accounts", "2330_TW_sma_20_60.json")
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}
