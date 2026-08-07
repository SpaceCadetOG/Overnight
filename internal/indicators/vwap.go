package indicators

import (
	"fmt"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

// VWAP calculates candle-based anchored VWAP using typical price.
func VWAP(candles []models.Candle) (float64, error) {
	if len(candles) == 0 {
		return 0, fmt.Errorf("cannot calculate VWAP with no candles")
	}

	var weightedTotal float64
	var volumeTotal float64

	for _, candle := range candles {
		typicalPrice := (candle.High + candle.Low + candle.Close) / 3

		weightedTotal += typicalPrice * candle.Volume
		volumeTotal += candle.Volume
	}

	if volumeTotal == 0 {
		return 0, fmt.Errorf("cannot calculate VWAP with zero total volume")
	}

	return weightedTotal / volumeTotal, nil
}
