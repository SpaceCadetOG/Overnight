package live

import (
	"fmt"
	"math"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
	"github.com/ogtrading/overnight-strategy/internal/universe"
)

type IntentState string

const (
	Created   IntentState = "CREATED"
	Validated IntentState = "VALIDATED"
	DryRun    IntentState = "DRY_RUN"
	Submitted IntentState = "SUBMITTED"
	Open      IntentState = "OPEN"
	Partial   IntentState = "PARTIAL"
	Filled    IntentState = "FILLED"
	TP1       IntentState = "TP1"
	Runner    IntentState = "RUNNER"
	Closed    IntentState = "CLOSED"
	Canceled  IntentState = "CANCELED"
	Rejected  IntentState = "REJECTED"
)

type Intent struct {
	ID              string           `json:"id"`
	SchemaVersion   int              `json:"schema_version"`
	SessionID       string           `json:"session_id,omitempty"`
	OpportunityID   string           `json:"opportunity_id,omitempty"`
	StrategyOrderID string           `json:"strategy_order_id,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	Symbol          string           `json:"symbol"`
	Plan            models.TradePlan `json:"plan"`
	RiskUSD         float64          `json:"risk_usd"`
	Quantity        float64          `json:"quantity"`
	State           IntentState      `json:"state"`
	AutomatedOrder  bool             `json:"automated_order"`
	Reason          string           `json:"reason,omitempty"`
}

type RiskPolicy struct {
	RiskPerAssetUSD  float64
	MaxBasketRiskUSD float64
	MaxPositions     int
}

func DefaultRiskPolicy() RiskPolicy {
	return RiskPolicy{RiskPerAssetUSD: 0.50, MaxBasketRiskUSD: 2.00, MaxPositions: 2}
}

func BuildIntent(symbol string, plan models.TradePlan, riskUSD float64) (Intent, error) {
	if _, err := universe.RequireTradable(symbol); err != nil {
		return Intent{}, err
	}
	return buildIntent(symbol, plan, riskUSD)
}

// BuildPaperIntent applies identical sizing to every registered control market
// without granting it funded execution authority.
func BuildPaperIntent(symbol string, plan models.TradePlan, riskUSD float64) (Intent, error) {
	if _, ok := universe.Find(symbol); !ok {
		return Intent{}, fmt.Errorf("asset %s is not registered", symbol)
	}
	return buildIntent(symbol, plan, riskUSD)
}

func buildIntent(symbol string, plan models.TradePlan, riskUSD float64) (Intent, error) {
	if !plan.Valid {
		return Intent{}, fmt.Errorf("invalid plan: %s", plan.InvalidReason)
	}
	if riskUSD <= 0 {
		return Intent{}, fmt.Errorf("risk must be positive")
	}
	distance := math.Abs(plan.Entry - plan.Stop)
	if distance <= 0 {
		return Intent{}, fmt.Errorf("invalid entry-stop distance")
	}
	created := time.Now().UTC()
	return Intent{ID: fmt.Sprintf("%s-%s", plan.Date.Format("20060102"), symbol), SchemaVersion: 1, CreatedAt: created, Symbol: symbol, Plan: plan, RiskUSD: riskUSD, Quantity: riskUSD / distance, State: Created}, nil
}

func ValidateBasket(intents []Intent, policy RiskPolicy) error {
	if len(intents) > policy.MaxPositions {
		return fmt.Errorf("intent count %d exceeds max positions %d", len(intents), policy.MaxPositions)
	}
	seen, total := map[string]bool{}, 0.0
	for _, intent := range intents {
		if seen[intent.Symbol] {
			return fmt.Errorf("duplicate intent for %s", intent.Symbol)
		}
		seen[intent.Symbol] = true
		if intent.RiskUSD > policy.RiskPerAssetUSD {
			return fmt.Errorf("%s risk %.2f exceeds per-asset limit %.2f", intent.Symbol, intent.RiskUSD, policy.RiskPerAssetUSD)
		}
		total += intent.RiskUSD
	}
	if total > policy.MaxBasketRiskUSD+1e-9 {
		return fmt.Errorf("basket risk %.2f exceeds limit %.2f", total, policy.MaxBasketRiskUSD)
	}
	return nil
}

func MarkDryRun(intent Intent) Intent {
	intent.State = DryRun
	intent.AutomatedOrder = false
	return intent
}

func LatestIntents(events []Intent) map[string]Intent {
	latest := make(map[string]Intent)
	for _, event := range events {
		current, ok := latest[event.ID]
		if !ok || event.CreatedAt.After(current.CreatedAt) {
			latest[event.ID] = event
		}
	}
	return latest
}

func Transition(intent Intent, next IntentState) (Intent, error) {
	allowed := map[IntentState]map[IntentState]bool{
		Created: {Validated: true, DryRun: true, Rejected: true}, Validated: {DryRun: true, Submitted: true, Rejected: true},
		Submitted: {Open: true, Partial: true, Filled: true, Rejected: true}, Open: {Partial: true, Filled: true, Canceled: true},
		Partial: {Filled: true, Canceled: true}, Filled: {TP1: true, Closed: true}, TP1: {Runner: true, Closed: true}, Runner: {Closed: true},
	}
	if !allowed[intent.State][next] {
		return intent, fmt.Errorf("invalid intent transition %s -> %s", intent.State, next)
	}
	intent.State = next
	if next == DryRun {
		intent.AutomatedOrder = false
	}
	return intent, nil
}
