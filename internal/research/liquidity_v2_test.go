package research

import (
	"testing"

	"github.com/ogtrading/overnight-strategy/internal/liquidity"
)

func TestAnalyzeLiquidityV2SeparatesValueAndPath(t *testing.T) {
	observations := []AuctionObservation{
		{Liquidity: liquidity.Context{ValueLocation: liquidity.InsideValue, Path: liquidity.LiquidityPath{CleanPath: true}}, Result: featureObservation(true, true, 2, 3, .5).Result},
		{Liquidity: liquidity.Context{ValueLocation: liquidity.BelowValueAcceptance}, Result: featureObservation(true, true, -1, .2, 1).Result},
	}
	analysis := AnalyzeLiquidityV2(observations)
	if analysis.ValueLocations[liquidity.InsideValue].Wins != 1 {
		t.Fatalf("inside=%+v", analysis.ValueLocations[liquidity.InsideValue])
	}
	if analysis.OpposingPath.Losses != 1 || analysis.ClearPath.TotalR != 2 {
		t.Fatalf("analysis=%+v", analysis)
	}
}
