package engine

import "time"

type EquityPoint struct {
	Time          time.Time
	Cash          float64
	Shares        int
	PositionValue float64
	Total         float64
}
