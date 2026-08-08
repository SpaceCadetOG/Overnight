package ws

import (
	"time"

	adapter "github.com/ogtrading/overnight-strategy/internal/adapters/lighter"
)

type PositionSnapshot struct {
	Symbol           string
	Side             string
	Size             float64
	EntryPrice       float64
	UnrealizedPnL    float64
	LiquidationPrice float64
	Timestamp        time.Time
}

func ReadPosition(
	client *adapter.Client,
	accountIndex int64,
	symbol string,
) (*PositionSnapshot, error) {

	position, err := client.GetPosition(
		accountIndex,
		symbol,
	)

	if err != nil {
		return nil, err
	}

	return &PositionSnapshot{
		Symbol:           position.Symbol,
		Side:             position.Side,
		Size:             position.Size,
		EntryPrice:       position.EntryPrice,
		UnrealizedPnL:    position.UnrealizedPnL,
		LiquidationPrice: position.LiquidationPrice,
		Timestamp:        time.Now().UTC(),
	}, nil
}
