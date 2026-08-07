package research

import (
	"fmt"

	"github.com/ogtrading/overnight-strategy/internal/liquidity"
)

type LiquidityV21Analysis struct {
	Transitions map[liquidity.ValueTransitionState]FeatureBucket
	Consumed    FeatureBucket
	Available   FeatureBucket
}

func AnalyzeLiquidityV21(observations []AuctionObservation) LiquidityV21Analysis {
	analysis := LiquidityV21Analysis{Transitions: make(map[liquidity.ValueTransitionState]FeatureBucket)}
	for _, observation := range observations {
		if !observation.Result.Filled {
			continue
		}
		state := observation.Liquidity.ValueTransition
		bucket := analysis.Transitions[state]
		accumulateFeatureBucket(&bucket, observation.Result)
		analysis.Transitions[state] = bucket
		if observation.Liquidity.LiquidityConsumedBeforeEntry {
			accumulateFeatureBucket(&analysis.Consumed, observation.Result)
		} else {
			accumulateFeatureBucket(&analysis.Available, observation.Result)
		}
	}
	for state, bucket := range analysis.Transitions {
		finalizeFeatureBucket(&bucket)
		analysis.Transitions[state] = bucket
	}
	finalizeFeatureBucket(&analysis.Consumed)
	finalizeFeatureBucket(&analysis.Available)
	return analysis
}

func PrintLiquidityV21Analysis(analysis LiquidityV21Analysis) {
	fmt.Println()
	fmt.Println("========================================================")
	fmt.Println(" LIQUIDITY V2.1 — VALUE TRANSITION ANALYSIS")
	fmt.Println("========================================================")
	fmt.Println()
	fmt.Println("VALUE TRANSITION")
	states := []liquidity.ValueTransitionState{liquidity.ValueAcceptance, liquidity.ValueRejection, liquidity.ValueRotation, liquidity.ValueContinuation}
	for _, state := range states {
		printLiquidityV21Bucket(string(state), analysis.Transitions[state])
	}
	fmt.Println()
	fmt.Println("LIQUIDITY_CONSUMED_BEFORE_ENTRY")
	printLiquidityV21Bucket("YES", analysis.Consumed)
	printLiquidityV21Bucket("NO", analysis.Available)
}

func printLiquidityV21Bucket(label string, bucket FeatureBucket) {
	fmt.Printf("%-20s | Trades %4d | Win %5.1f%% | Avg %7.3fR | PF %6s | Total %8.2fR\n",
		label, bucket.Filled, bucket.FilledWinRate, bucket.AverageRPerFilled,
		formatProfitFactor(bucket.ProfitFactor), bucket.TotalR)
}
