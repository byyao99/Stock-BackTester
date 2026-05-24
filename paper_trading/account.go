// Package papertrading runs daily simulated trading on top of the same
// strategies and broker math as the backtest engine. The state for one
// (symbol, strategy) pair lives in a single JSON file.
package papertrading

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"stock-backtester/engine"
)

const SchemaVersion = 1

// StratParams is the strategy-identity tuple persisted with the account.
// Two runs against the same account file must use deep-equal params.
type StratParams struct {
	Name      string  `json:"name"`
	Short     int     `json:"short,omitempty"`
	Long      int     `json:"long,omitempty"`
	RSIPeriod int     `json:"rsi_period,omitempty"`
	RSILow    float64 `json:"rsi_low,omitempty"`
	RSIHigh   float64 `json:"rsi_high,omitempty"`
	OddLot    bool    `json:"odd_lot,omitempty"`
}

// PendingSignal is a BUY/SELL decided at one bar's close, awaiting execution
// at the next bar's open. HOLD is never persisted.
type PendingSignal struct {
	DecidedOn time.Time   `json:"decided_on"`
	Side      engine.Side `json:"side"`
	RefClose  float64     `json:"ref_close"`
}

type Account struct {
	Symbol            string               `json:"symbol"`
	Market            string               `json:"market"`
	Strategy          StratParams          `json:"strategy"`
	InitialCash       float64              `json:"initial_cash"`
	Cash              float64              `json:"cash"`
	Shares            int                  `json:"shares"`
	AvgCost           float64              `json:"avg_cost"`
	LastProcessedDate time.Time            `json:"last_processed_date"`
	Pending           *PendingSignal       `json:"pending,omitempty"`
	Trades            []engine.Trade       `json:"trades"`
	DailyEquity       []engine.EquityPoint `json:"daily_equity"`
	CreatedAt         time.Time            `json:"created_at"`
	UpdatedAt         time.Time            `json:"updated_at"`
	SchemaVersion     int                  `json:"schema_version"`
}

// LoadAccount reads an account from disk. Returns (nil, nil) when the file
// does not exist so callers can branch on first-run.
func LoadAccount(path string) (*Account, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read account %s: %w", path, err)
	}
	var acc Account
	if err := json.Unmarshal(data, &acc); err != nil {
		return nil, fmt.Errorf("decode account %s: %w", path, err)
	}
	return &acc, nil
}

// SaveAccount writes the account atomically (tmp + rename) so a crash
// mid-write can never leave a half-written JSON file.
func SaveAccount(path string, acc *Account) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".paper_account.*.tmp")
	if err != nil {
		return fmt.Errorf("create tmp: %w", err)
	}
	tmpName := tmp.Name()
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(acc); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("encode account: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename tmp: %w", err)
	}
	return nil
}

// NewAccount builds a fresh account ready to be saved on first run.
func NewAccount(symbol, marketName string, params StratParams, initialCash float64, now time.Time) *Account {
	return &Account{
		Symbol:        symbol,
		Market:        marketName,
		Strategy:      params,
		InitialCash:   initialCash,
		Cash:          initialCash,
		Shares:        0,
		AvgCost:       0,
		Trades:        []engine.Trade{},
		DailyEquity:   []engine.EquityPoint{},
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: SchemaVersion,
	}
}

// VerifyIdentity returns a non-nil error when the on-disk account does not
// match the run's symbol+strategy. Refusing to load a mismatched file
// prevents silent ledger corruption.
func (a *Account) VerifyIdentity(symbol string, params StratParams) error {
	if a.Symbol != symbol {
		return fmt.Errorf("account symbol %q does not match run symbol %q", a.Symbol, symbol)
	}
	if a.Strategy != params {
		return fmt.Errorf("account strategy %+v does not match run strategy %+v (use a different --account-file or revert params)",
			a.Strategy, params)
	}
	return nil
}
