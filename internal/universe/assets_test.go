package universe

import "testing"

func TestUniverseSeparatesExecutionAndResearch(t *testing.T) {
	if len(Live()) != 5 || len(Observed()) != 4 {
		t.Fatalf("live=%d observed=%d", len(Live()), len(Observed()))
	}
	for _, symbol := range []string{"LINK", "HYPE", "XAU", "XAG"} {
		if _, err := RequireTradable(symbol); err == nil {
			t.Fatalf("research asset %s received execution authority", symbol)
		}
	}
	for _, symbol := range []string{"BTC", "ETH", "ZEC", "BNB", "SOL"} {
		if _, err := RequireTradable(symbol); err != nil {
			t.Fatalf("live asset %s rejected: %v", symbol, err)
		}
	}
}
