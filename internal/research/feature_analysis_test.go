package research

import (
	"math"
	"testing"

	"github.com/ogtrading/overnight-strategy/internal/auction"
	"github.com/ogtrading/overnight-strategy/internal/models"
)

func TestAnalyzeFeatureBooleansSeparatesPlansAndFilledTrades(t *testing.T) {
	observations := []AuctionObservation{
		featureObservation(true, false, 0, 0, 0),
		featureObservation(true, true, 2, 3, 0.5),
		featureObservation(true, true, -1, 0.4, 1),
		featureObservation(true, true, 0, 0.8, 0.2),
		featureObservation(false, true, 1, 1.5, 0.3),
	}

	analysis := AnalyzeFeatureBooleans(observations)
	report := findFeatureReport(t, analysis, "ENTRY_INSIDE_VALUE")

	assertFeatureBucket(t, report.True, FeatureBucket{
		ValidPlans:        4,
		Filled:            3,
		NoFill:            1,
		Wins:              1,
		Losses:            1,
		Breakeven:         1,
		TotalR:            1,
		TotalMFE:          4.2,
		TotalMAE:          1.7,
		GrossProfit:       2,
		GrossLoss:         1,
		FillRate:          75,
		FilledWinRate:     100.0 / 3.0,
		AverageRPerPlan:   0.25,
		AverageRPerFilled: 1.0 / 3.0,
		AverageMFE:        1.4,
		AverageMAE:        1.7 / 3.0,
		ProfitFactor:      2,
	})

	if report.False.ValidPlans != 1 || report.False.Filled != 1 ||
		report.False.Wins != 1 || report.False.NoFill != 0 {
		t.Fatalf("unexpected FALSE bucket: %+v", report.False)
	}
}

func TestAnalyzeFeatureBooleansUsesRealizedRTolerance(t *testing.T) {
	analysis := AnalyzeFeatureBooleans([]AuctionObservation{
		featureObservation(true, true, realizedRTolerance/2, 1, 1),
		featureObservation(true, true, realizedRTolerance*2, 1, 1),
		featureObservation(true, true, -realizedRTolerance*2, 1, 1),
	})

	bucket := findFeatureReport(t, analysis, "ENTRY_INSIDE_VALUE").True
	if bucket.Wins != 1 || bucket.Losses != 1 || bucket.Breakeven != 1 {
		t.Fatalf("unexpected tolerance classification: %+v", bucket)
	}
}

func TestAnalyzeFeatureBooleansProfitFactorWithoutLoss(t *testing.T) {
	analysis := AnalyzeFeatureBooleans([]AuctionObservation{
		featureObservation(true, true, 1, 1, 0),
	})

	bucket := findFeatureReport(t, analysis, "ENTRY_INSIDE_VALUE").True
	if !math.IsInf(bucket.ProfitFactor, 1) {
		t.Fatalf("ProfitFactor = %v, want +Inf", bucket.ProfitFactor)
	}
}

func featureObservation(feature, filled bool, realizedR, mfe, mae float64) AuctionObservation {
	outcome := models.OutcomeNoFill
	if filled {
		outcome = models.OutcomeTimeExit
	}

	return AuctionObservation{
		Structure: auction.AuctionStructure{EntryInsideValue: feature},
		Result: models.TradeResult{
			Outcome:   outcome,
			Filled:    filled,
			RealizedR: realizedR,
			MFER:      mfe,
			MAER:      mae,
		},
	}
}

func findFeatureReport(t *testing.T, analysis FeatureAnalysis, name string) FeatureReport {
	t.Helper()
	for _, report := range analysis.Reports {
		if report.Name == name {
			return report
		}
	}
	t.Fatalf("feature report %q not found", name)
	return FeatureReport{}
}

func assertFeatureBucket(t *testing.T, got, want FeatureBucket) {
	t.Helper()
	if got.ValidPlans != want.ValidPlans || got.Filled != want.Filled ||
		got.NoFill != want.NoFill || got.Wins != want.Wins ||
		got.Losses != want.Losses || got.Breakeven != want.Breakeven {
		t.Fatalf("counts = %+v, want %+v", got, want)
	}

	checks := map[string][2]float64{
		"TotalR":            {got.TotalR, want.TotalR},
		"TotalMFE":          {got.TotalMFE, want.TotalMFE},
		"TotalMAE":          {got.TotalMAE, want.TotalMAE},
		"GrossProfit":       {got.GrossProfit, want.GrossProfit},
		"GrossLoss":         {got.GrossLoss, want.GrossLoss},
		"FillRate":          {got.FillRate, want.FillRate},
		"FilledWinRate":     {got.FilledWinRate, want.FilledWinRate},
		"AverageRPerPlan":   {got.AverageRPerPlan, want.AverageRPerPlan},
		"AverageRPerFilled": {got.AverageRPerFilled, want.AverageRPerFilled},
		"AverageMFE":        {got.AverageMFE, want.AverageMFE},
		"AverageMAE":        {got.AverageMAE, want.AverageMAE},
		"ProfitFactor":      {got.ProfitFactor, want.ProfitFactor},
	}

	for name, values := range checks {
		if math.Abs(values[0]-values[1]) > 1e-9 {
			t.Errorf("%s = %.12f, want %.12f", name, values[0], values[1])
		}
	}
}
