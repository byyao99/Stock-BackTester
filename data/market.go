package data

import "strings"

type Market struct {
	Name        string
	LotSize     int
	BuyFeeRate  float64
	SellFeeRate float64
	SellTaxRate float64
	Currency    string
}

var (
	taiwanMarket = Market{
		Name:        "TW",
		LotSize:     1000,
		BuyFeeRate:  0.001425,
		SellFeeRate: 0.001425,
		SellTaxRate: 0.003,
		Currency:    "TWD",
	}
	usMarket = Market{
		Name:        "US",
		LotSize:     1,
		BuyFeeRate:  0,
		SellFeeRate: 0,
		SellTaxRate: 0,
		Currency:    "USD",
	}
)

func MarketOf(symbol string) Market {
	s := strings.ToUpper(symbol)
	if strings.HasSuffix(s, ".TW") || strings.HasSuffix(s, ".TWO") {
		return taiwanMarket
	}
	return usMarket
}
