package collector

import "testing"

func TestStreamClassification(t *testing.T) {
	cases := map[string]string{"order_book/0": "orderbook_events", "trade/0": "trade_flow", "ticker/0": "ticker_events", "market_stats/all": "market_stats"}
	for input, want := range cases {
		if got := streamName(input); got != want {
			t.Fatalf("%s=%s want %s", input, got, want)
		}
	}
}
