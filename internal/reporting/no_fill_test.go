package reporting

import (
	"testing"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

func TestAnalyzeNoFillProximity(t *testing.T) {
	results := []models.TradeResult{
		{Outcome: models.OutcomeNoFill, InitialRisk: 10, MissedEntryDistance: 1},
		{Outcome: models.OutcomeNoFill, InitialRisk: 10, MissedEntryDistance: 3},
		{Outcome: models.OutcomeNoFill, InitialRisk: 10, MissedEntryDistance: 15},
		{Outcome: models.OutcomeTP2, InitialRisk: 10},
	}
	got := AnalyzeNoFillProximity(results)
	if got.NoFills != 3 || got.Normalized != 3 || got.Within010R != 1 || got.Within050R != 1 || got.Within200R != 1 {
		t.Fatalf("analysis=%+v", got)
	}
	if got.AverageR != (0.1+0.3+1.5)/3 {
		t.Fatalf("average=%v", got.AverageR)
	}
}
