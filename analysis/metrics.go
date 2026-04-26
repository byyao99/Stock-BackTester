package analysis

import (
	"math"

	"stock-backtester/engine"
)

const (
	tradingDaysPerYear = 252.0
	riskFreeRate       = 0.02
)

type Metrics struct {
	TotalReturn    float64
	AnnualReturn   float64
	MaxDrawdown    float64
	SharpeRatio    float64
	WinRate        float64
	ProfitFactor   *float64 // nil when undefined (no completed round trips, or no losses)
	RoundTripCount int
	FinalEquity    float64
}

func Compute(curve []engine.EquityPoint, trades []engine.Trade, initialCash float64) Metrics {
	m := Metrics{FinalEquity: initialCash}
	if len(curve) == 0 || initialCash <= 0 {
		return m
	}

	final := curve[len(curve)-1].Total
	m.FinalEquity = final
	m.TotalReturn = final/initialCash - 1

	days := len(curve)
	if days > 1 {
		years := float64(days-1) / tradingDaysPerYear
		if years > 0 && (1+m.TotalReturn) > 0 {
			m.AnnualReturn = math.Pow(1+m.TotalReturn, 1/years) - 1
		}
	}

	m.MaxDrawdown = maxDrawdown(curve)
	m.SharpeRatio = sharpe(curve)

	wins, losses, rt := roundTripStats(trades)
	m.RoundTripCount = rt
	if rt > 0 {
		winCount := 0
		for _, p := range wins {
			if p > 0 {
				winCount++
			}
		}
		m.WinRate = float64(winCount) / float64(rt)
	}
	sumWins := 0.0
	for _, p := range wins {
		if p > 0 {
			sumWins += p
		}
	}
	sumLosses := 0.0
	for _, p := range losses {
		if p < 0 {
			sumLosses += -p
		}
	}
	if sumLosses > 0 {
		pf := sumWins / sumLosses
		m.ProfitFactor = &pf
	}
	return m
}

func maxDrawdown(curve []engine.EquityPoint) float64 {
	peak := curve[0].Total
	maxDD := 0.0
	for _, p := range curve {
		if p.Total > peak {
			peak = p.Total
		}
		if peak > 0 {
			dd := (peak - p.Total) / peak
			if dd > maxDD {
				maxDD = dd
			}
		}
	}
	return maxDD
}

func sharpe(curve []engine.EquityPoint) float64 {
	if len(curve) < 2 {
		return 0
	}
	rets := make([]float64, 0, len(curve)-1)
	for i := 1; i < len(curve); i++ {
		prev := curve[i-1].Total
		if prev <= 0 {
			continue
		}
		rets = append(rets, curve[i].Total/prev-1)
	}
	if len(rets) < 2 {
		return 0
	}
	mean := 0.0
	for _, r := range rets {
		mean += r
	}
	mean /= float64(len(rets))

	variance := 0.0
	for _, r := range rets {
		diff := r - mean
		variance += diff * diff
	}
	variance /= float64(len(rets) - 1)
	std := math.Sqrt(variance)
	if std == 0 {
		return 0
	}
	dailyRf := riskFreeRate / tradingDaysPerYear
	return (mean - dailyRf) / std * math.Sqrt(tradingDaysPerYear)
}

// roundTripStats pairs sequential BUY → SELL trades, returning per-pair PnL
// (separated into wins/losses for convenience) and the total round-trip count.
func roundTripStats(trades []engine.Trade) (wins, losses []float64, rt int) {
	var openCost float64
	open := false
	for _, t := range trades {
		switch t.Side {
		case engine.SideBuy:
			openCost = t.Price*float64(t.Shares) + t.Fee
			open = true
		case engine.SideSell:
			if !open {
				continue
			}
			proceeds := t.Price*float64(t.Shares) - t.Fee - t.Tax
			pnl := proceeds - openCost
			if pnl >= 0 {
				wins = append(wins, pnl)
			} else {
				losses = append(losses, pnl)
			}
			rt++
			open = false
		}
	}
	return
}
