package main

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	adapter "github.com/ogtrading/lighter-adapter/lighter"
	"github.com/ogtrading/overnight-strategy/internal/execution"
	"github.com/ogtrading/overnight-strategy/internal/lighterexec"
)

func TestLiveAssetMinimumIsSkippableAndDoesNotBecomeSubmissionFailure(t *testing.T) {
	status, skippable := classifyLiveAssetError(errors.New("notional 7.03 below minimum 10.00"))
	if status != "SKIPPED_MIN_NOTIONAL" || !skippable {
		t.Fatalf("status=%s skippable=%v", status, skippable)
	}
	status, skippable = classifyLiveAssetError(errors.New("HTTP 503"))
	if status != "FAILED_SUBMISSION" || skippable {
		t.Fatalf("infrastructure status=%s skippable=%v", status, skippable)
	}
	status, skippable = classifyLiveAssetError(&adapter.ExecutionError{Kind: adapter.ErrorTimeout, Operation: "risk portfolio", Retryable: true, Err: errors.New("timeout")})
	if status != "FAILED_SUBMISSION" || skippable {
		t.Fatalf("portfolio infrastructure failure status=%s skippable=%v", status, skippable)
	}
}

func TestRiskHierarchyRejectsAdapterCapBelowFrozenStrategy(t *testing.T) {
	setRiskEnvironment(t, "0.001")
	if _, err := riskConfigFromEnv(); err == nil || !strings.Contains(err.Error(), "conflicts with frozen strategy risk") {
		t.Fatalf("expected explicit hierarchy conflict, got %v", err)
	}
	setRiskEnvironment(t, "0.005")
	if _, err := riskConfigFromEnv(); err != nil {
		t.Fatalf("matching circuit breaker rejected: %v", err)
	}
}

func setRiskEnvironment(t *testing.T, fraction string) {
	t.Helper()
	values := map[string]string{
		"LIGHTER_MAX_ORDER_NOTIONAL": "11", "LIGHTER_MAX_PORTFOLIO_EXPOSURE": "20",
		"LIGHTER_BTC_MAX_EXPOSURE": "12", "LIGHTER_ETH_MAX_EXPOSURE": "12",
		"LIGHTER_MIN_AVAILABLE_COLLATERAL": "10", "LIGHTER_MAX_DAILY_LOSS": "1",
		"LIGHTER_MAX_RISK_FRACTION": fraction,
	}
	for key, value := range values {
		old, present := os.LookupEnv(key)
		if err := os.Setenv(key, value); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if present {
				_ = os.Setenv(key, old)
			} else {
				_ = os.Unsetenv(key)
			}
		})
	}
}

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

func TestCycle1FundedEntryBoundary(t *testing.T) {
	if cycle1EntryAuthorized(cycle1StartUTC.Add(-time.Nanosecond)) {
		t.Fatal("funded entry authorized before cycle start")
	}
	if !cycle1EntryAuthorized(cycle1StartUTC) || !cycle1EntryAuthorized(cycle1EndUTC.Add(-time.Nanosecond)) {
		t.Fatal("funded entry rejected inside cycle")
	}
	if cycle1EntryAuthorized(cycle1EndUTC) {
		t.Fatal("funded entry authorized at cycle end")
	}
}

func TestNextPlanUTC(t *testing.T) {
	location, _ := time.LoadLocation("America/Chicago")
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	if got := nextPlanUTC(now, location); !got.Equal(time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("next plan=%s", got)
	}
}
