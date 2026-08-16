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

type Market struct {
	Symbol     string `json:"symbol"`
	MarketID   int16  `json:"market_id"`
	MarketType string `json:"market_type"`
	Status     string `json:"status"`

	MinBaseAmount  string `json:"min_base_amount"`
	MinQuoteAmount string `json:"min_quote_amount"`

	SupportedSizeDecimals  int `json:"supported_size_decimals"`
	SupportedPriceDecimals int `json:"supported_price_decimals"`
	SupportedQuoteDecimals int `json:"supported_quote_decimals"`

	SizeDecimals  int `json:"size_decimals"`
	PriceDecimals int `json:"price_decimals"`

	MarkPrice      string  `json:"mark_price"`
	IndexPrice     string  `json:"index_price"`
	LastTradePrice float64 `json:"last_trade_price"`
}

type orderBookDetailsResponse struct {
	Code             int      `json:"code"`
	Message          string   `json:"message"`
	OrderBookDetails []Market `json:"order_book_details"`
}

func normalizeSymbol(symbol string) string {
	return strings.ToUpper(strings.TrimSpace(symbol))
}

func (m *Manager) Markets(
	ctx context.Context,
) ([]Market, error) {
	u, err := url.Parse(m.BaseURL + "/api/v1/orderBookDetails")
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		u.String(),
		nil,
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("accept", "application/json")

	res, err := m.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("market metadata request: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"market metadata HTTP %s: %s",
			res.Status,
			string(body),
		)
	}

	var response orderBookDetailsResponse

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf(
			"decode market metadata: %w",
			err,
		)
	}

	if response.Code != 200 {
		return nil, fmt.Errorf(
			"market metadata API error code=%d message=%q",
			response.Code,
			response.Message,
		)
	}

	if len(response.OrderBookDetails) == 0 {
		return nil, fmt.Errorf("market metadata returned no markets")
	}

	return response.OrderBookDetails, nil
}

func (m *Manager) MarketByID(
	ctx context.Context,
	marketID int16,
) (*Market, error) {
	u, err := url.Parse(m.BaseURL + "/api/v1/orderBookDetails")
	if err != nil {
		return nil, err
	}

	q := u.Query()
	q.Set("market_id", strconv.FormatInt(int64(marketID), 10))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		u.String(),
		nil,
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("accept", "application/json")

	res, err := m.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf(
			"market metadata request market_id=%d: %w",
			marketID,
			err,
		)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"market metadata HTTP %s: %s",
			res.Status,
			string(body),
		)
	}

	var response orderBookDetailsResponse

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf(
			"decode market metadata: %w",
			err,
		)
	}

	if response.Code != 200 {
		return nil, fmt.Errorf(
			"market metadata API error code=%d message=%q",
			response.Code,
			response.Message,
		)
	}

	for i := range response.OrderBookDetails {
		if response.OrderBookDetails[i].MarketID == marketID {
			return &response.OrderBookDetails[i], nil
		}
	}

	return nil, fmt.Errorf(
		"market_id=%d not found",
		marketID,
	)
}

func (m *Manager) MarketBySymbol(
	ctx context.Context,
	symbol string,
) (*Market, error) {
	wanted := normalizeSymbol(symbol)

	if wanted == "" {
		return nil, fmt.Errorf("market symbol is required")
	}

	markets, err := m.Markets(ctx)
	if err != nil {
		return nil, err
	}

	for i := range markets {
		if normalizeSymbol(markets[i].Symbol) == wanted {
			return &markets[i], nil
		}
	}

	return nil, fmt.Errorf(
		"market symbol %q not found",
		wanted,
	)
}

func (m Market) Validate() error {
	if strings.TrimSpace(m.Symbol) == "" {
		return fmt.Errorf("market symbol is empty")
	}

	if m.MarketID < 0 {
		return fmt.Errorf(
			"market %s has invalid market ID %d",
			m.Symbol,
			m.MarketID,
		)
	}

	if !strings.EqualFold(m.Status, "active") {
		return fmt.Errorf(
			"market %s is not active: status=%q",
			m.Symbol,
			m.Status,
		)
	}

	if m.SupportedSizeDecimals < 0 ||
		m.SupportedSizeDecimals > 18 {
		return fmt.Errorf(
			"market %s invalid supported size decimals %d",
			m.Symbol,
			m.SupportedSizeDecimals,
		)
	}

	if m.SupportedPriceDecimals < 0 ||
		m.SupportedPriceDecimals > 18 {
		return fmt.Errorf(
			"market %s invalid supported price decimals %d",
			m.Symbol,
			m.SupportedPriceDecimals,
		)
	}

	if strings.TrimSpace(m.MinBaseAmount) == "" {
		return fmt.Errorf(
			"market %s has empty minimum base amount",
			m.Symbol,
		)
	}

	if strings.TrimSpace(m.MinQuoteAmount) == "" {
		return fmt.Errorf(
			"market %s has empty minimum quote amount",
			m.Symbol,
		)
	}

	return nil
}
