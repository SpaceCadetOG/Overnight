package execution

import (
	"fmt"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

type PaperState string

const (
	Waiting     PaperState = "WAITING_FOR_FILL"
	PaperFilled PaperState = "FILLED"
	PaperTP1    PaperState = "TP1"
	PaperClosed PaperState = "CLOSED"
	PaperNoFill PaperState = "NO_FILL"
)

type PaperTrade struct {
	SchemaVersion   int        `json:"schema_version"`
	SessionDate     time.Time  `json:"session_date"`
	SessionID       string     `json:"session_id"`
	OpportunityID   string     `json:"opportunity_id"`
	StrategyOrderID string     `json:"strategy_order_id"`
	TradeID         string     `json:"trade_id"`
	RunID           string     `json:"run_id"`
	Order           Order      `json:"order"`
	State           PaperState `json:"state"`
	FillPrice       float64    `json:"fill_price,omitempty"`
	FillAt          time.Time  `json:"fill_at,omitempty"`
	ExitPrice       float64    `json:"exit_price,omitempty"`
	ExitAt          time.Time  `json:"exit_at,omitempty"`
	Outcome         string     `json:"outcome,omitempty"`
	TP1Hit          bool       `json:"tp1_hit"`
	TP1At           time.Time  `json:"tp1_at,omitempty"`
	MFE             float64    `json:"mfe"`
	MAE             float64    `json:"mae"`
	RMultiple       float64    `json:"r_multiple"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func Simulate(order Order, candles []models.Candle) (PaperTrade, error) {
	trade := PaperTrade{SchemaVersion: 1, Order: order, State: Waiting, UpdatedAt: time.Now().UTC()}
	long := order.Side == "BUY"
	risk := abs(order.Price - order.Stop)
	for _, candle := range candles {
		if candle.OpenTime.Unix() > order.ExpiresAt && trade.State == Waiting {
			trade.State = PaperNoFill
			trade.Outcome = "NO_FILL"
			return trade, nil
		}
		if trade.State == Waiting && candle.Low <= order.Price && candle.High >= order.Price {
			trade.State = PaperFilled
			trade.FillPrice = order.Price
			trade.FillAt = candle.OpenTime
		}
		if trade.State == PaperFilled || trade.State == PaperTP1 {
			if long {
				trade.MFE = max(trade.MFE, candle.High-order.Price)
				trade.MAE = max(trade.MAE, order.Price-candle.Low)
			} else {
				trade.MFE = max(trade.MFE, order.Price-candle.Low)
				trade.MAE = max(trade.MAE, candle.High-order.Price)
			}
			if long && candle.Low <= order.Stop || !long && candle.High >= order.Stop {
				trade.State = PaperClosed
				trade.ExitPrice = order.Stop
				trade.ExitAt = candle.OpenTime
				trade.Outcome = "STOPPED"
				if trade.TP1Hit {
					trade.Outcome = "TP1_THEN_STOP"
					trade.RMultiple = 0.5 * rewardR(order.Price, order.TP1, risk)
				} else {
					trade.RMultiple = -1
				}
				return trade, nil
			}
			if trade.State == PaperFilled && (long && candle.High >= order.TP1 || !long && candle.Low <= order.TP1) {
				trade.State = PaperTP1
				trade.TP1Hit = true
				trade.TP1At = candle.OpenTime
			}
			if long && candle.High >= order.TP2 || !long && candle.Low <= order.TP2 {
				trade.State = PaperClosed
				trade.ExitPrice = order.TP2
				trade.ExitAt = candle.OpenTime
				trade.Outcome = "TP2"
				trade.RMultiple = 0.5*rewardR(order.Price, order.TP1, risk) + 0.5*rewardR(order.Price, order.TP2, risk)
				return trade, nil
			}
		}
	}
	if trade.State == Waiting {
		trade.State = PaperNoFill
		trade.Outcome = "NO_FILL"
	}
	if trade.State == PaperTP1 {
		trade.Outcome = "TP1_OPEN"
	}
	if trade.State == PaperFilled {
		trade.Outcome = "OPEN"
	}
	if trade.State == "" {
		return trade, fmt.Errorf("invalid paper state")
	}
	return trade, nil
}

func rewardR(entry, target, risk float64) float64 { return abs(target-entry) / risk }
func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
