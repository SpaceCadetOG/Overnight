package ws

import (
	"encoding/json"
	"strconv"
	"time"
)

type PositionEvent struct {
	Channel   string                     `json:"channel"`
	Positions map[string]PositionPayload `json:"positions"`
	Type      string                     `json:"type"`
}

type PositionPayload struct {
	MarketID         int    `json:"market_id"`
	Symbol           string `json:"symbol"`
	Position         string `json:"position"`
	AvgEntryPrice    string `json:"avg_entry_price"`
	UnrealizedPnL    string `json:"unrealized_pnl"`
	LiquidationPrice string `json:"liquidation_price"`
	Sign             int    `json:"sign"`
}

func ParsePositionEvent(
	data []byte,
) ([]PositionSnapshot, error) {

	var event PositionEvent

	err := json.Unmarshal(
		data,
		&event,
	)

	if err != nil {
		return nil, err
	}

	out := make([]PositionSnapshot, 0)

	for _, p := range event.Positions {

		size, _ := strconv.ParseFloat(
			p.Position,
			64,
		)

		entry, _ := strconv.ParseFloat(
			p.AvgEntryPrice,
			64,
		)

		pnl, _ := strconv.ParseFloat(
			p.UnrealizedPnL,
			64,
		)

		liq, _ := strconv.ParseFloat(
			p.LiquidationPrice,
			64,
		)

		side := "FLAT"

		if size > 0 {

			if p.Sign == 1 {
				side = "LONG"
			} else {
				side = "SHORT"
			}

		}

		out = append(out, PositionSnapshot{

			Symbol:           p.Symbol,
			Side:             side,
			Size:             size,
			EntryPrice:       entry,
			UnrealizedPnL:    pnl,
			LiquidationPrice: liq,
			Timestamp:        time.Now().UTC(),
		})
	}

	return out, nil
}
