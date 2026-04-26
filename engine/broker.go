package engine

import (
	"math"

	"stock-backtester/data"
)

// MaxBuyShares returns the largest share count, respecting market lot size,
// such that price*shares*(1+buyFee) <= cash.
func MaxBuyShares(cash, price float64, m data.Market) int {
	if price <= 0 || cash <= 0 {
		return 0
	}
	costPerShare := price * (1 + m.BuyFeeRate)
	raw := math.Floor(cash / costPerShare)
	if m.LotSize > 1 {
		lots := math.Floor(raw / float64(m.LotSize))
		return int(lots) * m.LotSize
	}
	return int(raw)
}

// BuyCost returns (gross, fee, total) for buying `shares` at `price`.
func BuyCost(price float64, shares int, m data.Market) (gross, fee, total float64) {
	gross = price * float64(shares)
	fee = gross * m.BuyFeeRate
	total = gross + fee
	return
}

// SellProceeds returns (gross, fee, tax, net) for selling `shares` at `price`.
func SellProceeds(price float64, shares int, m data.Market) (gross, fee, tax, net float64) {
	gross = price * float64(shares)
	fee = gross * m.SellFeeRate
	tax = gross * m.SellTaxRate
	net = gross - fee - tax
	return
}
