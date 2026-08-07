package research

import (
	"testing"

	"github.com/ogtrading/overnight-strategy/internal/liquidity"
)

func TestAnalyzeStructuralLiquidityUsesDualPopulationBuckets(t *testing.T) {
	observations := []AuctionObservation{
		{Liquidity: liquidity.Context{Sequence: liquidity.SequenceSSLToBSL, PathScore: 8}, Result: featureObservation(true, true, 2, 3, .5).Result},
		{Liquidity: liquidity.Context{Sequence: liquidity.SequenceSSLToBSL, PathScore: 8}, Result: featureObservation(true, false, 0, 0, 0).Result},
		{Liquidity: liquidity.Context{Sequence: liquidity.SequenceNone, OpposingPresent: true, PathScore: 2}, Result: featureObservation(true, true, -1, .2, 1).Result},
	}
	analysis := AnalyzeStructuralLiquidity(observations)
	sequence := analysis.Sequences[liquidity.SequenceSSLToBSL]
	if sequence.ValidPlans != 2 || sequence.Filled != 1 || sequence.NoFill != 1 {
		t.Fatalf("sequence counts: %+v", sequence)
	}
	if analysis.ScoreHigh.TotalR != 2 || analysis.OpposingPresent.Losses != 1 {
		t.Fatalf("unexpected buckets: %+v", analysis)
	}
}
