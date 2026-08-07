package liquidity

import (
	"math"
	"sort"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

const EqualToleranceFraction = 0.0015

// DetectLevels derives confirmed three-candle pivots and equal-high/low pools.
// A pivot is confirmed by the candle immediately following it.
func DetectLevels(candles []models.Candle) []Level {
	if len(candles) == 0 {
		return nil
	}
	levels := []Level{
		{Kind: SessionHigh, Side: BuySide, Price: candles[0].High, FormedAt: candles[0].OpenTime, LastTime: candles[0].OpenTime, Touches: 1, External: true, Strength: 1},
		{Kind: SessionLow, Side: SellSide, Price: candles[0].Low, FormedAt: candles[0].OpenTime, LastTime: candles[0].OpenTime, Touches: 1, External: true, Strength: 1},
	}
	for _, candle := range candles[1:] {
		if candle.High > levels[0].Price {
			levels[0].Price, levels[0].FormedAt, levels[0].LastTime = candle.High, candle.OpenTime, candle.OpenTime
		}
		if candle.Low < levels[1].Price {
			levels[1].Price, levels[1].FormedAt, levels[1].LastTime = candle.Low, candle.OpenTime, candle.OpenTime
		}
	}

	pivots := make([]Level, 0)
	for i := 1; i+1 < len(candles); i++ {
		previous, current, next := candles[i-1], candles[i], candles[i+1]
		if current.High > previous.High && current.High >= next.High {
			pivots = append(pivots, Level{Kind: SwingHigh, Side: BuySide, Price: current.High, FormedAt: current.OpenTime, LastTime: current.OpenTime, Touches: 1, Strength: 1})
		}
		if current.Low < previous.Low && current.Low <= next.Low {
			pivots = append(pivots, Level{Kind: SwingLow, Side: SellSide, Price: current.Low, FormedAt: current.OpenTime, LastTime: current.OpenTime, Touches: 1, Strength: 1})
		}
	}
	levels = append(levels, pivots...)
	levels = append(levels, equalPools(pivots)...)
	markTaken(levels, candles)
	sort.SliceStable(levels, func(i, j int) bool { return levels[i].FormedAt.Before(levels[j].FormedAt) })
	return levels
}

func equalPools(pivots []Level) []Level {
	var pools []Level
	used := make([]bool, len(pivots))
	for i := range pivots {
		if used[i] {
			continue
		}
		members := []int{i}
		for j := i + 1; j < len(pivots); j++ {
			if pivots[i].Side != pivots[j].Side || !equalPrice(pivots[i].Price, pivots[j].Price) {
				continue
			}
			members = append(members, j)
		}
		if len(members) < 2 {
			continue
		}
		kind, total := EqualHigh, 0.0
		if pivots[i].Side == SellSide {
			kind = EqualLow
		}
		first, last := pivots[members[0]].FormedAt, pivots[members[0]].FormedAt
		for _, index := range members {
			used[index] = true
			total += pivots[index].Price
			if pivots[index].FormedAt.Before(first) {
				first = pivots[index].FormedAt
			}
			if pivots[index].FormedAt.After(last) {
				last = pivots[index].FormedAt
			}
		}
		pools = append(pools, Level{Kind: kind, Side: pivots[i].Side, Price: total / float64(len(members)), FormedAt: first, LastTime: last, Touches: len(members), Strength: len(members)})
	}
	return pools
}

func equalPrice(a, b float64) bool {
	scale := math.Max(math.Abs(a), math.Abs(b))
	return scale > 0 && math.Abs(a-b) <= scale*EqualToleranceFraction
}

func markTaken(levels []Level, candles []models.Candle) {
	for i := range levels {
		for _, candle := range candles {
			if !candle.OpenTime.After(levels[i].FormedAt) {
				continue
			}
			taken := levels[i].Side == BuySide && candle.High > levels[i].Price || levels[i].Side == SellSide && candle.Low < levels[i].Price
			if taken {
				levels[i].Taken, levels[i].TakenAt = true, candle.OpenTime
				break
			}
		}
	}
}
