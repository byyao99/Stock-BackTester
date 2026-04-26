package strategy

import (
	"fmt"
	"math"

	"stock-backtester/data"

	"github.com/cinar/indicator/v2/helper"
	"github.com/cinar/indicator/v2/trend"
)

type SMACrossover struct {
	Short int
	Long  int
}

func NewSMACrossover(short, long int) *SMACrossover {
	return &SMACrossover{Short: short, Long: long}
}

func (s *SMACrossover) Name() string {
	return fmt.Sprintf("sma_%d_%d", s.Short, s.Long)
}

// MinBars: SMA(long) becomes valid at index long-1 (NaN prev diff, no signal),
// first signal possible at index long, executed at index long+1 → need long+2 bars.
func (s *SMACrossover) MinBars() int {
	return s.Long + 2
}

func (s *SMACrossover) GenerateSignals(bars []data.Bar) []Signal {
	signals := make([]Signal, len(bars))
	if len(bars) < s.Long+1 {
		return signals
	}
	closes := data.Closes(bars)

	shortSMA := computeSMA(closes, s.Short)
	longSMA := computeSMA(closes, s.Long)

	prevDiff := math.NaN()
	for i := range bars {
		sv, lv := shortSMA[i], longSMA[i]
		if math.IsNaN(sv) || math.IsNaN(lv) {
			continue
		}
		diff := sv - lv
		if !math.IsNaN(prevDiff) {
			if prevDiff <= 0 && diff > 0 {
				signals[i] = SignalBuy
			} else if prevDiff >= 0 && diff < 0 {
				signals[i] = SignalSell
			}
		}
		prevDiff = diff
	}
	return signals
}

// computeSMA returns a slice the same length as input with NaN in the leading
// idle period, then the SMA values.
func computeSMA(values []float64, period int) []float64 {
	out := make([]float64, len(values))
	for i := range out {
		out[i] = math.NaN()
	}
	if period <= 0 || len(values) < period {
		return out
	}
	sma := trend.NewSmaWithPeriod[float64](period)
	in := helper.SliceToChan(values)
	res := helper.ChanToSlice(sma.Compute(in))
	idle := sma.IdlePeriod()
	for i, v := range res {
		out[idle+i] = v
	}
	return out
}
