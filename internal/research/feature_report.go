package research

import (
	"fmt"
	"math"
)

func PrintFeatureAnalysis(analysis FeatureAnalysis) {

	fmt.Println()
	fmt.Println("========================================================")
	fmt.Println(" AUCTION FEATURE ANALYSIS")
	fmt.Println("========================================================")

	for _, report := range analysis.Reports {

		fmt.Println()
		fmt.Println(report.Name)
		fmt.Println("--------------------------------------------------------")

		printFeatureBucket("TRUE", report.True)

		fmt.Println()

		printFeatureBucket("FALSE", report.False)

		fmt.Println()
	}
}

func printFeatureBucket(
	label string,
	bucket FeatureBucket,
) {

	fmt.Println(label)
	fmt.Println("----------------------------------------")

	fmt.Println("PLAN LEVEL")
	fmt.Printf("Valid Plans:          %d\n", bucket.ValidPlans)
	fmt.Printf("Filled:               %d\n", bucket.Filled)
	fmt.Printf("No Fill:              %d\n", bucket.NoFill)
	fmt.Printf("Fill Rate:            %.2f%%\n", bucket.FillRate)
	fmt.Printf("Total R:              %.2fR\n", bucket.TotalR)
	fmt.Printf("Average R / Plan:     %.3fR\n", bucket.AverageRPerPlan)

	fmt.Println()
	fmt.Println("FILLED ONLY")
	fmt.Printf("Wins:                 %d\n", bucket.Wins)
	fmt.Printf("Losses:               %d\n", bucket.Losses)
	fmt.Printf("True Breakevens:      %d\n", bucket.Breakeven)
	fmt.Printf("Filled Win Rate:      %.2f%%\n", bucket.FilledWinRate)
	fmt.Printf("Gross Profit:         %.2fR\n", bucket.GrossProfit)
	fmt.Printf("Gross Loss:           %.2fR\n", bucket.GrossLoss)

	fmt.Printf(
		"Profit Factor:        %s\n",
		formatFeatureProfitFactor(bucket.ProfitFactor),
	)

	fmt.Printf("Average R / Filled:   %.3fR\n", bucket.AverageRPerFilled)
	fmt.Printf("Average MFE:          %.2fR\n", bucket.AverageMFE)
	fmt.Printf("Average MAE:          %.2fR\n", bucket.AverageMAE)
}

func formatFeatureProfitFactor(value float64) string {

	if math.IsInf(value, 1) {
		return "∞"
	}

	return fmt.Sprintf("%.2f", value)
}
