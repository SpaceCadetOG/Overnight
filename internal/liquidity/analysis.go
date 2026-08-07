package liquidity

import (
	"math"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

func ClassifyValueLocation(entry, val, vah float64, preEntry []models.Candle) ValueLocationState {
	if entry > vah {
		return AboveValue
	}
	if entry >= val {
		return InsideValue
	}
	tradedBelow := false
	for _, candle := range preEntry {
		if candle.Low < val {
			tradedBelow = true
		}
		if tradedBelow && candle.Close >= val {
			return BelowValueSweepReclaim
		}
	}
	return BelowValueAcceptance
}

func BuildPath(levels []Level, plan models.TradePlan) LiquidityPath {
	path := LiquidityPath{CleanPath: true}
	targetDistance, opposingDistance := math.Inf(1), math.Inf(1)
	low, high := math.Min(plan.Entry, plan.TP1), math.Max(plan.Entry, plan.TP1)
	for _, level := range levels {
		if !isV2PathLevel(level.Kind) || level.Taken {
			continue
		}
		distance := math.Abs(level.Price - plan.Entry)
		if plan.Direction == models.BiasLong {
			if level.Price > plan.Entry && distance < targetDistance {
				path.TargetLiquidity, targetDistance = level.Price, distance
			}
			if level.Price < plan.Entry && distance < opposingDistance {
				path.OpposingLiquidity, opposingDistance = level.Price, distance
			}
		} else if plan.Direction == models.BiasShort {
			if level.Price < plan.Entry && distance < targetDistance {
				path.TargetLiquidity, targetDistance = level.Price, distance
			}
			if level.Price > plan.Entry && distance < opposingDistance {
				path.OpposingLiquidity, opposingDistance = level.Price, distance
			}
		}
		if level.Price > low && level.Price < high {
			path.CleanPath = false
		}
	}
	if !math.IsInf(targetDistance, 1) {
		path.TargetDistance = targetDistance
	}
	if !math.IsInf(opposingDistance, 1) {
		path.OpposingDistance = opposingDistance
	}
	return path
}

func ClassifyValueTransition(location ValueLocationState, plan models.TradePlan, val, vah, poc float64, preEntry []models.Candle, path LiquidityPath) ValueTransitionState {
	if location == BelowValueAcceptance {
		return ValueAcceptance
	}
	if location == BelowValueSweepReclaim {
		return ValueRejection
	}
	if location == AboveValue {
		for _, candle := range preEntry {
			if candle.Close >= val && candle.Close <= vah {
				return ValueRejection
			}
		}
		return ValueAcceptance
	}
	low, high := math.Min(plan.Entry, plan.TP1), math.Max(plan.Entry, plan.TP1)
	if poc > low && poc < high {
		return ValueRotation
	}
	return ValueContinuation
}

func isV2PathLevel(kind Kind) bool {
	return kind == SwingHigh || kind == SwingLow || kind == EqualHigh || kind == EqualLow || kind == SessionHigh || kind == SessionLow
}

func CountPathObstacles(levels []Level, session models.Session, entry, target float64) int {
	low, high := math.Min(entry, target), math.Max(entry, target)
	prices := []float64{session.POC, session.VAH, session.VAL, session.VWAP}
	for _, level := range levels {
		if isV2PathLevel(level.Kind) && !level.Taken {
			prices = append(prices, level.Price)
		}
	}
	unique := make([]float64, 0, len(prices))
	for _, price := range prices {
		if price <= low || price >= high {
			continue
		}
		duplicate := false
		for _, existing := range unique {
			if equalPrice(existing, price) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			unique = append(unique, price)
		}
	}
	return len(unique)
}

func HasExternalDirectionalTarget(levels []Level, plan models.TradePlan) bool {
	best := math.Inf(1)
	external := false
	for _, level := range levels {
		if level.Taken || !isV2PathLevel(level.Kind) {
			continue
		}
		inDirection := plan.Direction == models.BiasLong && level.Price > plan.Entry || plan.Direction == models.BiasShort && level.Price < plan.Entry
		if !inDirection {
			continue
		}
		distance := math.Abs(level.Price - plan.Entry)
		if distance < best {
			best, external = distance, level.External
		}
	}
	return !math.IsInf(best, 1) && external
}

func ClassifyTargetAvailability(levels []Level, plan models.TradePlan) TargetAvailabilityState {
	foundConsumed := false
	for _, level := range levels {
		if !isV2PathLevel(level.Kind) {
			continue
		}
		inDirection := plan.Direction == models.BiasLong && level.Price > plan.Entry || plan.Direction == models.BiasShort && level.Price < plan.Entry
		if !inDirection {
			continue
		}
		if !level.Taken {
			return TargetAvailableState
		}
		foundConsumed = true
	}
	if foundConsumed {
		return TargetConsumedState
	}
	return TargetAbsentState
}

func ClassifyEntryLiquidity(levels []Level, entry float64) EntryLiquidityType {
	best := math.Inf(1)
	result := EntryLiquidityNone
	for _, level := range levels {
		if level.Taken || !isV2PathLevel(level.Kind) {
			continue
		}
		if level.DistanceR <= 0 || level.DistanceR > EntryLiquidityMaxDistanceR {
			continue
		}
		distance := math.Abs(level.Price - entry)
		if distance >= best {
			continue
		}
		best = distance
		if level.Side == SellSide {
			if level.External {
				result = EntryExternalSSL
			} else {
				result = EntryInternalSSL
			}
		} else {
			if level.External {
				result = EntryExternalBSL
			} else {
				result = EntryInternalBSL
			}
		}
	}
	return result
}

func FindDirectionalTargets(levels []Level, plan models.TradePlan) (float64, float64) {
	internalDistance, externalDistance := math.Inf(1), math.Inf(1)
	var internalPrice, externalPrice float64
	for _, level := range levels {
		if level.Taken || !isV2PathLevel(level.Kind) {
			continue
		}
		inDirection := plan.Direction == models.BiasLong && level.Price > plan.Entry || plan.Direction == models.BiasShort && level.Price < plan.Entry
		if !inDirection {
			continue
		}
		distance := math.Abs(level.Price - plan.Entry)
		if level.External && distance < externalDistance {
			externalDistance, externalPrice = distance, level.Price
		}
		if !level.External && distance < internalDistance {
			internalDistance, internalPrice = distance, level.Price
		}
	}
	return internalPrice, externalPrice
}
