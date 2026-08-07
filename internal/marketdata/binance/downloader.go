package binance

import (
	"context"
	"fmt"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

const maxKlineLimit = 1500

// DownloadCandles downloads all candles between start and end.
func (c *Client) DownloadCandles(
	ctx context.Context,
	symbol string,
	interval string,
	start time.Time,
	end time.Time,
) ([]models.Candle, error) {
	if !end.After(start) {
		return nil, fmt.Errorf("end time must be after start time")
	}

	cursor := start.UTC()
	end = end.UTC()

	allCandles := make([]models.Candle, 0)

	for cursor.Before(end) {
		page, err := c.FetchCandles(
			ctx,
			symbol,
			interval,
			cursor,
			end,
			maxKlineLimit,
		)
		if err != nil {
			return nil, err
		}

		if len(page) == 0 {
			break
		}

		for _, candle := range page {
			if !candle.OpenTime.Before(end) {
				continue
			}

			allCandles = append(allCandles, candle)
		}

		last := page[len(page)-1]
		nextCursor := last.OpenTime.Add(time.Millisecond)

		if !nextCursor.After(cursor) {
			return nil, fmt.Errorf(
				"pagination did not advance from %s",
				cursor.Format(time.RFC3339),
			)
		}

		cursor = nextCursor

		if len(page) < maxKlineLimit {
			break
		}
	}

	return removeDuplicateCandles(allCandles), nil
}

func removeDuplicateCandles(candles []models.Candle) []models.Candle {
	if len(candles) == 0 {
		return candles
	}

	result := make([]models.Candle, 0, len(candles))
	seen := make(map[int64]struct{}, len(candles))

	for _, candle := range candles {
		key := candle.OpenTime.UnixMilli()

		if _, exists := seen[key]; exists {
			continue
		}

		seen[key] = struct{}{}
		result = append(result, candle)
	}

	return result
}
