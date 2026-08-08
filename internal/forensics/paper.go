package forensics

import (
	"fmt"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/execution"
	"github.com/ogtrading/overnight-strategy/internal/live"
	"github.com/ogtrading/overnight-strategy/internal/models"
)

func PaperLifecycle(snapshot live.MarketSnapshot, trade execution.PaperTrade, strategyVersion, runID string) ([]Envelope, error) {
	opportunityKey := PlanOpportunityKey(*snapshot.Plan)
	ids := IDs(snapshot.SessionDate, snapshot.Symbol, strategyVersion, opportunityKey, "PAPER", runID)
	planAt := snapshot.Timestamp
	if planAt.IsZero() {
		planAt = snapshot.SessionDate
	}
	sequence := uint64(0)
	events := []Envelope{}
	add := func(kind EventType, at time.Time, payload any) error {
		sequence++
		e, err := New(kind, at, sequence, ids, snapshot.Symbol, "PAPER", strategyVersion, payload)
		if err == nil {
			events = append(events, e)
		}
		return err
	}
	if err := add(SessionStarted, planAt, map[string]any{"session_date": snapshot.SessionDate}); err != nil {
		return nil, err
	}
	if err := add(PlanCreated, planAt, map[string]any{"market": snapshot, "plan": snapshot.Plan}); err != nil {
		return nil, err
	}
	if snapshot.Plan == nil || !snapshot.Plan.Valid {
		if err := add(PlanInvalidated, planAt, map[string]any{"reason": "invalid_or_missing_plan"}); err != nil {
			return nil, err
		}
		return events, nil
	}
	if err := add(PlanValidated, planAt, map[string]any{"entry": trade.Order.Price, "stop": trade.Order.Stop, "tp1": trade.Order.TP1, "tp2": trade.Order.TP2}); err != nil {
		return nil, err
	}
	if err := add(OrderSubmitted, planAt, map[string]any{"order": trade.Order, "simulated": true}); err != nil {
		return nil, err
	}
	if trade.State == execution.PaperNoFill {
		at := time.Unix(trade.Order.ExpiresAt, 0)
		if err := add(EntryExpired, at, map[string]any{"outcome": "NO_FILL"}); err != nil {
			return nil, err
		}
		return events, nil
	}
	if !trade.FillAt.IsZero() {
		if err := add(OrderFilled, trade.FillAt, map[string]any{"price": trade.FillPrice, "quantity": trade.Order.Quantity}); err != nil {
			return nil, err
		}
		if err := add(PositionOpened, trade.FillAt, map[string]any{"price": trade.FillPrice, "quantity": trade.Order.Quantity}); err != nil {
			return nil, err
		}
	}
	if trade.TP1Hit {
		if err := add(TP1Filled, trade.TP1At, map[string]any{"price": trade.Order.TP1, "position_fraction": .5}); err != nil {
			return nil, err
		}
		if err := add(StopMoved, trade.TP1At, map[string]any{"price": trade.FillPrice, "reason": "BREAKEVEN_AFTER_TP1"}); err != nil {
			return nil, err
		}
	}
	if trade.State == execution.PaperClosed {
		if err := add(PositionClosed, trade.ExitAt, map[string]any{"outcome": trade.Outcome, "exit_price": trade.ExitPrice, "r_multiple": trade.RMultiple, "mfe": trade.MFE, "mae": trade.MAE}); err != nil {
			return nil, err
		}
	}
	return events, nil
}

func OpportunityKey(order execution.Order) string {
	return fmt.Sprintf("%s|%.12g|%.12g|%.12g|%.12g", order.Side, order.Price, order.Stop, order.TP1, order.TP2)
}

func PlanOpportunityKey(plan models.TradePlan) string {
	return fmt.Sprintf("%s|%.12g|%.12g|%.12g|%.12g", plan.Direction, plan.Entry, plan.Stop, plan.TP1, plan.TP2)
}
