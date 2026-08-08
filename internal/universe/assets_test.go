package universe

import "testing"

func TestUniverseSeparatesExecutionAndResearch(t *testing.T) {
	live := Live()
	observed := Observed()

	if len(live) != 2 {
		t.Fatalf("expected 2 live assets, got %d", len(live))
	}

	if len(observed) != 10 {
		t.Fatalf("expected 10 research assets, got %d", len(observed))
	}

	research := []string{
		"SOL",
		"HYPE",
		"LIT",
		"XAU",
		"XAG",
		"LINK",
		"AAVE",
		"UNI",
		"ZEC",
		"BNB",
	}

	for _, symbol := range research {
		if _, err := RequireTradable(symbol); err == nil {
			t.Fatalf("research asset %s received execution authority", symbol)
		}
	}

	liveSymbols := []string{
		"BTC",
		"ETH",
	}

	for _, symbol := range liveSymbols {
		if _, err := RequireTradable(symbol); err != nil {
			t.Fatalf("live asset %s rejected: %v", symbol, err)
		}
	}

	if btc, _ := Find("BTC"); btc.MarketSymbol() != "BTC" {
		t.Fatalf("expected BTC identity mapping, got %s", btc.MarketSymbol())
	}
}
