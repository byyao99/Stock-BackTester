package papertrading

import (
	"stock-backtester/data"
	"stock-backtester/engine"
)

// ExecutePending fills the account's pending BUY/SELL at execBar.Open using
// the same lot/fee math as the backtest engine. Returns the resulting Trade
// or nil if the order could not fill (no cash for a single lot, or no shares
// to sell, or open price unavailable). The pending is consumed regardless —
// callers should clear acc.Pending after this call.
func ExecutePending(acc *Account, execBar data.Bar, market data.Market) *engine.Trade {
	if acc.Pending == nil {
		return nil
	}
	if execBar.Open <= 0 {
		return nil
	}
	side := acc.Pending.Side

	switch side {
	case engine.SideBuy:
		if acc.Shares != 0 {
			return nil
		}
		qty := engine.MaxBuyShares(acc.Cash, execBar.Open, market)
		if qty <= 0 {
			return nil
		}
		_, fee, total := engine.BuyCost(execBar.Open, qty, market)
		acc.Cash -= total
		acc.Shares = qty
		acc.AvgCost = total / float64(qty)
		t := engine.Trade{
			Time:      execBar.Time,
			Side:      engine.SideBuy,
			Price:     execBar.Open,
			Shares:    qty,
			Fee:       fee,
			Tax:       0,
			CashAfter: acc.Cash,
		}
		acc.Trades = append(acc.Trades, t)
		return &t

	case engine.SideSell:
		if acc.Shares <= 0 {
			return nil
		}
		qty := acc.Shares
		_, fee, tax, net := engine.SellProceeds(execBar.Open, qty, market)
		acc.Cash += net
		acc.Shares = 0
		acc.AvgCost = 0
		t := engine.Trade{
			Time:      execBar.Time,
			Side:      engine.SideSell,
			Price:     execBar.Open,
			Shares:    qty,
			Fee:       fee,
			Tax:       tax,
			CashAfter: acc.Cash,
		}
		acc.Trades = append(acc.Trades, t)
		return &t
	}
	return nil
}
