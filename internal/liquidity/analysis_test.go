package liquidity

import (
	"testing"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

func TestClassifyValueLocation(t *testing.T) {
	if got := ClassifyValueLocation(11, 8, 10, nil); got != AboveValue {
		t.Fatalf("above=%s", got)
	}
	if got := ClassifyValueLocation(9, 8, 10, nil); got != InsideValue {
		t.Fatalf("inside=%s", got)
	}
	if got := ClassifyValueLocation(7, 8, 10, nil); got != BelowValueAcceptance {
		t.Fatalf("acceptance=%s", got)
	}
	reclaim := []models.Candle{candleWithClose(0, 8.5, 7, 8.25)}
	if got := ClassifyValueLocation(7, 8, 10, reclaim); got != BelowValueSweepReclaim {
		t.Fatalf("reclaim=%s", got)
	}
}

func TestClassifyEntryLiquidityRequiresLevelWithinOneR(t *testing.T) {
	levels := []Level{{Kind: SwingLow, Side: SellSide, Price: 90, DistanceR: 2}}
	if got := ClassifyEntryLiquidity(levels, 100); got != EntryLiquidityNone {
		t.Fatalf("ClassifyEntryLiquidity = %s, want %s", got, EntryLiquidityNone)
	}

	levels[0].DistanceR = .75
	if got := ClassifyEntryLiquidity(levels, 100); got != EntryInternalSSL {
		t.Fatalf("ClassifyEntryLiquidity = %s, want %s", got, EntryInternalSSL)
	}
}

func TestBuildPathUsesStructuralLevelsOnly(t *testing.T) {
	levels := []Level{
		{Kind: SwingHigh, Price: 11}, {Kind: EqualHigh, Price: 12},
		{Kind: SwingLow, Price: 8}, {Kind: PreviousHigh, Price: 10.5},
	}
	path := BuildPath(levels, models.TradePlan{Direction: models.BiasLong, Entry: 9, TP1: 11.5})
	if path.TargetLiquidity != 11 || path.OpposingLiquidity != 8 {
		t.Fatalf("path=%+v", path)
	}
	if path.CleanPath {
		t.Fatal("expected swing high before TP1 to obstruct path")
	}
}

func TestClassifyValueTransition(t *testing.T) {
	plan := models.TradePlan{Entry: 7, TP1: 11}
	if got := ClassifyValueTransition(BelowValueAcceptance, plan, 8, 10, 9, nil, LiquidityPath{}); got != ValueAcceptance {
		t.Fatalf("acceptance=%s", got)
	}
	reclaim := []models.Candle{candleWithClose(0, 9, 7, 8.5)}
	if got := ClassifyValueTransition(BelowValueSweepReclaim, plan, 8, 10, 9, reclaim, LiquidityPath{}); got != ValueRejection {
		t.Fatalf("rejection=%s", got)
	}
	plan.Entry = 8.5
	if got := ClassifyValueTransition(InsideValue, plan, 8, 10, 9, nil, LiquidityPath{}); got != ValueRotation {
		t.Fatalf("rotation=%s", got)
	}
	plan.TP1 = 8.75
	if got := ClassifyValueTransition(InsideValue, plan, 8, 10, 9, nil, LiquidityPath{}); got != ValueContinuation {
		t.Fatalf("continuation=%s", got)
	}
}

func TestEqualPoolStoresClusterMetadata(t *testing.T) {
	pivots := []Level{
		{Kind: SwingHigh, Side: BuySide, Price: 100, FormedAt: candle(0, 1, 0).OpenTime},
		{Kind: SwingHigh, Side: BuySide, Price: 100.1, FormedAt: candle(1, 1, 0).OpenTime},
		{Kind: SwingHigh, Side: BuySide, Price: 99.95, FormedAt: candle(2, 1, 0).OpenTime},
	}
	pools := equalPools(pivots)
	if len(pools) != 1 || pools[0].Touches != 3 || pools[0].Strength != 3 {
		t.Fatalf("pools=%+v", pools)
	}
	if !pools[0].LastTime.After(pools[0].FormedAt) {
		t.Fatalf("times=%+v", pools[0])
	}
}

func TestClassifyTargetAvailability(t *testing.T) {
	plan := models.TradePlan{Direction: models.BiasLong, Entry: 100}
	if got := ClassifyTargetAvailability([]Level{{Kind: SwingHigh, Price: 110}}, plan); got != TargetAvailableState {
		t.Fatalf("available=%s", got)
	}
	if got := ClassifyTargetAvailability([]Level{{Kind: SwingHigh, Price: 110, Taken: true}}, plan); got != TargetConsumedState {
		t.Fatalf("consumed=%s", got)
	}
	if got := ClassifyTargetAvailability([]Level{{Kind: SwingLow, Price: 90}}, plan); got != TargetAbsentState {
		t.Fatalf("absent=%s", got)
	}
}
