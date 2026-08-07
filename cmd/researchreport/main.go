package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/ogtrading/overnight-strategy/internal/live"
	"github.com/ogtrading/overnight-strategy/internal/store"
)

func main() {
	root := flag.String("store", "data/research", "research event store")
	flag.Parse()
	snapshots, err := store.ReadAll[live.MarketSnapshot](*root, "market_snapshots")
	if err != nil {
		fatal(err)
	}
	intents, err := store.ReadAll[live.Intent](*root, "strategy_intents")
	if err != nil {
		fatal(err)
	}
	results, err := store.ReadAll[live.RecordedResult](*root, "trade_results")
	if err != nil {
		fatal(err)
	}
	reports := live.BuildAssetReports(snapshots, intents)
	output := map[string]any{"assets": reports, "rankings": live.RankResults(results), "market_maps": len(snapshots), "strategy_intents": len(intents), "trade_results": len(results), "automated_orders": false}
	if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
		fatal(err)
	}
}

func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
