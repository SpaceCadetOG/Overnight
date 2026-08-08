package execution

import (
	"fmt"
	"testing"
	"time"
)

type lifecycleExecutor struct {
	orders   []OrderRequest
	canceled []string
	closes   int
}

func (e *lifecycleExecutor) Submit(req OrderRequest) (OrderResponse, error) {
	e.orders = append(e.orders, req)
	return OrderResponse{OrderID: fmt.Sprintf("order-%d", len(e.orders))}, nil
}
func (e *lifecycleExecutor) Cancel(id string) error   { e.canceled = append(e.canceled, id); return nil }
func (*lifecycleExecutor) GetPosition(string) float64 { return 0 }
func (e *lifecycleExecutor) Close(string, string, float64, float64) (OrderResponse, error) {
	e.closes++
	return OrderResponse{}, nil
}

func TestFrozenLifecycleLongAndShort(t *testing.T) {
	for _, direction := range []string{"LONG", "SHORT"} {
		entry, stop, tp1, tp2 := 100.0, 90.0, 110.0, 120.0
		if direction == "SHORT" {
			stop, tp1, tp2 = 110, 90, 80
		}
		trade, err := NewManagedTrade("BTC", direction, 2, entry, stop, tp1, tp2, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		exec := &lifecycleExecutor{}
		if err := trade.OnEntryFilled(exec); err != nil {
			t.Fatal(err)
		}
		if len(exec.orders) != 2 || exec.orders[0].Size != 2 || exec.orders[1].Size != 1 {
			t.Fatalf("wrong initial protection: %+v", exec.orders)
		}
		if err := trade.OnTP1Filled(exec); err != nil {
			t.Fatal(err)
		}
		if len(exec.orders) != 4 || exec.orders[2].Price != entry || exec.orders[2].Size != 1 || exec.orders[3].Price != tp2 {
			t.Fatalf("wrong runner protection: %+v", exec.orders)
		}
		if len(exec.canceled) != 1 || trade.State != ProtectionRunner {
			t.Fatalf("wrong lifecycle state: %+v", trade)
		}
		if err := trade.OnTP1Filled(exec); err != nil || len(exec.orders) != 4 {
			t.Fatal("TP1 handling is not idempotent")
		}
	}
}

func TestResearchAssetCannotBecomeManagedLiveTrade(t *testing.T) {
	if _, err := NewManagedTrade("SOL", "LONG", 1, 100, 90, 110, 120, time.Now().Add(time.Hour)); err == nil {
		t.Fatal("expected live authority rejection")
	}
}

func TestExpiryCancelsWaitingEntryOrFlattensPosition(t *testing.T) {
	waiting, _ := NewManagedTrade("ETH", "LONG", 2, 100, 90, 110, 120, time.Now().Add(time.Hour))
	_ = waiting.SetEntryOrderID("entry")
	exec := &lifecycleExecutor{}
	if err := waiting.OnExpiry(exec, 0, 0); err != nil {
		t.Fatal(err)
	}
	if len(exec.canceled) != 1 || waiting.State != ProtectionClosed {
		t.Fatalf("waiting entry not canceled: %+v", waiting)
	}

	open, _ := NewManagedTrade("ETH", "LONG", 2, 100, 90, 110, 120, time.Now().Add(time.Hour))
	if err := open.OnEntryFilled(exec); err != nil {
		t.Fatal(err)
	}
	if err := open.OnExpiry(exec, 2, 99); err != nil {
		t.Fatal(err)
	}
	if exec.closes != 1 || open.State != ProtectionClosed {
		t.Fatalf("open position not flattened: %+v", open)
	}
}

func TestQuantitySplitPreservesExactExchangeUnits(t *testing.T) {
	trade, err := NewManagedTrade("ETH", "LONG", .0123, 100, 90, 110, 120, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if trade.TP1Quantity != .0061 || trade.RunnerQuantity != .0062 || trade.TP1Quantity+trade.RunnerQuantity != trade.Quantity {
		t.Fatalf("invalid exact split: %+v", trade)
	}
}
