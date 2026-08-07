package execution

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	lighterdata "github.com/ogtrading/overnight-strategy/internal/marketdata/lighter"
)

type MarketSpec struct {
	Symbol, Status                  string
	MarketID                        int16
	PriceDecimals, QuantityDecimals int
	TickSize, QuantityStep          float64
	MinimumBase, MinimumNotional    float64
}

func SpecFromMarket(m lighterdata.Market) (MarketSpec, error) {
	minBase, err := rawFloat(m.MinBaseAmount)
	if err != nil {
		return MarketSpec{}, fmt.Errorf("%s min base: %w", m.Symbol, err)
	}
	minQuote, err := rawFloat(m.MinQuoteAmount)
	if err != nil {
		return MarketSpec{}, fmt.Errorf("%s min quote: %w", m.Symbol, err)
	}
	return MarketSpec{Symbol: m.Symbol, Status: m.Status, MarketID: m.MarketID, PriceDecimals: m.PriceDecimals, QuantityDecimals: m.SizeDecimals, TickSize: math.Pow10(-m.PriceDecimals), QuantityStep: math.Pow10(-m.SizeDecimals), MinimumBase: minBase, MinimumNotional: minQuote}, nil
}

type Order struct {
	Symbol    string  `json:"symbol"`
	Side      string  `json:"side"`
	Price     float64 `json:"price"`
	Quantity  float64 `json:"quantity"`
	Stop      float64 `json:"stop"`
	TP1       float64 `json:"tp1"`
	TP2       float64 `json:"tp2"`
	ExpiresAt int64   `json:"expires_at"`
}

func (s MarketSpec) Normalize(order Order) Order {
	order.Price = roundDown(order.Price, s.TickSize)
	order.Quantity = roundDown(order.Quantity, s.QuantityStep)
	order.Stop = roundTo(order.Stop, s.TickSize)
	order.TP1 = roundTo(order.TP1, s.TickSize)
	order.TP2 = roundTo(order.TP2, s.TickSize)
	return order
}

func (s MarketSpec) Validate(order Order) error {
	if s.Status != "active" {
		return fmt.Errorf("market %s is not active", s.Symbol)
	}
	if order.Symbol != s.Symbol {
		return fmt.Errorf("order symbol %s does not match spec %s", order.Symbol, s.Symbol)
	}
	if order.Side != "BUY" && order.Side != "SELL" {
		return fmt.Errorf("invalid side %s", order.Side)
	}
	if order.Price <= 0 || order.Quantity <= 0 {
		return fmt.Errorf("price and quantity must be positive")
	}
	if !aligned(order.Price, s.TickSize) {
		return fmt.Errorf("price %.10f violates tick %.10f", order.Price, s.TickSize)
	}
	if !aligned(order.Quantity, s.QuantityStep) {
		return fmt.Errorf("quantity %.10f violates step %.10f", order.Quantity, s.QuantityStep)
	}
	if order.Quantity+1e-12 < s.MinimumBase {
		return fmt.Errorf("quantity %.10f below minimum %.10f", order.Quantity, s.MinimumBase)
	}
	if order.Price*order.Quantity+1e-9 < s.MinimumNotional {
		return fmt.Errorf("notional %.6f below minimum %.6f", order.Price*order.Quantity, s.MinimumNotional)
	}
	if order.ExpiresAt <= 0 {
		return fmt.Errorf("expiry is required")
	}
	if order.Side == "BUY" && !(order.Stop < order.Price && order.Price < order.TP1 && order.TP1 < order.TP2) {
		return fmt.Errorf("invalid BUY price geometry: stop < entry < tp1 < tp2 is required")
	}
	if order.Side == "SELL" && !(order.Stop > order.Price && order.Price > order.TP1 && order.TP1 > order.TP2) {
		return fmt.Errorf("invalid SELL price geometry: stop > entry > tp1 > tp2 is required")
	}
	return nil
}

func rawFloat(raw json.RawMessage) (float64, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return 0, nil
	}
	if trimmed[0] == '"' {
		var value string
		if err := json.Unmarshal(trimmed, &value); err != nil {
			return 0, err
		}
		return strconv.ParseFloat(value, 64)
	}
	return strconv.ParseFloat(string(trimmed), 64)
}

func roundDown(value, step float64) float64 { return math.Floor(value/step+1e-9) * step }
func roundTo(value, step float64) float64   { return math.Round(value/step) * step }
func aligned(value, step float64) bool      { return math.Abs(value/step-math.Round(value/step)) < 1e-7 }
