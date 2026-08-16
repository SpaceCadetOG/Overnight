package lighteradapter

import (
	"context"
	"net/url"
	"strconv"
	"time"
)

func (c *Client) AccountsByL1(ctx context.Context, address string) (AccountsResult, error) {
	now := time.Now().UTC()
	var response struct {
		Code     int       `json:"code"`
		Total    int       `json:"total"`
		Accounts []Account `json:"accounts"`
	}
	err := c.doGET(ctx, "/api/v1/account", url.Values{
		"by":    {"l1_address"},
		"value": {address},
	}, nil, &response)
	result := AccountsResult{
		Code:          response.Code,
		Total:         response.Total,
		Items:         response.Accounts,
		Freshness:     FreshnessFresh,
		RetrievedAt:   now,
		Authoritative: true,
	}
	if err != nil {
		result.Freshness = FreshnessFailed
		result.Authoritative = false
		result.Error = err.Error()
		return result, err
	}
	return result, nil
}

func (c *Client) AccountByIndex(ctx context.Context, accountIndex int64) (AccountsResult, error) {
	now := time.Now().UTC()
	var response struct {
		Code     int       `json:"code"`
		Total    int       `json:"total"`
		Accounts []Account `json:"accounts"`
	}
	err := c.doGET(ctx, "/api/v1/account", url.Values{
		"by":    {"index"},
		"value": {strconv.FormatInt(accountIndex, 10)},
	}, nil, &response)
	result := AccountsResult{
		Code:          response.Code,
		Total:         response.Total,
		Items:         response.Accounts,
		Freshness:     FreshnessFresh,
		RetrievedAt:   now,
		Authoritative: true,
	}
	if err != nil {
		result.Freshness = FreshnessFailed
		result.Authoritative = false
		result.Error = err.Error()
		return result, err
	}
	return result, nil
}

func (c *Client) ActiveOrders(ctx context.Context, accountIndex int64, marketID int) (ActiveOrdersResult, error) {
	now := time.Now().UTC()
	token, err := c.authToken()
	if err != nil {
		result := ActiveOrdersResult{
			Freshness:     FreshnessFailed,
			RetrievedAt:   now,
			Error:         err.Error(),
			Authoritative: false,
		}
		return result, err
	}

	var response struct {
		Code       int     `json:"code"`
		Message    string  `json:"message"`
		NextCursor string  `json:"next_cursor"`
		Orders     []Order `json:"orders"`
	}
	err = c.doGET(ctx, "/api/v1/accountActiveOrders", url.Values{
		"account_index": {strconv.FormatInt(accountIndex, 10)},
		"market_id":     {strconv.Itoa(marketID)},
		"auth":          {token},
	}, nil, &response)
	result := ActiveOrdersResult{
		Code:          response.Code,
		NextCursor:    response.NextCursor,
		Items:         response.Orders,
		Freshness:     FreshnessFresh,
		RetrievedAt:   now,
		Authoritative: true,
	}
	if err != nil {
		result.Freshness = FreshnessFailed
		result.Authoritative = false
		result.Error = err.Error()
		return result, err
	}
	return result, nil
}

func (c *Client) HistoricalOrders(ctx context.Context, accountIndex int64, marketID int, limit int) (HistoricalOrdersResult, error) {
	now := time.Now().UTC()
	token, err := c.authToken()
	if err != nil {
		result := HistoricalOrdersResult{
			Freshness:     FreshnessFailed,
			RetrievedAt:   now,
			Error:         err.Error(),
			Authoritative: false,
		}
		return result, err
	}
	if limit <= 0 {
		limit = 100
	}

	var response struct {
		Code       int     `json:"code"`
		Message    string  `json:"message"`
		NextCursor string  `json:"next_cursor"`
		Orders     []Order `json:"orders"`
	}
	err = c.doGET(ctx, "/api/v1/accountInactiveOrders", url.Values{
		"account_index": {strconv.FormatInt(accountIndex, 10)},
		"market_id":     {strconv.Itoa(marketID)},
		"limit":         {strconv.Itoa(limit)},
		"auth":          {token},
	}, nil, &response)
	result := HistoricalOrdersResult{
		Code:          response.Code,
		NextCursor:    response.NextCursor,
		Items:         response.Orders,
		Freshness:     FreshnessFresh,
		RetrievedAt:   now,
		Authoritative: true,
	}
	if err != nil {
		result.Freshness = FreshnessFailed
		result.Authoritative = false
		result.Error = err.Error()
		return result, err
	}
	return result, nil
}

func (c *Client) Fills(ctx context.Context, accountIndex int64, limit int) (FillsResult, error) {
	now := time.Now().UTC()
	token, err := c.authToken()
	if err != nil {
		result := FillsResult{
			Freshness:     FreshnessFailed,
			RetrievedAt:   now,
			Error:         err.Error(),
			Authoritative: false,
		}
		return result, err
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	var response struct {
		Code       int         `json:"code"`
		NextCursor string      `json:"next_cursor"`
		Trades     []TradeFill `json:"trades"`
	}
	err = c.doGET(ctx, "/api/v1/trades", url.Values{
		"market_id":     {"255"},
		"market_type":   {"all"},
		"account_index": {strconv.FormatInt(accountIndex, 10)},
		"sort_by":       {"timestamp"},
		"sort_dir":      {"desc"},
		"limit":         {strconv.Itoa(limit)},
	}, map[string]string{
		"Authorization": token,
	}, &response)
	result := FillsResult{
		Code:          response.Code,
		NextCursor:    response.NextCursor,
		Items:         response.Trades,
		Freshness:     FreshnessFresh,
		RetrievedAt:   now,
		Authoritative: true,
	}
	if err != nil {
		result.Freshness = FreshnessFailed
		result.Authoritative = false
		result.Error = err.Error()
		return result, err
	}
	return result, nil
}

func (c *Client) Funding(ctx context.Context, accountIndex int64, limit int, side string) (FundingResult, error) {
	now := time.Now().UTC()
	token, err := c.authToken()
	if err != nil {
		result := FundingResult{
			Freshness:     FreshnessFailed,
			RetrievedAt:   now,
			Error:         err.Error(),
			Authoritative: false,
		}
		return result, err
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	if side == "" {
		side = "all"
	}

	var response struct {
		Code             int             `json:"code"`
		NextCursor       string          `json:"next_cursor"`
		PositionFundings []FundingRecord `json:"position_fundings"`
		Fundings         []FundingRecord `json:"fundings"`
	}
	err = c.doGET(ctx, "/api/v1/positionFunding", url.Values{
		"account_index": {strconv.FormatInt(accountIndex, 10)},
		"limit":         {strconv.Itoa(limit)},
		"side":          {side},
	}, map[string]string{
		"Authorization": token,
	}, &response)
	items := make([]FundingRecord, 0, len(response.PositionFundings)+len(response.Fundings))
	items = append(items, response.PositionFundings...)
	items = append(items, response.Fundings...)
	result := FundingResult{
		Code:          response.Code,
		NextCursor:    response.NextCursor,
		Items:         items,
		Freshness:     FreshnessFresh,
		RetrievedAt:   now,
		Authoritative: true,
	}
	if err != nil {
		result.Freshness = FreshnessFailed
		result.Authoritative = false
		result.Error = err.Error()
		return result, err
	}
	return result, nil
}
