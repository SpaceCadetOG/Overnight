package config

import (
	"fmt"
	"time"
)

// Config contains the initial overnight-strategy settings.
type Config struct {
	Symbol   string
	Interval string
	Timezone string

	SessionStartHour int
	SessionEndHour   int
	OrderPlaceHour   int
	OrderCancelHour  int

	PositionNotional float64
	TP1Fraction      float64
	TP2Fraction      float64
	StopBufferBPS    float64
}

// Default returns the baseline strategy configuration.
func Default() Config {
	return Config{
		Symbol:   "BTCUSDT",
		Interval: "5m",
		Timezone: "America/Chicago",

		SessionStartHour: 19,
		SessionEndHour:   5,
		OrderPlaceHour:   5,
		OrderCancelHour:  11,

		PositionNotional: 100.00,
		TP1Fraction:      0.50,
		TP2Fraction:      0.50,
		StopBufferBPS:    2.0,
	}
}

// Location loads the configured strategy timezone.
func (c Config) Location() (*time.Location, error) {
	location, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return nil, fmt.Errorf("load timezone %q: %w", c.Timezone, err)
	}

	return location, nil
}

// Validate checks configuration integrity.
func (c Config) Validate() error {
	if c.Symbol == "" {
		return fmt.Errorf("symbol is required")
	}

	if c.Interval == "" {
		return fmt.Errorf("interval is required")
	}

	if c.SessionStartHour < 0 || c.SessionStartHour > 23 {
		return fmt.Errorf("invalid session start hour: %d", c.SessionStartHour)
	}

	if c.SessionEndHour < 0 || c.SessionEndHour > 23 {
		return fmt.Errorf("invalid session end hour: %d", c.SessionEndHour)
	}

	if c.OrderPlaceHour < 0 || c.OrderPlaceHour > 23 {
		return fmt.Errorf("invalid order placement hour: %d", c.OrderPlaceHour)
	}

	if c.OrderCancelHour < 0 || c.OrderCancelHour > 23 {
		return fmt.Errorf("invalid order cancellation hour: %d", c.OrderCancelHour)
	}

	if c.OrderCancelHour <= c.OrderPlaceHour {
		return fmt.Errorf("order cancellation must occur after placement")
	}

	if c.PositionNotional <= 0 {
		return fmt.Errorf("position notional must be positive")
	}

	if c.TP1Fraction < 0 || c.TP2Fraction < 0 {
		return fmt.Errorf("target allocations cannot be negative")
	}

	targetTotal := c.TP1Fraction + c.TP2Fraction
	if targetTotal < 0.999999 || targetTotal > 1.000001 {
		return fmt.Errorf(
			"target allocations must total 1.0, got %.4f",
			targetTotal,
		)
	}

	if c.StopBufferBPS < 0 {
		return fmt.Errorf("stop buffer cannot be negative")
	}

	if _, err := c.Location(); err != nil {
		return err
	}

	return nil
}
