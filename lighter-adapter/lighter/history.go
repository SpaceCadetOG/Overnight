package lighter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	maxAccountOrderIndexes = 20
	maxInactiveOrdersLimit = 100
)

type ordersResponse struct {
	Code       int     `json:"code"`
	Message    string  `json:"message"`
	Orders     []Order `json:"orders"`
	Cursor     string  `json:"cursor"`
	NextCursor string  `json:"next_cursor"`
}

// InactiveOrdersPage is one page of terminal order history. Cursor is empty
// when there are no more pages.
type InactiveOrdersPage struct {
	Orders []Order
	Cursor string
}

func (m *Manager) authenticatedOrdersGet(ctx context.Context, endpoint string, query url.Values) (*ordersResponse, error) {
	token, err := m.authToken()
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(m.BaseURL + endpoint)
	if err != nil {
		return nil, err
	}
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("accept", "application/json")
	req.Header.Set("Authorization", token)

	res, err := m.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s request: %w", strings.TrimPrefix(endpoint, "/api/v1/"), err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s HTTP %s: %s", strings.TrimPrefix(endpoint, "/api/v1/"), res.Status, string(body))
	}

	var response ordersResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode %s: %w", strings.TrimPrefix(endpoint, "/api/v1/"), err)
	}
	if response.Code != http.StatusOK {
		return nil, fmt.Errorf("%s API error code=%d message=%q", strings.TrimPrefix(endpoint, "/api/v1/"), response.Code, response.Message)
	}
	return &response, nil
}

// AccountOrders looks up orders directly by client order index.
func (m *Manager) AccountOrders(ctx context.Context, clientOrderIndexes []int64) ([]Order, error) {
	if len(clientOrderIndexes) == 0 {
		return nil, fmt.Errorf("at least one client order index is required")
	}
	if len(clientOrderIndexes) > maxAccountOrderIndexes {
		return nil, fmt.Errorf("accountOrders accepts at most %d client order indexes", maxAccountOrderIndexes)
	}

	indexes := make([]string, len(clientOrderIndexes))
	for i, index := range clientOrderIndexes {
		if index <= 0 {
			return nil, fmt.Errorf("client order index must be positive: %d", index)
		}
		indexes[i] = strconv.FormatInt(index, 10)
	}

	query := url.Values{}
	query.Set("account_index", strconv.FormatInt(m.AccountIndex, 10))
	query.Set("client_order_indexes", strings.Join(indexes, ","))
	response, err := m.authenticatedOrdersGet(ctx, "/api/v1/accountOrders", query)
	if err != nil {
		return nil, err
	}
	return response.Orders, nil
}

// InactiveOrders returns one page of terminal order history.
func (m *Manager) InactiveOrders(ctx context.Context, marketID *int16, cursor string, limit int) (*InactiveOrdersPage, error) {
	if limit < 1 || limit > maxInactiveOrdersLimit {
		return nil, fmt.Errorf("inactive orders limit must be between 1 and %d", maxInactiveOrdersLimit)
	}

	query := url.Values{}
	query.Set("account_index", strconv.FormatInt(m.AccountIndex, 10))
	query.Set("limit", strconv.Itoa(limit))
	if marketID != nil {
		query.Set("market_id", strconv.FormatInt(int64(*marketID), 10))
	}
	if cursor != "" {
		query.Set("cursor", cursor)
	}

	response, err := m.authenticatedOrdersGet(ctx, "/api/v1/accountInactiveOrders", query)
	if err != nil {
		return nil, err
	}
	next := response.NextCursor
	if next == "" {
		next = response.Cursor
	}
	return &InactiveOrdersPage{Orders: response.Orders, Cursor: next}, nil
}
