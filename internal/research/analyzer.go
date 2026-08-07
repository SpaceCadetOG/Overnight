package research

import (
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

func Analyze(results []models.TradeResult) Report {
	filled := make([]models.TradeResult, 0, len(results))

	for _, result := range results {
		if !result.Filled {
			continue
		}

		if result.Outcome == models.OutcomeInvalid ||
			result.Outcome == models.OutcomeNoFill {
			continue
		}

		filled = append(filled, result)
	}

	report := Report{
		Summary:    calculatePerformance(filled),
		Directions: make(map[string]PerformanceStats),
		Weekdays:   make(map[time.Weekday]PerformanceStats),
		Durations:  calculateDurations(filled),
		Outcomes:   calculateOutcomeReport(filled),
	}

	report.Directions[string(models.BiasLong)] = calculatePerformance(
		filterResults(filled, func(result models.TradeResult) bool {
			return result.Direction == models.BiasLong
		}),
	)

	report.Directions[string(models.BiasShort)] = calculatePerformance(
		filterResults(filled, func(result models.TradeResult) bool {
			return result.Direction == models.BiasShort
		}),
	)

	for weekday := time.Sunday; weekday <= time.Saturday; weekday++ {
		group := filterResults(
			filled,
			func(result models.TradeResult) bool {
				return result.Date.Weekday() == weekday
			},
		)

		if len(group) == 0 {
			continue
		}

		report.Weekdays[weekday] = calculatePerformance(group)
	}

	report.MAEDistribution = buildDistribution(
		filled,
		func(result models.TradeResult) float64 {
			return result.MAER
		},
		[]float64{0, 0.25, 0.50, 0.75, 1.0, math.Inf(1)},
	)

	report.MFEDistribution = buildDistribution(
		filled,
		func(result models.TradeResult) float64 {
			return result.MFER
		},
		[]float64{0, 0.50, 1.0, 1.5, 2.0, 3.0, math.Inf(1)},
	)

	return report
}

func calculatePerformance(
	results []models.TradeResult,
) PerformanceStats {
	stats := PerformanceStats{
		Trades: len(results),
	}

	if len(results) == 0 {
		return stats
	}

	var (
		grossProfit float64
		grossLoss   float64
		totalMFE    float64
		totalMAE    float64
		equity      float64
		peak        float64
	)

	for _, result := range results {
		r := result.RealizedR

		stats.TotalR += r
		totalMFE += result.MFER
		totalMAE += result.MAER

		switch {
		case r > 0:
			stats.Wins++
			grossProfit += r

		case r < 0:
			stats.Losses++
			grossLoss += math.Abs(r)

		default:
			stats.Breakeven++
		}

		equity += r

		if equity > peak {
			peak = equity
		}

		drawdown := peak - equity
		if drawdown > stats.MaxDrawdown {
			stats.MaxDrawdown = drawdown
		}
	}

	trades := float64(stats.Trades)

	stats.AverageR = stats.TotalR / trades
	stats.WinRate = float64(stats.Wins) / trades * 100
	stats.AverageMFE = totalMFE / trades
	stats.AverageMAE = totalMAE / trades

	if grossLoss > 0 {
		stats.ProfitFactor = grossProfit / grossLoss
	} else if grossProfit > 0 {
		stats.ProfitFactor = math.Inf(1)
	}

	return stats
}

func calculateDurations(
	results []models.TradeResult,
) DurationStats {
	all := make([]int, 0, len(results))
	winners := make([]int, 0)
	losers := make([]int, 0)

	for _, result := range results {
		if result.MinutesInTrade < 0 {
			continue
		}

		all = append(all, result.MinutesInTrade)

		switch {
		case result.RealizedR > 0:
			winners = append(winners, result.MinutesInTrade)

		case result.RealizedR < 0:
			losers = append(losers, result.MinutesInTrade)
		}
	}

	return DurationStats{
		All:     summarizeDurations(all),
		Winners: summarizeDurations(winners),
		Losers:  summarizeDurations(losers),
	}
}

func summarizeDurations(values []int) DurationGroup {
	group := DurationGroup{
		Trades: len(values),
	}

	if len(values) == 0 {
		return group
	}

	sorted := append([]int(nil), values...)
	sort.Ints(sorted)

	total := 0
	for _, value := range sorted {
		total += value
	}

	group.AverageMinutes = float64(total) / float64(len(sorted))
	group.MedianMinutes = percentileInts(sorted, 0.50)
	group.P90Minutes = percentileInts(sorted, 0.90)
	group.MinimumMinutes = sorted[0]
	group.MaximumMinutes = sorted[len(sorted)-1]

	return group
}

func percentileInts(sorted []int, percentile float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	if len(sorted) == 1 {
		return float64(sorted[0])
	}

	position := percentile * float64(len(sorted)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))

	if lower == upper {
		return float64(sorted[lower])
	}

	weight := position - float64(lower)

	return float64(sorted[lower])*(1-weight) +
		float64(sorted[upper])*weight
}

func buildDistribution(
	results []models.TradeResult,
	value func(models.TradeResult) float64,
	bounds []float64,
) []DistributionBucket {
	if len(bounds) < 2 {
		return nil
	}

	buckets := make([]DistributionBucket, 0, len(bounds)-1)

	for index := 0; index < len(bounds)-1; index++ {
		lower := bounds[index]
		upper := bounds[index+1]

		buckets = append(
			buckets,
			DistributionBucket{
				Label:      bucketLabel(lower, upper),
				LowerBound: lower,
				UpperBound: upper,
			},
		)
	}

	for _, result := range results {
		current := value(result)

		for index := range buckets {
			bucket := &buckets[index]

			if current < bucket.LowerBound {
				continue
			}

			isFinalBucket := index == len(buckets)-1

			if current < bucket.UpperBound ||
				(isFinalBucket && math.IsInf(bucket.UpperBound, 1)) {
				bucket.Count++
				break
			}
		}
	}

	if len(results) > 0 {
		for index := range buckets {
			buckets[index].Percent =
				float64(buckets[index].Count) /
					float64(len(results)) *
					100
		}
	}

	return buckets
}

func bucketLabel(lower float64, upper float64) string {
	if math.IsInf(upper, 1) {
		return formatR(lower) + "+"
	}

	return formatR(lower) + "–" + formatR(upper)
}

func formatR(value float64) string {
	switch {
	case value == math.Trunc(value):
		return formatFloat(value, 0) + "R"

	case math.Abs(value*10-math.Round(value*10)) < 0.000001:
		return formatFloat(value, 1) + "R"

	default:
		return formatFloat(value, 2) + "R"
	}
}

func formatFloat(value float64, decimals int) string {
	if decimals == 0 {
		return strconvFormat(value, 'f', 0)
	}

	if decimals == 1 {
		return strconvFormat(value, 'f', 1)
	}

	return strconvFormat(value, 'f', 2)
}

func strconvFormat(
	value float64,
	format byte,
	precision int,
) string {
	return strconv.FormatFloat(value, format, precision, 64)
}

func filterResults(
	results []models.TradeResult,
	keep func(models.TradeResult) bool,
) []models.TradeResult {
	filtered := make([]models.TradeResult, 0)

	for _, result := range results {
		if keep(result) {
			filtered = append(filtered, result)
		}
	}

	return filtered
}
