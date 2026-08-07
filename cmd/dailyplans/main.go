package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/execution"
	"github.com/ogtrading/overnight-strategy/internal/live"
	lighterdata "github.com/ogtrading/overnight-strategy/internal/marketdata/lighter"
	"github.com/ogtrading/overnight-strategy/internal/models"
	"github.com/ogtrading/overnight-strategy/internal/store"
	"github.com/ogtrading/overnight-strategy/internal/universe"
)

func main() {
	root := flag.String("store", "data/research", "research event store")
	dryRun := flag.Bool("dry-run", true, "generate and validate intents without submitting orders")
	equity := flag.Float64("paper-equity", 100, "paper account equity in USD")
	flag.Parse()
	mode := "DRY_RUN"
	if !*dryRun {
		mode = string(execution.Paper)
	}
	location, err := time.LoadLocation("America/Chicago")
	if err != nil {
		fatal(err)
	}
	events, err := store.NewJSONL(*root)
	if err != nil {
		fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	client := lighterdata.New(os.Getenv("LIGHTER_BASE_URL"), nil)
	markets, err := client.MarketMap(ctx)
	if err != nil {
		fatal(err)
	}
	end := time.Now().UTC().Truncate(5 * time.Minute)
	start := end.Add(-72 * time.Hour)
	intents := []live.Intent{}
	paperTrades := []execution.PaperTrade{}
	riskPerTrade, basketRisk, err := execution.DefaultRiskLimits().Budget(*equity, len(universe.Live()))
	if err != nil {
		fatal(err)
	}
	existingEvents, err := store.ReadAll[live.Intent](*root, "strategy_intents")
	if err != nil {
		fatal(err)
	}
	existing := live.LatestIntents(existingEvents)
	for _, asset := range universe.All() {
		market, ok := markets[asset.Symbol]
		if !ok {
			fatal(fmt.Errorf("market %s missing", asset.Symbol))
		}
		candles, err := client.Candles(ctx, market.MarketID, "5m", start, end)
		if err != nil {
			fatal(fmt.Errorf("%s: %w", asset.Symbol, err))
		}
		snapshot, err := live.BuildMarketSnapshot(asset.Symbol, candles, location)
		if err != nil {
			fatal(fmt.Errorf("%s snapshot: %w", asset.Symbol, err))
		}
		if err := events.Append("market_snapshots", snapshot); err != nil {
			fatal(err)
		}
		if snapshot.Plan != nil && snapshot.Plan.Valid {
			intent, err := live.BuildIntent(asset.Symbol, *snapshot.Plan, riskPerTrade)
			if err != nil {
				fatal(err)
			}
			intent = live.MarkDryRun(intent)
			intents = append(intents, intent)
			if _, alreadyGenerated := existing[intent.ID]; !alreadyGenerated {
				if err := events.Append("strategy_intents", intent); err != nil {
					fatal(err)
				}
			}
			if !*dryRun {
				if err := execution.GateFromEnvironment(execution.Paper).Authorize(asset.Symbol, time.Now()); err != nil {
					fatal(err)
				}
				spec, err := execution.SpecFromMarket(market)
				if err != nil {
					fatal(err)
				}
				side := "BUY"
				if snapshot.Plan.Direction == "SHORT" {
					side = "SELL"
				}
				expiry := time.Date(snapshot.SessionDate.Year(), snapshot.SessionDate.Month(), snapshot.SessionDate.Day(), 16, 0, 0, 0, location)
				order := spec.Normalize(execution.Order{Symbol: asset.Symbol, Side: side, Price: snapshot.Plan.Entry, Quantity: intent.Quantity, Stop: snapshot.Plan.Stop, TP1: snapshot.Plan.TP1, TP2: snapshot.Plan.TP2, ExpiresAt: expiry.Unix()})
				if err := spec.Validate(order); err != nil {
					fatal(fmt.Errorf("%s precision: %w", asset.Symbol, err))
				}
				paper, err := execution.Simulate(order, executionWindow(candles, snapshot.SessionDate, location))
				if err != nil {
					fatal(err)
				}
				paperTrades = append(paperTrades, paper)
				if err := events.Append("paper_trades", paper); err != nil {
					fatal(err)
				}
			}
		}
		printJSON(map[string]any{"symbol": asset.Symbol, "classification": asset.Classification, "order_authorized": snapshot.OrderAuthorized, "plan": snapshot.Plan})
	}
	if err := live.ValidateBasket(intents, live.DefaultRiskPolicy()); err != nil {
		fatal(err)
	}
	printJSON(map[string]any{"status": "PASS", "mode": mode, "market_maps": len(universe.All()), "intents": len(intents), "paper_trades": len(paperTrades), "risk_per_trade_usd": riskPerTrade, "basket_risk_limit_usd": basketRisk, "automated_orders": false})
}

func printJSON(value any) { _ = json.NewEncoder(os.Stdout).Encode(value) }
func fatal(err error)     { fmt.Fprintln(os.Stderr, err); os.Exit(1) }

func executionWindow(candles []models.Candle, sessionDate time.Time, location *time.Location) []models.Candle {
	start := time.Date(sessionDate.Year(), sessionDate.Month(), sessionDate.Day(), 5, 0, 0, 0, location)
	end := time.Date(sessionDate.Year(), sessionDate.Month(), sessionDate.Day(), 16, 0, 0, 0, location)
	out := make([]models.Candle, 0)
	for _, candle := range candles {
		local := candle.OpenTime.In(location)
		if !local.Before(start) && local.Before(end) {
			out = append(out, candle)
		}
	}
	return out
}
