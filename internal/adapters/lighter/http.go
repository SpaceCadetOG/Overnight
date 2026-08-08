package lighter

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"
)

func defaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 12 * time.Second,
	}
}

func (c *Client) doGET(path string, query map[string]string) ([]byte, error) {

	u, err := url.Parse(c.baseURL + path)

	if err != nil {
		return nil, err
	}

	q := u.Query()

	for k, v := range query {
		q.Set(k, v)
	}

	u.RawQuery = q.Encode()

	req, err := http.NewRequest(
		http.MethodGet,
		u.String(),
		nil,
	)

	if err != nil {
		return nil, err
	}

	if c.auth != "" {
		req.Header.Set(
			"Authorization",
			c.auth,
		)
	}

	resp, err := c.http.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return nil, err
	}

	return body, nil
}

func (c *Client) getOrders(path string, query map[string]string) (*OrdersResponse, error) {
	b, err := c.doGET(path, query)
	if err != nil {
		return nil, err
	}

	var out OrdersResponse

	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}

	return &out, nil
}
