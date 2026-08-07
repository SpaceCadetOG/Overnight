package models

import (
	"fmt"
	"time"
)

type TradePlan struct {
	Date      time.Time
	Direction Bias

	Entry float64
	Stop  float64
	TP1   float64
	TP2   float64

	EntrySource string
	StopSource  string
	TP1Source   string

	RiskDistance    float64
	Reward1Distance float64
	Reward2Distance float64

	RR1 float64
	RR2 float64

	Valid         bool
	InvalidReason string
}

func (p TradePlan) Validate() error {
	if p.Direction != BiasLong && p.Direction != BiasShort {
		return fmt.Errorf("invalid trade direction: %s", p.Direction)
	}

	if p.Entry <= 0 {
		return fmt.Errorf("entry must be positive")
	}

	if p.Stop <= 0 {
		return fmt.Errorf("stop must be positive")
	}

	if p.TP1 <= 0 || p.TP2 <= 0 {
		return fmt.Errorf("targets must be positive")
	}

	switch p.Direction {
	case BiasLong:
		if p.Stop >= p.Entry {
			return fmt.Errorf(
				"long stop %.2f must be below entry %.2f",
				p.Stop,
				p.Entry,
			)
		}

		if p.TP1 <= p.Entry {
			return fmt.Errorf(
				"long TP1 %.2f must be above entry %.2f",
				p.TP1,
				p.Entry,
			)
		}

		if p.TP2 <= p.Entry {
			return fmt.Errorf(
				"long TP2 %.2f must be above entry %.2f",
				p.TP2,
				p.Entry,
			)
		}

	case BiasShort:
		if p.Stop <= p.Entry {
			return fmt.Errorf(
				"short stop %.2f must be above entry %.2f",
				p.Stop,
				p.Entry,
			)
		}

		if p.TP1 >= p.Entry {
			return fmt.Errorf(
				"short TP1 %.2f must be below entry %.2f",
				p.TP1,
				p.Entry,
			)
		}

		if p.TP2 >= p.Entry {
			return fmt.Errorf(
				"short TP2 %.2f must be below entry %.2f",
				p.TP2,
				p.Entry,
			)
		}
	}

	return nil
}
