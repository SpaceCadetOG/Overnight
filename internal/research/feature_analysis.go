package research

import (
	"math"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

const realizedRTolerance = 1e-9

// AnalyzeFeatureBooleans builds raw statistics for every boolean auction
// feature and computes the derived statistics.
func AnalyzeFeatureBooleans(
	observations []AuctionObservation,
) FeatureAnalysis {

	analysis := FeatureAnalysis{
		Reports: []FeatureReport{
			{Name: "ENTRY_INSIDE_VALUE"},
			{Name: "ENTRY_ABOVE_VAH"},
			{Name: "ENTRY_BELOW_VAL"},
			{Name: "POC_BETWEEN_ENTRY_AND_TP1"},
			{Name: "POC_BEHIND_ENTRY"},
			{Name: "POC_BEYOND_TP1"},
			{Name: "FIB618_ABOVE_POC"},
			{Name: "FIB618_BELOW_POC"},
			{Name: "VWAP_SUPPORTS_DIRECTION"},
		},
	}

	for _, obs := range observations {

		values := []bool{
			obs.Structure.EntryInsideValue,
			obs.Structure.EntryAboveVAH,
			obs.Structure.EntryBelowVAL,
			obs.Structure.POCBetweenEntryAndTP1,
			obs.Structure.POCBehindEntry,
			obs.Structure.POCBeyondTP1,
			obs.Structure.Fib618AbovePOC,
			obs.Structure.Fib618BelowPOC,
			obs.Structure.VWAPSupportsDirection,
		}

		for i := range analysis.Reports {

			var bucket *FeatureBucket

			if values[i] {
				bucket = &analysis.Reports[i].True
			} else {
				bucket = &analysis.Reports[i].False
			}

			accumulateFeatureBucket(bucket, obs.Result)
		}
	}

	finalizeFeatureAnalysis(&analysis)

	return analysis
}

func accumulateFeatureBucket(
	bucket *FeatureBucket,
	result models.TradeResult,
) {
	bucket.ValidPlans++

	if !result.Filled || result.Outcome == models.OutcomeNoFill {
		bucket.NoFill++
		return
	}

	bucket.Filled++

	bucket.TotalR += result.RealizedR
	bucket.TotalMFE += result.MFER
	bucket.TotalMAE += result.MAER

	switch {
	case result.RealizedR > realizedRTolerance:
		bucket.Wins++
		bucket.GrossProfit += result.RealizedR
	case result.RealizedR < -realizedRTolerance:
		bucket.Losses++
		bucket.GrossLoss += -result.RealizedR
	default:
		bucket.Breakeven++
	}
}

func finalizeFeatureAnalysis(analysis *FeatureAnalysis) {
	for i := range analysis.Reports {
		finalizeFeatureBucket(&analysis.Reports[i].True)
		finalizeFeatureBucket(&analysis.Reports[i].False)
	}
}

func finalizeFeatureBucket(bucket *FeatureBucket) {
	if bucket.ValidPlans > 0 {
		bucket.FillRate = float64(bucket.Filled) /
			float64(bucket.ValidPlans) * 100
		bucket.AverageRPerPlan = bucket.TotalR /
			float64(bucket.ValidPlans)
	}

	if bucket.Filled > 0 {
		bucket.FilledWinRate = float64(bucket.Wins) /
			float64(bucket.Filled) * 100
		bucket.AverageRPerFilled = bucket.TotalR /
			float64(bucket.Filled)
		bucket.AverageMFE = bucket.TotalMFE /
			float64(bucket.Filled)
		bucket.AverageMAE = bucket.TotalMAE /
			float64(bucket.Filled)
	}

	switch {

	case bucket.GrossLoss == 0 &&
		bucket.GrossProfit > 0:

		bucket.ProfitFactor = math.Inf(1)

	case bucket.GrossLoss == 0:

		bucket.ProfitFactor = 0

	default:

		bucket.ProfitFactor =
			bucket.GrossProfit /
				bucket.GrossLoss
	}
}
