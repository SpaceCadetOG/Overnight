package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/marketdata/cache"
	lighterdata "github.com/ogtrading/overnight-strategy/internal/marketdata/lighter"
	"github.com/ogtrading/overnight-strategy/internal/universe"
)

func main() {
	days := flag.Int("days", 2, "number of UTC days to collect")
	out := flag.String("out", "data/raw/lighter", "output directory")
	flag.Parse()
	if *days <= 0 {
		fatal(fmt.Errorf("days must be positive"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	client := lighterdata.New(os.Getenv("LIGHTER_BASE_URL"), nil)
	markets, err := client.MarketMap(ctx)
	if err != nil {
		fatal(err)
	}
	end := time.Now().UTC().Truncate(5 * time.Minute)
	start := end.AddDate(0, 0, -*days)
	for _, asset := range universe.All() {
		market, ok := markets[asset.Symbol]
		if !ok {
			fatal(fmt.Errorf("Lighter market %s not found", asset.Symbol))
		}
		candles, err := client.Candles(ctx, market.MarketID, "5m", start, end)
		if err != nil {
			fatal(fmt.Errorf("%s candles: %w", asset.Symbol, err))
		}
		path := filepath.Join(*out, fmt.Sprintf("Lighter_%s_5m.csv", asset.Symbol))
		if err := cache.WriteCandlesCSV(path, candles); err != nil {
			fatal(err)
		}
		funding, err := client.RawFundings(ctx, market.MarketID, start, end)
		if err != nil {
			fatal(fmt.Errorf("%s funding: %w", asset.Symbol, err))
		}
		if err := writeJSON(filepath.Join(*out, fmt.Sprintf("Lighter_%s_funding.json", asset.Symbol)), funding); err != nil {
			fatal(err)
		}
		fmt.Printf("%s candles=%d market_id=%d research_only=%t\n", asset.Symbol, len(candles), market.MarketID, asset.ResearchOnly)
	}
}

func writeJSON(path string, raw json.RawMessage) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o640)
}

func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
