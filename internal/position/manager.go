package position

import "fmt"

type Side string

const (
	Flat  Side = "FLAT"
	Long  Side = "LONG"
	Short Side = "SHORT"
)

type Position struct {
	Symbol string
	Side   Side
	Size   float64
	Entry  float64
}

type Manager struct {
	current Position
}

func NewManager() *Manager {

	return &Manager{
		current: Position{
			Side: Flat,
		},
	}
}

func (m *Manager) Open(
	symbol string,
	side Side,
	size float64,
	entry float64,
) error {

	if m.current.Side != Flat {
		return fmt.Errorf(
			"position already exists",
		)
	}

	m.current = Position{
		Symbol: symbol,
		Side:   side,
		Size:   size,
		Entry:  entry,
	}

	return nil
}

func (m *Manager) Close() {

	m.current = Position{
		Side: Flat,
	}

}

func (m *Manager) Get() Position {

	return m.current

}
