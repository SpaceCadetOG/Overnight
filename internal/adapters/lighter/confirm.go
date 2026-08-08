package lighter

import (
	"fmt"
	"time"
)

func (c *Client) ConfirmOrder(
	accountIndex int64,
	marketID int16,
	clientOrderIndex int64,
	timeout time.Duration,
) (*Order, error) {

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {

		resp, err := c.AccountActiveOrders(
			accountIndex,
			marketID,
		)

		if err != nil {
			return nil, err
		}

		for _, order := range resp.Orders {

			if order.ClientOrderIndex == clientOrderIndex {

				return &order, nil

			}
		}

		time.Sleep(1 * time.Second)
	}

	return nil, fmt.Errorf(
		"order confirmation timeout",
	)
}
