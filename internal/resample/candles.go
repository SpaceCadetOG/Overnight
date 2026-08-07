package resample

import (
	"fmt"
	"sort"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

// Candles aggregates smaller candles into a larger fixed duration.
//
// Example: one-minute candles can be aggregated into five-minute candles.
// Buckets are aligned to UTC clock boundaries.
func Candles(
	input []models.Candle,
	duration time.Duration,
) ([]models.Candle, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("cannot resample an empty candle set")
	}

	if duration <= 0 {
		return nil, fmt.Errorf("duration must be positive")
	}

	sorted := append([]models.Candle(nil), input...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].OpenTime.Before(sorted[j].OpenTime)
	})

	output := make([]models.Candle, 0, len(sorted)/5+1)

	var current models.Candle
	var currentBucket time.Time
	var initialized bool

	flush := func() {
		if !initialized {
			return
		}

		output = append(output, current)
	}

	for _, candle := range sorted {
		bucket := candle.OpenTime.UTC().Truncate(duration)

		if !initialized || !bucket.Equal(currentBucket) {
			flush()

			currentBucket = bucket
			current = models.Candle{
				OpenTime:  bucket,
				CloseTime: bucket.Add(duration - time.Millisecond),
				Open:      candle.Open,
				High:      candle.High,
				Low:       candle.Low,
				Close:     candle.Close,
				Volume:    candle.Volume,
			}
			initialized = true
			continue
		}

		if candle.High > current.High {
			current.High = candle.High
		}

		if candle.Low < current.Low {
			current.Low = candle.Low
		}

		current.Close = candle.Close
		current.Volume += candle.Volume
	}

	flush()

	return output, nil
}
