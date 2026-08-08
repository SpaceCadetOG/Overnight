package strategy

type Direction string

const (
	Long  Direction = "LONG"
	Short Direction = "SHORT"
	None  Direction = "NONE"
)

type MarketContext struct {
	Symbol string

	Price float64

	SwingHigh float64

	SwingLow float64

	Fib382 float64

	Fib500 float64

	Fib618 float64

	POC float64
}

type Signal struct {
	Valid bool

	Direction Direction

	Entry float64

	Stop float64

	Target float64

	Reason string
}
