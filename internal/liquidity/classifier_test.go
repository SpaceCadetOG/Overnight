package liquidity

import (
	"testing"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

func TestClassifyLiquidityEvents(t *testing.T) {
	level := []Level{{Side: BuySide, Price: 10}}
	grab := []models.Candle{candleWithClose(0, 11, 9, 9.5)}
	if got := ClassifyEvent(level, grab); got != EventGrab {
		t.Fatalf("grab=%s", got)
	}
	sweep := []models.Candle{candleWithClose(0, 11, 9, 10.5), candleWithClose(1, 10, 9, 9.5)}
	if got := ClassifyEvent(level, sweep); got != EventSweep {
		t.Fatalf("sweep=%s", got)
	}
	run := []models.Candle{candleWithClose(0, 11, 9, 10.5), candleWithClose(1, 12, 10, 11)}
	if got := ClassifyEvent(level, run); got != EventRun {
		t.Fatalf("run=%s", got)
	}
}

func candleWithClose(minute int, high, low, close float64) models.Candle {
	start := time.Date(2026, 1, 1, 0, minute, 0, 0, time.UTC)
	return models.Candle{OpenTime: start, CloseTime: start.Add(5 * time.Minute), Open: close, High: high, Low: low, Close: close, Volume: 1}
}

func TestPathScoreRewardsCleanDirectionalSequence(t *testing.T) {
	if got := score(Context{Sequence: SequenceSSLToBSL, DirectionalTarget: true}); got != 10 {
		t.Fatalf("score=%d", got)
	}
	if got := score(Context{OpposingPresent: true, ObstacleCount: 2}); got != 0 {
		t.Fatalf("score=%d", got)
	}
}

func TestHasConsumedLiquidityIsNotHiddenByAnotherRun(t *testing.T) {
	levels := []Level{{Side: BuySide, Price: 10}, {Side: BuySide, Price: 12}}
	candles := []models.Candle{
		candleWithClose(0, 11, 9, 10.5),
		candleWithClose(1, 13, 10, 11),
	}
	if !HasConsumedLiquidity(levels, candles) {
		t.Fatal("expected rejected 12 level to count as consumed")
	}
}
