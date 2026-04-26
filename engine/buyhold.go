package engine

import "stock-backtester/data"

// BuyHold computes the return of a passive "buy at the second bar's open,
// mark-to-market at the last bar's close" baseline, using the same lot/fee
// rules as the strategy. Entry uses bars[1].Open (matching the strategy's
// earliest possible execution under the t-close / t+1-open model).
// Returns nil when initial cash can't afford even one lot at entry.
func BuyHold(bars []data.Bar, initialCash float64, market data.Market) *float64 {
	if len(bars) < 2 || initialCash <= 0 {
		return nil
	}
	entryPrice := bars[1].Open
	shares := MaxBuyShares(initialCash, entryPrice, market)
	if shares == 0 {
		return nil
	}
	_, _, total := BuyCost(entryPrice, shares, market)
	leftover := initialCash - total

	exitPrice := bars[len(bars)-1].Close
	finalEquity := leftover + float64(shares)*exitPrice
	ret := finalEquity/initialCash - 1
	return &ret
}
