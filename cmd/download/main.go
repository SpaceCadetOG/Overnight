package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/marketdata/cache"
	"github.com/ogtrading/overnight-strategy/internal/marketdata/hyperliquid"
)

func main() {
	coin := flag.String("coin", "BTC", "Hyperliquid coin")
	interval := flag.String("interval", "5m", "candle interval")
	days := flag.Int("days", 7, "number of days to download")
	output := flag.String("output", "", "optional CSV path")
	flag.Parse()

	if *days <= 0 {
		log.Fatal("days must be positive")
	}

	end := time.Now().UTC()
	start := end.AddDate(0, 0, -*days)

	outputPath := *output
	if outputPath == "" {
		outputPath = filepath.Join(
			"data",
			"raw",
			fmt.Sprintf(
				"HL_%s_%s_%s_%s.csv",
				*coin,
				*interval,
				start.Format("20060102"),
				end.Format("20060102"),
			),
		)
	}

	client := hyperliquid.NewClient()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		2*time.Minute,
	)
	defer cancel()

	fmt.Printf(
		"Downloading Hyperliquid %s %s candles from %s through %s...\n",
		*coin,
		*interval,
		start.Format(time.RFC3339),
		end.Format(time.RFC3339),
	)

	candles, err := client.DownloadCandles(
		ctx,
		*coin,
		*interval,
		start,
		end,
	)
	if err != nil {
		log.Fatalf("download candles: %v", err)
	}

	if err := cache.WriteCandlesCSV(outputPath, candles); err != nil {
		log.Fatalf("write candle cache: %v", err)
	}

	fmt.Printf("Downloaded %d candles\n", len(candles))
	fmt.Printf("Saved: %s\n", outputPath)
}
