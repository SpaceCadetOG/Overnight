package research

import (
	"fmt"
	"sort"

	"github.com/ogtrading/overnight-strategy/internal/liquidity"
)

type StructuralLiquidityAnalysis struct {
	Sequences       map[liquidity.Sequence]FeatureBucket
	OpposingPresent FeatureBucket
	OpposingAbsent  FeatureBucket
	ScoreLow        FeatureBucket
	ScoreMedium     FeatureBucket
	ScoreHigh       FeatureBucket
}

func AnalyzeStructuralLiquidity(observations []AuctionObservation) StructuralLiquidityAnalysis {
	analysis := StructuralLiquidityAnalysis{Sequences: make(map[liquidity.Sequence]FeatureBucket)}
	for _, observation := range observations {
		sequence := analysis.Sequences[observation.Liquidity.Sequence]
		accumulateFeatureBucket(&sequence, observation.Result)
		analysis.Sequences[observation.Liquidity.Sequence] = sequence

		if observation.Liquidity.OpposingPresent {
			accumulateFeatureBucket(&analysis.OpposingPresent, observation.Result)
		} else {
			accumulateFeatureBucket(&analysis.OpposingAbsent, observation.Result)
		}
		switch {
		case observation.Liquidity.PathScore <= 3:
			accumulateFeatureBucket(&analysis.ScoreLow, observation.Result)
		case observation.Liquidity.PathScore <= 6:
			accumulateFeatureBucket(&analysis.ScoreMedium, observation.Result)
		default:
			accumulateFeatureBucket(&analysis.ScoreHigh, observation.Result)
		}
	}
	for key, bucket := range analysis.Sequences {
		finalizeFeatureBucket(&bucket)
		analysis.Sequences[key] = bucket
	}
	finalizeFeatureBucket(&analysis.OpposingPresent)
	finalizeFeatureBucket(&analysis.OpposingAbsent)
	finalizeFeatureBucket(&analysis.ScoreLow)
	finalizeFeatureBucket(&analysis.ScoreMedium)
	finalizeFeatureBucket(&analysis.ScoreHigh)
	return analysis
}

func PrintStructuralLiquidityAnalysis(analysis StructuralLiquidityAnalysis) {
	fmt.Println()
	fmt.Println("========================================================")
	fmt.Println(" STRUCTURAL LIQUIDITY ANALYSIS")
	fmt.Println("========================================================")
	fmt.Println()
	fmt.Println("SEQUENCE PERFORMANCE")
	keys := make([]string, 0, len(analysis.Sequences))
	for key := range analysis.Sequences {
		keys = append(keys, string(key))
	}
	sort.Strings(keys)
	for _, key := range keys {
		printLiquidityBucket(key, analysis.Sequences[liquidity.Sequence(key)])
	}
	fmt.Println()
	fmt.Println("OPPOSING LIQUIDITY")
	printLiquidityBucket("PRESENT", analysis.OpposingPresent)
	printLiquidityBucket("ABSENT", analysis.OpposingAbsent)
	fmt.Println()
	fmt.Println("PATH SCORE")
	printLiquidityBucket("0-3", analysis.ScoreLow)
	printLiquidityBucket("4-6", analysis.ScoreMedium)
	printLiquidityBucket("7-10", analysis.ScoreHigh)
}

func printLiquidityBucket(label string, bucket FeatureBucket) {
	fmt.Printf("%-12s | Plans %4d | Filled %4d | Fill %5.1f%% | Avg/plan %7.3fR | Avg/fill %7.3fR | PF %6s | Total %8.2fR\n",
		label, bucket.ValidPlans, bucket.Filled, bucket.FillRate, bucket.AverageRPerPlan, bucket.AverageRPerFilled, formatProfitFactor(bucket.ProfitFactor), bucket.TotalR)
}
