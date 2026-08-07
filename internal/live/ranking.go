package live

import (
	"sort"

	"github.com/ogtrading/overnight-strategy/internal/models"
	"github.com/ogtrading/overnight-strategy/internal/reporting"
	"github.com/ogtrading/overnight-strategy/internal/universe"
)

type RecordedResult struct {
	Symbol string             `json:"symbol"`
	Result models.TradeResult `json:"result"`
}

type Ranking struct {
	Rank               int                  `json:"rank"`
	Symbol             string               `json:"symbol"`
	Statistics         reporting.Statistics `json:"statistics"`
	ResearchScore      float64              `json:"research_score"`
	ExecutionAuthority bool                 `json:"execution_authority"`
}

func RankResults(rows []RecordedResult) []Ranking {
	grouped := map[string][]models.TradeResult{}
	for _, row := range rows {
		grouped[row.Symbol] = append(grouped[row.Symbol], row.Result)
	}
	out := make([]Ranking, 0, len(grouped))
	for symbol, results := range grouped {
		stats := reporting.CalculateStatistics(results)
		score := stats.AverageRFilled*100 + stats.ProfitFactor*10 - stats.MaxDrawdownR
		asset, _ := universe.Find(symbol)
		out = append(out, Ranking{Symbol: symbol, Statistics: stats, ResearchScore: score, ExecutionAuthority: asset.Tradable})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ResearchScore == out[j].ResearchScore {
			return out[i].Symbol < out[j].Symbol
		}
		return out[i].ResearchScore > out[j].ResearchScore
	})
	for i := range out {
		out[i].Rank = i + 1
	}
	return out
}
