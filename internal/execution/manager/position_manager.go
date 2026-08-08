package manager

import (
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

type Action string

const (
	Hold     Action = "HOLD"
	TP1Hit   Action = "TP1_HIT"
	MoveStop Action = "MOVE_STOP"
	TP2Hit   Action = "TP2_HIT"
	StopHit  Action = "STOP_HIT"
)

type State struct {
	Symbol string

	Entry float64

	Size float64

	Current float64

	Action Action

	Timestamp time.Time
}

func Evaluate(
	plan models.TradePlan,
	symbol string,
	size float64,
	price float64,
) State {

	action := Hold

	switch plan.Direction {

	case models.BiasLong:

		if price <= plan.Stop {
			action = StopHit
		}

		if price >= plan.TP2 {
			action = TP2Hit
		} else if price >= plan.TP1 {
			action = TP1Hit
		}

	case models.BiasShort:

		if price >= plan.Stop {
			action = StopHit
		}

		if price <= plan.TP2 {
			action = TP2Hit
		} else if price <= plan.TP1 {
			action = TP1Hit
		}
	}

	return State{

		Symbol: symbol,

		Entry: plan.Entry,

		Size: size,

		Current: price,

		Action: action,

		Timestamp: time.Now().UTC(),
	}
}
