package indicators

import (
	"fmt"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

type Fibonacci struct {
	Range float64

	Long382 float64
	Long500 float64
	Long618 float64

	Short382 float64
	Short500 float64
	Short618 float64
}

// CalculateFibonacci calculates bullish and bearish retracement levels.
func CalculateFibonacci(session models.Session) (Fibonacci, error) {
	if session.High <= session.Low {
		return Fibonacci{}, fmt.Errorf(
			"invalid session range: high %.8f low %.8f",
			session.High,
			session.Low,
		)
	}

	rangeSize := session.High - session.Low

	return Fibonacci{
		Range: rangeSize,

		Long382: session.Low + rangeSize*0.382,
		Long500: session.Low + rangeSize*0.500,
		Long618: session.Low + rangeSize*0.618,

		Short382: session.High - rangeSize*0.382,
		Short500: session.High - rangeSize*0.500,
		Short618: session.High - rangeSize*0.618,
	}, nil
}
