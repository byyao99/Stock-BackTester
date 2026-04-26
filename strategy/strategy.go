package strategy

import "stock-backtester/data"

type Signal int

const (
	SignalHold Signal = iota
	SignalBuy
	SignalSell
)

func (s Signal) String() string {
	switch s {
	case SignalBuy:
		return "BUY"
	case SignalSell:
		return "SELL"
	default:
		return "HOLD"
	}
}

// Strategy decides on each bar (after its close) whether to act on the next bar.
// GenerateSignals returns a slice with the same length as bars; signals[i] is the
// decision made using information available up to and including bars[i].Close.
type Strategy interface {
	Name() string
	GenerateSignals(bars []data.Bar) []Signal
	// MinBars is the minimum bar count needed for any chance of producing
	// an executable signal (signal at bar i requires bar i+1 to fill on).
	MinBars() int
}
