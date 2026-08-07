package research

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

func Print(report Report) {
	fmt.Println()
	fmt.Println("==========================================")
	fmt.Println(" OVERNIGHT STRATEGY RESEARCH — PHASE 1")
	fmt.Println("==========================================")
	fmt.Println()

	printPerformance("OVERALL PERFORMANCE", report.Summary)

	fmt.Println()
	fmt.Println("LONG VS SHORT")
	fmt.Println("------------------------------------------")

	printCompactPerformance(
		"LONG",
		report.Directions["LONG"],
	)
	printCompactPerformance(
		"SHORT",
		report.Directions["SHORT"],
	)

	fmt.Println()
	fmt.Println("WEEKDAY PERFORMANCE")
	fmt.Println("------------------------------------------")
	fmt.Printf(
		"%-10s %7s %8s %8s %8s %9s\n",
		"Day",
		"Trades",
		"Win %",
		"Avg R",
		"PF",
		"Total R",
	)

	weekdays := []time.Weekday{
		time.Monday,
		time.Tuesday,
		time.Wednesday,
		time.Thursday,
		time.Friday,
		time.Saturday,
		time.Sunday,
	}

	for _, weekday := range weekdays {
		stats, exists := report.Weekdays[weekday]
		if !exists {
			continue
		}

		fmt.Printf(
			"%-10s %7d %7.1f%% %8.2f %8s %9.2f\n",
			weekday.String(),
			stats.Trades,
			stats.WinRate,
			stats.AverageR,
			formatProfitFactor(stats.ProfitFactor),
			stats.TotalR,
		)
	}

	fmt.Println()
	fmt.Println("TRADE DURATION")
	fmt.Println("------------------------------------------")
	fmt.Printf(
		"%-10s %7s %10s %10s %10s %8s %8s\n",
		"Group",
		"Trades",
		"Average",
		"Median",
		"P90",
		"Min",
		"Max",
	)

	printDuration("All", report.Durations.All)
	printDuration("Winners", report.Durations.Winners)
	printDuration("Losers", report.Durations.Losers)

	fmt.Println()
	printDistribution("MAE DISTRIBUTION", report.MAEDistribution)

	fmt.Println()
	printDistribution("MFE DISTRIBUTION", report.MFEDistribution)

	fmt.Println()
	fmt.Println("OUTCOME BREAKDOWN")
	fmt.Println("------------------------------------------")
	fmt.Printf("%-12s %7s %8s %8s %8s %8s\n",
		"Outcome", "Trades", "Avg R", "Win %", "PF", "Total R")

	printCompactPerformance("TP2", report.Outcomes.TP2)
	printCompactPerformance("TP1+BE", report.Outcomes.TP1BE)
	printCompactPerformance("STOP", report.Outcomes.Stopped)
	printCompactPerformance("TIME", report.Outcomes.TimeExit)

	fmt.Println()
	fmt.Println("==========================================")
}

func printPerformance(
	title string,
	stats PerformanceStats,
) {
	fmt.Println(title)
	fmt.Println("------------------------------------------")
	fmt.Printf("Trades:            %d\n", stats.Trades)
	fmt.Printf("Wins:              %d\n", stats.Wins)
	fmt.Printf("Losses:            %d\n", stats.Losses)
	fmt.Printf("Breakeven:         %d\n", stats.Breakeven)
	fmt.Printf("Win rate:          %.1f%%\n", stats.WinRate)
	fmt.Printf("Total R:           %.2fR\n", stats.TotalR)
	fmt.Printf("Average R/trade:   %.3fR\n", stats.AverageR)
	fmt.Printf(
		"Profit factor:     %s\n",
		formatProfitFactor(stats.ProfitFactor),
	)
	fmt.Printf("Maximum drawdown:  %.2fR\n", stats.MaxDrawdown)
	fmt.Printf("Average MFE:       %.2fR\n", stats.AverageMFE)
	fmt.Printf("Average MAE:       %.2fR\n", stats.AverageMAE)
}

func printCompactPerformance(
	label string,
	stats PerformanceStats,
) {
	fmt.Printf(
		"%-6s | Trades %4d | Win %5.1f%% | "+
			"Avg %6.3fR | PF %6s | Total %8.2fR | DD %6.2fR\n",
		label,
		stats.Trades,
		stats.WinRate,
		stats.AverageR,
		formatProfitFactor(stats.ProfitFactor),
		stats.TotalR,
		stats.MaxDrawdown,
	)
}

func printDuration(label string, stats DurationGroup) {
	fmt.Printf(
		"%-10s %7d %8.1fm %8.1fm %8.1fm %6dm %6dm\n",
		label,
		stats.Trades,
		stats.AverageMinutes,
		stats.MedianMinutes,
		stats.P90Minutes,
		stats.MinimumMinutes,
		stats.MaximumMinutes,
	)
}

func printDistribution(
	title string,
	buckets []DistributionBucket,
) {
	fmt.Println(title)
	fmt.Println("------------------------------------------")

	sorted := append([]DistributionBucket(nil), buckets...)

	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].LowerBound < sorted[j].LowerBound
	})

	for _, bucket := range sorted {
		barLength := int(math.Round(bucket.Percent / 2))

		if barLength > 50 {
			barLength = 50
		}

		fmt.Printf(
			"%-12s %5d %6.1f%% %s\n",
			bucket.Label,
			bucket.Count,
			bucket.Percent,
			strings.Repeat("█", barLength),
		)
	}
}

func formatProfitFactor(value float64) string {
	if math.IsInf(value, 1) {
		return "∞"
	}

	return fmt.Sprintf("%.2f", value)
}
