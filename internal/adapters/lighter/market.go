package lighter

import (
	"encoding/json"
	"fmt"
)

type MarketDetails struct {
	Symbol        string `json:"symbol"`
	MarketID      int16  `json:"market_id"`
	MarkPrice     string `json:"mark_price"`
	PriceDecimals int    `json:"price_decimals"`
}

type OrderBookDetailsResponse struct {
	Code             int             `json:"code"`
	OrderBookDetails []MarketDetails `json:"order_book_details"`
}

func (c *Client) MarketData(marketID int) (*MarketDetails, error) {

	b, err := c.doGET(
		"/api/v1/orderBookDetails",
		map[string]string{
			"market_id": fmt.Sprintf("%d", marketID),
			"filter":    "perp",
		},
	)

	if err != nil {
		return nil, err
	}

	var out OrderBookDetailsResponse

	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}

	if len(out.OrderBookDetails) == 0 {
		return nil, fmt.Errorf("market not found")
	}

	return &out.OrderBookDetails[0], nil
}
