package runtime

import (
	"sync"

	"github.com/ogtrading/overnight-strategy/internal/execution"
	"github.com/ogtrading/overnight-strategy/internal/execution/lighter/ws"
)

type Sync struct {
	mu sync.RWMutex

	Positions map[string]ws.PositionSnapshot

	Manager *execution.PositionManager
}

func NewSync(
	manager *execution.PositionManager,
) *Sync {

	return &Sync{
		Positions: make(map[string]ws.PositionSnapshot),
		Manager:   manager,
	}
}

func (s *Sync) Update(
	position ws.PositionSnapshot,
) {

	s.mu.Lock()
	defer s.mu.Unlock()

	s.Positions[position.Symbol] = position

	if position.Size == 0 {

		s.Manager.Reset()

		return
	}

	s.Manager.Open(
		execution.Position{
			Symbol: position.Symbol,
			Side:   position.Side,
			Size:   position.Size,
			Entry:  position.EntryPrice,
		},
	)
}

func (s *Sync) Get(
	symbol string,
) (ws.PositionSnapshot, bool) {

	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.Positions[symbol]

	return p, ok
}
