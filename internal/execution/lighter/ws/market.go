package ws

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

type PriceSnapshot struct {
	Symbol string

	Price float64

	Timestamp time.Time
}

func ParseMarketPrice(
	data []byte,
) (*PriceSnapshot, error) {

	var raw struct {
		Channel   string `json:"channel"`
		MarketID  string `json:"market_id"`
		Symbol    string `json:"symbol"`
		Price     string `json:"price"`
		MarkPrice string `json:"mark_price"`
		Type      string `json:"type"`
	}

	err := json.Unmarshal(
		data,
		&raw,
	)

	if err != nil {
		return nil, err
	}

	priceString := raw.Price

	if priceString == "" {
		priceString = raw.MarkPrice
	}

	if priceString == "" {

		return nil,
			fmt.Errorf("no price field")
	}

	price, err := strconv.ParseFloat(
		priceString,
		64,
	)

	if err != nil {
		return nil, err
	}

	if raw.Symbol == "" {

		return nil,
			fmt.Errorf("missing symbol")
	}

	return &PriceSnapshot{

		Symbol: raw.Symbol,

		Price: price,

		Timestamp: time.Now().UTC(),
	}, nil
}
