package models

import (
	"fmt"
	"time"
)

// Candle represents one completed OHLCV candle.
type Candle struct {
	OpenTime  time.Time
	CloseTime time.Time

	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

// Validate verifies that the candle contains valid market data.
func (c Candle) Validate() error {
	if c.OpenTime.IsZero() {
		return fmt.Errorf("open time is required")
	}

	if c.CloseTime.Before(c.OpenTime) {
		return fmt.Errorf("close time cannot be before open time")
	}

	if c.High < c.Low {
		return fmt.Errorf("high %.8f is below low %.8f", c.High, c.Low)
	}

	if c.Open < c.Low || c.Open > c.High {
		return fmt.Errorf(
			"open %.8f is outside candle range %.8f–%.8f",
			c.Open,
			c.Low,
			c.High,
		)
	}

	if c.Close < c.Low || c.Close > c.High {
		return fmt.Errorf(
			"close %.8f is outside candle range %.8f–%.8f",
			c.Close,
			c.Low,
			c.High,
		)
	}

	if c.Volume < 0 {
		return fmt.Errorf("volume cannot be negative")
	}

	return nil
}
