package runtime

import (
	"sync"

	"github.com/ogtrading/overnight-strategy/internal/execution/lighter/ws"
)

type MarketManager struct {
	mu sync.RWMutex

	prices map[string]float64
}

func NewMarketManager() *MarketManager {

	return &MarketManager{
		prices: make(map[string]float64),
	}
}

func (m *MarketManager) Update(
	price ws.PriceSnapshot,
) {

	m.mu.Lock()
	defer m.mu.Unlock()

	m.prices[price.Symbol] = price.Price
}

func (m *MarketManager) Get(
	symbol string,
) (float64, bool) {

	m.mu.RLock()
	defer m.mu.RUnlock()

	price, ok := m.prices[symbol]

	return price, ok
}
