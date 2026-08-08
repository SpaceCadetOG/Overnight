package execution

import (
	"fmt"
	"math"
	"sync"
	"time"

	lightertx "github.com/elliottech/lighter-go/types/txtypes"
)

type ProtectionState string

const (
	ProtectionWaiting ProtectionState = "WAITING_ENTRY_FILL"
	ProtectionInitial ProtectionState = "INITIAL_PROTECTION_ACTIVE"
	ProtectionRunner  ProtectionState = "RUNNER_PROTECTION_ACTIVE"
	ProtectionClosed  ProtectionState = "CLOSED"
)

type ManagedTrade struct {
	Symbol, Direction                                 string
	Quantity, Fill, Stop, TP1, TP2                    float64
	TP1Quantity, RunnerQuantity                       float64
	Expiry                                            time.Time
	State                                             ProtectionState
	EntryOrderID, StopOrderID, TP1OrderID, TP2OrderID string
	mu                                                sync.Mutex
}

func (t *ManagedTrade) SetEntryOrderID(orderID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if orderID == "" {
		return fmt.Errorf("entry order ID is required")
	}
	if t.EntryOrderID != "" && t.EntryOrderID != orderID {
		return fmt.Errorf("entry order ID already set")
	}
	t.EntryOrderID = orderID
	return nil
}

func NewManagedTrade(symbol, direction string, quantity, fill, stop, tp1, tp2 float64, expiry time.Time) (*ManagedTrade, error) {
	if symbol != "BTC" && symbol != "ETH" {
		return nil, fmt.Errorf("%s has no funded execution authority", symbol)
	}
	if direction != "LONG" && direction != "SHORT" {
		return nil, fmt.Errorf("invalid direction %s", direction)
	}
	if quantity <= 0 || fill <= 0 || expiry.IsZero() {
		return nil, fmt.Errorf("quantity, fill, and expiry are required")
	}
	if direction == "LONG" && !(stop < fill && fill < tp1 && tp1 < tp2) {
		return nil, fmt.Errorf("invalid long geometry")
	}
	if direction == "SHORT" && !(stop > fill && fill > tp1 && tp1 > tp2) {
		return nil, fmt.Errorf("invalid short geometry")
	}
	decimals := 5
	if symbol == "ETH" {
		decimals = 4
	}
	scale := math.Pow10(decimals)
	units := int64(math.Round(quantity * scale))
	if units < 2 {
		return nil, fmt.Errorf("quantity is too small to split into TP1 and runner")
	}
	tp1Units := units / 2
	runnerUnits := units - tp1Units
	return &ManagedTrade{Symbol: symbol, Direction: direction, Quantity: float64(units) / scale, TP1Quantity: float64(tp1Units) / scale, RunnerQuantity: float64(runnerUnits) / scale, Fill: fill, Stop: stop, TP1: tp1, TP2: tp2, Expiry: expiry, State: ProtectionWaiting}, nil
}

func (t *ManagedTrade) OnEntryFilled(executor Executor) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.State != ProtectionWaiting {
		return nil
	}
	exitSide := "SELL"
	if t.Direction == "SHORT" {
		exitSide = "BUY"
	}
	stop, err := executor.Submit(OrderRequest{Symbol: t.Symbol, Side: exitSide, Price: t.Stop, Size: t.Quantity, ExpiresAt: t.Expiry, ReduceOnly: true, OrderType: lightertx.StopLossOrder, TriggerPrice: t.Stop})
	if err != nil {
		return fmt.Errorf("submit initial stop: %w", err)
	}
	tp1, err := executor.Submit(OrderRequest{Symbol: t.Symbol, Side: exitSide, Price: t.TP1, Size: t.TP1Quantity, ExpiresAt: t.Expiry, ReduceOnly: true, OrderType: lightertx.TakeProfitOrder, TriggerPrice: t.TP1})
	if err != nil {
		_ = executor.Cancel(stop.OrderID)
		return fmt.Errorf("submit TP1: %w", err)
	}
	t.StopOrderID, t.TP1OrderID, t.State = stop.OrderID, tp1.OrderID, ProtectionInitial
	return nil
}

func (t *ManagedTrade) OnTP1Filled(executor Executor) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.State == ProtectionRunner || t.State == ProtectionClosed {
		return nil
	}
	if t.State != ProtectionInitial {
		return fmt.Errorf("TP1 fill received in state %s", t.State)
	}
	t.TP1OrderID = ""
	if err := executor.Cancel(t.StopOrderID); err != nil {
		return fmt.Errorf("cancel initial stop: %w", err)
	}
	exitSide := "SELL"
	if t.Direction == "SHORT" {
		exitSide = "BUY"
	}
	remaining := t.RunnerQuantity
	be, err := executor.Submit(OrderRequest{Symbol: t.Symbol, Side: exitSide, Price: t.Fill, Size: remaining, ExpiresAt: t.Expiry, ReduceOnly: true, OrderType: lightertx.StopLossOrder, TriggerPrice: t.Fill})
	if err != nil {
		return fmt.Errorf("submit breakeven stop: %w", err)
	}
	tp2, err := executor.Submit(OrderRequest{Symbol: t.Symbol, Side: exitSide, Price: t.TP2, Size: remaining, ExpiresAt: t.Expiry, ReduceOnly: true, OrderType: lightertx.TakeProfitOrder, TriggerPrice: t.TP2})
	if err != nil {
		_ = executor.Cancel(be.OrderID)
		return fmt.Errorf("submit TP2: %w", err)
	}
	t.StopOrderID, t.TP2OrderID, t.State = be.OrderID, tp2.OrderID, ProtectionRunner
	return nil
}

func (t *ManagedTrade) OnClosed(executor Executor) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.State == ProtectionClosed {
		return nil
	}
	var first error
	for _, id := range []string{t.StopOrderID, t.TP1OrderID, t.TP2OrderID} {
		if id == "" {
			continue
		}
		if err := executor.Cancel(id); err != nil && first == nil {
			first = err
		}
	}
	t.State = ProtectionClosed
	return first
}

// OnExpiry enforces the 16:00 CT terminal condition. Waiting entries are
// canceled; any remaining filled size is closed reduce-only at market.
func (t *ManagedTrade) OnExpiry(executor Executor, remainingSize, referencePrice float64) error {
	t.mu.Lock()
	state, entryID := t.State, t.EntryOrderID
	t.mu.Unlock()
	if state == ProtectionClosed {
		return nil
	}
	if state == ProtectionWaiting {
		if entryID == "" {
			return fmt.Errorf("cannot expire entry without order ID")
		}
		if err := executor.Cancel(entryID); err != nil {
			return err
		}
		t.mu.Lock()
		t.State = ProtectionClosed
		t.mu.Unlock()
		return nil
	}
	if err := t.OnClosed(executor); err != nil {
		return err
	}
	if remainingSize <= 0 {
		return nil
	}
	if referencePrice <= 0 {
		return fmt.Errorf("reference price is required to flatten at expiry")
	}
	_, err := executor.Close(t.Symbol, t.Direction, remainingSize, referencePrice)
	return err
}
