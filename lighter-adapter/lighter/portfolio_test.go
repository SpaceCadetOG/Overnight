package lighter

import (
	"context"
	"testing"
)

type stubExecution struct {
	positions *PositionSnapshot
	orders    []Order
}

func (s *stubExecution) PlaceOrder(context.Context, PlaceOrderRequest) (*OrderSubmission, error) {
	panic("not used")
}
func (s *stubExecution) CancelOrder(context.Context, int64) error { panic("not used") }
func (s *stubExecution) CancelAll(context.Context) error          { panic("not used") }
func (s *stubExecution) GetActiveOrders(context.Context) ([]Order, error) {
	return s.orders, nil
}
func (s *stubExecution) GetOrderStatus(context.Context, int64) (*ReconciledOrder, error) {
	panic("not used")
}
func (s *stubExecution) GetPositions(context.Context) (*PositionSnapshot, error) {
	return s.positions, nil
}
func (s *stubExecution) Reconcile(context.Context) (*RecoveryReport, error) { panic("not used") }

func TestPortfolioSnapshotIncludesPositionsAndOrders(t *testing.T) {
	execution := &stubExecution{
		positions: &PositionSnapshot{
			AccountIndex: 7, Collateral: "2000", AvailableBalance: "1500", TransactionTime: 99,
			Positions: []CanonicalPosition{{MarketID: 1, Symbol: "BTC", Side: PositionSideLong, Size: "0.01", PositionValue: "600"}},
		},
		orders: []Order{{ClientOrderIndex: 5, MarketIndex: 1, RemainingBaseAmount: "0.01", Price: "60000"}},
	}
	manager, err := NewPortfolioManager(execution)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := manager.Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.GrossExposure != "1200.000000000000000000" || snapshot.IsFlat {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	exposure, err := snapshot.SymbolExposure("btc", 1)
	if err != nil {
		t.Fatal(err)
	}
	if canonicalDecimal(exposure) != snapshot.GrossExposure {
		t.Fatalf("symbol exposure=%s", canonicalDecimal(exposure))
	}
	flat := snapshot.VerifyFlat()
	if flat.Flat || len(flat.OpenPositions) != 1 || len(flat.ActiveOrders) != 1 {
		t.Fatalf("flat verification=%+v", flat)
	}
}

func TestPortfolioFlatVerification(t *testing.T) {
	snapshot := PortfolioSnapshot{Positions: []CanonicalPosition{{Symbol: "BTC", Side: PositionSideFlat, Size: "0"}}}
	if result := snapshot.VerifyFlat(); !result.Flat {
		t.Fatalf("result=%+v", result)
	}
}
