package lighter

import "testing"

func btcTestMarket() Market {
	return Market{
		Symbol:                 "BTC",
		MarketID:               1,
		MarketType:             "perp",
		Status:                 "active",
		MinBaseAmount:          "0.00010",
		MinQuoteAmount:         "10.000000",
		SupportedSizeDecimals:  5,
		SupportedPriceDecimals: 1,
		SupportedQuoteDecimals: 6,
		SizeDecimals:           5,
		PriceDecimals:          1,
	}
}

func ethTestMarket() Market {
	return Market{
		Symbol:                 "ETH",
		MarketID:               0,
		MarketType:             "perp",
		Status:                 "active",
		MinBaseAmount:          "0.0050",
		MinQuoteAmount:         "10.000000",
		SupportedSizeDecimals:  4,
		SupportedPriceDecimals: 2,
		SupportedQuoteDecimals: 6,
		SizeDecimals:           4,
		PriceDecimals:          2,
	}
}

func TestEncodeBTCOrder(t *testing.T) {
	market := btcTestMarket()

	got, err := market.EncodeOrder(
		0.00020,
		60000.0,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got.MarketIndex != 1 {
		t.Fatalf(
			"market index got=%d want=1",
			got.MarketIndex,
		)
	}

	if got.BaseAmount != 20 {
		t.Fatalf(
			"base amount got=%d want=20",
			got.BaseAmount,
		)
	}

	if got.Price != 600000 {
		t.Fatalf(
			"price got=%d want=600000",
			got.Price,
		)
	}
}

func TestEncodeETHOrder(t *testing.T) {
	market := ethTestMarket()

	got, err := market.EncodeOrder(
		0.0100,
		1882.25,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got.BaseAmount != 100 {
		t.Fatalf(
			"base amount got=%d want=100",
			got.BaseAmount,
		)
	}

	if got.Price != 188225 {
		t.Fatalf(
			"price got=%d want=188225",
			got.Price,
		)
	}
}

func TestRejectInactiveMarket(t *testing.T) {
	market := btcTestMarket()
	market.Status = "maintenance"

	_, err := market.EncodeOrder(
		0.00020,
		60000.0,
	)

	if err == nil {
		t.Fatal("expected inactive market error")
	}
}

func TestRejectBelowMinimumBase(t *testing.T) {
	market := btcTestMarket()

	_, err := market.EncodeOrder(
		0.00001,
		60000.0,
	)

	if err == nil {
		t.Fatal("expected minimum base error")
	}
}

func TestRejectBelowMinimumQuote(t *testing.T) {
	market := btcTestMarket()

	_, err := market.EncodeOrder(
		0.00010,
		50000.0,
	)

	if err == nil {
		t.Fatal("expected minimum quote error")
	}
}

func TestRejectExcessQuantityPrecision(t *testing.T) {
	market := btcTestMarket()

	_, err := market.EncodeOrder(
		0.000201,
		60000.0,
	)

	if err == nil {
		t.Fatal("expected quantity precision error")
	}
}

func TestRejectExcessPricePrecision(t *testing.T) {
	market := btcTestMarket()

	_, err := market.EncodeOrder(
		0.00020,
		60000.01,
	)

	if err == nil {
		t.Fatal("expected price precision error")
	}
}

func TestNormalizeSymbol(t *testing.T) {
	got := normalizeSymbol(" btc ")

	if got != "BTC" {
		t.Fatalf(
			"got=%q want=BTC",
			got,
		)
	}
}
