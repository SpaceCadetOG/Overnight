package liquidity

import (
	"math"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

func Map(session models.Session, plan models.TradePlan) Context {
	return MapWithMarketData(session, plan, models.TradeResult{}, nil, nil)
}

// MapWithMarketData adds previous-day external levels and only uses execution
// candles strictly before an actual fill when annotating pre-entry sweeps.
func MapWithMarketData(session models.Session, plan models.TradePlan, result models.TradeResult, allCandles []models.Candle, location *time.Location) Context {
	context := Context{Levels: DetectLevels(session.Candles), Sequence: SequenceNone}
	context.Levels = append(context.Levels, previousDayLevels(session.Date, allCandles, location)...)
	preEntry := preEntryCandles(result, allCandles, location)
	context.Event = ClassifyEvent(context.Levels, preEntry)
	context.LiquidityConsumedBeforeEntry = HasConsumedLiquidity(context.Levels, preEntry)
	context.BuySideTaken, context.SellSideTaken, context.InternalTakenBeforeEntry = markFreshPreEntryTakes(context.Levels, preEntry)
	risk := math.Abs(plan.Entry - plan.Stop)
	for i := range context.Levels {
		level := &context.Levels[i]
		if risk > 1e-9 {
			level.DistanceR = math.Abs(level.Price-plan.Entry) / risk
		}
		if level.Price > plan.Entry && (context.NearestAbove == nil || level.Price < context.NearestAbove.Price) {
			copy := *level
			context.NearestAbove = &copy
		}
		if level.Price < plan.Entry && (context.NearestBelow == nil || level.Price > context.NearestBelow.Price) {
			copy := *level
			context.NearestBelow = &copy
		}
		if !level.Taken && level.Price > math.Min(plan.Entry, plan.TP1) && level.Price < math.Max(plan.Entry, plan.TP1) {
			context.ObstacleCount++
		}
	}
	if plan.Direction == models.BiasLong {
		context.OpposingPresent = context.NearestBelow != nil && !context.NearestBelow.Taken
		context.DirectionalTarget = context.NearestAbove != nil
		if context.SellSideTaken && context.DirectionalTarget {
			context.Sequence = SequenceSSLToBSL
		}
	} else if plan.Direction == models.BiasShort {
		context.OpposingPresent = context.NearestAbove != nil && !context.NearestAbove.Taken
		context.DirectionalTarget = context.NearestBelow != nil
		if context.BuySideTaken && context.DirectionalTarget {
			context.Sequence = SequenceBSLToSSL
		}
	}
	context.PathScore = score(context)
	context.ValueLocation = ClassifyValueLocation(plan.Entry, session.VAL, session.VAH, preEntry)
	context.Path = BuildPath(context.Levels, plan)
	context.TargetAvailable = context.Path.TargetLiquidity > 0
	context.TargetAvailability = ClassifyTargetAvailability(context.Levels, plan)
	context.TP1ObstacleCount = CountPathObstacles(context.Levels, session, plan.Entry, plan.TP1)
	context.TP2ObstacleCount = CountPathObstacles(context.Levels, session, plan.Entry, plan.TP2)
	context.ExternalTarget = HasExternalDirectionalTarget(context.Levels, plan)
	context.InternalToExternal = context.InternalTakenBeforeEntry && context.ExternalTarget
	context.EntryLiquidity = ClassifyEntryLiquidity(context.Levels, plan.Entry)
	context.InternalTargetPrice, context.ExternalTargetPrice = FindDirectionalTargets(context.Levels, plan)
	context.ValueTransition = ClassifyValueTransition(context.ValueLocation, plan, session.VAL, session.VAH, session.POC, preEntry, context.Path)
	return context
}

func markFreshPreEntryTakes(levels []Level, candles []models.Candle) (bool, bool, bool) {
	buySide, sellSide, internal := false, false, false
	for i := range levels {
		if levels[i].Taken {
			continue
		}
		for _, candle := range candles {
			taken := levels[i].Side == BuySide && candle.High > levels[i].Price || levels[i].Side == SellSide && candle.Low < levels[i].Price
			if !taken {
				continue
			}
			levels[i].Taken, levels[i].TakenAt = true, candle.OpenTime
			if levels[i].Side == BuySide {
				buySide = true
			} else {
				sellSide = true
			}
			if !levels[i].External {
				internal = true
			}
			break
		}
	}
	return buySide, sellSide, internal
}

func previousDayLevels(sessionDate time.Time, candles []models.Candle, location *time.Location) []Level {
	if location == nil {
		return nil
	}
	day := sessionDate.In(location).AddDate(0, 0, -1)
	var high, low float64
	var highTime, lowTime time.Time
	found := false
	for _, candle := range candles {
		local := candle.OpenTime.In(location)
		if local.Year() != day.Year() || local.YearDay() != day.YearDay() {
			continue
		}
		if !found || candle.High > high {
			high, highTime = candle.High, candle.OpenTime
		}
		if !found || candle.Low < low {
			low, lowTime = candle.Low, candle.OpenTime
		}
		found = true
	}
	if !found {
		return nil
	}
	return []Level{
		{Kind: PreviousHigh, Side: BuySide, Price: high, FormedAt: highTime, LastTime: highTime, Touches: 1, External: true, Strength: 1},
		{Kind: PreviousLow, Side: SellSide, Price: low, FormedAt: lowTime, LastTime: lowTime, Touches: 1, External: true, Strength: 1},
	}
}

func preEntryCandles(result models.TradeResult, candles []models.Candle, location *time.Location) []models.Candle {
	if !result.Filled || result.FillTime.IsZero() || location == nil {
		return nil
	}
	var selected []models.Candle
	for _, candle := range candles {
		local := candle.OpenTime.In(location)
		if !local.Before(result.Date.In(location)) && local.Before(result.FillTime.In(location)) {
			selected = append(selected, candle)
		}
	}
	return selected
}
