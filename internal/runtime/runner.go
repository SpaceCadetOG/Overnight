package runtime

import (
	"fmt"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/live"
	"github.com/ogtrading/overnight-strategy/internal/models"
	"github.com/ogtrading/overnight-strategy/internal/universe"
)

type Runner struct {
	Location *time.Location
	RiskUSD  float64
}

type Result struct {
	Snapshot live.MarketSnapshot
	Intent   *live.Intent
	Route    universe.Destination
}

func NewRunner(location *time.Location, riskUSD float64) *Runner {
	return &Runner{
		Location: location,
		RiskUSD:  riskUSD,
	}
}

func (r *Runner) RunAsset(
	symbol string,
	candles []models.Candle,
) (Result, error) {

	snapshot, err := live.BuildMarketSnapshot(
		symbol,
		candles,
		r.Location,
	)

	if err != nil {
		return Result{}, err
	}

	result := Result{
		Snapshot: snapshot,
	}

	route, err := universe.Resolve(symbol)

	if err != nil {
		return Result{}, err
	}

	result.Route = route

	if snapshot.Plan == nil {
		return result, nil
	}

	intent, err := live.BuildIntent(
		symbol,
		*snapshot.Plan,
		r.RiskUSD,
	)

	if err != nil {
		return Result{}, err
	}

	result.Intent = &intent

	return result, nil
}

func PrintResult(result Result) {

	fmt.Println()
	fmt.Println(result.Snapshot.Symbol)
	fmt.Println("----------------")

	if result.Snapshot.Plan == nil {
		fmt.Println("NO PLAN")
		fmt.Println("ORDER AUTHORIZED:", result.Snapshot.OrderAuthorized)
		return
	}

	fmt.Println(
		"DIRECTION:",
		result.Snapshot.Plan.Direction,
	)

	fmt.Println(
		"ENTRY:",
		result.Snapshot.Plan.Entry,
	)

	fmt.Println(
		"STOP:",
		result.Snapshot.Plan.Stop,
	)

	fmt.Println(
		"TP1:",
		result.Snapshot.Plan.TP1,
	)

	fmt.Println(
		"TP2:",
		result.Snapshot.Plan.TP2,
	)

	fmt.Println(
		"ROUTE:",
		result.Route,
	)

	fmt.Println(
		"INTENT:",
		result.Intent.State,
	)
}
