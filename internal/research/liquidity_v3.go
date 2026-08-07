package research

import (
	"fmt"
	"math"

	"github.com/ogtrading/overnight-strategy/internal/liquidity"
	"github.com/ogtrading/overnight-strategy/internal/models"
)

type LiquidityV3Observation struct {
	Context  liquidity.Context
	Baseline models.TradeResult
	Shadow   models.TradeResult
}

type LiquidityV3Stats struct {
	Plans, Filled, Wins, Losses                            int
	WinRate, TotalR, AverageR, ProfitFactor, MaxDrawdown   float64
	AverageMAE, AverageMFE, AverageHoldMinutes, MFECapture float64
}

type LiquidityV3Analysis struct {
	Baseline       LiquidityV3Stats
	Shadow         LiquidityV3Stats
	EntryLocations map[liquidity.EntryLiquidityType]FeatureBucket
}

func BuildLiquidityExitPlan(plan models.TradePlan, context liquidity.Context) models.TradePlan {
	shadow := plan
	candidateTP1 := plan.TP1
	candidateTP2 := plan.TP2

	if validTarget(plan.Direction, plan.Entry, context.InternalTargetPrice) {
		candidateTP1 = context.InternalTargetPrice
	}
	if validTarget(plan.Direction, plan.Entry, context.ExternalTargetPrice) &&
		farther(plan.Entry, context.ExternalTargetPrice, candidateTP1) {
		candidateTP2 = context.ExternalTargetPrice
	}

	// Only use the liquidity targets when their ordering is executable.
	// Otherwise retain the complete baseline target pair.
	if farther(plan.Entry, candidateTP2, candidateTP1) {
		shadow.TP1 = candidateTP1
		shadow.TP2 = candidateTP2
		if candidateTP1 != plan.TP1 {
			shadow.TP1Source = "LIQUIDITY_INTERNAL"
		}
	}
	shadow.Reward1Distance = math.Abs(shadow.TP1 - shadow.Entry)
	shadow.Reward2Distance = math.Abs(shadow.TP2 - shadow.Entry)
	if shadow.RiskDistance > 0 {
		shadow.RR1 = shadow.Reward1Distance / shadow.RiskDistance
		shadow.RR2 = shadow.Reward2Distance / shadow.RiskDistance
	}
	return shadow
}

func validTarget(direction models.Bias, entry, target float64) bool {
	return target > 0 && (direction == models.BiasLong && target > entry || direction == models.BiasShort && target < entry)
}
func farther(entry, candidate, first float64) bool {
	return math.Abs(candidate-entry) > math.Abs(first-entry)
}

func AnalyzeLiquidityV3(observations []LiquidityV3Observation) LiquidityV3Analysis {
	a := LiquidityV3Analysis{EntryLocations: map[liquidity.EntryLiquidityType]FeatureBucket{}}
	baseline, shadow := make([]models.TradeResult, 0, len(observations)), make([]models.TradeResult, 0, len(observations))
	for _, observation := range observations {
		baseline = append(baseline, observation.Baseline)
		shadow = append(shadow, observation.Shadow)
		bucket := a.EntryLocations[observation.Context.EntryLiquidity]
		accumulateFeatureBucket(&bucket, observation.Baseline)
		a.EntryLocations[observation.Context.EntryLiquidity] = bucket
	}
	for key, bucket := range a.EntryLocations {
		finalizeFeatureBucket(&bucket)
		a.EntryLocations[key] = bucket
	}
	a.Baseline, a.Shadow = calculateV3Stats(baseline), calculateV3Stats(shadow)
	return a
}

func calculateV3Stats(results []models.TradeResult) LiquidityV3Stats {
	s := LiquidityV3Stats{Plans: len(results)}
	var grossProfit, grossLoss, equity, peak, totalMAE, totalMFE, totalHold, totalCapture float64
	for _, result := range results {
		if !result.Filled {
			continue
		}
		s.Filled++
		s.TotalR += result.RealizedR
		totalMAE += result.MAER
		totalMFE += result.MFER
		totalHold += float64(result.MinutesInTrade)
		if result.RealizedR > realizedRTolerance {
			s.Wins++
			grossProfit += result.RealizedR
		} else if result.RealizedR < -realizedRTolerance {
			s.Losses++
			grossLoss += -result.RealizedR
		}
		// Capture measures realized upside as a share of available favorable
		// excursion. Losing trades contribute zero rather than a misleading
		// negative percentage with an unbounded magnitude.
		if result.MFER > realizedRTolerance && result.RealizedR > realizedRTolerance {
			totalCapture += math.Min(result.RealizedR/result.MFER, 1)
		}
		equity += result.RealizedR
		if equity > peak {
			peak = equity
		}
		if peak-equity > s.MaxDrawdown {
			s.MaxDrawdown = peak - equity
		}
	}
	if s.Filled > 0 {
		denominator := float64(s.Filled)
		s.WinRate = float64(s.Wins) / denominator * 100
		s.AverageR = s.TotalR / denominator
		s.AverageMAE = totalMAE / denominator
		s.AverageMFE = totalMFE / denominator
		s.AverageHoldMinutes = totalHold / denominator
		s.MFECapture = totalCapture / denominator * 100
	}
	if grossLoss > 0 {
		s.ProfitFactor = grossProfit / grossLoss
	} else if grossProfit > 0 {
		s.ProfitFactor = math.Inf(1)
	}
	return s
}

func PrintLiquidityV3Analysis(a LiquidityV3Analysis) {
	fmt.Println()
	fmt.Println("========================================================")
	fmt.Println(" BASELINE VS LIQUIDITY V3 SHADOW EXIT")
	fmt.Println("========================================================")
	printV3Stats("BASELINE", a.Baseline)
	printV3Stats("LIQUIDITY SHADOW", a.Shadow)
	fmt.Println()
	fmt.Println("ENTRY LIQUIDITY LOCATION")
	order := []liquidity.EntryLiquidityType{liquidity.EntryInternalSSL, liquidity.EntryInternalBSL, liquidity.EntryExternalSSL, liquidity.EntryExternalBSL, liquidity.EntryLiquidityNone}
	for _, key := range order {
		printLiquidityV21Bucket(string(key), a.EntryLocations[key])
	}
}

func printV3Stats(label string, s LiquidityV3Stats) {
	fmt.Printf("\n%s\n", label)
	fmt.Printf("Plans %d | Filled %d | Win %.1f%% | Avg %.3fR | PF %s | Total %.2fR | DD %.2fR\n", s.Plans, s.Filled, s.WinRate, s.AverageR, formatProfitFactor(s.ProfitFactor), s.TotalR, s.MaxDrawdown)
	fmt.Printf("Avg MAE %.2fR | Avg MFE %.2fR | Avg hold %.1fm | MFE capture %.1f%%\n", s.AverageMAE, s.AverageMFE, s.AverageHoldMinutes, s.MFECapture)
}
