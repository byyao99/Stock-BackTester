package output

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"stock-backtester/engine"
)

func ensureDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

func WriteTrades(path string, trades []engine.Trade) error {
	if err := ensureDir(path); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write([]string{"time", "side", "price", "shares", "fee", "tax", "cash_after"}); err != nil {
		return err
	}
	for _, t := range trades {
		row := []string{
			t.Time.UTC().Format(time.RFC3339),
			string(t.Side),
			strconv.FormatFloat(t.Price, 'f', 6, 64),
			strconv.Itoa(t.Shares),
			strconv.FormatFloat(t.Fee, 'f', 6, 64),
			strconv.FormatFloat(t.Tax, 'f', 6, 64),
			strconv.FormatFloat(t.CashAfter, 'f', 6, 64),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return nil
}

func WriteEquity(path string, curve []engine.EquityPoint) error {
	if err := ensureDir(path); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write([]string{"time", "cash", "shares", "position_value", "total"}); err != nil {
		return err
	}
	for _, p := range curve {
		row := []string{
			p.Time.UTC().Format(time.RFC3339),
			strconv.FormatFloat(p.Cash, 'f', 6, 64),
			strconv.Itoa(p.Shares),
			strconv.FormatFloat(p.PositionValue, 'f', 6, 64),
			strconv.FormatFloat(p.Total, 'f', 6, 64),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return nil
}
