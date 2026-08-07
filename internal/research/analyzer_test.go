package research

import (
	"math"
	"testing"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

func TestAnalyzeCalculatesSummaryAndDirectionStats(t *testing.T) {
	results := []models.TradeResult{
		{
			Date:           time.Date(2026, time.July, 20, 0, 0, 0, 0, time.UTC),
			Direction:      models.BiasLong,
			Outcome:        models.OutcomeTP2,
			Filled:         true,
			RealizedR:      2,
			MFER:           2.5,
			MAER:           0.25,
			MinutesInTrade: 120,
		},
		{
			Date:           time.Date(2026, time.July, 21, 0, 0, 0, 0, time.UTC),
			Direction:      models.BiasShort,
			Outcome:        models.OutcomeStopped,
			Filled:         true,
			RealizedR:      -1,
			MFER:           0.4,
			MAER:           1,
			MinutesInTrade: 30,
		},
		{
			Date:      time.Date(2026, time.July, 22, 0, 0, 0, 0, time.UTC),
			Direction: models.BiasLong,
			Outcome:   models.OutcomeNoFill,
			Filled:    false,
		},
	}

	report := Analyze(results)

	if report.Summary.Trades != 2 {
		t.Fatalf(
			"expected 2 filled trades, got %d",
			report.Summary.Trades,
		)
	}

	if report.Summary.Wins != 1 {
		t.Fatalf(
			"expected 1 win, got %d",
			report.Summary.Wins,
		)
	}

	if report.Summary.Losses != 1 {
		t.Fatalf(
			"expected 1 loss, got %d",
			report.Summary.Losses,
		)
	}

	if math.Abs(report.Summary.TotalR-1) > 0.000001 {
		t.Fatalf(
			"expected total 1R, got %.6fR",
			report.Summary.TotalR,
		)
	}

	if math.Abs(report.Summary.ProfitFactor-2) > 0.000001 {
		t.Fatalf(
			"expected PF 2, got %.6f",
			report.Summary.ProfitFactor,
		)
	}

	longStats := report.Directions[string(models.BiasLong)]
	if longStats.Trades != 1 || longStats.TotalR != 2 {
		t.Fatalf(
			"unexpected long stats: %+v",
			longStats,
		)
	}

	shortStats := report.Directions[string(models.BiasShort)]
	if shortStats.Trades != 1 || shortStats.TotalR != -1 {
		t.Fatalf(
			"unexpected short stats: %+v",
			shortStats,
		)
	}
}

func TestDurationStatistics(t *testing.T) {
	results := []models.TradeResult{
		{
			Filled:         true,
			Outcome:        models.OutcomeTP2,
			RealizedR:      1,
			MinutesInTrade: 10,
		},
		{
			Filled:         true,
			Outcome:        models.OutcomeTP2,
			RealizedR:      1,
			MinutesInTrade: 20,
		},
		{
			Filled:         true,
			Outcome:        models.OutcomeStopped,
			RealizedR:      -1,
			MinutesInTrade: 30,
		},
	}

	report := Analyze(results)

	if math.Abs(report.Durations.All.AverageMinutes-20) > 0.000001 {
		t.Fatalf(
			"expected average duration 20, got %.2f",
			report.Durations.All.AverageMinutes,
		)
	}

	if math.Abs(report.Durations.All.MedianMinutes-20) > 0.000001 {
		t.Fatalf(
			"expected median duration 20, got %.2f",
			report.Durations.All.MedianMinutes,
		)
	}

	if report.Durations.Winners.Trades != 2 {
		t.Fatalf(
			"expected 2 winning durations, got %d",
			report.Durations.Winners.Trades,
		)
	}

	if report.Durations.Losers.Trades != 1 {
		t.Fatalf(
			"expected 1 losing duration, got %d",
			report.Durations.Losers.Trades,
		)
	}
}

func TestDistributionCountsValues(t *testing.T) {
	results := []models.TradeResult{
		{
			Filled:  true,
			Outcome: models.OutcomeStopped,
			MAER:    0.10,
			MFER:    0.25,
		},
		{
			Filled:  true,
			Outcome: models.OutcomeStopped,
			MAER:    0.60,
			MFER:    1.25,
		},
		{
			Filled:  true,
			Outcome: models.OutcomeTP2,
			MAER:    1.20,
			MFER:    3.50,
		},
	}

	report := Analyze(results)

	totalMAE := 0
	for _, bucket := range report.MAEDistribution {
		totalMAE += bucket.Count
	}

	if totalMAE != 3 {
		t.Fatalf(
			"expected 3 MAE observations, got %d",
			totalMAE,
		)
	}

	totalMFE := 0
	for _, bucket := range report.MFEDistribution {
		totalMFE += bucket.Count
	}

	if totalMFE != 3 {
		t.Fatalf(
			"expected 3 MFE observations, got %d",
			totalMFE,
		)
	}
}
