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
	SchemaVersion    int        `json:"schema_version"`
	LifecycleVersion string     `json:"lifecycle_version"`
	SessionDate      time.Time  `json:"session_date"`
	SessionID        string     `json:"session_id"`
	OpportunityID    string     `json:"opportunity_id"`
	StrategyOrderID  string     `json:"strategy_order_id"`
	TradeID          string     `json:"trade_id"`
	RunID            string     `json:"run_id"`
	Order            Order      `json:"order"`
	State            PaperState `json:"state"`
	FillPrice        float64    `json:"fill_price,omitempty"`
	FillAt           time.Time  `json:"fill_at,omitempty"`
	ExitPrice        float64    `json:"exit_price,omitempty"`
	ExitAt           time.Time  `json:"exit_at,omitempty"`
	Outcome          string     `json:"outcome,omitempty"`
	TP1Hit           bool       `json:"tp1_hit"`
	TP1At            time.Time  `json:"tp1_at,omitempty"`
	MFE              float64    `json:"mfe"`
	MAE              float64    `json:"mae"`
	RMultiple        float64    `json:"r_multiple"`
	UpdatedAt        time.Time  `json:"updated_at"`
	LastCandleAt     time.Time  `json:"last_candle_at,omitempty"`
}

// AdvancePaper applies one new candle to a durable paper trade. Stop-first
// ordering is deliberately conservative when a single bar crosses multiple
// levels. After TP1, the remaining half is protected at the actual entry.
func AdvancePaper(trade PaperTrade, candle models.Candle) (PaperTrade, bool, error) {
	if !trade.LastCandleAt.IsZero() && !candle.OpenTime.After(trade.LastCandleAt) {
		return trade, false, nil
	}
	if trade.State == PaperClosed || trade.State == PaperNoFill {
		return trade, false, nil
	}
	changed := true
	trade.LastCandleAt = candle.OpenTime
	trade.UpdatedAt = time.Now().UTC()
	plan := PlanFromOrder(trade.StrategyOrderID, trade.Order)
	decision, err := EvaluateOvernightLifecycle(plan, paperLifecycleState(trade), LifecycleInput{At: candle.OpenTime, Low: candle.Low, High: candle.High, Mark: candle.Close, HasPriceRange: true})
	if err != nil {
		return trade, false, err
	}
	trade = applyPaperDecision(trade, decision, candle.OpenTime)
	if trade.State != PaperFilled && trade.State != PaperTP1 && trade.State != PaperClosed {
		return trade, changed, nil
	}
	risk := abs(trade.Order.Price - trade.Order.Stop)
	if risk <= 0 {
		return trade, changed, fmt.Errorf("paper trade has zero risk")
	}
	if trade.Order.Side == "BUY" {
		trade.MFE = max(trade.MFE, candle.High-trade.Order.Price)
		trade.MAE = max(trade.MAE, trade.Order.Price-candle.Low)
	} else {
		trade.MFE = max(trade.MFE, trade.Order.Price-candle.Low)
		trade.MAE = max(trade.MAE, candle.High-trade.Order.Price)
	}
	switch trade.Outcome {
	case "STOPPED":
		trade.RMultiple = -1
	case "TP1_THEN_BE":
		trade.RMultiple = .5 * rewardR(trade.Order.Price, trade.Order.TP1, risk)
	case "TP2":
		trade.RMultiple = .5*rewardR(trade.Order.Price, trade.Order.TP1, risk) + .5*rewardR(trade.Order.Price, trade.Order.TP2, risk)
	}
	return trade, changed, nil
}

func ExpirePaper(trade PaperTrade, at time.Time, mark float64) (PaperTrade, bool) {
	if trade.State == PaperClosed || trade.State == PaperNoFill || at.Unix() < trade.Order.ExpiresAt {
		return trade, false
	}
	decision, err := EvaluateOvernightLifecycle(PlanFromOrder(trade.StrategyOrderID, trade.Order), paperLifecycleState(trade), LifecycleInput{At: at, Mark: mark, Expired: true})
	if err != nil {
		return trade, false
	}
	trade = applyPaperDecision(trade, decision, at)
	trade.UpdatedAt = at.UTC()
	if trade.State == PaperNoFill {
		return trade, true
	}
	risk := abs(trade.Order.Price - trade.Order.Stop)
	move := mark - trade.Order.Price
	if trade.Order.Side == "SELL" {
		move = -move
	}
	trade.RMultiple = move / risk
	if trade.TP1Hit {
		trade.RMultiple = .5*rewardR(trade.Order.Price, trade.Order.TP1, risk) + .5*trade.RMultiple
	}
	return trade, true
}

func Simulate(order Order, candles []models.Candle) (PaperTrade, error) {
	trade := PaperTrade{SchemaVersion: 1, LifecycleVersion: LifecycleVersion, Order: order, State: Waiting, UpdatedAt: time.Now().UTC()}
	for _, candle := range candles {
		var err error
		trade, _, err = AdvancePaper(trade, candle)
		if err != nil {
			return trade, err
		}
		if trade.State == PaperClosed || trade.State == PaperNoFill {
			return trade, nil
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

func paperLifecycleState(trade PaperTrade) LifecycleState {
	state := LifecycleState{FillPrice: trade.FillPrice, ActiveStop: trade.Order.Stop, TP1Hit: trade.TP1Hit}
	switch trade.State {
	case PaperFilled:
		state.Phase = LifecycleInitial
	case PaperTP1:
		state.Phase = LifecycleRunner
		state.ActiveStop = trade.FillPrice
	case PaperClosed:
		state.Phase = LifecycleClosed
	case PaperNoFill:
		state.Phase = LifecycleNoFill
	default:
		state.Phase = LifecycleWaiting
	}
	return state
}

func applyPaperDecision(trade PaperTrade, decision LifecycleDecision, at time.Time) PaperTrade {
	if hasLifecycleAction(decision.Actions, ActionEntryFilled) {
		trade.FillPrice, trade.FillAt = decision.State.FillPrice, at
	}
	if hasLifecycleAction(decision.Actions, ActionTakeTP1) {
		trade.TP1Hit, trade.TP1At = true, at
	}
	switch decision.State.Phase {
	case LifecycleWaiting:
		trade.State = Waiting
	case LifecycleInitial:
		trade.State = PaperFilled
	case LifecycleRunner:
		trade.State = PaperTP1
	case LifecycleClosed:
		trade.State, trade.ExitPrice, trade.ExitAt = PaperClosed, decision.ExitPrice, at
	case LifecycleNoFill:
		trade.State = PaperNoFill
	}
	if decision.Outcome != "" {
		trade.Outcome = decision.Outcome
	}
	return trade
}

func hasLifecycleAction(actions []LifecycleAction, wanted LifecycleAction) bool {
	for _, action := range actions {
		if action == wanted {
			return true
		}
	}
	return false
}

func rewardR(entry, target, risk float64) float64 { return abs(target-entry) / risk }
func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
