package auction

import "github.com/ogtrading/overnight-strategy/internal/models"

const DefaultTolerance = 0.0000001

// Position describes the position of one price level relative to another.
type Position uint8

const (
	PositionUnknown Position = iota
	PositionBelow
	PositionEqual
	PositionAbove
)

func (p Position) String() string {
	switch p {
	case PositionBelow:
		return "BELOW"
	case PositionEqual:
		return "EQUAL"
	case PositionAbove:
		return "ABOVE"
	default:
		return "UNKNOWN"
	}
}

// AuctionStructure captures the pre-entry arrangement of overnight,
// volume-profile, VWAP, Fibonacci, and planned trade levels.
//
// It is descriptive research infrastructure only. It does not decide
// whether a trade should be taken and does not modify strategy behavior.
type AuctionStructure struct {
	// Raw session and trade-plan levels.
	Bias models.Bias

	OvernightHigh  float64
	OvernightLow   float64
	OvernightRange float64

	VWAP float64
	POC  float64
	VAH  float64
	VAL  float64

	Fib382 float64
	Fib500 float64
	Fib618 float64

	Entry float64
	Stop  float64
	TP1   float64

	// Relative positions.
	POCVsEntry  Position
	POCVsTP1    Position
	VWAPVsEntry Position
	Fib618VsPOC Position
	VAHVsEntry  Position
	VALVsEntry  Position

	// Risk-normalized absolute distances.
	EntryToPOCR  float64
	EntryToVWAPR float64
	POCToTP1R    float64
	VAHToTP1R    float64
	VALToEntryR  float64
	Fib618ToPOCR float64

	// Structural features.
	EntryInsideValue      bool
	EntryAboveVAH         bool
	EntryBelowVAL         bool
	POCBetweenEntryAndTP1 bool
	POCBehindEntry        bool
	POCBeyondTP1          bool
	Fib618AbovePOC        bool
	Fib618BelowPOC        bool
	VWAPSupportsDirection bool
}
