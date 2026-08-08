package journal

import (
	"testing"
	"time"
)

func TestBuildDailyDeduplicatesAndRanks(t *testing.T) {
	date := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	records := []TradeRecord{
		{ID: "BTC", SessionDate: date, RecordedAt: date, Symbol: "BTC", Outcome: "STOPPED", ActualFill: 100, RMultiple: -1},
		{ID: "BTC", SessionDate: date, RecordedAt: date.Add(time.Minute), Symbol: "BTC", Outcome: "TP2", ActualFill: 100, RMultiple: 2},
		{ID: "ETH", SessionDate: date, RecordedAt: date, Symbol: "ETH", Outcome: "NO_FILL"},
	}
	report := BuildDaily(records, date, 23)
	if report.Records != 2 || report.Filled != 1 || report.NoFill != 1 || report.TotalR != 2 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if report.Assets[0].Symbol != "BTC" || report.Coverage != 2.0/23.0 {
		t.Fatalf("unexpected ranking/coverage: %+v", report)
	}
}
