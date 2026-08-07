package research

import (
	"testing"

	"github.com/ogtrading/overnight-strategy/internal/liquidity"
	"github.com/ogtrading/overnight-strategy/internal/models"
)

func TestBuildLiquidityExitPlanChangesOnlyValidTargets(t *testing.T) {
	plan := models.TradePlan{Direction: models.BiasLong, Entry: 100, Stop: 95, TP1: 110, TP2: 120, RiskDistance: 5, Valid: true}
	got := BuildLiquidityExitPlan(plan, liquidity.Context{InternalTargetPrice: 108, ExternalTargetPrice: 125})
	if got.Entry != plan.Entry || got.Stop != plan.Stop || got.Direction != plan.Direction {
		t.Fatalf("core plan changed: %+v", got)
	}
	if got.TP1 != 108 || got.TP2 != 125 || got.RR1 != 1.6 || got.RR2 != 5 {
		t.Fatalf("targets=%+v", got)
	}
}

func TestAnalyzeLiquidityV3KeepsBaselineAndShadowSeparate(t *testing.T) {
	baseline := featureObservation(true, true, 2, 3, .5).Result
	shadow := featureObservation(true, true, 1, 2, .4).Result
	a := AnalyzeLiquidityV3([]LiquidityV3Observation{{Context: liquidity.Context{EntryLiquidity: liquidity.EntryInternalSSL}, Baseline: baseline, Shadow: shadow}})
	if a.Baseline.TotalR != 2 || a.Shadow.TotalR != 1 || a.EntryLocations[liquidity.EntryInternalSSL].TotalR != 2 {
		t.Fatalf("analysis=%+v", a)
	}
}
