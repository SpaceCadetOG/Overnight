package paper

type Direction string

const (
	LONG  Direction = "LONG"
	SHORT Direction = "SHORT"
)

type TradePlan struct {
	Symbol string

	Direction Direction

	Entry float64

	Stop float64

	TP1 float64

	TP2 float64

	Size float64
}

type State string

const (
	FLAT State = "FLAT"

	ORDER_CREATED State = "ORDER_CREATED"

	OPEN State = "OPEN"

	CLOSED State = "CLOSED"
)

type Position struct {
	Symbol string

	Direction Direction

	Size float64

	Entry float64
}
