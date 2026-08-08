package lighter

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (c *Client) SendTx(values url.Values) (string, error) {

	req, err := http.NewRequest(
		http.MethodPost,
		c.baseURL+"/api/v1/sendTx",
		strings.NewReader(values.Encode()),
	)

	if err != nil {
		return "", err
	}

	req.Header.Set(
		"Content-Type",
		"application/x-www-form-urlencoded",
	)

	resp, err := c.http.Do(req)

	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return "", err
	}

	var result struct {
		Code    int    `json:"code"`
		TxHash  string `json:"tx_hash"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decode Lighter send response: %w", err)
	}
	if result.Code != 200 {
		return "", fmt.Errorf(
			"lighter send failed: %s",
			string(body),
		)
	}
	return result.TxHash, nil
}
