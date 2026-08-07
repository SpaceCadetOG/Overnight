package reporting

import (
	"fmt"
	"sort"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

type NoFillProximity struct {
	NoFills    int
	Normalized int
	AverageR   float64
	MedianR    float64
	P75R       float64
	P90R       float64
	MinimumR   float64
	MaximumR   float64
	Within010R int
	Within025R int
	Within050R int
	Within100R int
	Within200R int
	Beyond200R int
}

func AnalyzeNoFillProximity(results []models.TradeResult) NoFillProximity {
	analysis := NoFillProximity{}
	values := make([]float64, 0)
	for _, result := range results {
		if result.Outcome != models.OutcomeNoFill {
			continue
		}
		analysis.NoFills++
		if result.InitialRisk <= 1e-9 {
			continue
		}
		missR := result.MissedEntryDistance / result.InitialRisk
		values = append(values, missR)
		analysis.AverageR += missR
		switch {
		case missR <= .10:
			analysis.Within010R++
		case missR <= .25:
			analysis.Within025R++
		case missR <= .50:
			analysis.Within050R++
		case missR <= 1:
			analysis.Within100R++
		case missR <= 2:
			analysis.Within200R++
		default:
			analysis.Beyond200R++
		}
	}
	analysis.Normalized = len(values)
	if len(values) == 0 {
		return analysis
	}
	sort.Float64s(values)
	analysis.AverageR /= float64(len(values))
	analysis.MinimumR = values[0]
	analysis.MedianR = percentile(values, .50)
	analysis.P75R = percentile(values, .75)
	analysis.P90R = percentile(values, .90)
	analysis.MaximumR = values[len(values)-1]
	return analysis
}

func percentile(sorted []float64, fraction float64) float64 {
	index := int(float64(len(sorted)-1) * fraction)
	return sorted[index]
}

func PrintNoFillProximity(analysis NoFillProximity) {
	fmt.Println("\n========================================================")
	fmt.Println(" NO-FILL PROXIMITY (NORMALIZED BY PLANNED RISK)")
	fmt.Println("========================================================")
	fmt.Printf("No fills: %d | Normalized: %d\n", analysis.NoFills, analysis.Normalized)
	fmt.Printf("Minimum %.3fR | Median %.3fR | Mean %.3fR | P75 %.3fR | P90 %.3fR | Maximum %.3fR\n", analysis.MinimumR, analysis.MedianR, analysis.AverageR, analysis.P75R, analysis.P90R, analysis.MaximumR)
	printNoFillBucket("0–0.10R", analysis.Within010R, analysis.Normalized)
	printNoFillBucket("0.10–0.25R", analysis.Within025R, analysis.Normalized)
	printNoFillBucket("0.25–0.50R", analysis.Within050R, analysis.Normalized)
	printNoFillBucket("0.50–1.00R", analysis.Within100R, analysis.Normalized)
	printNoFillBucket("1.00–2.00R", analysis.Within200R, analysis.Normalized)
	printNoFillBucket("2.00R+", analysis.Beyond200R, analysis.Normalized)
}

func printNoFillBucket(label string, count, total int) {
	percent := 0.0
	if total > 0 {
		percent = float64(count) / float64(total) * 100
	}
	fmt.Printf("%-12s %4d %6.1f%%\n", label, count, percent)
}
