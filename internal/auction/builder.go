package auction

import (
	"fmt"
	"math"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

// BuildAuctionStructure creates a read-only research snapshot from the
// completed overnight session and the final trade plan.
//
// It does not modify session data, trade-plan values, strategy decisions,
// entries, stops, targets, execution, or optimizer behavior.
func BuildAuctionStructure(
	session models.Session,
	plan models.TradePlan,
) (AuctionStructure, error) {
	if err := validateInputs(session, plan); err != nil {
		return AuctionStructure{}, err
	}

	structure := AuctionStructure{
		Bias: plan.Direction,

		OvernightHigh:  session.High,
		OvernightLow:   session.Low,
		OvernightRange: session.High - session.Low,

		VWAP: session.VWAP,
		POC:  session.POC,
		VAH:  session.VAH,
		VAL:  session.VAL,

		Fib382: session.Fib382,
		Fib500: session.Fib500,
		Fib618: session.Fib618,

		Entry: plan.Entry,
		Stop:  plan.Stop,
		TP1:   plan.TP1,
	}

	populateRelationships(&structure)
	populateDistances(&structure)
	populateFeatures(&structure)

	return structure, nil
}

func validateInputs(session models.Session, plan models.TradePlan) error {
	if plan.Direction != models.BiasLong &&
		plan.Direction != models.BiasShort {
		return fmt.Errorf(
			"auction structure requires LONG or SHORT direction, got %q",
			plan.Direction,
		)
	}

	levels := []struct {
		name  string
		value float64
	}{
		{"overnight high", session.High},
		{"overnight low", session.Low},
		{"VWAP", session.VWAP},
		{"POC", session.POC},
		{"VAH", session.VAH},
		{"VAL", session.VAL},
		{"Fib382", session.Fib382},
		{"Fib500", session.Fib500},
		{"Fib618", session.Fib618},
		{"entry", plan.Entry},
		{"stop", plan.Stop},
		{"TP1", plan.TP1},
	}

	for _, level := range levels {
		if math.IsNaN(level.value) || math.IsInf(level.value, 0) {
			return fmt.Errorf(
				"%s must be finite, got %v",
				level.name,
				level.value,
			)
		}

		if level.value <= 0 {
			return fmt.Errorf(
				"%s must be positive, got %.8f",
				level.name,
				level.value,
			)
		}
	}

	if session.High < session.Low {
		return fmt.Errorf(
			"overnight high %.8f must be greater than or equal to overnight low %.8f",
			session.High,
			session.Low,
		)
	}

	if session.VAH < session.VAL {
		return fmt.Errorf(
			"VAH %.8f must be greater than or equal to VAL %.8f",
			session.VAH,
			session.VAL,
		)
	}

	return nil
}

func populateRelationships(structure *AuctionStructure) {
	structure.POCVsEntry = RelativePosition(
		structure.POC,
		structure.Entry,
		DefaultTolerance,
	)

	structure.POCVsTP1 = RelativePosition(
		structure.POC,
		structure.TP1,
		DefaultTolerance,
	)

	structure.VWAPVsEntry = RelativePosition(
		structure.VWAP,
		structure.Entry,
		DefaultTolerance,
	)

	structure.Fib618VsPOC = RelativePosition(
		structure.Fib618,
		structure.POC,
		DefaultTolerance,
	)

	structure.VAHVsEntry = RelativePosition(
		structure.VAH,
		structure.Entry,
		DefaultTolerance,
	)

	structure.VALVsEntry = RelativePosition(
		structure.VAL,
		structure.Entry,
		DefaultTolerance,
	)
}
