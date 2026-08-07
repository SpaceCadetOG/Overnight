package live

import (
	"testing"
	"time"
)

func TestReportsKeepResearchAssetsIntentFree(t *testing.T) {
	reports := BuildAssetReports([]MarketSnapshot{{Symbol: "BTC", Classification: "LIVE_EXECUTION", OrderAuthorized: true, Timestamp: time.Now()}, {Symbol: "XAU", Classification: "RESEARCH", Timestamp: time.Now()}}, []Intent{{Symbol: "BTC", State: DryRun}})
	for _, report := range reports {
		if report.Symbol == "XAU" && report.Intents != 0 {
			t.Fatal("research asset has intent")
		}
	}
}
