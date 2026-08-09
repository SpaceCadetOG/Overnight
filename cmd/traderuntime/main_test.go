package main

import (
	"errors"
	"testing"

	"github.com/ogtrading/overnight-strategy/internal/execution"
	"github.com/ogtrading/overnight-strategy/internal/lighterexec"
)

func TestRuntimeErrorSummaryHidesHTTPBody(t *testing.T) {
	key, message := runtimeErrorSummary(errors.New("BTC paper: Lighter /api/v1/candles returned HTTP 503: <html><body>Service Temporarily Unavailable</body></html>"))
	if key != "lighter-http-503" || message != "Lighter API temporarily unavailable (HTTP 503)." {
		t.Fatalf("key=%q message=%q", key, message)
	}
}

func TestRuntimeErrorSummaryBoundsUnknownErrors(t *testing.T) {
	_, message := runtimeErrorSummary(errors.New(string(make([]byte, 500))))
	if len(message) > 180 {
		t.Fatalf("message was not bounded: %d", len(message))
	}
}

func TestLiveRiskUsesAuthenticatedAccountEquity(t *testing.T) {
	snapshot := lighterexec.Snapshot{Account: map[string]any{"total_asset_value": "28.949788"}}
	equity := accountEquity(snapshot)
	perTrade, basket, err := execution.DefaultRiskLimits().Budget(equity, 2)
	if err != nil {
		t.Fatal(err)
	}
	if perTrade < .1447 || perTrade > .1448 {
		t.Fatalf("per trade=%v", perTrade)
	}
	if basket < .5789 || basket > .5791 {
		t.Fatalf("basket=%v", basket)
	}
}

func TestAccountEquityFallsBackToCollateral(t *testing.T) {
	snapshot := lighterexec.Snapshot{Account: map[string]any{"total_asset_value": "0", "collateral": "28.949788"}}
	if got := accountEquity(snapshot); got != 28.949788 {
		t.Fatalf("equity=%v", got)
	}
}
