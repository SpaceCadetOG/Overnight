package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

const defaultBaseURL = "https://fapi.binance.com"

// Client downloads public Binance USDⓈ-M Futures market data.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient returns a Binance market-data client.
func NewClient() *Client {
	return &Client{
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

// NewClientWithBaseURL is primarily useful for tests.
func NewClientWithBaseURL(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}

	return &Client{
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

// FetchCandles downloads one page of futures candles.
func (c *Client) FetchCandles(
	ctx context.Context,
	symbol string,
	interval string,
	start time.Time,
	end time.Time,
	limit int,
) ([]models.Candle, error) {
	if symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}

	if interval == "" {
		return nil, fmt.Errorf("interval is required")
	}

	if !end.After(start) {
		return nil, fmt.Errorf("end time must be after start time")
	}

	if limit <= 0 || limit > 1500 {
		return nil, fmt.Errorf("limit must be between 1 and 1500")
	}

	endpoint, err := url.Parse(c.baseURL + "/fapi/v1/klines")
	if err != nil {
		return nil, fmt.Errorf("parse Binance URL: %w", err)
	}

	query := endpoint.Query()
	query.Set("symbol", symbol)
	query.Set("interval", interval)
	query.Set("startTime", strconv.FormatInt(start.UnixMilli(), 10))
	query.Set("endTime", strconv.FormatInt(end.UnixMilli(), 10))
	query.Set("limit", strconv.Itoa(limit))
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		endpoint.String(),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("create Binance request: %w", err)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request Binance candles: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read Binance response: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"Binance returned status %d: %s",
			response.StatusCode,
			string(body),
		)
	}

	var rows [][]json.RawMessage
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode Binance candles: %w", err)
	}

	candles := make([]models.Candle, 0, len(rows))

	for index, row := range rows {
		candle, err := parseKline(row)
		if err != nil {
			return nil, fmt.Errorf("parse candle %d: %w", index, err)
		}

		if err := candle.Validate(); err != nil {
			return nil, fmt.Errorf("validate candle %d: %w", index, err)
		}

		candles = append(candles, candle)
	}

	return candles, nil
}

func parseKline(row []json.RawMessage) (models.Candle, error) {
	if len(row) < 6 {
		return models.Candle{}, fmt.Errorf(
			"expected at least 6 kline fields, got %d",
			len(row),
		)
	}

	openTimeMS, err := parseInt64(row[0])
	if err != nil {
		return models.Candle{}, fmt.Errorf("open time: %w", err)
	}

	open, err := parseFloat(row[1])
	if err != nil {
		return models.Candle{}, fmt.Errorf("open: %w", err)
	}

	high, err := parseFloat(row[2])
	if err != nil {
		return models.Candle{}, fmt.Errorf("high: %w", err)
	}

	low, err := parseFloat(row[3])
	if err != nil {
		return models.Candle{}, fmt.Errorf("low: %w", err)
	}

	closePrice, err := parseFloat(row[4])
	if err != nil {
		return models.Candle{}, fmt.Errorf("close: %w", err)
	}

	volume, err := parseFloat(row[5])
	if err != nil {
		return models.Candle{}, fmt.Errorf("volume: %w", err)
	}

	closeTime := time.UnixMilli(openTimeMS)

	if len(row) > 6 {
		closeTimeMS, parseErr := parseInt64(row[6])
		if parseErr == nil {
			closeTime = time.UnixMilli(closeTimeMS)
		}
	}

	return models.Candle{
		OpenTime:  time.UnixMilli(openTimeMS).UTC(),
		CloseTime: closeTime.UTC(),
		Open:      open,
		High:      high,
		Low:       low,
		Close:     closePrice,
		Volume:    volume,
	}, nil
}

func parseFloat(raw json.RawMessage) (float64, error) {
	var value string

	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("decode numeric string: %w", err)
	}

	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %q: %w", value, err)
	}

	return number, nil
}

func parseInt64(raw json.RawMessage) (int64, error) {
	var value int64

	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("decode integer: %w", err)
	}

	return value, nil
}
