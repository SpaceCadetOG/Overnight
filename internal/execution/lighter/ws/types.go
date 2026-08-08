package ws

import "time"

type Position struct {
	Symbol           string
	Size             float64
	EntryPrice       float64
	MarkPrice        float64
	UnrealizedPnL    float64
	LiquidationPrice float64
	Timestamp        time.Time
}
