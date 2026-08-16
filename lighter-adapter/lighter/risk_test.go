package lighter

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func testRiskConfig() RiskConfig {
	return RiskConfig{
		AllowedSymbols: []string{"BTC", "ETH"}, MaxOrderNotional: "1000",
		MaxPortfolioExposure: "2000", MaxSymbolExposure: map[string]string{"BTC": "1500", "ETH": "1000"},
		MinAvailableCollateral: "100", MaxDailyLoss: "100", MaxRiskFraction: "0.01",
	}
}

func testRiskMarketManager(t *testing.T) (*Manager, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":200,"order_book_details":[{"symbol":"BTC","market_id":1,"market_type":"perp","status":"active","min_base_amount":"0.001","min_quote_amount":"10","supported_size_decimals":3,"supported_price_decimals":2},{"symbol":"ETH","market_id":0,"market_type":"perp","status":"active","min_base_amount":"0.01","min_quote_amount":"10","supported_size_decimals":2,"supported_price_decimals":2}]}`)
	}))
	return testManager(server), server.Close
}

func TestRiskRejectsDisallowedAndOverexposedOrders(t *testing.T) {
	risk, err := NewRiskManager(testRiskConfig(), filepath.Join(t.TempDir(), "risk.json"))
	if err != nil {
		t.Fatal(err)
	}
	manager, closeServer := testRiskMarketManager(t)
	defer closeServer()
	portfolio := PortfolioSnapshot{Collateral: "5000", AvailableCollateral: "5000", GrossExposure: "1400"}

	_, err = risk.ValidateOrder(t.Context(), manager, portfolio, PlaceOrderRequest{
		Symbol: "SOL", Side: SideBuy, Quantity: 1, Price: 100, Type: ExecutionOrderLimit,
	})
	if !errors.Is(err, ErrRiskRejected) {
		t.Fatalf("disallowed error=%v", err)
	}
	_, err = risk.ValidateOrder(t.Context(), manager, portfolio, PlaceOrderRequest{
		Symbol: "BTC", Side: SideBuy, Quantity: 0.011, Price: 60000, StopPrice: 59000, Type: ExecutionOrderLimit,
	})
	if !errors.Is(err, ErrRiskRejected) {
		t.Fatalf("exposure error=%v", err)
	}
}

func TestRiskReduceOnlyCannotOverCloseOrIncrease(t *testing.T) {
	risk, err := NewRiskManager(testRiskConfig(), filepath.Join(t.TempDir(), "risk.json"))
	if err != nil {
		t.Fatal(err)
	}
	manager, closeServer := testRiskMarketManager(t)
	defer closeServer()
	portfolio := PortfolioSnapshot{
		AvailableCollateral: "500", GrossExposure: "600",
		Positions: []CanonicalPosition{{MarketID: 1, Symbol: "BTC", Side: PositionSideLong, Size: "0.01", PositionValue: "600"}},
	}

	for _, request := range []PlaceOrderRequest{
		{Symbol: "BTC", Side: SideBuy, Quantity: 0.005, Price: 60000, Type: ExecutionOrderLimit, ReduceOnly: true},
		{Symbol: "BTC", Side: SideSell, Quantity: 0.011, Price: 60000, Type: ExecutionOrderLimit, ReduceOnly: true},
	} {
		if _, err := risk.ValidateOrder(t.Context(), manager, portfolio, request); !errors.Is(err, ErrRiskRejected) {
			t.Fatalf("request=%+v error=%v", request, err)
		}
	}
	if err := risk.EngageKillSwitch("test emergency"); err != nil {
		t.Fatal(err)
	}
	portfolio.AvailableCollateral = "0"
	decision, err := risk.ValidateOrder(t.Context(), manager, portfolio, PlaceOrderRequest{
		Symbol: "BTC", Side: SideSell, Quantity: 0.005, Price: 60000, Type: ExecutionOrderLimit, ReduceOnly: true,
	})
	if err != nil || !decision.Approved {
		t.Fatalf("decision=%+v error=%v", decision, err)
	}
}

func TestRiskFractionIsEnforcedForEveryEntry(t *testing.T) {
	risk, err := NewRiskManager(testRiskConfig(), filepath.Join(t.TempDir(), "risk.json"))
	if err != nil {
		t.Fatal(err)
	}
	manager, closeServer := testRiskMarketManager(t)
	defer closeServer()
	portfolio := PortfolioSnapshot{Collateral: "1000", AvailableCollateral: "1000", GrossExposure: "0"}
	approved, err := risk.ValidateOrder(t.Context(), manager, portfolio, PlaceOrderRequest{Symbol: "BTC", Side: SideBuy, Quantity: 0.01, Price: 60000, StopPrice: 59950, Type: ExecutionOrderLimit})
	if err != nil || !approved.Approved || approved.RiskAmount != "0.500000000000000000" || approved.RiskBudget != "10.000000000000000000" {
		t.Fatalf("approved=%+v err=%v", approved, err)
	}
	_, err = risk.ValidateOrder(t.Context(), manager, portfolio, PlaceOrderRequest{Symbol: "BTC", Side: SideBuy, Quantity: 0.01, Price: 60000, StopPrice: 58000, Type: ExecutionOrderLimit})
	if !errors.Is(err, ErrRiskRejected) {
		t.Fatalf("risk error=%v", err)
	}
	_, err = risk.ValidateOrder(t.Context(), manager, portfolio, PlaceOrderRequest{Symbol: "BTC", Side: SideBuy, Quantity: 0.01, Price: 60000, Type: ExecutionOrderLimit})
	if !errors.Is(err, ErrRiskRejected) {
		t.Fatalf("missing stop error=%v", err)
	}
}

func TestDailyLossKillSwitchPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "risk.json")
	risk, err := NewRiskManager(testRiskConfig(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := risk.RecordRealizedPnL("-100"); err != nil {
		t.Fatal(err)
	}
	if !risk.State().KillSwitch {
		t.Fatal("daily loss did not engage kill switch")
	}
	if err := risk.ResetKillSwitch(); err == nil {
		t.Fatal("daily loss kill switch was reset during the same day")
	}
	reopened, err := NewRiskManager(testRiskConfig(), path)
	if err != nil {
		t.Fatal(err)
	}
	if state := reopened.State(); !state.KillSwitch || state.KillReason == "" {
		t.Fatalf("state=%+v", state)
	}
}

func TestSizeForRisk(t *testing.T) {
	risk, err := NewRiskManager(testRiskConfig(), filepath.Join(t.TempDir(), "risk.json"))
	if err != nil {
		t.Fatal(err)
	}
	size, err := risk.SizeForRisk("10000", "60000", "59900")
	if err != nil {
		t.Fatal(err)
	}
	if size != "1.000000000000000000" {
		t.Fatalf("size=%s", size)
	}
}
