package lighteradapter

import (
	"context"
	"net/url"
	"time"
)

func (c *Client) Markets(ctx context.Context) (MarketsResult, error) {
	now := time.Now().UTC()
	var response struct {
		Code             int      `json:"code"`
		OrderBookDetails []Market `json:"order_book_details"`
	}
	err := c.doGET(ctx, "/api/v1/orderBookDetails", nil, nil, &response)
	result := MarketsResult{
		Code:          response.Code,
		Items:         response.OrderBookDetails,
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

func (c *Client) ExchangeStats(ctx context.Context) (ExchangeStatsResult, error) {
	now := time.Now().UTC()
	var response struct {
		Code           int            `json:"code"`
		OrderBookStats []ExchangeStat `json:"order_book_stats"`
	}
	err := c.doGET(ctx, "/api/v1/exchangeStats", nil, nil, &response)
	result := ExchangeStatsResult{
		Code:          response.Code,
		Items:         response.OrderBookStats,
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

func (c *Client) FundingRates(ctx context.Context) (FundingRatesResult, error) {
	now := time.Now().UTC()
	var response struct {
		Code         int           `json:"code"`
		FundingRates []FundingRate `json:"funding_rates"`
	}
	err := c.doGET(ctx, "/api/v1/funding-rates", url.Values{}, nil, &response)
	result := FundingRatesResult{
		Code:          response.Code,
		Items:         response.FundingRates,
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
