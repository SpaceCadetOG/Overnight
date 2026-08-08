package ws

import "time"

type Event struct {
	Type      string
	Symbol    string
	Size      float64
	Price     float64
	Timestamp time.Time
}
