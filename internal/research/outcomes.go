package research

import "github.com/ogtrading/overnight-strategy/internal/models"

type OutcomeReport struct {
	TP2      PerformanceStats
	TP1BE    PerformanceStats
	Stopped  PerformanceStats
	TimeExit PerformanceStats
}

func calculateOutcomeReport(results []models.TradeResult) OutcomeReport {
	return OutcomeReport{
		TP2: calculatePerformance(
			filterResults(results, func(r models.TradeResult) bool {
				return r.Outcome == models.OutcomeTP2
			}),
		),
		TP1BE: calculatePerformance(
			filterResults(results, func(r models.TradeResult) bool {
				return r.Outcome == models.OutcomeTP1Breakeven
			}),
		),
		Stopped: calculatePerformance(
			filterResults(results, func(r models.TradeResult) bool {
				return r.Outcome == models.OutcomeStopped
			}),
		),
		TimeExit: calculatePerformance(
			filterResults(results, func(r models.TradeResult) bool {
				return r.Outcome == models.OutcomeTimeExit
			}),
		),
	}
}
