package execution

import (
	"fmt"
	"time"
)

// LifecycleVersion changes only when runtime transition behavior changes. The
// frozen strategy version remains independent from runtime correctness fixes.
const LifecycleVersion = "overnight-lifecycle-v2.0.1"

type LifecyclePhase string

const (
	LifecycleWaiting LifecyclePhase = "WAITING"
	LifecycleInitial LifecyclePhase = "INITIAL"
	LifecycleRunner  LifecyclePhase = "RUNNER"
	LifecycleClosed  LifecyclePhase = "CLOSED"
	LifecycleNoFill  LifecyclePhase = "NO_FILL"
)

type LifecycleAction string

const (
	ActionEntryFilled       LifecycleAction = "ENTRY_FILLED"
	ActionActivateInitial   LifecycleAction = "ACTIVATE_INITIAL_PROTECTION"
	ActionTakeTP1           LifecycleAction = "TAKE_TP1"
	ActionMoveStopBreakeven LifecycleAction = "MOVE_STOP_BREAKEVEN"
	ActionActivateTP2       LifecycleAction = "ACTIVATE_TP2"
	ActionCloseStop         LifecycleAction = "CLOSE_STOP"
	ActionCloseTP2          LifecycleAction = "CLOSE_TP2"
	ActionCancelExpired     LifecycleAction = "CANCEL_EXPIRED_ENTRY"
	ActionCloseExpired      LifecycleAction = "CLOSE_EXPIRED_POSITION"
	ActionReconcileClosed   LifecycleAction = "RECONCILE_CLOSED"
)

// LifecyclePlan is the immutable decision artifact consumed by both paper and
// live. Adapters may normalize executable quantity, but may not reinterpret
// direction, bracket geometry, expiry, or partial-close policy.
type LifecyclePlan struct {
	StrategyOrderID  string
	Symbol           string
	Side             string
	Entry            float64
	Stop             float64
	TP1              float64
	TP2              float64
	ExpiresAt        time.Time
	TP1Fraction      float64
	MoveToBEAfterTP1 bool
}

type LifecycleState struct {
	Phase      LifecyclePhase
	FillPrice  float64
	ActiveStop float64
	TP1Hit     bool
}

// LifecycleInput contains observations, never policy. Paper supplies a candle
// range; live supplies confirmed exchange/account events.
type LifecycleInput struct {
	At             time.Time
	Low            float64
	High           float64
	Mark           float64
	HasPriceRange  bool
	EntryFilled    bool
	TP1Filled      bool
	PositionClosed bool
	Expired        bool
}

type LifecycleDecision struct {
	State     LifecycleState
	Actions   []LifecycleAction
	Outcome   string
	ExitPrice float64
}

func PlanFromOrder(strategyOrderID string, order Order) LifecyclePlan {
	return LifecyclePlan{
		StrategyOrderID: strategyOrderID,
		Symbol:          order.Symbol, Side: order.Side, Entry: order.Price, Stop: order.Stop,
		TP1: order.TP1, TP2: order.TP2, ExpiresAt: time.Unix(order.ExpiresAt, 0).UTC(),
		TP1Fraction: .5, MoveToBEAfterTP1: true,
	}
}

func ValidateLifecyclePlan(plan LifecyclePlan) error {
	if plan.Symbol == "" || (plan.Side != "BUY" && plan.Side != "SELL") {
		return fmt.Errorf("lifecycle plan requires symbol and BUY/SELL side")
	}
	if plan.Entry <= 0 || plan.ExpiresAt.IsZero() {
		return fmt.Errorf("lifecycle plan requires entry and expiry")
	}
	if plan.TP1Fraction <= 0 || plan.TP1Fraction >= 1 {
		return fmt.Errorf("TP1 fraction must be between zero and one")
	}
	if plan.Side == "BUY" && !(plan.Stop < plan.Entry && plan.Entry < plan.TP1 && plan.TP1 < plan.TP2) {
		return fmt.Errorf("invalid BUY lifecycle geometry")
	}
	if plan.Side == "SELL" && !(plan.Stop > plan.Entry && plan.Entry > plan.TP1 && plan.TP1 > plan.TP2) {
		return fmt.Errorf("invalid SELL lifecycle geometry")
	}
	return nil
}

// EvaluateOvernightLifecycle is deterministic and side-effect free. Stop-first
// ordering is conservative when one paper candle crosses multiple levels.
func EvaluateOvernightLifecycle(plan LifecyclePlan, state LifecycleState, in LifecycleInput) (LifecycleDecision, error) {
	if err := ValidateLifecyclePlan(plan); err != nil {
		return LifecycleDecision{}, err
	}
	decision := LifecycleDecision{State: state}
	if state.Phase == LifecycleClosed || state.Phase == LifecycleNoFill {
		return decision, nil
	}
	if in.At.IsZero() {
		return LifecycleDecision{}, fmt.Errorf("lifecycle input timestamp is required")
	}
	expired := in.Expired || in.At.After(plan.ExpiresAt)
	if state.Phase == LifecycleWaiting && expired {
		decision.State.Phase = LifecycleNoFill
		decision.Outcome = "NO_FILL"
		decision.Actions = append(decision.Actions, ActionCancelExpired)
		return decision, nil
	}
	entryTouched := in.EntryFilled || rangeTouches(in, plan.Entry)
	if state.Phase == LifecycleWaiting && entryTouched {
		decision.State = LifecycleState{Phase: LifecycleInitial, FillPrice: plan.Entry, ActiveStop: plan.Stop}
		decision.Actions = append(decision.Actions, ActionEntryFilled, ActionActivateInitial)
	}
	if decision.State.Phase == LifecycleWaiting {
		return decision, nil
	}
	if in.PositionClosed {
		decision.State.Phase = LifecycleClosed
		decision.Outcome = "RECONCILED_FLAT"
		decision.Actions = append(decision.Actions, ActionReconcileClosed)
		return decision, nil
	}
	if expired {
		if in.Mark <= 0 {
			return LifecycleDecision{}, fmt.Errorf("positive mark is required to expire an open position")
		}
		decision.State.Phase = LifecycleClosed
		decision.Outcome, decision.ExitPrice = "EXPIRY", in.Mark
		decision.Actions = append(decision.Actions, ActionCloseExpired)
		return decision, nil
	}
	if decision.State.ActiveStop == 0 {
		decision.State.ActiveStop = plan.Stop
		if decision.State.TP1Hit && plan.MoveToBEAfterTP1 {
			decision.State.ActiveStop = decision.State.FillPrice
		}
	}
	if stopTouched(plan.Side, in, decision.State.ActiveStop) {
		decision.State.Phase = LifecycleClosed
		decision.ExitPrice = decision.State.ActiveStop
		decision.Outcome = "STOPPED"
		if decision.State.TP1Hit {
			decision.Outcome = "TP1_THEN_BE"
		}
		decision.Actions = append(decision.Actions, ActionCloseStop)
		return decision, nil
	}
	tp1Touched := in.TP1Filled || targetTouched(plan.Side, in, plan.TP1)
	if decision.State.Phase == LifecycleInitial && tp1Touched {
		decision.State.Phase, decision.State.TP1Hit = LifecycleRunner, true
		decision.Actions = append(decision.Actions, ActionTakeTP1)
		if plan.MoveToBEAfterTP1 {
			decision.State.ActiveStop = decision.State.FillPrice
			decision.Actions = append(decision.Actions, ActionMoveStopBreakeven)
		}
		decision.Actions = append(decision.Actions, ActionActivateTP2)
	}
	if targetTouched(plan.Side, in, plan.TP2) {
		decision.State.Phase = LifecycleClosed
		decision.Outcome, decision.ExitPrice = "TP2", plan.TP2
		decision.Actions = append(decision.Actions, ActionCloseTP2)
	}
	return decision, nil
}

func rangeTouches(in LifecycleInput, price float64) bool {
	return in.HasPriceRange && in.Low <= price && in.High >= price
}

func stopTouched(side string, in LifecycleInput, stop float64) bool {
	if !in.HasPriceRange || stop <= 0 {
		return false
	}
	if side == "BUY" {
		return in.Low <= stop
	}
	return in.High >= stop
}

func targetTouched(side string, in LifecycleInput, target float64) bool {
	if !in.HasPriceRange || target <= 0 {
		return false
	}
	if side == "BUY" {
		return in.High >= target
	}
	return in.Low <= target
}
