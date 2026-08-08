package planner

type Side string

const (
	Long  Side = "LONG"
	Short Side = "SHORT"
)

type TradePlan struct {
	Symbol string

	Side Side

	Entry float64
	Stop  float64

	TP1 float64
	TP2 float64

	Valid bool
}
