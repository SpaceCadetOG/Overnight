package live

import (
	"sort"
	"time"
)

type AssetReport struct {
	Symbol          string    `json:"symbol"`
	Classification  string    `json:"classification"`
	LatestMap       time.Time `json:"latest_map"`
	Maps            int       `json:"maps"`
	Intents         int       `json:"intents"`
	DryRuns         int       `json:"dry_runs"`
	OrderAuthorized bool      `json:"order_authorized"`
}

func BuildAssetReports(snapshots []MarketSnapshot, intents []Intent) []AssetReport {
	bySymbol := map[string]*AssetReport{}
	for _, snapshot := range snapshots {
		report := bySymbol[snapshot.Symbol]
		if report == nil {
			report = &AssetReport{Symbol: snapshot.Symbol, Classification: snapshot.Classification, OrderAuthorized: snapshot.OrderAuthorized}
			bySymbol[snapshot.Symbol] = report
		}
		report.Maps++
		if snapshot.Timestamp.After(report.LatestMap) {
			report.LatestMap = snapshot.Timestamp
		}
	}
	for _, intent := range intents {
		report := bySymbol[intent.Symbol]
		if report == nil {
			report = &AssetReport{Symbol: intent.Symbol}
			bySymbol[intent.Symbol] = report
		}
		report.Intents++
		if intent.State == DryRun {
			report.DryRuns++
		}
	}
	out := make([]AssetReport, 0, len(bySymbol))
	for _, report := range bySymbol {
		out = append(out, *report)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Symbol < out[j].Symbol })
	return out
}
