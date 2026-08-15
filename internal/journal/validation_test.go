package journal

import (
	"testing"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/execution"
)

func validTerminalRecord() TradeRecord {
	now := time.Date(2026, 8, 10, 21, 0, 0, 0, time.UTC)
	return TradeRecord{
		SchemaVersion: 2, ID: "trade-1", SessionID: "session-1", OpportunityID: "opp-1",
		StrategyOrderID: "strategy-order-1", RunID: "run-1", RecordedAt: now,
		SessionDate: now, StrategyVersion: "baseline-v1-20260810", RuntimeVersion: execution.LifecycleVersion,
		Order: execution.Order{Price: 100, Stop: 99, TP1: 101, TP2: 103}, PlannedEntry: 100,
		ActualFill: 100, State: execution.PaperClosed, Outcome: "EXPIRY", ExitPrice: 100.5,
	}
}

func TestValidateForResearchRejectsZeroExitPrice(t *testing.T) {
	record := validTerminalRecord()
	record.ExitPrice = 0
	if err := record.ValidateForResearch(); err == nil {
		t.Fatal("zero-price terminal row was accepted")
	}
}

func TestValidateForResearchAcceptsValidTerminalAndNoFill(t *testing.T) {
	record := validTerminalRecord()
	if err := record.ValidateForResearch(); err != nil {
		t.Fatalf("valid terminal rejected: %v", err)
	}
	record.State, record.Outcome, record.ActualFill, record.ExitPrice = execution.PaperNoFill, "NO_FILL", 0, 0
	if err := record.ValidateForResearch(); err != nil {
		t.Fatalf("valid no-fill rejected: %v", err)
	}
}
