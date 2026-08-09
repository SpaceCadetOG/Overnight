package main

import (
	"strings"
	"testing"

	"github.com/ogtrading/overnight-strategy/internal/journal"
)

func TestEODMessageIncludesPerformanceAndAssets(t *testing.T) {
	report := journal.DailyReport{
		SessionDate:     "2026-08-09",
		ExpectedMarkets: 12,
		Records:         12,
		Filled:          7,
		Wins:            4,
		Losses:          3,
		NoFill:          5,
		WinRate:         4.0 / 7.0,
		TotalR:          -0.17,
		AverageRPerFill: -0.02,
		Assets: []journal.AssetDaily{
			{Symbol: "ETH", Outcome: "TP2", RMultiple: 0.95},
			{Symbol: "XAG", Outcome: "STOPPED", RMultiple: -1},
		},
	}
	got := eodMessage(report, "PASS", 0)
	for _, want := range []string{"Coverage: 12/12", "Wins: 4", "Losses: 3", "Result: -0.17R", "ETH: TP2 +0.95R", "XAG: STOPPED -1.00R"} {
		if !strings.Contains(got, want) {
			t.Fatalf("message missing %q:\n%s", want, got)
		}
	}
}
