package research

import (
	"testing"

	"github.com/ogtrading/overnight-strategy/internal/liquidity"
)

func TestAnalyzeLiquidityV21UsesFilledTradesOnly(t *testing.T) {
	observations := []AuctionObservation{
		{Liquidity: liquidity.Context{ValueTransition: liquidity.ValueAcceptance, LiquidityConsumedBeforeEntry: true}, Result: featureObservation(true, true, 2, 2, 0).Result},
		{Liquidity: liquidity.Context{ValueTransition: liquidity.ValueAcceptance}, Result: featureObservation(true, false, 0, 0, 0).Result},
		{Liquidity: liquidity.Context{ValueTransition: liquidity.ValueRejection}, Result: featureObservation(true, true, -1, 0, 1).Result},
	}
	analysis := AnalyzeLiquidityV21(observations)
	if analysis.Transitions[liquidity.ValueAcceptance].Filled != 1 {
		t.Fatalf("acceptance=%+v", analysis.Transitions[liquidity.ValueAcceptance])
	}
	if analysis.Consumed.TotalR != 2 || analysis.Available.TotalR != -1 {
		t.Fatalf("analysis=%+v", analysis)
	}
}
