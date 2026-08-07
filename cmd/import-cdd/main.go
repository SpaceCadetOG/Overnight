package main

import (
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/marketdata/cache"
	"github.com/ogtrading/overnight-strategy/internal/marketdata/cryptodatadownload"
	"github.com/ogtrading/overnight-strategy/internal/resample"
)

func main() {
	input := flag.String(
		"input",
		"",
		"path to the downloaded CryptoDataDownload CSV",
	)

	output := flag.String(
		"output",
		"data/raw/CDD_Coinbase_BTCUSD_5m.csv",
		"normalized five-minute output CSV",
	)

	keepOneMinute := flag.String(
		"output-1m",
		"data/raw/CDD_Coinbase_BTCUSD_1m.csv",
		"normalized one-minute output CSV; empty disables",
	)

	flag.Parse()

	if *input == "" {
		log.Fatal("-input is required")
	}

	fmt.Printf("Reading CryptoDataDownload file: %s\n", *input)

	oneMinute, err := cryptodatadownload.ReadCandlesCSV(*input)
	if err != nil {
		log.Fatalf("read CryptoDataDownload data: %v", err)
	}

	if len(oneMinute) == 0 {
		log.Fatal("input contained no candles")
	}

	fmt.Printf("Parsed one-minute candles: %d\n", len(oneMinute))
	fmt.Printf(
		"History: %s through %s\n",
		oneMinute[0].OpenTime.Format(time.RFC3339),
		oneMinute[len(oneMinute)-1].OpenTime.Format(time.RFC3339),
	)

	if *keepOneMinute != "" {
		if err := cache.WriteCandlesCSV(*keepOneMinute, oneMinute); err != nil {
			log.Fatalf("write normalized one-minute CSV: %v", err)
		}

		fmt.Printf(
			"Saved normalized 1m: %s\n",
			filepath.Clean(*keepOneMinute),
		)
	}

	fiveMinute, err := resample.Candles(oneMinute, 5*time.Minute)
	if err != nil {
		log.Fatalf("resample to five minutes: %v", err)
	}

	if err := cache.WriteCandlesCSV(*output, fiveMinute); err != nil {
		log.Fatalf("write normalized five-minute CSV: %v", err)
	}

	fmt.Printf("Created five-minute candles: %d\n", len(fiveMinute))
	fmt.Printf("Saved normalized 5m: %s\n", filepath.Clean(*output))
}
