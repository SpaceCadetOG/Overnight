package journal

import (
	"sort"
	"time"
)

type AssetDaily struct {
	Rank        int     `json:"rank"`
	Symbol      string  `json:"symbol"`
	Mode        string  `json:"mode"`
	Outcome     string  `json:"outcome"`
	Filled      bool    `json:"filled"`
	RMultiple   float64 `json:"r_multiple"`
	MFER        float64 `json:"mfe_r"`
	MAER        float64 `json:"mae_r"`
	SlippageBPS float64 `json:"entry_slippage_bps"`
}

type DailyReport struct {
	SessionDate        string                `json:"session_date"`
	StrategyVersion    string                `json:"strategy_version"`
	RuntimeVersions    []string              `json:"runtime_versions"`
	ExpectedMarkets    int                   `json:"expected_markets"`
	Records            int                   `json:"records"`
	Coverage           float64               `json:"coverage"`
	Plans              int                   `json:"plans"`
	Filled             int                   `json:"filled"`
	NoFill             int                   `json:"no_fill"`
	Open               int                   `json:"open"`
	Wins               int                   `json:"wins"`
	Losses             int                   `json:"losses"`
	Scratches          int                   `json:"scratches"`
	WinRate            float64               `json:"win_rate"`
	TotalR             float64               `json:"total_r"`
	AverageRPerFill    float64               `json:"average_r_per_fill"`
	AverageSlippageBPS float64               `json:"average_entry_slippage_bps"`
	Assets             []AssetDaily          `json:"assets"`
	LivePaper          []LivePaperComparison `json:"live_paper_comparison,omitempty"`
}

type LivePaperComparison struct {
	Symbol               string  `json:"symbol"`
	PaperOutcome         string  `json:"paper_outcome"`
	LiveOutcome          string  `json:"live_outcome"`
	PaperR               float64 `json:"paper_r"`
	LiveR                float64 `json:"live_r"`
	ExecutionDifferenceR float64 `json:"execution_difference_r"`
	LiveSlippageBPS      float64 `json:"live_entry_slippage_bps"`
}

func BuildDaily(records []TradeRecord, date time.Time, expected int) DailyReport {
	latest := map[string]TradeRecord{}
	for _, record := range records {
		if !sameDate(record.SessionDate, date) {
			continue
		}
		current, ok := latest[record.ID]
		if !ok || record.RecordedAt.After(current.RecordedAt) {
			latest[record.ID] = record
		}
	}
	report := DailyReport{SessionDate: date.Format("2006-01-02"), ExpectedMarkets: expected, Records: len(latest), Plans: len(latest)}
	if expected > 0 {
		report.Coverage = float64(report.Records) / float64(expected)
	}
	slippageCount := 0
	runtimeVersions := map[string]bool{}
	for _, record := range latest {
		filled := record.ActualFill > 0
		row := AssetDaily{Symbol: record.Symbol, Mode: record.Mode, Outcome: record.Outcome, Filled: filled, RMultiple: record.RMultiple, MFER: record.MFER, MAER: record.MAER, SlippageBPS: record.EntrySlippageBPS}
		report.Assets = append(report.Assets, row)
		report.StrategyVersion = record.StrategyVersion
		if record.RuntimeVersion != "" {
			runtimeVersions[record.RuntimeVersion] = true
		}
		if record.Outcome == "NO_FILL" {
			report.NoFill++
			continue
		}
		if !filled || record.Outcome == "OPEN" || record.Outcome == "TP1_OPEN" {
			report.Open++
			continue
		}
		report.Filled++
		report.TotalR += record.RMultiple
		if record.EntrySlippageBPS != 0 {
			report.AverageSlippageBPS += record.EntrySlippageBPS
			slippageCount++
		}
		switch {
		case record.RMultiple > 0:
			report.Wins++
		case record.RMultiple < 0:
			report.Losses++
		default:
			report.Scratches++
		}
	}
	if report.Filled > 0 {
		report.WinRate = float64(report.Wins) / float64(report.Filled)
		report.AverageRPerFill = report.TotalR / float64(report.Filled)
	}
	if slippageCount > 0 {
		report.AverageSlippageBPS /= float64(slippageCount)
	}
	for version := range runtimeVersions {
		report.RuntimeVersions = append(report.RuntimeVersions, version)
	}
	sort.Strings(report.RuntimeVersions)
	sort.Slice(report.Assets, func(i, j int) bool {
		if report.Assets[i].RMultiple == report.Assets[j].RMultiple {
			return report.Assets[i].Symbol < report.Assets[j].Symbol
		}
		return report.Assets[i].RMultiple > report.Assets[j].RMultiple
	})
	for i := range report.Assets {
		report.Assets[i].Rank = i + 1
	}
	bySymbol := map[string]map[string]TradeRecord{}
	for _, record := range latest {
		if bySymbol[record.Symbol] == nil {
			bySymbol[record.Symbol] = map[string]TradeRecord{}
		}
		bySymbol[record.Symbol][record.Mode] = record
	}
	for symbol, modes := range bySymbol {
		paper, hasPaper := modes["PAPER_EXECUTION"]
		liveRecord, hasLive := modes["LIVE_EXECUTION"]
		if !hasPaper || !hasLive {
			continue
		}
		report.LivePaper = append(report.LivePaper, LivePaperComparison{Symbol: symbol, PaperOutcome: paper.Outcome, LiveOutcome: liveRecord.Outcome, PaperR: paper.RMultiple, LiveR: liveRecord.RMultiple, ExecutionDifferenceR: liveRecord.RMultiple - paper.RMultiple, LiveSlippageBPS: liveRecord.EntrySlippageBPS})
	}
	sort.Slice(report.LivePaper, func(i, j int) bool { return report.LivePaper[i].Symbol < report.LivePaper[j].Symbol })
	return report
}

func sameDate(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}
