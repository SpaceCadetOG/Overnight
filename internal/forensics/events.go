package forensics

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/execution"
)

const SchemaVersion = 2

type EventType string

const (
	SessionStarted  EventType = "SESSION_STARTED"
	PlanCreated     EventType = "PLAN_CREATED"
	PlanValidated   EventType = "PLAN_VALIDATED"
	PlanInvalidated EventType = "PLAN_INVALIDATED"
	OrderSubmitted  EventType = "ORDER_SUBMITTED"
	OrderFilled     EventType = "ORDER_FILLED"
	EntryExpired    EventType = "ENTRY_EXPIRED"
	PositionOpened  EventType = "POSITION_OPENED"
	TP1Filled       EventType = "TP1_FILLED"
	StopMoved       EventType = "STOP_MOVED"
	PositionClosed  EventType = "POSITION_CLOSED"
)

type Identity struct {
	SessionID       string `json:"session_id"`
	OpportunityID   string `json:"opportunity_id,omitempty"`
	StrategyOrderID string `json:"strategy_order_id,omitempty"`
	TradeID         string `json:"trade_id,omitempty"`
	OrderID         string `json:"order_id,omitempty"`
	PositionID      string `json:"position_id,omitempty"`
	CaseID          string `json:"case_id,omitempty"`
	RunID           string `json:"run_id"`
}

type Envelope struct {
	EventID         string          `json:"event_id"`
	EventType       EventType       `json:"event_type"`
	SchemaVersion   int             `json:"schema_version"`
	OccurredAt      time.Time       `json:"occurred_at"`
	RecordedAt      time.Time       `json:"recorded_at"`
	SessionID       string          `json:"session_id"`
	OpportunityID   string          `json:"opportunity_id,omitempty"`
	StrategyOrderID string          `json:"strategy_order_id,omitempty"`
	TradeID         string          `json:"trade_id,omitempty"`
	OrderID         string          `json:"order_id,omitempty"`
	PositionID      string          `json:"position_id,omitempty"`
	CaseID          string          `json:"case_id,omitempty"`
	Symbol          string          `json:"symbol"`
	Venue           string          `json:"venue"`
	Mode            string          `json:"mode"`
	StrategyVersion string          `json:"strategy_version"`
	RuntimeVersion  string          `json:"runtime_version"`
	RunID           string          `json:"run_id"`
	Source          string          `json:"source"`
	Sequence        uint64          `json:"sequence"`
	Payload         json.RawMessage `json:"payload"`
}

// IDs creates stable cross-mode identity. opportunityKey identifies one
// specific plan, so paper and live executions pair without colliding with
// retries or another opportunity for the same symbol and session.
func IDs(sessionDate time.Time, symbol, strategyVersion, opportunityKey, mode, runID string) Identity {
	session := stable("ses", sessionDate.Format("2006-01-02"), symbol, strategyVersion)
	opportunity := stable("opp", session, opportunityKey)
	mode = strings.ToUpper(strings.TrimSpace(mode))
	return Identity{SessionID: session, OpportunityID: opportunity, StrategyOrderID: stable("sord", opportunity, strategyVersion), TradeID: stable("trd", opportunity, mode, runID), OrderID: stable("ord", opportunity, mode, runID, "entry"), PositionID: stable("pos", opportunity, mode, runID), CaseID: stable("case", opportunity), RunID: runID}
}

func New(eventType EventType, occurredAt time.Time, sequence uint64, ids Identity, symbol, mode, strategyVersion string, payload any) (Envelope, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	eventID := stable("evt", ids.RunID, ids.OpportunityID, string(eventType), fmt.Sprint(sequence))
	return Envelope{EventID: eventID, EventType: eventType, SchemaVersion: SchemaVersion, OccurredAt: occurredAt.UTC(), RecordedAt: time.Now().UTC(), SessionID: ids.SessionID, OpportunityID: ids.OpportunityID, StrategyOrderID: ids.StrategyOrderID, TradeID: ids.TradeID, OrderID: ids.OrderID, PositionID: ids.PositionID, CaseID: ids.CaseID, Symbol: symbol, Venue: "LIGHTER", Mode: strings.ToUpper(mode), StrategyVersion: strategyVersion, RuntimeVersion: execution.LifecycleVersion, RunID: ids.RunID, Source: "tradepi", Sequence: sequence, Payload: body}, nil
}

func stable(prefix string, parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return prefix + "_" + hex.EncodeToString(h[:12])
}
