package models

import "time"

type Bias string

const (
	BiasLong  Bias = "LONG"
	BiasShort Bias = "SHORT"
	BiasNone  Bias = "NONE"
)

// Session represents one completed overnight trading range.
type Session struct {
	Date time.Time

	Start time.Time
	End   time.Time

	Candles []Candle

	Open  float64
	High  float64
	Low   float64
	Close float64

	VWAP float64

	Fib382 float64
	Fib500 float64
	Fib618 float64

	Entry float64

	POC float64
	VAH float64
	VAL float64

	Bias Bias
}
