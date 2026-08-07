package hyperliquid

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

const defaultInfoURL = "https://api.hyperliquid.xyz/info"

type Client struct {
	infoURL    string
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		infoURL: defaultInfoURL,
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

type candleRequest struct {
	Type string           `json:"type"`
	Req  candleRequestReq `json:"req"`
}

type candleRequestReq struct {
	Coin      string `json:"coin"`
	Interval  string `json:"interval"`
	StartTime int64  `json:"startTime"`
	EndTime   int64  `json:"endTime"`
}

type candleResponse struct {
	OpenTime  int64  `json:"t"`
	CloseTime int64  `json:"T"`
	Symbol    string `json:"s"`
	Interval  string `json:"i"`

	Open   string `json:"o"`
	Close  string `json:"c"`
	High   string `json:"h"`
	Low    string `json:"l"`
	Volume string `json:"v"`

	Trades int `json:"n"`
}

func (c *Client) FetchCandles(
	ctx context.Context,
	coin string,
	interval string,
	start time.Time,
	end time.Time,
) ([]models.Candle, error) {
	if coin == "" {
		return nil, fmt.Errorf("coin is required")
	}

	if interval == "" {
		return nil, fmt.Errorf("interval is required")
	}

	if !end.After(start) {
		return nil, fmt.Errorf("end must be after start")
	}

	payload := candleRequest{
		Type: "candleSnapshot",
		Req: candleRequestReq{
			Coin:      coin,
			Interval:  interval,
			StartTime: start.UTC().UnixMilli(),
			EndTime:   end.UTC().UnixMilli(),
		},
	}

	requestBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal candle request: %w", err)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.infoURL,
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return nil, fmt.Errorf("create candle request: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request Hyperliquid candles: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read Hyperliquid response: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"Hyperliquid returned status %d: %s",
			response.StatusCode,
			string(body),
		)
	}

	var rawCandles []candleResponse
	if err := json.Unmarshal(body, &rawCandles); err != nil {
		return nil, fmt.Errorf("decode Hyperliquid candles: %w", err)
	}

	candles := make([]models.Candle, 0, len(rawCandles))

	for i, raw := range rawCandles {
		candle, err := raw.toModel()
		if err != nil {
			return nil, fmt.Errorf("parse candle %d: %w", i, err)
		}

		if err := candle.Validate(); err != nil {
			return nil, fmt.Errorf("validate candle %d: %w", i, err)
		}

		candles = append(candles, candle)
	}

	return candles, nil
}

func (c candleResponse) toModel() (models.Candle, error) {
	open, err := parseFloat(c.Open)
	if err != nil {
		return models.Candle{}, fmt.Errorf("open: %w", err)
	}

	high, err := parseFloat(c.High)
	if err != nil {
		return models.Candle{}, fmt.Errorf("high: %w", err)
	}

	low, err := parseFloat(c.Low)
	if err != nil {
		return models.Candle{}, fmt.Errorf("low: %w", err)
	}

	closePrice, err := parseFloat(c.Close)
	if err != nil {
		return models.Candle{}, fmt.Errorf("close: %w", err)
	}

	volume, err := parseFloat(c.Volume)
	if err != nil {
		return models.Candle{}, fmt.Errorf("volume: %w", err)
	}

	return models.Candle{
		OpenTime:  time.UnixMilli(c.OpenTime).UTC(),
		CloseTime: time.UnixMilli(c.CloseTime).UTC(),
		Open:      open,
		High:      high,
		Low:       low,
		Close:     closePrice,
		Volume:    volume,
	}, nil
}

func parseFloat(value string) (float64, error) {
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %q: %w", value, err)
	}

	return number, nil
}
