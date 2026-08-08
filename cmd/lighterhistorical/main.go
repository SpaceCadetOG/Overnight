package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/marketdata/cache"
	lighterdata "github.com/ogtrading/overnight-strategy/internal/marketdata/lighter"
	"github.com/ogtrading/overnight-strategy/internal/universe"
)

func main() {
	days := flag.Int("days", 2, "number of UTC days to collect")
	out := flag.String("out", "data/raw/lighter", "output directory")
	symbolsFlag := flag.String("symbols", "", "optional comma-separated market symbols; default is the configured universe")
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
	symbols := make([]string, 0)
	if strings.TrimSpace(*symbolsFlag) == "" {
		for _, asset := range universe.All() {
			symbols = append(symbols, asset.MarketSymbol())
		}
	} else {
		for _, symbol := range strings.Split(*symbolsFlag, ",") {
			if symbol = strings.ToUpper(strings.TrimSpace(symbol)); symbol != "" {
				symbols = append(symbols, symbol)
			}
		}
	}
	for _, symbol := range symbols {
		market, ok := markets[symbol]
		if !ok {
			fatal(fmt.Errorf("Lighter market %s not found", symbol))
		}
		candles, err := client.Candles(ctx, market.MarketID, "5m", start, end)
		if err != nil {
			fatal(fmt.Errorf("%s candles: %w", symbol, err))
		}
		path := filepath.Join(*out, fmt.Sprintf("Lighter_%s_5m.csv", symbol))
		if err := cache.WriteCandlesCSV(path, candles); err != nil {
			fatal(err)
		}
		funding, err := client.RawFundings(ctx, market.MarketID, start, end)
		if err != nil {
			fatal(fmt.Errorf("%s funding: %w", symbol, err))
		}
		if err := writeJSON(filepath.Join(*out, fmt.Sprintf("Lighter_%s_funding.json", symbol)), funding); err != nil {
			fatal(err)
		}
		_, configured := universe.Find(symbol)
		fmt.Printf("%s candles=%d market_id=%d configured=%t\n", symbol, len(candles), market.MarketID, configured)
	}
}

func writeJSON(path string, raw json.RawMessage) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o640)
}

func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
