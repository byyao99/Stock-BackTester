package engine

import (
	"stock-backtester/data"
	"stock-backtester/strategy"
)

type Config struct {
	InitialCash float64
	Market      data.Market
}

type Result struct {
	Trades      []Trade
	EquityCurve []EquityPoint
	FinalCash   float64
	FinalShares int
}

// Run executes the backtest with t-close signal / t+1-open execution.
// signals must have the same length as bars.
func Run(bars []data.Bar, signals []strategy.Signal, cfg Config) Result {
	res := Result{}
	cash := cfg.InitialCash
	shares := 0

	if len(bars) == 0 {
		res.FinalCash = cash
		return res
	}

	// Day 0: no signal yet acted upon — record opening equity.
	res.EquityCurve = append(res.EquityCurve, EquityPoint{
		Time:          bars[0].Time,
		Cash:          cash,
		Shares:        shares,
		PositionValue: 0,
		Total:         cash,
	})

	for i := 0; i < len(bars)-1; i++ {
		signal := signals[i]
		next := bars[i+1]
		execPrice := next.Open

		switch signal {
		case strategy.SignalBuy:
			if shares == 0 && execPrice > 0 {
				qty := MaxBuyShares(cash, execPrice, cfg.Market)
				if qty > 0 {
					_, fee, total := BuyCost(execPrice, qty, cfg.Market)
					cash -= total
					shares += qty
					res.Trades = append(res.Trades, Trade{
						Time:      next.Time,
						Side:      SideBuy,
						Price:     execPrice,
						Shares:    qty,
						Fee:       fee,
						Tax:       0,
						CashAfter: cash,
					})
				}
			}
		case strategy.SignalSell:
			if shares > 0 && execPrice > 0 {
				_, fee, tax, net := SellProceeds(execPrice, shares, cfg.Market)
				qty := shares
				cash += net
				shares = 0
				res.Trades = append(res.Trades, Trade{
					Time:      next.Time,
					Side:      SideSell,
					Price:     execPrice,
					Shares:    qty,
					Fee:       fee,
					Tax:       tax,
					CashAfter: cash,
				})
			}
		}

		positionValue := float64(shares) * next.Close
		res.EquityCurve = append(res.EquityCurve, EquityPoint{
			Time:          next.Time,
			Cash:          cash,
			Shares:        shares,
			PositionValue: positionValue,
			Total:         cash + positionValue,
		})
	}

	res.FinalCash = cash
	res.FinalShares = shares
	return res
}
