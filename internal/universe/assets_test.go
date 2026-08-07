package universe

import "testing"

func TestUniverseSeparatesExecutionAndResearch(t *testing.T) {
	live := Live()
	observed := Observed()

	if len(live) != 5 {
		t.Fatalf("expected 5 live assets, got %d", len(live))
	}

	if len(observed) != 5 {
		t.Fatalf("expected 5 research assets, got %d", len(observed))
	}

	research := []string{
		"LINK",
		"HYPE",
		"LIT",
		"XAU",
		"XAG",
	}

	for _, symbol := range research {
		if _, err := RequireTradable(symbol); err == nil {
			t.Fatalf("research asset %s received execution authority", symbol)
		}
	}

	liveSymbols := []string{
		"BTC",
		"ETH",
		"ZEC",
		"BNB",
		"SOL",
	}

	for _, symbol := range liveSymbols {
		if _, err := RequireTradable(symbol); err != nil {
			t.Fatalf("live asset %s rejected: %v", symbol, err)
		}
	}
}
