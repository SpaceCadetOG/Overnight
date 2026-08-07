package indicators

import (
	"fmt"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

const (
	DefaultProfileBins = 48
	DefaultValueArea   = 0.70
)

// VolumeBin represents estimated traded volume in one price bracket.
type VolumeBin struct {
	Index  int
	Low    float64
	High   float64
	Mid    float64
	Volume float64
}

// VolumeProfile contains the overnight candle-based volume profile.
type VolumeProfile struct {
	SessionLow  float64
	SessionHigh float64
	BinSize     float64

	Bins []VolumeBin

	POC       float64
	POCIndex  int
	POCVolume float64

	VAH float64
	VAL float64

	TotalVolume     float64
	ValueAreaVolume float64
}

// CalculateVolumeProfile creates a candle-based approximation of
// volume-by-price.
//
// Each candle's volume is distributed evenly across every price bin
// touched by the candle's high-low range.
func CalculateVolumeProfile(
	candles []models.Candle,
	binCount int,
	valueAreaPercent float64,
) (VolumeProfile, error) {
	if len(candles) == 0 {
		return VolumeProfile{}, fmt.Errorf(
			"cannot calculate volume profile with no candles",
		)
	}

	if binCount < 5 {
		return VolumeProfile{}, fmt.Errorf(
			"volume profile requires at least 5 bins",
		)
	}

	if valueAreaPercent <= 0 || valueAreaPercent > 1 {
		return VolumeProfile{}, fmt.Errorf(
			"value area percent must be greater than 0 and at most 1",
		)
	}

	sessionLow := candles[0].Low
	sessionHigh := candles[0].High

	for _, candle := range candles[1:] {
		if candle.Low < sessionLow {
			sessionLow = candle.Low
		}

		if candle.High > sessionHigh {
			sessionHigh = candle.High
		}
	}

	if sessionHigh <= sessionLow {
		return VolumeProfile{}, fmt.Errorf(
			"invalid profile range: high %.8f low %.8f",
			sessionHigh,
			sessionLow,
		)
	}

	binSize := (sessionHigh - sessionLow) / float64(binCount)

	bins := make([]VolumeBin, binCount)

	for index := range bins {
		binLow := sessionLow + float64(index)*binSize
		binHigh := binLow + binSize

		bins[index] = VolumeBin{
			Index: index,
			Low:   binLow,
			High:  binHigh,
			Mid:   (binLow + binHigh) / 2,
		}
	}

	var totalVolume float64

	for _, candle := range candles {
		if candle.Volume <= 0 {
			continue
		}

		firstBin := priceToBin(
			candle.Low,
			sessionLow,
			binSize,
			binCount,
		)

		lastBin := priceToBin(
			candle.High,
			sessionLow,
			binSize,
			binCount,
		)

		if lastBin < firstBin {
			firstBin, lastBin = lastBin, firstBin
		}

		touchedBins := lastBin - firstBin + 1
		if touchedBins <= 0 {
			continue
		}

		volumePerBin := candle.Volume / float64(touchedBins)

		for index := firstBin; index <= lastBin; index++ {
			bins[index].Volume += volumePerBin
		}

		totalVolume += candle.Volume
	}

	if totalVolume <= 0 {
		return VolumeProfile{}, fmt.Errorf(
			"volume profile contains no positive volume",
		)
	}

	pocIndex := 0

	for index := 1; index < len(bins); index++ {
		if bins[index].Volume > bins[pocIndex].Volume {
			pocIndex = index
		}
	}

	valueAreaLowIndex, valueAreaHighIndex, valueAreaVolume :=
		calculateValueArea(
			bins,
			pocIndex,
			totalVolume*valueAreaPercent,
		)

	return VolumeProfile{
		SessionLow:      sessionLow,
		SessionHigh:     sessionHigh,
		BinSize:         binSize,
		Bins:            bins,
		POC:             bins[pocIndex].Mid,
		POCIndex:        pocIndex,
		POCVolume:       bins[pocIndex].Volume,
		VAH:             bins[valueAreaHighIndex].High,
		VAL:             bins[valueAreaLowIndex].Low,
		TotalVolume:     totalVolume,
		ValueAreaVolume: valueAreaVolume,
	}, nil
}

func priceToBin(
	price float64,
	sessionLow float64,
	binSize float64,
	binCount int,
) int {
	index := int((price - sessionLow) / binSize)

	if index < 0 {
		return 0
	}

	if index >= binCount {
		return binCount - 1
	}

	return index
}

// calculateValueArea starts at the POC and expands one adjacent bin at a
// time, always selecting the side with greater volume, until the requested
// amount of session volume is included.
func calculateValueArea(
	bins []VolumeBin,
	pocIndex int,
	targetVolume float64,
) (lowIndex int, highIndex int, includedVolume float64) {
	lowIndex = pocIndex
	highIndex = pocIndex
	includedVolume = bins[pocIndex].Volume

	for includedVolume < targetVolume {
		canExpandLower := lowIndex > 0
		canExpandHigher := highIndex < len(bins)-1

		if !canExpandLower && !canExpandHigher {
			break
		}

		switch {
		case canExpandLower && canExpandHigher:
			lowerVolume := bins[lowIndex-1].Volume
			higherVolume := bins[highIndex+1].Volume

			if higherVolume >= lowerVolume {
				highIndex++
				includedVolume += bins[highIndex].Volume
			} else {
				lowIndex--
				includedVolume += bins[lowIndex].Volume
			}

		case canExpandHigher:
			highIndex++
			includedVolume += bins[highIndex].Volume

		case canExpandLower:
			lowIndex--
			includedVolume += bins[lowIndex].Volume
		}
	}

	return lowIndex, highIndex, includedVolume
}
