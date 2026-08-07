package strategy

import (
	"math"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

// FairValueGap represents a classic three-candle price imbalance.
//
// Bullish:
// Candle 1 High < Candle 3 Low
//
// Bearish:
// Candle 1 Low > Candle 3 High
type FairValueGap struct {
	Direction models.Bias

	// FirstIndex, MiddleIndex, and ThirdIndex identify the three candles
	// that created the gap.
	FirstIndex  int
	MiddleIndex int
	ThirdIndex  int

	// Lower and Upper are the price boundaries of the gap.
	Lower float64
	Upper float64

	// Filled is true when price fully trades through the opposite boundary.
	Filled bool

	// FillIndex is the candle that fully filled the gap.
	// It remains -1 while the gap is unfilled.
	FillIndex int

	// LowestClose and HighestClose track closes from later candles whose
	// ranges overlap the FVG.
	LowestClose  float64
	HighestClose float64

	// HasCloseObservation indicates that at least one candle after the
	// three-candle formation traded into the gap.
	HasCloseObservation bool
}

// FindFairValueGaps detects every bullish and bearish FVG in candles and
// evaluates each gap through the final candle in the slice.
func FindFairValueGaps(candles []models.Candle) []FairValueGap {
	return FindFairValueGapsThrough(candles, len(candles)-1)
}

// FindFairValueGapsThrough detects FVGs and evaluates mitigation only through
// throughIndex, inclusive.
//
// This allows future planner logic to inspect only information that existed
// before a planned entry and prevents look-ahead bias.
func FindFairValueGapsThrough(
	candles []models.Candle,
	throughIndex int,
) []FairValueGap {
	if len(candles) < 3 || throughIndex < 2 {
		return nil
	}

	if throughIndex >= len(candles) {
		throughIndex = len(candles) - 1
	}

	gaps := make([]FairValueGap, 0)

	for thirdIndex := 2; thirdIndex <= throughIndex; thirdIndex++ {
		firstIndex := thirdIndex - 2

		first := candles[firstIndex]
		third := candles[thirdIndex]

		if first.High < third.Low {
			gap := newFairValueGap(
				models.BiasLong,
				firstIndex,
				thirdIndex,
				first.High,
				third.Low,
			)

			evaluateFairValueGap(
				&gap,
				candles,
				throughIndex,
			)

			gaps = append(gaps, gap)
		}

		if first.Low > third.High {
			gap := newFairValueGap(
				models.BiasShort,
				firstIndex,
				thirdIndex,
				third.High,
				first.Low,
			)

			evaluateFairValueGap(
				&gap,
				candles,
				throughIndex,
			)

			gaps = append(gaps, gap)
		}
	}

	return gaps
}

func newFairValueGap(
	direction models.Bias,
	firstIndex int,
	thirdIndex int,
	lower float64,
	upper float64,
) FairValueGap {
	return FairValueGap{
		Direction:   direction,
		FirstIndex:  firstIndex,
		MiddleIndex: firstIndex + 1,
		ThirdIndex:  thirdIndex,
		Lower:       lower,
		Upper:       upper,
		FillIndex:   -1,
	}
}

// evaluateFairValueGap tracks mitigation and candle closes beginning with the
// first candle after the FVG formation.
//
// A bullish FVG is fully filled when a later candle low reaches Lower.
//
// A bearish FVG is fully filled when a later candle high reaches Upper.
func evaluateFairValueGap(
	gap *FairValueGap,
	candles []models.Candle,
	throughIndex int,
) {
	lowestClose := math.Inf(1)
	highestClose := math.Inf(-1)

	for index := gap.ThirdIndex + 1; index <= throughIndex; index++ {
		candle := candles[index]

		if candleOverlapsFVG(candle, *gap) {
			gap.HasCloseObservation = true

			if candle.Close < lowestClose {
				lowestClose = candle.Close
			}

			if candle.Close > highestClose {
				highestClose = candle.Close
			}
		}

		if gap.Filled {
			continue
		}

		switch gap.Direction {
		case models.BiasLong:
			if candle.Low <= gap.Lower {
				gap.Filled = true
				gap.FillIndex = index
			}

		case models.BiasShort:
			if candle.High >= gap.Upper {
				gap.Filled = true
				gap.FillIndex = index
			}
		}
	}

	if gap.HasCloseObservation {
		gap.LowestClose = lowestClose
		gap.HighestClose = highestClose
	}
}

func candleOverlapsFVG(
	candle models.Candle,
	gap FairValueGap,
) bool {
	return candle.High >= gap.Lower &&
		candle.Low <= gap.Upper
}

// FindMostRecentActiveFVG returns the newest direction-compatible FVG that
// remained unfilled through beforeIndex.
//
// beforeIndex is inclusive. Passing a value beyond the candle slice uses the
// final candle.
func FindMostRecentActiveFVG(
	candles []models.Candle,
	direction models.Bias,
	beforeIndex int,
) (FairValueGap, bool) {
	gaps := FindFairValueGapsThrough(
		candles,
		beforeIndex,
	)

	for index := len(gaps) - 1; index >= 0; index-- {
		gap := gaps[index]

		if gap.Direction == direction && !gap.Filled {
			return gap, true
		}
	}

	return FairValueGap{}, false
}

// FindActiveBullishFVG returns the most recent unfilled bullish FVG.
func FindActiveBullishFVG(
	candles []models.Candle,
	beforeIndex int,
) (FairValueGap, bool) {
	return FindMostRecentActiveFVG(
		candles,
		models.BiasLong,
		beforeIndex,
	)
}

// FindActiveBearishFVG returns the most recent unfilled bearish FVG.
func FindActiveBearishFVG(
	candles []models.Candle,
	beforeIndex int,
) (FairValueGap, bool) {
	return FindMostRecentActiveFVG(
		candles,
		models.BiasShort,
		beforeIndex,
	)
}
