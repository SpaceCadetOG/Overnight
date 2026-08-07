package main

import (
	"flag"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/backtest"
	"github.com/ogtrading/overnight-strategy/internal/marketdata/cache"
	"github.com/ogtrading/overnight-strategy/internal/models"
	"github.com/ogtrading/overnight-strategy/internal/reporting"
	"github.com/ogtrading/overnight-strategy/internal/session"
	"github.com/ogtrading/overnight-strategy/internal/strategy"
)

type optimizationResult struct {
	Config strategy.PlanConfig
	Stats  reporting.Statistics
}

func main() {
	input := flag.String(
		"input",
		"data/raw/Binance_BTCUSDT_5m.csv",
		"input candle CSV",
	)

	timezone := flag.String(
		"timezone",
		"America/Chicago",
		"strategy timezone",
	)

	limit := flag.Int(
		"limit",
		20,
		"maximum number of ranked configurations to print; use 0 for all",
	)

	flag.Parse()

	location, err := time.LoadLocation(*timezone)
	if err != nil {
		log.Fatalf("load timezone: %v", err)
	}

	candles, err := cache.ReadCandlesCSV(*input)
	if err != nil {
		log.Fatalf("read candles: %v", err)
	}

	sessions, err := session.BuildOvernightSessions(
		candles,
		location,
	)
	if err != nil {
		log.Fatalf("build sessions: %v", err)
	}

	analyzedSessions := make(
		[]models.Session,
		0,
		len(sessions),
	)

	for _, overnight := range sessions {
		analyzed, err := strategy.AnalyzeSession(overnight)
		if err != nil {
			log.Fatalf(
				"analyze %s: %v",
				overnight.Date.Format("2006-01-02"),
				err,
			)
		}

		analyzedSessions = append(
			analyzedSessions,
			analyzed,
		)
	}

	configurations := generateConfigurations()

	optimizationResults := make(
		[]optimizationResult,
		0,
		len(configurations),
	)

	for _, config := range configurations {
		results := make(
			[]models.TradeResult,
			0,
			len(analyzedSessions),
		)

		for _, analyzed := range analyzedSessions {
			plan := strategy.BuildTradePlanWithConfig(
				analyzed,
				config,
			)

			result, err := backtest.SimulateTrade(
				plan,
				candles,
				location,
			)
			if err != nil {
				log.Fatalf(
					"simulate %s: %v",
					analyzed.Date.Format("2006-01-02"),
					err,
				)
			}

			results = append(results, result)
		}

		optimizationResults = append(
			optimizationResults,
			optimizationResult{
				Config: config,
				Stats: reporting.CalculateStatistics(
					results,
				),
			},
		)
	}

	sortOptimizationResults(optimizationResults)

	printOptimizerHeader(
		*input,
		len(analyzedSessions),
		len(optimizationResults),
	)

	printOptimizationResults(
		optimizationResults,
		*limit,
	)

	printBestConfiguration(optimizationResults)
}

func generateConfigurations() []strategy.PlanConfig {
	entryMethods := []strategy.EntryMethod{
		strategy.EntryFib382,
		strategy.EntryMidpoint,
		strategy.EntryFib500,
	}

	// Phase 3 baseline stops only.
	//
	// FVG_CLOSE remains available for isolated research but is not part of
	// the baseline optimization matrix.
	stopMethods := []strategy.StopMethod{
		strategy.StopProfileFib,
		strategy.StopValueArea,
		strategy.StopFib382,
	}

	tp1Methods := []strategy.TP1Method{
		strategy.TP1Fib618,
		strategy.TP1POC,
		strategy.TP1NearestValid,
	}

	configurations := make(
		[]strategy.PlanConfig,
		0,
		len(entryMethods)*len(stopMethods)*len(tp1Methods),
	)

	for _, entryMethod := range entryMethods {
		for _, stopMethod := range stopMethods {
			for _, tp1Method := range tp1Methods {
				config := strategy.DefaultPlanConfig()
				config.EntryMethod = entryMethod
				config.StopMethod = stopMethod
				config.TP1Method = tp1Method

				configurations = append(
					configurations,
					config,
				)
			}
		}
	}

	return configurations
}

func sortOptimizationResults(
	results []optimizationResult,
) {
	sort.SliceStable(
		results,
		func(i, j int) bool {
			left := results[i].Stats
			right := results[j].Stats

			if left.TotalR != right.TotalR {
				return left.TotalR > right.TotalR
			}

			if left.ProfitFactor != right.ProfitFactor {
				return left.ProfitFactor > right.ProfitFactor
			}

			if left.MaxDrawdownR != right.MaxDrawdownR {
				return left.MaxDrawdownR < right.MaxDrawdownR
			}

			return left.FillRate > right.FillRate
		},
	)
}

func printOptimizerHeader(
	input string,
	sessionCount int,
	configurationCount int,
) {
	fmt.Println()
	fmt.Println("============================================================")
	fmt.Println(" PHASE 3.1 — BASELINE VALIDATION")
	fmt.Println("============================================================")
	fmt.Printf("Dataset:        %s\n", input)
	fmt.Printf("Sessions:       %d\n", sessionCount)
	fmt.Printf("Configurations: %d\n", configurationCount)
	fmt.Println()
}

func printOptimizationResults(
	results []optimizationResult,
	limit int,
) {
	printCount := len(results)

	if limit > 0 && limit < printCount {
		printCount = limit
	}

	fmt.Println(
		"RANK | ENTRY            | STOP        | TP1           | " +
			"FILL/VLD | FILL% | WIN%  | TOTAL R | AVG/F | PF    | DD    | " +
			"MFE  | MAE  | TP2% | BE%  | STOP% | TIME%",
	)

	fmt.Println(
		"-----+------------------+-------------+---------------+" +
			"----------+-------+-------+---------+-------+-------+-------+" +
			"------+------+------+------+-------+------",
	)

	for index := 0; index < printCount; index++ {
		result := results[index]
		stats := result.Stats

		fmt.Printf(
			"%4d | %-16s | %-11s | %-13s | "+
				"%4d/%-3d | %5.1f | %5.1f | %7.2f | %5.2f | "+
				"%5.2f | %5.2f | %4.2f | %4.2f | "+
				"%4.1f | %4.1f | %5.1f | %4.1f\n",
			index+1,
			result.Config.EntryMethod,
			result.Config.StopMethod,
			result.Config.TP1Method,
			stats.Filled,
			stats.ValidPlans,
			stats.FillRate*100,
			stats.WinRate*100,
			stats.TotalR,
			stats.AverageRFilled,
			stats.ProfitFactor,
			stats.MaxDrawdownR,
			stats.AverageMFER,
			stats.AverageMAER,
			outcomeRate(stats.TP2Count, stats.Filled),
			outcomeRate(stats.TP1BECount, stats.Filled),
			outcomeRate(stats.StoppedCount, stats.Filled),
			outcomeRate(stats.TimeExitCount, stats.Filled),
		)
	}

	fmt.Println()
}

func printBestConfiguration(
	results []optimizationResult,
) {
	if len(results) == 0 {
		fmt.Println("No optimization results.")
		return
	}

	best := results[0]
	stats := best.Stats

	fmt.Println("============================================================")
	fmt.Println(" BASELINE LEADER — SORTED BY TOTAL R")
	fmt.Println("============================================================")
	fmt.Printf("Entry:             %s\n", best.Config.EntryMethod)
	fmt.Printf("Stop:              %s\n", best.Config.StopMethod)
	fmt.Printf("TP1:               %s\n", best.Config.TP1Method)
	fmt.Println()
	fmt.Printf("Valid plans:       %d\n", stats.ValidPlans)
	fmt.Printf("Filled:            %d\n", stats.Filled)
	fmt.Printf("Fill rate:         %.1f%%\n", stats.FillRate*100)
	fmt.Printf("Wins:              %d\n", stats.Wins)
	fmt.Printf("Losses:            %d\n", stats.Losses)
	fmt.Printf("Win rate:          %.1f%%\n", stats.WinRate*100)
	fmt.Printf("Total R:           %.2fR\n", stats.TotalR)
	fmt.Printf("Average R/session: %.3fR\n", stats.AverageR)
	fmt.Printf("Average R/filled:  %.3fR\n", stats.AverageRFilled)
	fmt.Printf("Profit factor:     %.2f\n", stats.ProfitFactor)
	fmt.Printf("Maximum drawdown:  %.2fR\n", stats.MaxDrawdownR)
	fmt.Printf("Average MFE:       %.2fR\n", stats.AverageMFER)
	fmt.Printf("Average MAE:       %.2fR\n", stats.AverageMAER)
	fmt.Println()
	fmt.Printf(
		"Outcomes: TP2 %.1f%% | TP1+BE %.1f%% | STOP %.1f%% | TIME %.1f%%\n",
		outcomeRate(stats.TP2Count, stats.Filled),
		outcomeRate(stats.TP1BECount, stats.Filled),
		outcomeRate(stats.StoppedCount, stats.Filled),
		outcomeRate(stats.TimeExitCount, stats.Filled),
	)
	fmt.Println("============================================================")
}

func outcomeRate(
	count int,
	filled int,
) float64 {
	if filled == 0 {
		return 0
	}

	return float64(count) / float64(filled) * 100
}
