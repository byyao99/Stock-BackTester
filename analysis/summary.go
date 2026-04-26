package analysis

import "time"

type Summary struct {
	Symbol         string    `json:"symbol"`
	Strategy       string    `json:"strategy"`
	Market         string    `json:"market"`
	Start          time.Time `json:"start"`
	End            time.Time `json:"end"`
	TradingDays    int       `json:"trading_days"`
	InitialCash    float64   `json:"initial_cash"`
	FinalEquity    float64   `json:"final_equity"`
	TotalReturn    float64   `json:"total_return"`
	AnnualReturn   float64   `json:"annual_return"`
	MaxDrawdown    float64   `json:"max_drawdown"`
	SharpeRatio    float64   `json:"sharpe_ratio"`
	WinRate        float64   `json:"win_rate"`
	ProfitFactor   *float64  `json:"profit_factor"`
	TradeCount     int       `json:"trade_count"`
	RoundTripCount int       `json:"round_trip_count"`
	BuyHoldReturn  *float64  `json:"buy_hold_return"` // nil if cash can't afford 1 lot
	Alpha          *float64  `json:"alpha"`           // total_return - buy_hold_return
}
