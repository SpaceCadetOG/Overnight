package lighter

import (
	"encoding/json"
	"fmt"
)

func (c *Client) AccountInactiveOrders(
	accountIndex int64,
	marketID int16,
	limit int,
) (*OrdersResponse, error) {

	q := map[string]string{
		"account_index": fmt.Sprintf("%d", accountIndex),
		"market_id":     fmt.Sprintf("%d", marketID),
		"limit":         fmt.Sprintf("%d", limit),
	}

	b, err := c.doGET(
		"/api/v1/accountInactiveOrders",
		q,
	)

	if err != nil {
		return nil, err
	}

	var out OrdersResponse

	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}

	return &out, nil
}
