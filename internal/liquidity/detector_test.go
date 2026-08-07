package liquidity

import (
	"testing"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

func TestDetectLevelsFindsPivotsEqualHighsAndSweeps(t *testing.T) {
	candles := []models.Candle{
		candle(0, 10, 8), candle(1, 12, 9), candle(2, 11, 7),
		candle(3, 12.004, 8), candle(4, 11, 7.5), candle(5, 13, 6),
	}
	levels := DetectLevels(candles)
	assertLevel(t, levels, SwingHigh, 12, true)
	assertLevel(t, levels, SwingLow, 7, true)
	assertLevel(t, levels, EqualHigh, 12.002, true)
}

func TestMapBuildsDirectionAwareContext(t *testing.T) {
	session := models.Session{Candles: []models.Candle{
		candle(0, 10, 8), candle(1, 12, 9), candle(2, 11, 7), candle(3, 10, 8), candle(4, 13, 6),
	}}
	context := Map(session, models.TradePlan{Direction: models.BiasLong, Entry: 9, Stop: 8, TP1: 11, TP2: 12})
	if context.Sequence != SequenceNone {
		t.Fatalf("sequence=%s", context.Sequence)
	}
	if context.NearestAbove == nil || context.NearestBelow == nil {
		t.Fatal("expected liquidity on both sides")
	}
	if context.PathScore < 3 {
		t.Fatalf("score=%d", context.PathScore)
	}
}

func candle(minute int, high, low float64) models.Candle {
	start := time.Date(2026, 1, 1, 0, minute, 0, 0, time.UTC)
	return models.Candle{OpenTime: start, CloseTime: start.Add(5 * time.Minute), Open: (high + low) / 2, High: high, Low: low, Close: (high + low) / 2, Volume: 1}
}

func assertLevel(t *testing.T, levels []Level, kind Kind, price float64, taken bool) {
	t.Helper()
	for _, level := range levels {
		if level.Kind == kind && equalPrice(level.Price, price) {
			if level.Taken != taken {
				t.Fatalf("%s taken=%t", kind, level.Taken)
			}
			return
		}
	}
	t.Fatalf("missing %s near %.3f", kind, price)
}
