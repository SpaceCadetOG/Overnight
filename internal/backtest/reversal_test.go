package backtest

import (
	"math"
	"testing"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

func TestSimulateReversalTarget(t *testing.T) {
	location := mustChicago(t)
	date := time.Date(2026, time.August, 1, 0, 0, 0, 0, location)
	entryTime := time.Date(2026, time.August, 1, 5, 4, 59, 999000000, location)
	plan := ReversalPlan{Date: date, Direction: models.BiasLong, EntryTime: entryTime, Entry: 100, Stop: 95, Target: 110, Valid: true}
	candles := []models.Candle{
		makeCandle(location, date, 5, 0, 96, 101, 94, 100),
		makeCandle(location, date, 5, 5, 100, 111, 99, 110),
	}
	result, err := SimulateReversalWithConfig(plan, candles, location, IdealExecutionConfig())
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != models.OutcomeTP2 || math.Abs(result.RealizedR-2) > 1e-9 {
		t.Fatalf("result=%+v", result)
	}
}
