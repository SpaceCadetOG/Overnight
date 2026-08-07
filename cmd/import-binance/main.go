package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/marketdata/binanceimport"
	"github.com/ogtrading/overnight-strategy/internal/marketdata/cache"
	"github.com/ogtrading/overnight-strategy/internal/resample"
)

func main() {
	input := flag.String(
		"input",
		"",
		"path to the merged Binance one-minute CSV",
	)

	output := flag.String(
		"output",
		"data/raw/Binance_BTCUSDT_5m.csv",
		"normalized five-minute output CSV",
	)

	flag.Parse()

	if *input == "" {
		log.Fatal("-input is required")
	}

	fmt.Printf("Reading Binance CSV: %s\n", filepath.Clean(*input))

	started := time.Now()

	oneMinute, stats, err := binanceimport.ReadCandlesCSV(*input)
	if err != nil {
		log.Fatalf("read Binance candles: %v", err)
	}

	fmt.Printf("Rows read:            %d\n", stats.RowsRead)
	fmt.Printf("Valid 1m candles:     %d\n", stats.CandlesParsed)
	fmt.Printf("Malformed rows:       %d\n", stats.MalformedRows)
	fmt.Printf("Duplicate timestamps: %d\n", stats.DuplicateRows)

	fmt.Printf(
		"1m history:           %s through %s\n",
		oneMinute[0].OpenTime.Format(time.RFC3339),
		oneMinute[len(oneMinute)-1].OpenTime.Format(time.RFC3339),
	)

	fmt.Println("Resampling one-minute candles to five minutes...")

	fiveMinute, err := resample.Candles(oneMinute, 5*time.Minute)
	if err != nil {
		log.Fatalf("resample Binance candles: %v", err)
	}

	if err := cache.WriteCandlesCSV(*output, fiveMinute); err != nil {
		log.Fatalf("write five-minute Binance CSV: %v", err)
	}

	fileInfo, err := os.Stat(*output)
	if err != nil {
		log.Fatalf("stat output file: %v", err)
	}

	fmt.Printf("5m candles written:   %d\n", len(fiveMinute))
	fmt.Printf(
		"5m history:           %s through %s\n",
		fiveMinute[0].OpenTime.Format(time.RFC3339),
		fiveMinute[len(fiveMinute)-1].OpenTime.Format(time.RFC3339),
	)
	fmt.Printf("Output:               %s\n", filepath.Clean(*output))
	fmt.Printf("Output size:          %.1f MB\n", float64(fileInfo.Size())/(1024*1024))
	fmt.Printf("Elapsed:              %s\n", time.Since(started).Round(time.Second))
}
