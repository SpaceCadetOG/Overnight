package lighter

import (
	"context"
	"fmt"
	"math/big"
	"strings"
)

type OrderState string

const (
	OrderStateUnknown   OrderState = "UNKNOWN"
	OrderStateOpen      OrderState = "OPEN"
	OrderStatePartial   OrderState = "PARTIAL"
	OrderStateFilled    OrderState = "FILLED"
	OrderStateCancelled OrderState = "CANCELLED"
	OrderStateExpired   OrderState = "EXPIRED"
	OrderStateRejected  OrderState = "REJECTED"
)

type ReconciledOrder struct {
	ClientOrderIndex    int64      `json:"client_order_index"`
	ExchangeOrderIndex  int64      `json:"exchange_order_index"`
	MarketIndex         int16      `json:"market_index"`
	State               OrderState `json:"state"`
	InitialBaseAmount   string     `json:"initial_base_amount"`
	RemainingBaseAmount string     `json:"remaining_base_amount"`
	FilledBaseAmount    string     `json:"filled_base_amount"`
	FilledQuoteAmount   string     `json:"filled_quote_amount"`
	AvgFillPrice        string     `json:"avg_fill_price"`
	RawStatus           string     `json:"raw_status"`
	UpdatedAt           int64      `json:"updated_at"`
}

func decimal(value string) (*big.Rat, bool) {
	r := new(big.Rat)
	if strings.TrimSpace(value) == "" {
		return r, false
	}
	_, ok := r.SetString(value)
	return r, ok
}

func normalizeOrderState(order Order) OrderState {
	status := strings.ToLower(strings.TrimSpace(order.Status))
	if status == "canceled-expired" || status == "cancelled-expired" {
		return OrderStateExpired
	}
	if strings.HasPrefix(status, "canceled") || strings.HasPrefix(status, "cancelled") {
		return OrderStateCancelled
	}
	switch status {
	case "expired":
		return OrderStateExpired
	case "rejected", "failed":
		return OrderStateRejected
	case "filled":
		return OrderStateFilled
	}

	initial, initialOK := decimal(order.InitialBaseAmount)
	remaining, remainingOK := decimal(order.RemainingBaseAmount)
	filled, filledOK := decimal(order.FilledBaseAmount)
	if filledOK && filled.Sign() > 0 {
		if initialOK && filled.Cmp(initial) >= 0 {
			return OrderStateFilled
		}
		if remainingOK && remaining.Sign() == 0 {
			return OrderStateFilled
		}
		return OrderStatePartial
	}
	if initialOK && remainingOK {
		if initial.Sign() > 0 && remaining.Sign() == 0 {
			return OrderStateFilled
		}
		if remaining.Cmp(initial) < 0 {
			return OrderStatePartial
		}
	}

	switch status {
	case "open", "active", "pending", "triggered", "in-progress":
		return OrderStateOpen
	default:
		return OrderStateUnknown
	}
}

func averageFillPrice(order Order) string {
	base, baseOK := decimal(order.FilledBaseAmount)
	quote, quoteOK := decimal(order.FilledQuoteAmount)
	if !baseOK || !quoteOK || base.Sign() == 0 {
		return ""
	}
	return new(big.Rat).Quo(quote, base).FloatString(18)
}

func reconcile(order Order) *ReconciledOrder {
	return &ReconciledOrder{
		ClientOrderIndex: order.ClientOrderIndex, ExchangeOrderIndex: order.OrderIndex,
		MarketIndex: order.MarketIndex, State: normalizeOrderState(order),
		InitialBaseAmount: order.InitialBaseAmount, RemainingBaseAmount: order.RemainingBaseAmount,
		FilledBaseAmount: order.FilledBaseAmount, FilledQuoteAmount: order.FilledQuoteAmount,
		AvgFillPrice: averageFillPrice(order), RawStatus: order.Status, UpdatedAt: order.UpdatedAt,
	}
}

func findClientOrder(orders []Order, clientOrderIndex int64) *Order {
	for i := range orders {
		if orders[i].ClientOrderIndex == clientOrderIndex {
			return &orders[i]
		}
	}
	return nil
}

// ReconcileOrder resolves the latest exchange state using the direct lookup,
// then falls back to paginated inactive history for recovery.
func (m *Manager) ReconcileOrder(ctx context.Context, clientOrderIndex int64) (*ReconciledOrder, error) {
	orders, err := m.AccountOrders(ctx, []int64{clientOrderIndex})
	if err != nil {
		return nil, fmt.Errorf("lookup account order: %w", err)
	}
	if order := findClientOrder(orders, clientOrderIndex); order != nil {
		return reconcile(*order), nil
	}

	cursor := ""
	for {
		page, err := m.InactiveOrders(ctx, nil, cursor, maxInactiveOrdersLimit)
		if err != nil {
			return nil, fmt.Errorf("search inactive orders: %w", err)
		}
		if order := findClientOrder(page.Orders, clientOrderIndex); order != nil {
			return reconcile(*order), nil
		}
		if page.Cursor == "" {
			break
		}
		if page.Cursor == cursor {
			return nil, fmt.Errorf("inactive orders returned repeated cursor %q", cursor)
		}
		cursor = page.Cursor
	}
	return &ReconciledOrder{
		ClientOrderIndex: clientOrderIndex,
		State:            OrderStateUnknown,
	}, nil
}
