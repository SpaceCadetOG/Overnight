package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/journal"
	"github.com/ogtrading/overnight-strategy/internal/store"
	"github.com/ogtrading/overnight-strategy/internal/universe"
)

func main() {
	root := flag.String("store", "data/research", "research event store")
	dateFlag := flag.String("date", "", "session date YYYY-MM-DD; defaults to today in America/Chicago")
	jsonFlag := flag.Bool("json", false, "print JSON")
	flag.Parse()
	location, err := time.LoadLocation("America/Chicago")
	if err != nil {
		fatal(err)
	}
	date := time.Now().In(location)
	if *dateFlag != "" {
		date, err = time.ParseInLocation("2006-01-02", *dateFlag, location)
		if err != nil {
			fatal(err)
		}
	}
	records, err := store.ReadAll[journal.TradeRecord](*root, "trade_journal")
	if err != nil {
		fatal(err)
	}
	report := journal.BuildDaily(records, date, len(universe.All()))
	if *jsonFlag {
		_ = json.NewEncoder(os.Stdout).Encode(report)
		return
	}
	fmt.Printf("OVERNIGHT CONTROL REPORT %s | %s\n", report.SessionDate, report.StrategyVersion)
	fmt.Printf("Coverage %d/%d (%.1f%%) | Plans %d | Filled %d | No fill %d | Open %d\n", report.Records, report.ExpectedMarkets, report.Coverage*100, report.Plans, report.Filled, report.NoFill, report.Open)
	fmt.Printf("Wins %d | Losses %d | Scratch %d | Win rate %.1f%% | Total %+.2fR | Avg/fill %+.2fR | Slippage %.2fbps\n\n", report.Wins, report.Losses, report.Scratches, report.WinRate*100, report.TotalR, report.AverageRPerFill, report.AverageSlippageBPS)
	for _, row := range report.Assets {
		fmt.Printf("%2d. %-5s %-16s %+.2fR | MFE %.2fR | MAE %.2fR | %s\n", row.Rank, row.Symbol, row.Outcome, row.RMultiple, row.MFER, row.MAER, row.Mode)
	}
	if len(report.LivePaper) > 0 {
		fmt.Println("\nLIVE VS PAPER")
		for _, row := range report.LivePaper {
			fmt.Printf("%-5s paper %+.2fR (%s) | live %+.2fR (%s) | difference %+.2fR | slippage %.2fbps\n", row.Symbol, row.PaperR, row.PaperOutcome, row.LiveR, row.LiveOutcome, row.ExecutionDifferenceR, row.LiveSlippageBPS)
		}
	}
}

func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
