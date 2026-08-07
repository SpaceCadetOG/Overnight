package reporting

import (
	"math"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

type Statistics struct {
	Sessions   int
	ValidPlans int

	Filled int
	NoFill int

	Wins   int
	Losses int

	TP2Count      int
	TP1BECount    int
	StoppedCount  int
	TimeExitCount int

	FillRate float64
	WinRate  float64

	TotalR         float64
	AverageR       float64
	AverageRFilled float64

	AverageMFER          float64
	AverageMAER          float64
	AverageMissDistance  float64
	AverageMinutesToFill float64

	ProfitFactor float64
	MaxDrawdownR float64
}

func CalculateStatistics(
	results []models.TradeResult,
) Statistics {
	stats := Statistics{
		Sessions: len(results),
	}

	var positiveR float64
	var negativeR float64
	var filledR float64
	var totalMFE float64
	var totalMAE float64
	var totalMissDistance float64
	var totalMinutesToFill float64
	var noFillWithDistance int

	var equity float64
	var equityPeak float64

	for _, result := range results {
		if result.Outcome == models.OutcomeInvalid {
			continue
		}

		stats.ValidPlans++
		stats.TotalR += result.RealizedR

		if result.Filled {
			stats.Filled++
			filledR += result.RealizedR
			totalMFE += result.MFER
			totalMAE += result.MAER
			totalMinutesToFill += float64(result.MinutesToFill)

			if result.RealizedR > 0 {
				stats.Wins++
				positiveR += result.RealizedR
			} else if result.RealizedR < 0 {
				stats.Losses++
				negativeR += math.Abs(result.RealizedR)
			}
		} else {
			stats.NoFill++

			if result.MissedEntryDistance > 0 {
				totalMissDistance += result.MissedEntryDistance
				noFillWithDistance++
			}
		}

		switch result.Outcome {
		case models.OutcomeTP2:
			stats.TP2Count++
		case models.OutcomeTP1Breakeven:
			stats.TP1BECount++
		case models.OutcomeStopped:
			stats.StoppedCount++
		case models.OutcomeTimeExit:
			stats.TimeExitCount++
		}

		equity += result.RealizedR

		if equity > equityPeak {
			equityPeak = equity
		}

		drawdown := equityPeak - equity
		if drawdown > stats.MaxDrawdownR {
			stats.MaxDrawdownR = drawdown
		}
	}

	if stats.ValidPlans > 0 {
		stats.FillRate =
			float64(stats.Filled) /
				float64(stats.ValidPlans)

		stats.AverageR =
			stats.TotalR /
				float64(stats.ValidPlans)
	}

	if stats.Filled > 0 {
		stats.WinRate =
			float64(stats.Wins) /
				float64(stats.Filled)

		stats.AverageRFilled =
			filledR /
				float64(stats.Filled)

		stats.AverageMFER =
			totalMFE /
				float64(stats.Filled)

		stats.AverageMAER =
			totalMAE /
				float64(stats.Filled)

		stats.AverageMinutesToFill =
			totalMinutesToFill /
				float64(stats.Filled)
	}

	if noFillWithDistance > 0 {
		stats.AverageMissDistance =
			totalMissDistance /
				float64(noFillWithDistance)
	}

	if negativeR > 0 {
		stats.ProfitFactor = positiveR / negativeR
	} else if positiveR > 0 {
		stats.ProfitFactor = math.Inf(1)
	}

	return stats
}
