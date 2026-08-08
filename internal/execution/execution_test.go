package execution

import (
	"encoding/json"
	"testing"
	"time"

	lighterdata "github.com/ogtrading/overnight-strategy/internal/marketdata/lighter"
	"github.com/ogtrading/overnight-strategy/internal/models"
)

func TestPrecisionRejectsBelowMinimum(t *testing.T) {
	market := lighterdata.Market{Symbol: "BTC", MarketID: 1, Status: "active", PriceDecimals: 1, SizeDecimals: 5, MinBaseAmount: json.RawMessage(`"0.0001"`), MinQuoteAmount: json.RawMessage(`"10"`)}
	spec, err := SpecFromMarket(market)
	if err != nil {
		t.Fatal(err)
	}
	order := spec.Normalize(Order{Symbol: "BTC", Side: "BUY", Price: 60000.12, Quantity: 0.0001, Stop: 59000, TP1: 61000, TP2: 62000, ExpiresAt: time.Now().Add(time.Hour).Unix()})
	if err := spec.Validate(order); err == nil {
		t.Fatal("below-minimum notional accepted")
	}
}

func TestKillSwitchOverridesApproval(t *testing.T) {
	gate := Gate{Mode: Live, KillSwitch: true, AllowedSymbols: map[string]bool{"BTC": true}}
	if err := gate.Authorize("BTC", time.Now()); err == nil {
		t.Fatal("kill switch did not block")
	}
}

func TestLiveGateUsesCanonicalAllowlist(t *testing.T) {
	gate := Gate{Mode: Live, AllowedSymbols: map[string]bool{"BTC": true, "ETH": true}}
	if err := gate.Authorize("BTC", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := gate.Authorize("ETH", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := gate.Authorize("SOL", time.Now()); err == nil {
		t.Fatal("unstaged SOL accepted")
	}
}

func TestRiskBudgetMatchesTwoAssetLiveRollout(t *testing.T) {
	perTrade, basket, err := DefaultRiskLimits().Budget(100, 2)
	if err != nil {
		t.Fatal(err)
	}
	if perTrade != 0.5 || basket != 2 {
		t.Fatalf("perTrade=%v basket=%v", perTrade, basket)
	}
}

func TestPaperSimulation(t *testing.T) {
	now := time.Now().UTC()
	order := Order{Symbol: "BTC", Side: "BUY", Price: 100, Stop: 90, TP1: 110, TP2: 120, ExpiresAt: now.Add(time.Hour).Unix()}
	trade, err := Simulate(order, []models.Candle{{OpenTime: now, CloseTime: now.Add(5 * time.Minute), Open: 105, High: 121, Low: 99, Close: 120, Volume: 1}})
	if err != nil {
		t.Fatal(err)
	}
	// Conservative intrabar ordering checks the protective stop before targets;
	// this candle never reaches the stop and therefore completes at TP2.
	if trade.Outcome != "TP2" {
		t.Fatalf("trade=%+v", trade)
	}
}
