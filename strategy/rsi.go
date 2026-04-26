package strategy

import (
	"fmt"
	"math"

	"stock-backtester/data"

	"github.com/cinar/indicator/v2/helper"
	"github.com/cinar/indicator/v2/momentum"
)

type RSI struct {
	Period     int
	Oversold   float64
	Overbought float64
}

func NewRSI(period int, oversold, overbought float64) *RSI {
	return &RSI{Period: period, Oversold: oversold, Overbought: overbought}
}

func (r *RSI) Name() string {
	return fmt.Sprintf("rsi_%d_%.0f_%.0f", r.Period, r.Oversold, r.Overbought)
}

// MinBars: RSI(period) becomes valid at index period, executed at index period+1.
func (r *RSI) MinBars() int {
	return r.Period + 2
}

func (r *RSI) GenerateSignals(bars []data.Bar) []Signal {
	signals := make([]Signal, len(bars))
	if len(bars) < r.Period+2 {
		return signals
	}
	closes := data.Closes(bars)
	rsi := computeRSI(closes, r.Period)

	holding := false
	for i := range bars {
		v := rsi[i]
		if math.IsNaN(v) {
			continue
		}
		switch {
		case v < r.Oversold && !holding:
			signals[i] = SignalBuy
			holding = true
		case v > r.Overbought && holding:
			signals[i] = SignalSell
			holding = false
		}
	}
	return signals
}

func computeRSI(values []float64, period int) []float64 {
	out := make([]float64, len(values))
	for i := range out {
		out[i] = math.NaN()
	}
	if period <= 0 || len(values) < period+1 {
		return out
	}
	rsi := momentum.NewRsiWithPeriod[float64](period)
	in := helper.SliceToChan(values)
	res := helper.ChanToSlice(rsi.Compute(in))
	idle := rsi.IdlePeriod()
	for i, v := range res {
		out[idle+i] = v
	}
	return out
}
