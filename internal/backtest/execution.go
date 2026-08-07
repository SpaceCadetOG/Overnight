package backtest

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

const (
	OrderStartHour = 5
	OrderEndHour   = 16
)

// SimulateTrade preserves the original ideal-execution behavior.
func SimulateTrade(
	plan models.TradePlan,
	allCandles []models.Candle,
	location *time.Location,
) (models.TradeResult, error) {
	return SimulateTradeWithConfig(
		plan,
		allCandles,
		location,
		IdealExecutionConfig(),
	)
}

// SimulateTradeWithConfig replays the 05:00–16:00 CT execution window
// using configurable fees and adverse execution slippage.
func SimulateTradeWithConfig(
	plan models.TradePlan,
	allCandles []models.Candle,
	location *time.Location,
	execution ExecutionConfig,
) (models.TradeResult, error) {
	if location == nil {
		return models.TradeResult{}, fmt.Errorf("location is required")
	}

	if err := execution.Validate(); err != nil {
		return models.TradeResult{}, fmt.Errorf(
			"invalid execution config: %w",
			err,
		)
	}

	if !plan.Valid {
		return models.TradeResult{
			Date:      plan.Date,
			Direction: plan.Direction,
			Outcome:   models.OutcomeInvalid,
			Notes:     plan.InvalidReason,
		}, nil
	}

	executionCandles := executionWindow(
		allCandles,
		plan.Date,
		location,
	)

	fillPrice := adversePrice(
		plan.Entry,
		plan.Direction,
		execution.EntrySlippageBps,
		true,
	)

	result := models.TradeResult{
		Date:      plan.Date,
		Direction: plan.Direction,
		Outcome:   models.OutcomeNoFill,

		Entry:       plan.Entry,
		EntrySource: plan.EntrySource,
		Stop:        plan.Stop,
		StopSource:  plan.StopSource,
		TP1:         plan.TP1,
		TP1Source:   plan.TP1Source,
		TP2:         plan.TP2,

		FillPrice:        fillPrice,
		InitialRisk:      math.Abs(fillPrice - plan.Stop),
		EntrySlippageBps: execution.EntrySlippageBps,
		HighestAfterFill: fillPrice,
		LowestAfterFill:  fillPrice,
	}

	if result.InitialRisk <= 0 {
		return models.TradeResult{}, fmt.Errorf(
			"invalid initial risk: fill %.8f stop %.8f",
			fillPrice,
			plan.Stop,
		)
	}

	if len(executionCandles) == 0 {
		result.Notes = "no execution-window candles"
		return result, nil
	}

	windowStart := executionCandles[0].OpenTime.In(location)

	result.WindowHigh = executionCandles[0].High
	result.WindowLow = executionCandles[0].Low

	filled := false
	tp1Hit := false

	var lastClose float64
	var lastTime time.Time

	for _, candle := range executionCandles {
		if candle.High > result.WindowHigh {
			result.WindowHigh = candle.High
		}

		if candle.Low < result.WindowLow {
			result.WindowLow = candle.Low
		}

		lastClose = candle.Close
		lastTime = candle.CloseTime.In(location)

		if !filled {
			if !limitTouched(plan.Direction, plan.Entry, candle) {
				continue
			}

			filled = true
			result.Filled = true
			result.FillTime = candle.OpenTime.In(location)
			result.MinutesToFill = int(
				result.FillTime.Sub(windowStart).Minutes(),
			)

			// Entry is treated as a maker fill.
			addFee(
				&result,
				result.FillPrice,
				1.0,
				execution.MakerFeeBps,
			)

			if stopTouched(plan.Direction, plan.Stop, candle) {
				exitPrice := adversePrice(
					plan.Stop,
					plan.Direction,
					execution.StopSlippageBps,
					false,
				)

				initializeExcursionAtEntry(&result)
				updateExcursionToExit(&result, exitPrice)

				result.Outcome = models.OutcomeStopped
				result.ExitTime = candle.CloseTime.In(location)
				result.ExitPrice = exitPrice
				result.ExitSlippageBps = execution.StopSlippageBps

				result.GrossR = directionalR(
					plan.Direction,
					result.FillPrice,
					exitPrice,
					result.InitialRisk,
				)

				addFee(
					&result,
					exitPrice,
					1.0,
					execution.TakerFeeBps,
				)
				finalizeNetR(&result)

				result.MinutesInTrade = int(
					result.ExitTime.Sub(result.FillTime).Minutes(),
				)
				result.Notes = "entry and stop touched in fill candle"
				return result, nil
			}

			updateRunningExcursion(&result, candle)

			if targetTouched(plan.Direction, plan.TP1, candle) {
				tp1Hit = true
				result.TP1Hit = true
				result.TP1Time = candle.CloseTime.In(location)

				tp1Fill := adversePrice(
					plan.TP1,
					plan.Direction,
					execution.TPSlippageBps,
					false,
				)
				result.TP1FillPrice = tp1Fill

				addFee(
					&result,
					tp1Fill,
					0.5,
					execution.MakerFeeBps,
				)

				continue
			}

			continue
		}

		if !tp1Hit {
			if stopTouched(plan.Direction, plan.Stop, candle) {
				exitPrice := adversePrice(
					plan.Stop,
					plan.Direction,
					execution.StopSlippageBps,
					false,
				)

				updateExcursionToExit(&result, exitPrice)

				result.Outcome = models.OutcomeStopped
				result.ExitTime = candle.CloseTime.In(location)
				result.ExitPrice = exitPrice
				result.ExitSlippageBps = execution.StopSlippageBps

				result.GrossR = directionalR(
					plan.Direction,
					result.FillPrice,
					exitPrice,
					result.InitialRisk,
				)

				addFee(
					&result,
					exitPrice,
					1.0,
					execution.TakerFeeBps,
				)
				finalizeNetR(&result)

				result.MinutesInTrade = int(
					result.ExitTime.Sub(result.FillTime).Minutes(),
				)
				return result, nil
			}

			updateRunningExcursion(&result, candle)

			if targetTouched(plan.Direction, plan.TP1, candle) {
				tp1Hit = true
				result.TP1Hit = true
				result.TP1Time = candle.CloseTime.In(location)

				tp1Fill := adversePrice(
					plan.TP1,
					plan.Direction,
					execution.TPSlippageBps,
					false,
				)
				result.TP1FillPrice = tp1Fill

				addFee(
					&result,
					tp1Fill,
					0.5,
					execution.MakerFeeBps,
				)

				continue
			}

			continue
		}

		// The runner's breakeven is its actual entry fill price.
		if breakevenTouched(
			plan.Direction,
			result.FillPrice,
			candle,
		) {
			exitPrice := adversePrice(
				result.FillPrice,
				plan.Direction,
				execution.StopSlippageBps,
				false,
			)

			updateExcursionToExit(&result, exitPrice)

			result.Outcome = models.OutcomeTP1Breakeven
			result.ExitTime = candle.CloseTime.In(location)
			result.ExitPrice = exitPrice
			result.ExitSlippageBps = execution.StopSlippageBps

			tp1R := directionalR(
				plan.Direction,
				result.FillPrice,
				result.TP1FillPrice,
				result.InitialRisk,
			)
			runnerR := directionalR(
				plan.Direction,
				result.FillPrice,
				exitPrice,
				result.InitialRisk,
			)

			result.GrossR = 0.5*tp1R + 0.5*runnerR

			addFee(
				&result,
				exitPrice,
				0.5,
				execution.TakerFeeBps,
			)
			finalizeNetR(&result)

			result.MinutesInTrade = int(
				result.ExitTime.Sub(result.FillTime).Minutes(),
			)
			result.Notes = "runner exited at simulated breakeven"
			return result, nil
		}

		if targetTouched(plan.Direction, plan.TP2, candle) {
			exitPrice := adversePrice(
				plan.TP2,
				plan.Direction,
				execution.TPSlippageBps,
				false,
			)

			updateExcursionToTarget(
				&result,
				candle,
				exitPrice,
			)

			result.Outcome = models.OutcomeTP2
			result.ExitTime = candle.CloseTime.In(location)
			result.ExitPrice = exitPrice
			result.ExitSlippageBps = execution.TPSlippageBps

			tp1R := directionalR(
				plan.Direction,
				result.FillPrice,
				result.TP1FillPrice,
				result.InitialRisk,
			)
			tp2R := directionalR(
				plan.Direction,
				result.FillPrice,
				exitPrice,
				result.InitialRisk,
			)

			result.GrossR = 0.5*tp1R + 0.5*tp2R

			addFee(
				&result,
				exitPrice,
				0.5,
				execution.MakerFeeBps,
			)
			finalizeNetR(&result)

			result.MinutesInTrade = int(
				result.ExitTime.Sub(result.FillTime).Minutes(),
			)
			return result, nil
		}

		updateRunningExcursion(&result, candle)
	}

	if !filled {
		result.Outcome = models.OutcomeNoFill
		result.ExitTime = lastTime
		result.MissedEntryDistance = missedEntryDistance(
			plan.Direction,
			plan.Entry,
			result.WindowHigh,
			result.WindowLow,
		)
		result.Notes = "limit order not filled by 16:00 CT"
		return result, nil
	}

	exitPrice := adversePrice(
		lastClose,
		plan.Direction,
		execution.TimeSlippageBps,
		false,
	)

	result.Outcome = models.OutcomeTimeExit
	result.ExitTime = lastTime
	result.ExitPrice = exitPrice
	result.ExitSlippageBps = execution.TimeSlippageBps
	result.MinutesInTrade = int(
		result.ExitTime.Sub(result.FillTime).Minutes(),
	)

	if tp1Hit {
		tp1R := directionalR(
			plan.Direction,
			result.FillPrice,
			result.TP1FillPrice,
			result.InitialRisk,
		)
		runnerR := directionalR(
			plan.Direction,
			result.FillPrice,
			exitPrice,
			result.InitialRisk,
		)

		result.GrossR = 0.5*tp1R + 0.5*runnerR

		addFee(
			&result,
			exitPrice,
			0.5,
			execution.TakerFeeBps,
		)
		finalizeNetR(&result)

		result.Notes = "runner closed at 16:00 CT"
		return result, nil
	}

	result.GrossR = directionalR(
		plan.Direction,
		result.FillPrice,
		exitPrice,
		result.InitialRisk,
	)

	addFee(
		&result,
		exitPrice,
		1.0,
		execution.TakerFeeBps,
	)
	finalizeNetR(&result)

	result.Notes = "full position closed at 16:00 CT before TP1"
	return result, nil
}

func adversePrice(
	price float64,
	direction models.Bias,
	bps float64,
	isEntry bool,
) float64 {
	if bps <= 0 {
		return price
	}

	multiplier := bps / 10000.0

	switch direction {
	case models.BiasLong:
		if isEntry {
			return price * (1 + multiplier)
		}
		return price * (1 - multiplier)

	case models.BiasShort:
		if isEntry {
			return price * (1 - multiplier)
		}
		return price * (1 + multiplier)

	default:
		return price
	}
}

func addFee(
	result *models.TradeResult,
	price float64,
	positionFraction float64,
	feeBps float64,
) {
	if feeBps <= 0 || positionFraction <= 0 {
		return
	}

	result.TotalFees += price *
		positionFraction *
		(feeBps / 10000.0)
}

func finalizeNetR(result *models.TradeResult) {
	if result.InitialRisk <= 0 {
		result.RealizedR = result.GrossR
		return
	}

	result.FeeR = result.TotalFees / result.InitialRisk
	result.RealizedR = result.GrossR - result.FeeR
}

func initializeExcursionAtEntry(result *models.TradeResult) {
	result.HighestAfterFill = result.FillPrice
	result.LowestAfterFill = result.FillPrice
	updateExcursionMetrics(result)
}

func updateRunningExcursion(
	result *models.TradeResult,
	candle models.Candle,
) {
	if result.HighestAfterFill == 0 && result.LowestAfterFill == 0 {
		initializeExcursionAtEntry(result)
	}

	if candle.High > result.HighestAfterFill {
		result.HighestAfterFill = candle.High
	}

	if candle.Low < result.LowestAfterFill {
		result.LowestAfterFill = candle.Low
	}

	updateExcursionMetrics(result)
}

func updateExcursionToExit(
	result *models.TradeResult,
	exitPrice float64,
) {
	if result.HighestAfterFill == 0 && result.LowestAfterFill == 0 {
		initializeExcursionAtEntry(result)
	}

	switch result.Direction {
	case models.BiasLong:
		if exitPrice < result.LowestAfterFill {
			result.LowestAfterFill = exitPrice
		}

	case models.BiasShort:
		if exitPrice > result.HighestAfterFill {
			result.HighestAfterFill = exitPrice
		}
	}

	updateExcursionMetrics(result)
}

func updateExcursionToTarget(
	result *models.TradeResult,
	candle models.Candle,
	target float64,
) {
	if result.HighestAfterFill == 0 && result.LowestAfterFill == 0 {
		initializeExcursionAtEntry(result)
	}

	switch result.Direction {
	case models.BiasLong:
		if candle.Low < result.LowestAfterFill {
			result.LowestAfterFill = candle.Low
		}
		if target > result.HighestAfterFill {
			result.HighestAfterFill = target
		}

	case models.BiasShort:
		if candle.High > result.HighestAfterFill {
			result.HighestAfterFill = candle.High
		}
		if target < result.LowestAfterFill {
			result.LowestAfterFill = target
		}
	}

	updateExcursionMetrics(result)
}

func updateExcursionMetrics(result *models.TradeResult) {
	if result.InitialRisk <= 0 {
		return
	}

	switch result.Direction {
	case models.BiasLong:
		result.MFER = (result.HighestAfterFill - result.FillPrice) / result.InitialRisk

		result.MAER = (result.FillPrice - result.LowestAfterFill) / result.InitialRisk

	case models.BiasShort:
		result.MFER = (result.FillPrice - result.LowestAfterFill) / result.InitialRisk

		result.MAER = (result.HighestAfterFill - result.FillPrice) / result.InitialRisk
	}

	if result.MFER < 0 {
		result.MFER = 0
	}

	if result.MAER < 0 {
		result.MAER = 0
	}
}

func missedEntryDistance(
	direction models.Bias,
	entry float64,
	windowHigh float64,
	windowLow float64,
) float64 {
	switch direction {
	case models.BiasLong:
		if windowLow <= entry {
			return 0
		}
		return windowLow - entry

	case models.BiasShort:
		if windowHigh >= entry {
			return 0
		}
		return entry - windowHigh

	default:
		return 0
	}
}

func executionWindow(
	candles []models.Candle,
	sessionDate time.Time,
	location *time.Location,
) []models.Candle {
	start := time.Date(
		sessionDate.Year(),
		sessionDate.Month(),
		sessionDate.Day(),
		OrderStartHour,
		0,
		0,
		0,
		location,
	)

	end := time.Date(
		sessionDate.Year(),
		sessionDate.Month(),
		sessionDate.Day(),
		OrderEndHour,
		0,
		0,
		0,
		location,
	)

	result := make([]models.Candle, 0)

	for _, candle := range candles {
		local := candle.OpenTime.In(location)

		if local.Before(start) || !local.Before(end) {
			continue
		}

		result = append(result, candle)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].OpenTime.Before(result[j].OpenTime)
	})

	return result
}

func limitTouched(
	direction models.Bias,
	entry float64,
	candle models.Candle,
) bool {
	switch direction {
	case models.BiasLong:
		return candle.Low <= entry
	case models.BiasShort:
		return candle.High >= entry
	default:
		return false
	}
}

func stopTouched(
	direction models.Bias,
	stop float64,
	candle models.Candle,
) bool {
	switch direction {
	case models.BiasLong:
		return candle.Low <= stop
	case models.BiasShort:
		return candle.High >= stop
	default:
		return false
	}
}

func targetTouched(
	direction models.Bias,
	target float64,
	candle models.Candle,
) bool {
	switch direction {
	case models.BiasLong:
		return candle.High >= target
	case models.BiasShort:
		return candle.Low <= target
	default:
		return false
	}
}

func breakevenTouched(
	direction models.Bias,
	entry float64,
	candle models.Candle,
) bool {
	switch direction {
	case models.BiasLong:
		return candle.Low <= entry
	case models.BiasShort:
		return candle.High >= entry
	default:
		return false
	}
}

func directionalR(
	direction models.Bias,
	entry float64,
	exit float64,
	initialRisk float64,
) float64 {
	if initialRisk <= 0 {
		return 0
	}

	switch direction {
	case models.BiasLong:
		return (exit - entry) / initialRisk
	case models.BiasShort:
		return (entry - exit) / initialRisk
	default:
		return 0
	}
}
