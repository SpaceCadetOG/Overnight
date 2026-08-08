package runtime

import (
	"sync"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/execution/lighter/ws"
)

type OrderManager struct {
	mu sync.Mutex

	orders map[string]ws.OrderSnapshot
}

func NewOrderManager() *OrderManager {

	return &OrderManager{
		orders: make(map[string]ws.OrderSnapshot),
	}
}

func (m *OrderManager) Update(
	order ws.OrderSnapshot,
) {

	m.mu.Lock()
	defer m.mu.Unlock()

	m.orders[order.OrderID] = order
}

func (m *OrderManager) Get(
	orderID string,
) (ws.OrderSnapshot, bool) {

	m.mu.Lock()
	defer m.mu.Unlock()

	order, ok := m.orders[orderID]

	return order, ok
}

func (m *OrderManager) Active() []ws.OrderSnapshot {

	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]ws.OrderSnapshot, 0)

	for _, order := range m.orders {

		switch order.Status {

		case ws.OrderSubmitted,
			ws.OrderOpen,
			ws.OrderPartial:

			out = append(out, order)
		}
	}

	return out
}

func (m *OrderManager) FilledSince(
	t time.Time,
) []ws.OrderSnapshot {

	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]ws.OrderSnapshot, 0)

	for _, order := range m.orders {

		if order.Status == ws.OrderFilled &&
			order.Timestamp.After(t) {

			out = append(out, order)
		}
	}

	return out
}
