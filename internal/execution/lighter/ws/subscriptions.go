package ws

import "fmt"

type Subscribe struct {
	Type    string `json:"type"`
	Channel string `json:"channel"`
	Auth    string `json:"auth,omitempty"`
}

func AccountPositions(
	account int64,
	auth string,
) Subscribe {

	return Subscribe{
		Type:    "subscribe",
		Channel: "account_all_positions/" + formatAccount(account),
		Auth:    auth,
	}
}

func formatAccount(v int64) string {
	return fmt.Sprintf("%d", v)
}

func MarketStats(
	symbol string,
) Subscribe {

	return Subscribe{
		Type:    "subscribe",
		Channel: "market_stats/" + symbol,
	}
}
