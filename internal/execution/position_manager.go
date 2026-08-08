package execution

type Position struct {
	Symbol string
	Side   string
	Size   float64
	Entry  float64
}

type PositionManager struct {
	State    PositionState
	Position *Position
}

func NewPositionManager() *PositionManager {
	return &PositionManager{
		State: StateFlat,
	}
}

func (pm *PositionManager) BeginOpen() {
	pm.State = StateOpening
}

func (pm *PositionManager) Open(position Position) {
	pm.Position = &position
	pm.State = StateOpen
}

func (pm *PositionManager) BeginClose() {
	pm.State = StateClosing
}

func (pm *PositionManager) Close() {
	pm.Position = nil
	pm.State = StateClosed
}

func (pm *PositionManager) Reset() {
	pm.Position = nil
	pm.State = StateFlat
}

func (pm *PositionManager) HasPosition() bool {
	return pm.Position != nil
}
