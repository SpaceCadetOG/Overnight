package lighter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

const DefaultBaseURL = "https://mainnet.zklighter.elliot.ai"

type Market struct {
	Symbol                string          `json:"symbol"`
	MarketID              int16           `json:"market_id"`
	Status                string          `json:"status"`
	MarketType            string          `json:"market_type"`
	DailyBaseTokenVolume  json.RawMessage `json:"daily_base_token_volume"`
	DailyQuoteTokenVolume json.RawMessage `json:"daily_quote_token_volume"`
	DailyTradesCount      json.RawMessage `json:"daily_trades_count"`
	OpenInterest          json.RawMessage `json:"open_interest"`
	MarkPrice             json.RawMessage `json:"mark_price"`
	MinBaseAmount         json.RawMessage `json:"min_base_amount"`
	MinQuoteAmount        json.RawMessage `json:"min_quote_amount"`
	PriceDecimals         int             `json:"supported_price_decimals"`
	SizeDecimals          int             `json:"supported_size_decimals"`
	QuoteDecimals         int             `json:"supported_quote_decimals"`
}

type Funding struct {
	Timestamp int64           `json:"timestamp"`
	Raw       json.RawMessage `json:"-"`
}

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string, client *http.Client) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: client}
}

func (c *Client) Markets(ctx context.Context) ([]Market, error) {
	var response struct {
		Markets []Market `json:"order_book_details"`
	}
	if err := c.get(ctx, "/api/v1/orderBookDetails", nil, &response); err != nil {
		return nil, err
	}
	return response.Markets, nil
}

func (c *Client) MarketMap(ctx context.Context) (map[string]Market, error) {
	markets, err := c.Markets(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Market, len(markets))
	for _, market := range markets {
		out[market.Symbol] = market
	}
	return out, nil
}

func (c *Client) Candles(ctx context.Context, marketID int16, resolution string, start, end time.Time) ([]models.Candle, error) {
	if !end.After(start) {
		return nil, fmt.Errorf("end must be after start")
	}
	if resolution == "" {
		resolution = "5m"
	}
	const page = 500
	step, err := resolutionDuration(resolution)
	if err != nil {
		return nil, err
	}
	out := []models.Candle{}
	for cursor := start.UTC(); cursor.Before(end); {
		pageEnd := cursor.Add(step * page)
		if pageEnd.After(end) {
			pageEnd = end
		}
		query := url.Values{
			"market_id": {strconv.Itoa(int(marketID))}, "resolution": {resolution},
			"start_timestamp": {strconv.FormatInt(cursor.UnixMilli(), 10)},
			"end_timestamp":   {strconv.FormatInt(pageEnd.UnixMilli(), 10)}, "count_back": {"0"},
		}
		var response struct {
			Candles []struct {
				T             int64 `json:"t"`
				O, H, L, C, V float64
			} `json:"c"`
		}
		if err := c.get(ctx, "/api/v1/candles", query, &response); err != nil {
			return nil, err
		}
		for _, row := range response.Candles {
			open := time.UnixMilli(row.T).UTC()
			out = append(out, models.Candle{OpenTime: open, CloseTime: open.Add(step), Open: row.O, High: row.H, Low: row.L, Close: row.C, Volume: row.V})
		}
		if !pageEnd.After(cursor) {
			break
		}
		cursor = pageEnd
	}
	return dedupeCandles(out), nil
}

func (c *Client) RawFundings(ctx context.Context, marketID int16, start, end time.Time) (json.RawMessage, error) {
	query := url.Values{"market_id": {strconv.Itoa(int(marketID))}, "resolution": {"1h"}, "start_timestamp": {strconv.FormatInt(start.UnixMilli(), 10)}, "end_timestamp": {strconv.FormatInt(end.UnixMilli(), 10)}, "count_back": {"0"}}
	var raw json.RawMessage
	if err := c.get(ctx, "/api/v1/fundings", query, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (c *Client) RawTrades(ctx context.Context, marketID int16, limit int) (json.RawMessage, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	query := url.Values{"market_id": {strconv.Itoa(int(marketID))}, "market_type": {"perp"}, "sort_by": {"timestamp"}, "sort_dir": {"desc"}, "limit": {strconv.Itoa(limit)}}
	var raw json.RawMessage
	if err := c.get(ctx, "/api/v1/trades", query, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values, output any) error {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return fmt.Errorf("Lighter %s returned HTTP %d", path, response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(output); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func resolutionDuration(value string) (time.Duration, error) {
	switch value {
	case "1m":
		return time.Minute, nil
	case "5m":
		return 5 * time.Minute, nil
	case "15m":
		return 15 * time.Minute, nil
	case "30m":
		return 30 * time.Minute, nil
	case "1h":
		return time.Hour, nil
	case "4h":
		return 4 * time.Hour, nil
	case "12h":
		return 12 * time.Hour, nil
	case "1d":
		return 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported resolution %q", value)
	}
}

func dedupeCandles(input []models.Candle) []models.Candle {
	seen := map[int64]bool{}
	out := make([]models.Candle, 0, len(input))
	for _, candle := range input {
		key := candle.OpenTime.UnixMilli()
		if !seen[key] {
			seen[key] = true
			out = append(out, candle)
		}
	}
	return out
}
