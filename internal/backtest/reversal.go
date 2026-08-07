package backtest

import (
	"fmt"
	"math"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

// ReversalPlan is a research-only market re-entry after a baseline stop.
type ReversalPlan struct {
	Date      time.Time
	Direction models.Bias
	EntryTime time.Time
	Entry     float64
	Stop      float64
	Target    float64
	Valid     bool
}

// SimulateReversalWithConfig replays a confirmed stop-reclaim entry without
// modifying or replacing the original baseline result.
func SimulateReversalWithConfig(plan ReversalPlan, candles []models.Candle, location *time.Location, execution ExecutionConfig) (models.TradeResult, error) {
	if location == nil {
		return models.TradeResult{}, fmt.Errorf("location is required")
	}
	if err := execution.Validate(); err != nil {
		return models.TradeResult{}, err
	}
	result := models.TradeResult{Date: plan.Date, Direction: plan.Direction, Outcome: models.OutcomeInvalid}
	if !plan.Valid {
		return result, nil
	}

	entry := adversePrice(plan.Entry, plan.Direction, execution.EntrySlippageBps, true)
	risk := math.Abs(entry - plan.Stop)
	if risk <= 1e-9 {
		return models.TradeResult{}, fmt.Errorf("invalid reversal risk")
	}
	result = models.TradeResult{
		Date:             plan.Date,
		Direction:        plan.Direction,
		Outcome:          models.OutcomeTimeExit,
		Entry:            plan.Entry,
		EntrySource:      "STOP_SWEEP_RECLAIM_CLOSE",
		Stop:             plan.Stop,
		StopSource:       "SWEEP_EXTREME",
		TP1:              plan.Target,
		TP1Source:        "LIQUIDITY_PRIORITY_TARGET",
		TP2:              plan.Target,
		FillPrice:        entry,
		Filled:           true,
		FillTime:         plan.EntryTime,
		InitialRisk:      risk,
		HighestAfterFill: entry,
		LowestAfterFill:  entry,
	}
	addFee(&result, entry, 1, execution.TakerFeeBps)

	var lastClose float64
	var lastTime time.Time
	for _, candle := range executionWindow(candles, plan.Date, location) {
		if !candle.OpenTime.In(location).After(plan.EntryTime) {
			continue
		}
		lastClose = candle.Close
		lastTime = candle.CloseTime.In(location)
		if stopTouched(plan.Direction, plan.Stop, candle) {
			exit := adversePrice(plan.Stop, plan.Direction, execution.StopSlippageBps, false)
			updateExcursionToExit(&result, exit)
			result.Outcome = models.OutcomeStopped
			result.ExitPrice, result.ExitTime = exit, candle.CloseTime.In(location)
			result.GrossR = directionalR(plan.Direction, entry, exit, risk)
			addFee(&result, exit, 1, execution.TakerFeeBps)
			finalizeNetR(&result)
			result.MinutesInTrade = int(result.ExitTime.Sub(result.FillTime).Minutes())
			return result, nil
		}
		if targetTouched(plan.Direction, plan.Target, candle) {
			exit := adversePrice(plan.Target, plan.Direction, execution.TPSlippageBps, false)
			updateExcursionToTarget(&result, candle, exit)
			result.Outcome = models.OutcomeTP2
			result.ExitPrice, result.ExitTime = exit, candle.CloseTime.In(location)
			result.GrossR = directionalR(plan.Direction, entry, exit, risk)
			addFee(&result, exit, 1, execution.MakerFeeBps)
			finalizeNetR(&result)
			result.MinutesInTrade = int(result.ExitTime.Sub(result.FillTime).Minutes())
			return result, nil
		}
		updateRunningExcursion(&result, candle)
	}

	if lastTime.IsZero() {
		result.Outcome = models.OutcomeInvalid
		result.Filled = false
		return result, nil
	}
	exit := adversePrice(lastClose, plan.Direction, execution.TimeSlippageBps, false)
	updateExcursionToExit(&result, exit)
	result.ExitPrice, result.ExitTime = exit, lastTime
	result.GrossR = directionalR(plan.Direction, entry, exit, risk)
	addFee(&result, exit, 1, execution.TakerFeeBps)
	finalizeNetR(&result)
	result.MinutesInTrade = int(result.ExitTime.Sub(result.FillTime).Minutes())
	return result, nil
}
