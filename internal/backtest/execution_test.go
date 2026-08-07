package backtest

import (
	"math"
	"testing"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

func TestNoFill(t *testing.T) {
	location := mustChicago(t)
	date := time.Date(2026, time.July, 26, 0, 0, 0, 0, location)

	plan := models.TradePlan{
		Date:      date,
		Direction: models.BiasLong,
		Entry:     100,
		Stop:      95,
		TP1:       105,
		TP2:       110,
		RR1:       1,
		RR2:       2,
		Valid:     true,
	}

	candles := []models.Candle{
		makeCandle(location, date, 5, 0, 101, 104, 101, 103),
		makeCandle(location, date, 15, 55, 103, 104, 102, 103),
	}

	result, err := SimulateTrade(plan, candles, location)
	if err != nil {
		t.Fatalf("simulate trade: %v", err)
	}

	if result.Outcome != models.OutcomeNoFill {
		t.Fatalf("expected NO_FILL, got %s", result.Outcome)
	}
}

func TestLongTP1ThenBreakeven(t *testing.T) {
	location := mustChicago(t)
	date := time.Date(2026, time.July, 26, 0, 0, 0, 0, location)

	plan := models.TradePlan{
		Date:      date,
		Direction: models.BiasLong,
		Entry:     100,
		Stop:      95,
		TP1:       105,
		TP2:       110,
		RR1:       1,
		RR2:       2,
		Valid:     true,
	}

	candles := []models.Candle{
		makeCandle(location, date, 5, 0, 102, 103, 99, 101),
		makeCandle(location, date, 5, 5, 101, 106, 101, 105),
		makeCandle(location, date, 5, 10, 105, 106, 99, 100),
	}

	result, err := SimulateTrade(plan, candles, location)
	if err != nil {
		t.Fatalf("simulate trade: %v", err)
	}

	if result.Outcome != models.OutcomeTP1Breakeven {
		t.Fatalf(
			"expected TP1_BE, got %s",
			result.Outcome,
		)
	}

	if math.Abs(result.RealizedR-0.5) > 0.000001 {
		t.Fatalf(
			"expected 0.5R, got %.6f",
			result.RealizedR,
		)
	}
}

func TestLongTP2(t *testing.T) {
	location := mustChicago(t)
	date := time.Date(2026, time.July, 26, 0, 0, 0, 0, location)

	plan := models.TradePlan{
		Date:      date,
		Direction: models.BiasLong,
		Entry:     100,
		Stop:      95,
		TP1:       105,
		TP2:       110,
		RR1:       1,
		RR2:       2,
		Valid:     true,
	}

	candles := []models.Candle{
		makeCandle(location, date, 5, 0, 102, 103, 99, 101),
		makeCandle(location, date, 5, 5, 101, 106, 101, 105),
		makeCandle(location, date, 5, 10, 105, 111, 104, 110),
	}

	result, err := SimulateTrade(plan, candles, location)
	if err != nil {
		t.Fatalf("simulate trade: %v", err)
	}

	if result.Outcome != models.OutcomeTP2 {
		t.Fatalf("expected TP2, got %s", result.Outcome)
	}

	if math.Abs(result.RealizedR-1.5) > 0.000001 {
		t.Fatalf(
			"expected 1.5R, got %.6f",
			result.RealizedR,
		)
	}
}

func TestFillAndStopSameCandleUsesStop(t *testing.T) {
	location := mustChicago(t)
	date := time.Date(2026, time.July, 26, 0, 0, 0, 0, location)

	plan := models.TradePlan{
		Date:      date,
		Direction: models.BiasLong,
		Entry:     100,
		Stop:      95,
		TP1:       105,
		TP2:       110,
		RR1:       1,
		RR2:       2,
		Valid:     true,
	}

	candles := []models.Candle{
		makeCandle(location, date, 5, 0, 102, 106, 94, 101),
	}

	result, err := SimulateTrade(plan, candles, location)
	if err != nil {
		t.Fatalf("simulate trade: %v", err)
	}

	if result.Outcome != models.OutcomeStopped {
		t.Fatalf(
			"expected STOPPED, got %s",
			result.Outcome,
		)
	}
}

func mustChicago(t *testing.T) *time.Location {
	t.Helper()

	location, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatalf("load timezone: %v", err)
	}

	return location
}

func makeCandle(
	location *time.Location,
	date time.Time,
	hour int,
	minute int,
	open float64,
	high float64,
	low float64,
	closePrice float64,
) models.Candle {
	openLocal := time.Date(
		date.Year(),
		date.Month(),
		date.Day(),
		hour,
		minute,
		0,
		0,
		location,
	)

	return models.Candle{
		OpenTime:  openLocal.UTC(),
		CloseTime: openLocal.Add(5*time.Minute - time.Millisecond).UTC(),
		Open:      open,
		High:      high,
		Low:       low,
		Close:     closePrice,
		Volume:    10,
	}
}

func TestStoppedTradeCapsMAEAtStop(t *testing.T) {
	location := mustChicago(t)
	date := time.Date(2026, time.July, 26, 0, 0, 0, 0, location)

	plan := models.TradePlan{
		Date:      date,
		Direction: models.BiasLong,
		Entry:     100,
		Stop:      95,
		TP1:       105,
		TP2:       110,
		RR1:       1,
		RR2:       2,
		Valid:     true,
	}

	candles := []models.Candle{
		makeCandle(location, date, 5, 0, 101, 102, 99, 100),
		makeCandle(location, date, 5, 5, 100, 101, 80, 85),
		makeCandle(location, date, 5, 10, 85, 90, 60, 65),
	}

	result, err := SimulateTrade(plan, candles, location)
	if err != nil {
		t.Fatalf("simulate trade: %v", err)
	}

	if result.Outcome != models.OutcomeStopped {
		t.Fatalf("expected STOPPED, got %s", result.Outcome)
	}

	if math.Abs(result.MAER-1.0) > 0.000001 {
		t.Fatalf("expected MAE capped at 1.0R, got %.6fR", result.MAER)
	}
}

func TestShortStoppedTradeCapsMAEAtStop(t *testing.T) {
	location := mustChicago(t)
	date := time.Date(2026, time.July, 26, 0, 0, 0, 0, location)

	plan := models.TradePlan{
		Date:      date,
		Direction: models.BiasShort,
		Entry:     100,
		Stop:      105,
		TP1:       95,
		TP2:       90,
		RR1:       1,
		RR2:       2,
		Valid:     true,
	}

	candles := []models.Candle{
		makeCandle(location, date, 5, 0, 99, 101, 98, 100),
		makeCandle(location, date, 5, 5, 100, 120, 99, 115),
		makeCandle(location, date, 5, 10, 115, 140, 110, 135),
	}

	result, err := SimulateTrade(plan, candles, location)
	if err != nil {
		t.Fatalf("simulate trade: %v", err)
	}

	if result.Outcome != models.OutcomeStopped {
		t.Fatalf("expected STOPPED, got %s", result.Outcome)
	}

	if math.Abs(result.MAER-1.0) > 0.000001 {
		t.Fatalf("expected MAE capped at 1.0R, got %.6fR", result.MAER)
	}
}

func TestIdealExecutionPreservesLegacyStopResult(t *testing.T) {
	location := mustChicago(t)
	date := time.Date(2026, time.July, 26, 0, 0, 0, 0, location)

	plan := models.TradePlan{
		Date:      date,
		Direction: models.BiasLong,
		Entry:     100,
		Stop:      95,
		TP1:       105,
		TP2:       110,
		RR1:       1,
		RR2:       2,
		Valid:     true,
	}

	candles := []models.Candle{
		makeCandle(location, date, 5, 0, 101, 102, 99, 100),
		makeCandle(location, date, 5, 5, 100, 101, 94, 95),
	}

	result, err := SimulateTradeWithConfig(
		plan,
		candles,
		location,
		IdealExecutionConfig(),
	)
	if err != nil {
		t.Fatalf("simulate trade: %v", err)
	}

	if math.Abs(result.FillPrice-100) > 0.000001 {
		t.Fatalf("expected ideal fill 100, got %.6f", result.FillPrice)
	}

	if math.Abs(result.ExitPrice-95) > 0.000001 {
		t.Fatalf("expected ideal exit 95, got %.6f", result.ExitPrice)
	}

	if math.Abs(result.RealizedR-(-1)) > 0.000001 {
		t.Fatalf("expected ideal result -1R, got %.6fR", result.RealizedR)
	}

	if result.FeeR != 0 {
		t.Fatalf("expected zero ideal fees, got %.6fR", result.FeeR)
	}
}

func TestRealisticExecutionReducesStoppedTradeResult(t *testing.T) {
	location := mustChicago(t)
	date := time.Date(2026, time.July, 26, 0, 0, 0, 0, location)

	plan := models.TradePlan{
		Date:      date,
		Direction: models.BiasLong,
		Entry:     100,
		Stop:      95,
		TP1:       105,
		TP2:       110,
		RR1:       1,
		RR2:       2,
		Valid:     true,
	}

	candles := []models.Candle{
		makeCandle(location, date, 5, 0, 101, 102, 99, 100),
		makeCandle(location, date, 5, 5, 100, 101, 94, 95),
	}

	result, err := SimulateTradeWithConfig(
		plan,
		candles,
		location,
		RealisticExecutionConfig(),
	)
	if err != nil {
		t.Fatalf("simulate trade: %v", err)
	}

	if result.FillPrice <= plan.Entry {
		t.Fatalf(
			"expected adverse long entry slippage, got %.6f",
			result.FillPrice,
		)
	}

	if result.ExitPrice >= plan.Stop {
		t.Fatalf(
			"expected adverse long stop slippage, got %.6f",
			result.ExitPrice,
		)
	}

	if result.FeeR <= 0 {
		t.Fatalf("expected positive fee cost, got %.6fR", result.FeeR)
	}

	if result.RealizedR >= -1 {
		t.Fatalf(
			"expected realistic stop worse than -1R, got %.6fR",
			result.RealizedR,
		)
	}
}

func TestParseExecutionConfigRejectsUnknownMode(t *testing.T) {
	_, err := ParseExecutionConfig("banana")
	if err == nil {
		t.Fatal("expected unsupported execution-mode error")
	}
}
