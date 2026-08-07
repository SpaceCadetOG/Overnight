package research

import (
	"testing"

	"github.com/ogtrading/overnight-strategy/internal/liquidity"
)

func TestAnalyzeLiquidityV22BuildsFullConditionMatrix(t *testing.T) {
	observation := AuctionObservation{Liquidity: liquidity.Context{
		Sequence: liquidity.SequenceSSLToBSL, TargetAvailable: true, TargetAvailability: liquidity.TargetAvailableState, TP1ObstacleCount: 0, TP2ObstacleCount: 2,
		InternalTakenBeforeEntry: true, ExternalTarget: true, InternalToExternal: true, Event: liquidity.EventSweep,
		ValueLocation: liquidity.InsideValue, ValueTransition: liquidity.ValueRotation,
	}, Result: featureObservation(true, true, 2, 3, .5).Result}
	a := AnalyzeLiquidityV22([]AuctionObservation{observation})
	if a.TargetAvailability["AVAILABLE"].TotalR != 2 || a.TP1Paths["CLEAR"].Filled != 1 {
		t.Fatalf("analysis=%+v", a)
	}
	if a.InternalToExternal["INTERNAL_TO_EXTERNAL"].Wins != 1 || a.Events[liquidity.EventSweep].Wins != 1 {
		t.Fatalf("analysis=%+v", a)
	}
}
