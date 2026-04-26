package engine

import "time"

type Side string

const (
	SideBuy  Side = "BUY"
	SideSell Side = "SELL"
)

type Trade struct {
	Time      time.Time
	Side      Side
	Price     float64
	Shares    int
	Fee       float64
	Tax       float64
	CashAfter float64
}
