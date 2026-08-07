package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/marketdata/binance"
	"github.com/ogtrading/overnight-strategy/internal/marketdata/cache"
)

func main() {
	symbol := flag.String("symbol", "BTCUSDT", "Binance USD-M futures symbol")
	interval := flag.String("interval", "5m", "candle interval")
	startText := flag.String("start", "2022-01-01", "inclusive UTC start date")
	endText := flag.String("end", "2026-08-01", "exclusive UTC end date")
	output := flag.String("output", "", "optional output CSV path")
	flag.Parse()

	start, err := time.Parse("2006-01-02", *startText)
	if err != nil {
		log.Fatalf("parse start: %v", err)
	}

	end, err := time.Parse("2006-01-02", *endText)
	if err != nil {
		log.Fatalf("parse end: %v", err)
	}

	outputPath := *output
	if outputPath == "" {
		outputPath = filepath.Join(
			"data",
			"raw",
			fmt.Sprintf("Binance_%s_%s.csv", *symbol, *interval),
		)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	fmt.Printf(
		"Downloading Binance %s %s candles from %s through %s...\n",
		*symbol,
		*interval,
		start.Format("2006-01-02"),
		end.Add(-time.Nanosecond).Format("2006-01-02"),
	)

	candles, err := binance.NewClient().DownloadCandles(
		ctx,
		*symbol,
		*interval,
		start,
		end,
	)
	if err != nil {
		log.Fatalf("download candles: %v", err)
	}

	if len(candles) == 0 {
		log.Fatal("download returned no candles")
	}

	if err := cache.WriteCandlesCSV(outputPath, candles); err != nil {
		log.Fatalf("write candles: %v", err)
	}

	fmt.Printf("Downloaded %d candles\n", len(candles))
	fmt.Printf(
		"History: %s through %s\n",
		candles[0].OpenTime.Format(time.RFC3339),
		candles[len(candles)-1].OpenTime.Format(time.RFC3339),
	)
	fmt.Printf("Saved: %s\n", outputPath)
}
