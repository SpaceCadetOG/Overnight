package research

import (
	"fmt"
	"math"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/backtest"
	"github.com/ogtrading/overnight-strategy/internal/liquidity"
	"github.com/ogtrading/overnight-strategy/internal/models"
)

type StopReversalCandidate struct {
	LiquiditySweep bool
	Reclaimed      bool
	Plan           backtest.ReversalPlan
}

type LiquidityV5Observation struct {
	Baseline          models.TradeResult
	Managed           models.TradeResult
	ReversalCandidate StopReversalCandidate
	Reversal          models.TradeResult
}

type LiquidityV5Analysis struct {
	Baseline             LiquidityV3Stats
	Managed              LiquidityV3Stats
	Reversals            LiquidityV3Stats
	BaselinePlusReversal LiquidityV3Stats
	ManagedPlusReversal  LiquidityV3Stats
	OriginalStops        int
	StopsWithSweep       int
	StopsWithReclaim     int
	ValidReversalEntries int
}

// BuildLiquidityManagedWinnerPlan preserves the baseline entry, stop and TP1.
// It only shortens the runner target when a known directional level lies
// beyond TP1 but before the original TP2.
func BuildLiquidityManagedWinnerPlan(plan models.TradePlan, context liquidity.Context, session models.Session) models.TradePlan {
	shadow := plan
	bestDistance := math.Abs(plan.TP2 - plan.Entry)
	candidate := plan.TP2
	prices := []float64{session.POC}
	if plan.Direction == models.BiasLong {
		prices = append(prices, session.VAH, session.High)
	} else {
		prices = append(prices, session.VAL, session.Low)
	}
	for _, level := range context.Levels {
		if level.Taken {
			continue
		}
		if plan.Direction == models.BiasLong && level.Side == liquidity.BuySide ||
			plan.Direction == models.BiasShort && level.Side == liquidity.SellSide {
			prices = append(prices, level.Price)
		}
	}
	for _, price := range prices {
		if !validTarget(plan.Direction, plan.Entry, price) ||
			!farther(plan.Entry, price, plan.TP1) ||
			!farther(plan.Entry, plan.TP2, price) {
			continue
		}
		distance := math.Abs(price - plan.Entry)
		if distance < bestDistance {
			bestDistance, candidate = distance, price
		}
	}
	shadow.TP2 = candidate
	shadow.Reward2Distance = math.Abs(shadow.TP2 - shadow.Entry)
	if shadow.RiskDistance > 0 {
		shadow.RR2 = shadow.Reward2Distance / shadow.RiskDistance
	}
	return shadow
}

func BuildStopReversalCandidate(session models.Session, plan models.TradePlan, baseline models.TradeResult, context liquidity.Context, candles []models.Candle, location *time.Location) StopReversalCandidate {
	candidate := StopReversalCandidate{}
	if baseline.Outcome != models.OutcomeStopped || location == nil {
		return candidate
	}

	extreme := plan.Stop
	for _, candle := range candles {
		localOpen := candle.OpenTime.In(location)
		if localOpen.Before(baseline.FillTime) || candle.CloseTime.In(location).Before(baseline.ExitTime) ||
			localOpen.Year() != plan.Date.Year() || localOpen.YearDay() != plan.Date.YearDay() || localOpen.Hour() >= backtest.OrderEndHour {
			continue
		}
		if plan.Direction == models.BiasLong {
			if candle.Low < extreme {
				extreme = candle.Low
			}
			if candle.Low < plan.Stop && takesSide(context.Levels, liquidity.SellSide, candle) {
				candidate.LiquiditySweep = true
			}
			if candidate.LiquiditySweep && candle.Close > plan.Stop && reclaimedStructure(models.BiasLong, extreme, candle.Close, session) {
				candidate.Reclaimed = true
				candidate.Plan = reversalPlan(session, plan, candle.CloseTime.In(location), candle.Close, extreme, context)
				return candidate
			}
		} else if plan.Direction == models.BiasShort {
			if candle.High > extreme {
				extreme = candle.High
			}
			if candle.High > plan.Stop && takesSide(context.Levels, liquidity.BuySide, candle) {
				candidate.LiquiditySweep = true
			}
			if candidate.LiquiditySweep && candle.Close < plan.Stop && reclaimedStructure(models.BiasShort, extreme, candle.Close, session) {
				candidate.Reclaimed = true
				candidate.Plan = reversalPlan(session, plan, candle.CloseTime.In(location), candle.Close, extreme, context)
				return candidate
			}
		}
	}
	return candidate
}

func takesSide(levels []liquidity.Level, side liquidity.Side, candle models.Candle) bool {
	for _, level := range levels {
		if level.Taken || level.Side != side {
			continue
		}
		if side == liquidity.SellSide && candle.Low < level.Price || side == liquidity.BuySide && candle.High > level.Price {
			return true
		}
	}
	return false
}

func reclaimedStructure(direction models.Bias, extreme, close float64, session models.Session) bool {
	levels := []float64{session.VWAP, session.VAL, session.Fib382, session.Fib500, session.Fib618}
	for _, level := range levels {
		if direction == models.BiasLong && extreme < level && close >= level ||
			direction == models.BiasShort && extreme > level && close <= level {
			return true
		}
	}
	return false
}

func reversalPlan(session models.Session, original models.TradePlan, entryTime time.Time, entry, stop float64, context liquidity.Context) backtest.ReversalPlan {
	prices := []float64{session.POC}
	if original.Direction == models.BiasLong {
		prices = append(prices, session.VAH)
	} else {
		prices = append(prices, session.VAL)
	}
	prices = append(prices, original.TP1, original.TP2)
	for _, level := range context.Levels {
		if !level.Taken && (original.Direction == models.BiasLong && level.Side == liquidity.BuySide || original.Direction == models.BiasShort && level.Side == liquidity.SellSide) {
			prices = append(prices, level.Price)
		}
	}
	var target float64
	for _, price := range prices {
		if validTarget(original.Direction, entry, price) {
			target = price
			break
		}
	}
	validStop := original.Direction == models.BiasLong && stop < entry || original.Direction == models.BiasShort && stop > entry
	return backtest.ReversalPlan{Date: original.Date, Direction: original.Direction, EntryTime: entryTime, Entry: entry, Stop: stop, Target: target, Valid: validStop && target > 0}
}

func AnalyzeLiquidityV5(observations []LiquidityV5Observation) LiquidityV5Analysis {
	a := LiquidityV5Analysis{}
	baseline := make([]models.TradeResult, 0, len(observations))
	managed := make([]models.TradeResult, 0, len(observations))
	reversals := make([]models.TradeResult, 0)
	baselineCombined := make([]models.TradeResult, 0, len(observations))
	managedCombined := make([]models.TradeResult, 0, len(observations))
	for _, observation := range observations {
		baseline = append(baseline, observation.Baseline)
		managed = append(managed, observation.Managed)
		if observation.Baseline.Outcome == models.OutcomeStopped {
			a.OriginalStops++
		}
		if observation.ReversalCandidate.LiquiditySweep {
			a.StopsWithSweep++
		}
		if observation.ReversalCandidate.Reclaimed {
			a.StopsWithReclaim++
		}
		if observation.ReversalCandidate.Plan.Valid {
			a.ValidReversalEntries++
			reversals = append(reversals, observation.Reversal)
		}
		baselineCombined = append(baselineCombined, combineResults(observation.Baseline, observation.Reversal))
		managedCombined = append(managedCombined, combineResults(observation.Managed, observation.Reversal))
	}
	a.Baseline = calculateV3Stats(baseline)
	a.Managed = calculateV3Stats(managed)
	a.Reversals = calculateV3Stats(reversals)
	a.BaselinePlusReversal = calculateV3Stats(baselineCombined)
	a.ManagedPlusReversal = calculateV3Stats(managedCombined)
	return a
}

func combineResults(primary, reversal models.TradeResult) models.TradeResult {
	combined := primary
	if reversal.Filled {
		combined.RealizedR += reversal.RealizedR
		combined.GrossR += reversal.GrossR
	}
	return combined
}

func PrintLiquidityV5Analysis(a LiquidityV5Analysis) {
	fmt.Println("\n========================================================")
	fmt.Println(" LIQUIDITY V5 COMBINED RESEARCH")
	fmt.Println("========================================================")
	fmt.Println("\nREPORT 1 — FROZEN BASELINE")
	printV3Stats("BASELINE", a.Baseline)
	fmt.Println("\nREPORT 2 — POST-TP1 RUNNER MANAGEMENT")
	printV3Stats("BASELINE EXIT", a.Baseline)
	printV3Stats("LIQUIDITY-MANAGED RUNNER", a.Managed)
	fmt.Printf("Difference: %+.2fR\n", a.Managed.TotalR-a.Baseline.TotalR)
	fmt.Println("\nREPORT 3 — STOP REVERSAL")
	fmt.Printf("Original stops: %d\n", a.OriginalStops)
	fmt.Printf("Stopped trades with liquidity sweep: %d\n", a.StopsWithSweep)
	fmt.Printf("Stopped trades with reclaim: %d\n", a.StopsWithReclaim)
	fmt.Printf("Valid reversal entries: %d\n", a.ValidReversalEntries)
	printV3Stats("REVERSAL TRADES ONLY", a.Reversals)
	fmt.Println("\nCOMBINED TOTALS")
	fmt.Printf("Baseline only: %.2fR\n", a.Baseline.TotalR)
	fmt.Printf("Baseline + liquidity exits: %.2fR\n", a.Managed.TotalR)
	fmt.Printf("Baseline + reversals: %.2fR\n", a.BaselinePlusReversal.TotalR)
	fmt.Printf("Baseline + both: %.2fR\n", a.ManagedPlusReversal.TotalR)
}
