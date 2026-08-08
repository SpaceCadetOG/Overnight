package execution

type PositionState string

const (
	StateFlat    PositionState = "FLAT"
	StateOpening PositionState = "OPENING"
	StateOpen    PositionState = "OPEN"
	StateClosing PositionState = "CLOSING"
	StateClosed  PositionState = "CLOSED"
)

func (s PositionState) String() string {
	return string(s)
}
