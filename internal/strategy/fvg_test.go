package strategy

import (
	"testing"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

func testFVGCandle(
	index int,
	open float64,
	high float64,
	low float64,
	closePrice float64,
) models.Candle {
	openTime := time.Date(
		2026,
		time.January,
		1,
		0,
		index*5,
		0,
		0,
		time.UTC,
	)

	return models.Candle{
		OpenTime:  openTime,
		CloseTime: openTime.Add(5 * time.Minute),
		Open:      open,
		High:      high,
		Low:       low,
		Close:     closePrice,
		Volume:    100,
	}
}

func TestFindBullishFairValueGap(t *testing.T) {
	candles := []models.Candle{
		testFVGCandle(0, 100, 101, 99, 100),
		testFVGCandle(1, 101, 104, 100, 103),
		testFVGCandle(2, 105, 107, 103, 106),
	}

	gaps := FindFairValueGaps(candles)

	if len(gaps) != 1 {
		t.Fatalf("expected 1 FVG, got %d", len(gaps))
	}

	gap := gaps[0]

	if gap.Direction != models.BiasLong {
		t.Fatalf(
			"expected LONG direction, got %s",
			gap.Direction,
		)
	}

	if gap.Lower != 101 {
		t.Fatalf(
			"expected lower boundary 101, got %.2f",
			gap.Lower,
		)
	}

	if gap.Upper != 103 {
		t.Fatalf(
			"expected upper boundary 103, got %.2f",
			gap.Upper,
		)
	}

	if gap.FirstIndex != 0 || gap.ThirdIndex != 2 {
		t.Fatalf(
			"expected formation indices 0-2, got %d-%d",
			gap.FirstIndex,
			gap.ThirdIndex,
		)
	}

	if gap.Filled {
		t.Fatal("expected new bullish FVG to be unfilled")
	}
}

func TestFindBearishFairValueGap(t *testing.T) {
	candles := []models.Candle{
		testFVGCandle(0, 110, 112, 109, 111),
		testFVGCandle(1, 108, 110, 106, 107),
		testFVGCandle(2, 104, 108, 102, 103),
	}

	gaps := FindFairValueGaps(candles)

	if len(gaps) != 1 {
		t.Fatalf("expected 1 FVG, got %d", len(gaps))
	}

	gap := gaps[0]

	if gap.Direction != models.BiasShort {
		t.Fatalf(
			"expected SHORT direction, got %s",
			gap.Direction,
		)
	}

	if gap.Lower != 108 {
		t.Fatalf(
			"expected lower boundary 108, got %.2f",
			gap.Lower,
		)
	}

	if gap.Upper != 109 {
		t.Fatalf(
			"expected upper boundary 109, got %.2f",
			gap.Upper,
		)
	}

	if gap.Filled {
		t.Fatal("expected new bearish FVG to be unfilled")
	}
}

func TestBullishFVGTracksMitigationCloses(t *testing.T) {
	candles := []models.Candle{
		testFVGCandle(0, 100, 101, 99, 100),
		testFVGCandle(1, 101, 104, 100, 103),
		testFVGCandle(2, 105, 107, 103, 106),

		// Range overlaps the 101-103 gap.
		testFVGCandle(3, 104, 104, 102, 102.50),

		// Range also overlaps the gap.
		testFVGCandle(4, 103, 103.50, 101.50, 101.75),
	}

	gaps := FindFairValueGaps(candles)

	if len(gaps) == 0 {
		t.Fatal("expected bullish FVG")
	}

	gap := gaps[0]

	if !gap.HasCloseObservation {
		t.Fatal("expected close observations inside the FVG")
	}

	if gap.LowestClose != 101.75 {
		t.Fatalf(
			"expected lowest close 101.75, got %.2f",
			gap.LowestClose,
		)
	}

	if gap.HighestClose != 102.50 {
		t.Fatalf(
			"expected highest close 102.50, got %.2f",
			gap.HighestClose,
		)
	}

	if gap.Filled {
		t.Fatal("expected partially mitigated FVG to remain active")
	}
}

func TestBullishFVGBecomesFilled(t *testing.T) {
	candles := []models.Candle{
		testFVGCandle(0, 100, 101, 99, 100),
		testFVGCandle(1, 101, 104, 100, 103),
		testFVGCandle(2, 105, 107, 103, 106),

		// Bullish lower boundary is 101.
		testFVGCandle(3, 103, 104, 100.75, 101.25),
	}

	gaps := FindFairValueGaps(candles)

	if len(gaps) == 0 {
		t.Fatal("expected bullish FVG")
	}

	gap := gaps[0]

	if !gap.Filled {
		t.Fatal("expected bullish FVG to be filled")
	}

	if gap.FillIndex != 3 {
		t.Fatalf(
			"expected fill index 3, got %d",
			gap.FillIndex,
		)
	}
}

func TestBearishFVGBecomesFilled(t *testing.T) {
	candles := []models.Candle{
		testFVGCandle(0, 110, 112, 109, 111),
		testFVGCandle(1, 108, 110, 106, 107),
		testFVGCandle(2, 104, 108, 102, 103),

		// Bearish upper boundary is 109.
		testFVGCandle(3, 107, 109.25, 106, 108.50),
	}

	gaps := FindFairValueGaps(candles)

	if len(gaps) == 0 {
		t.Fatal("expected bearish FVG")
	}

	gap := gaps[0]

	if !gap.Filled {
		t.Fatal("expected bearish FVG to be filled")
	}

	if gap.FillIndex != 3 {
		t.Fatalf(
			"expected fill index 3, got %d",
			gap.FillIndex,
		)
	}
}

func TestMostRecentActiveBullishFVGIsSelected(t *testing.T) {
	candles := []models.Candle{
		// First bullish FVG: 101-103.
		testFVGCandle(0, 100, 101, 99, 100),
		testFVGCandle(1, 101, 104, 100, 103),
		testFVGCandle(2, 105, 107, 103, 106),

		testFVGCandle(3, 106, 108, 105, 107),

		// Second bullish FVG: 108-110.
		testFVGCandle(4, 108, 109, 107, 108),
		testFVGCandle(5, 109, 112, 108, 111),
		testFVGCandle(6, 111, 114, 110, 113),
	}

	gap, found := FindActiveBullishFVG(
		candles,
		len(candles)-1,
	)

	if !found {
		t.Fatal("expected an active bullish FVG")
	}

	if gap.FirstIndex != 4 || gap.ThirdIndex != 6 {
		t.Fatalf(
			"expected most recent formation at indices 4-6, got %d-%d",
			gap.FirstIndex,
			gap.ThirdIndex,
		)
	}

	if gap.Lower != 109 || gap.Upper != 110 {
		t.Fatalf(
			"expected boundaries 109-110, got %.2f-%.2f",
			gap.Lower,
			gap.Upper,
		)
	}
}

func TestFilledGapIsExcludedFromActiveSelection(t *testing.T) {
	candles := []models.Candle{
		testFVGCandle(0, 100, 101, 99, 100),
		testFVGCandle(1, 101, 104, 100, 103),
		testFVGCandle(2, 105, 107, 103, 106),

		// Fully fills the 101-103 bullish FVG.
		testFVGCandle(3, 103, 104, 100.50, 101),
	}

	_, found := FindActiveBullishFVG(
		candles,
		len(candles)-1,
	)

	if found {
		t.Fatal("expected filled bullish FVG to be excluded")
	}
}

func TestActiveSelectionHonorsBeforeIndex(t *testing.T) {
	candles := []models.Candle{
		testFVGCandle(0, 100, 101, 99, 100),
		testFVGCandle(1, 101, 104, 100, 103),
		testFVGCandle(2, 105, 107, 103, 106),

		// This later candle fills the bullish FVG.
		testFVGCandle(3, 103, 104, 100.50, 101),
	}

	_, activeBeforeFill := FindActiveBullishFVG(
		candles,
		2,
	)

	if !activeBeforeFill {
		t.Fatal("expected FVG to be active before fill candle")
	}

	_, activeAfterFill := FindActiveBullishFVG(
		candles,
		3,
	)

	if activeAfterFill {
		t.Fatal("expected FVG to be inactive after fill candle")
	}
}

func TestNoFVGIsDetectedWhenRangesOverlap(t *testing.T) {
	candles := []models.Candle{
		testFVGCandle(0, 100, 103, 99, 102),
		testFVGCandle(1, 102, 104, 101, 103),
		testFVGCandle(2, 103, 105, 102, 104),
	}

	gaps := FindFairValueGaps(candles)

	if len(gaps) != 0 {
		t.Fatalf(
			"expected no FVG, got %d",
			len(gaps),
		)
	}
}
