package forensics

import (
	"time"

	"github.com/ogtrading/overnight-strategy/internal/execution"
	"github.com/ogtrading/overnight-strategy/internal/journal"
)

const CheckpointSchemaVersion = 1

type Checkpoint struct {
	Name            string    `json:"name"`
	AsOfTime        time.Time `json:"as_of_time,omitempty"`
	Reconstructable bool      `json:"reconstructable"`
	Reason          string    `json:"reason,omitempty"`
}

type CheckpointManifest struct {
	SchemaVersion int          `json:"schema_version"`
	SessionID     string       `json:"session_id"`
	OpportunityID string       `json:"opportunity_id"`
	TradeID       string       `json:"trade_id"`
	Symbol        string       `json:"symbol"`
	Checkpoints   []Checkpoint `json:"checkpoints"`
}

func Checkpoints(record journal.TradeRecord, trade execution.PaperTrade) CheckpointManifest {
	m := CheckpointManifest{SchemaVersion: CheckpointSchemaVersion, SessionID: record.SessionID, OpportunityID: record.OpportunityID, TradeID: record.ID, Symbol: record.Symbol}
	add := func(name string, at time.Time, ok bool, reason string) {
		m.Checkpoints = append(m.Checkpoints, Checkpoint{Name: name, AsOfTime: at, Reconstructable: ok, Reason: reason})
	}
	add("PLAN", record.Market.Timestamp, true, "")
	filled := !trade.FillAt.IsZero()
	reason := "entry was not touched or filled"
	add("T_MINUS_5_MIN", trade.FillAt.Add(-5*time.Minute), filled, reason)
	add("T_MINUS_1_MIN", trade.FillAt.Add(-time.Minute), filled, reason)
	add("ENTRY_TOUCH", trade.FillAt, filled, reason)
	add("FILL", trade.FillAt, filled, reason)
	add("POST_FILL_30_SECONDS", trade.FillAt.Add(30*time.Second), filled, reason)
	add("POST_FILL_1_MINUTE", trade.FillAt.Add(time.Minute), filled, reason)
	add("POST_FILL_5_MINUTES", trade.FillAt.Add(5*time.Minute), filled, reason)
	add("TP1", trade.TP1At, !trade.TP1At.IsZero(), "TP1 was not reached")
	add("TP2", trade.ExitAt, trade.Outcome == "TP2", "TP2 was not reached")
	add("STOP", trade.ExitAt, trade.Outcome == "STOPPED" || trade.Outcome == "TP1_THEN_STOP", "stop was not reached")
	expiry := time.Unix(trade.Order.ExpiresAt, 0).UTC()
	add("EXPIRY", expiry, trade.State == execution.PaperNoFill, "entry did not expire unfilled")
	return m
}
