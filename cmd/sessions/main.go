package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/marketdata/cache"
	"github.com/ogtrading/overnight-strategy/internal/session"
	"github.com/ogtrading/overnight-strategy/internal/strategy"
)

func main() {
	input := flag.String(
		"input",
		"data/raw/HL_BTC_5m_20260720_20260727.csv",
		"input candle CSV",
	)

	timezone := flag.String(
		"timezone",
		"America/Chicago",
		"session timezone",
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

	sessions, err := session.BuildOvernightSessions(candles, location)
	if err != nil {
		log.Fatalf("build sessions: %v", err)
	}

	fmt.Printf("Loaded candles: %d\n", len(candles))
	fmt.Printf("Complete overnight sessions: %d\n\n", len(sessions))

	for index, overnight := range sessions {
		analyzed, err := strategy.AnalyzeSession(overnight)
		if err != nil {
			log.Fatalf(
				"analyze session %s: %v",
				overnight.Date.Format("2006-01-02"),
				err,
			)
		}

		fmt.Printf(
			"Session %d — %s\n",
			index+1,
			analyzed.Date.Format("2006-01-02"),
		)

		fmt.Printf(
			"  Start:   %s\n",
			analyzed.Start.Format("2006-01-02 15:04 MST"),
		)

		fmt.Printf(
			"  End:     %s\n",
			analyzed.End.Format("2006-01-02 15:04:05 MST"),
		)

		fmt.Printf("  Candles: %d\n", len(analyzed.Candles))
		fmt.Printf("  Open:    %.2f\n", analyzed.Open)
		fmt.Printf("  High:    %.2f\n", analyzed.High)
		fmt.Printf("  Low:     %.2f\n", analyzed.Low)
		fmt.Printf("  Close:   %.2f\n", analyzed.Close)
		fmt.Printf("  VWAP:    %.2f\n", analyzed.VWAP)
		fmt.Printf("  Bias:    %s\n", analyzed.Bias)
		fmt.Printf("  Fib382:  %.2f\n", analyzed.Fib382)
		fmt.Printf("  Fib500:  %.2f\n", analyzed.Fib500)
		fmt.Printf("  Fib618:  %.2f\n", analyzed.Fib618)
		fmt.Printf("  Entry:   %.2f\n", analyzed.Entry)
		fmt.Printf("  POC:     %.2f\n", analyzed.POC)
		fmt.Printf("  VAH:     %.2f\n", analyzed.VAH)
		fmt.Printf("  VAL:     %.2f\n", analyzed.VAL)

		plan := strategy.BuildTradePlan(
			analyzed,
			strategy.DefaultStopBufferBPS,
		)

		fmt.Printf("  Plan:    %s\n", strategy.FormatTradePlan(plan))
		fmt.Println()
	}
}
