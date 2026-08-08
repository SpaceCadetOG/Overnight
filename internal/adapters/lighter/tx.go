package lighter

import (
	"encoding/json"
	"fmt"
)

type TxResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Hash    string `json:"hash"`
	Type    int    `json:"type"`
	Info    string `json:"info"`
	Status  int64  `json:"status"`
}

func (c *Client) TxByHash(hash string) (*TxResponse, error) {

	b, err := c.doGET(
		"/api/v1/tx",
		map[string]string{
			"by":    "hash",
			"value": hash,
		},
	)

	if err != nil {
		return nil, err
	}

	var out TxResponse

	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}

	if out.Code != 200 {
		return nil, fmt.Errorf(
			"lighter tx error %d: %s",
			out.Code,
			out.Message,
		)
	}

	return &out, nil
}
