package lighter

import (
	"encoding/json"
	"fmt"
)

func (c *Client) AccountActiveOrders(
	accountIndex int64,
	marketID int16,
) (*OrdersResponse, error) {

	q := map[string]string{
		"account_index": fmt.Sprintf("%d", accountIndex),
		"market_id":     fmt.Sprintf("%d", marketID),
	}

	b, err := c.doGET("/api/v1/accountActiveOrders", q)
	if err != nil {
		return nil, err
	}

	var out OrdersResponse

	err = json.Unmarshal(b, &out)
	if err != nil {
		return nil, err
	}

	if out.Code != 200 {
		return nil, fmt.Errorf(
			"lighter error %d: %s",
			out.Code,
			out.Message,
		)
	}

	return &out, nil
}
