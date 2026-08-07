package research

import (
	"testing"

	"github.com/ogtrading/overnight-strategy/internal/backtest"
	"github.com/ogtrading/overnight-strategy/internal/liquidity"
	"github.com/ogtrading/overnight-strategy/internal/models"
)

func TestBuildLiquidityManagedWinnerPlanOnlyChangesRunner(t *testing.T) {
	plan := models.TradePlan{Direction: models.BiasLong, Entry: 100, Stop: 95, TP1: 110, TP2: 130, RiskDistance: 5}
	context := liquidity.Context{Levels: []liquidity.Level{{Side: liquidity.BuySide, Price: 120}}}
	got := BuildLiquidityManagedWinnerPlan(plan, context, models.Session{})
	if got.Entry != plan.Entry || got.Stop != plan.Stop || got.TP1 != plan.TP1 {
		t.Fatalf("baseline plan fields changed: %+v", got)
	}
	if got.TP2 != 120 || got.RR2 != 4 {
		t.Fatalf("runner target=%+v", got)
	}
}

func TestAnalyzeLiquidityV5KeepsLayersSeparate(t *testing.T) {
	baseline := featureObservation(true, true, -1, 0, 1).Result
	baseline.Outcome = models.OutcomeStopped
	managed := baseline
	reversal := featureObservation(true, true, 2, 2, .5).Result
	a := AnalyzeLiquidityV5([]LiquidityV5Observation{{
		Baseline: baseline,
		Managed:  managed,
		ReversalCandidate: StopReversalCandidate{
			LiquiditySweep: true,
			Reclaimed:      true,
			Plan:           backtest.ReversalPlan{Valid: true},
		},
		Reversal: reversal,
	}})
	if a.Baseline.TotalR != -1 || a.Reversals.TotalR != 2 || a.BaselinePlusReversal.TotalR != 1 {
		t.Fatalf("analysis=%+v", a)
	}
	if a.OriginalStops != 1 || a.StopsWithSweep != 1 || a.StopsWithReclaim != 1 || a.ValidReversalEntries != 1 {
		t.Fatalf("funnel=%+v", a)
	}
}
