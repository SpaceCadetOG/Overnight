package research

import (
	"fmt"

	"github.com/ogtrading/overnight-strategy/internal/liquidity"
)

type LiquidityV2Analysis struct {
	ValueLocations map[liquidity.ValueLocationState]FeatureBucket
	ClearPath      FeatureBucket
	OpposingPath   FeatureBucket
}

func AnalyzeLiquidityV2(observations []AuctionObservation) LiquidityV2Analysis {
	analysis := LiquidityV2Analysis{ValueLocations: make(map[liquidity.ValueLocationState]FeatureBucket)}
	for _, observation := range observations {
		state := observation.Liquidity.ValueLocation
		bucket := analysis.ValueLocations[state]
		accumulateFeatureBucket(&bucket, observation.Result)
		analysis.ValueLocations[state] = bucket
		if observation.Liquidity.Path.CleanPath {
			accumulateFeatureBucket(&analysis.ClearPath, observation.Result)
		} else {
			accumulateFeatureBucket(&analysis.OpposingPath, observation.Result)
		}
	}
	for state, bucket := range analysis.ValueLocations {
		finalizeFeatureBucket(&bucket)
		analysis.ValueLocations[state] = bucket
	}
	finalizeFeatureBucket(&analysis.ClearPath)
	finalizeFeatureBucket(&analysis.OpposingPath)
	return analysis
}

func PrintLiquidityV2Analysis(analysis LiquidityV2Analysis) {
	fmt.Println()
	fmt.Println("========================================================")
	fmt.Println(" LIQUIDITY V2 ANALYSIS")
	fmt.Println("========================================================")
	fmt.Println()
	fmt.Println("VALUE LOCATION")
	states := []liquidity.ValueLocationState{liquidity.AboveValue, liquidity.InsideValue, liquidity.BelowValueAcceptance, liquidity.BelowValueSweepReclaim}
	for _, state := range states {
		printLiquidityV2Bucket(string(state), analysis.ValueLocations[state])
	}
	fmt.Println()
	fmt.Println("LIQUIDITY PATH")
	printLiquidityV2Bucket("CLEAR", analysis.ClearPath)
	printLiquidityV2Bucket("OPPOSING", analysis.OpposingPath)
}

func printLiquidityV2Bucket(label string, bucket FeatureBucket) {
	fmt.Printf("%-27s | Plans %4d | Filled %4d | W %4d | L %4d | Win %5.1f%% | Avg/fill %7.3fR | PF %6s | Total %8.2fR\n",
		label, bucket.ValidPlans, bucket.Filled, bucket.Wins, bucket.Losses,
		bucket.FilledWinRate, bucket.AverageRPerFilled,
		formatProfitFactor(bucket.ProfitFactor), bucket.TotalR)
}
