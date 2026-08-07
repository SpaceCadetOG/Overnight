package hyperliquid

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

// DownloadCandles downloads all available candles between start and end.
func (c *Client) DownloadCandles(
	ctx context.Context,
	coin string,
	interval string,
	start time.Time,
	end time.Time,
) ([]models.Candle, error) {
	if !end.After(start) {
		return nil, fmt.Errorf("end must be after start")
	}

	cursor := start.UTC()
	end = end.UTC()

	all := make([]models.Candle, 0)

	for cursor.Before(end) {
		page, err := c.FetchCandles(
			ctx,
			coin,
			interval,
			cursor,
			end,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"fetch candles from %s: %w",
				cursor.Format(time.RFC3339),
				err,
			)
		}

		if len(page) == 0 {
			break
		}

		// Do not assume the API response is already sorted.
		sort.Slice(page, func(i, j int) bool {
			return page[i].OpenTime.Before(page[j].OpenTime)
		})

		added := 0

		for _, candle := range page {
			if candle.OpenTime.Before(cursor) {
				continue
			}

			if !candle.OpenTime.Before(end) {
				continue
			}

			all = append(all, candle)
			added++
		}

		last := page[len(page)-1]

		// Advance beyond the final completed candle.
		nextCursor := last.CloseTime.Add(time.Millisecond)

		// Some responses may not include a useful close timestamp.
		if !nextCursor.After(cursor) {
			nextCursor = last.OpenTime.Add(intervalDuration(interval))
		}

		if !nextCursor.After(cursor) {
			return nil, fmt.Errorf(
				"pagination cursor did not advance: cursor=%s "+
					"last_open=%s last_close=%s page=%d added=%d",
				cursor.Format(time.RFC3339Nano),
				last.OpenTime.Format(time.RFC3339Nano),
				last.CloseTime.Format(time.RFC3339Nano),
				len(page),
				added,
			)
		}

		cursor = nextCursor

		if len(page) < 500 {
			break
		}

		time.Sleep(100 * time.Millisecond)
	}

	result := removeDuplicates(all)

	sort.Slice(result, func(i, j int) bool {
		return result[i].OpenTime.Before(result[j].OpenTime)
	})

	return result, nil
}

func intervalDuration(interval string) time.Duration {
	switch interval {
	case "1m":
		return time.Minute
	case "3m":
		return 3 * time.Minute
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "30m":
		return 30 * time.Minute
	case "1h":
		return time.Hour
	case "2h":
		return 2 * time.Hour
	case "4h":
		return 4 * time.Hour
	case "8h":
		return 8 * time.Hour
	case "12h":
		return 12 * time.Hour
	case "1d":
		return 24 * time.Hour
	default:
		return time.Millisecond
	}
}

func removeDuplicates(candles []models.Candle) []models.Candle {
	seen := make(map[int64]struct{}, len(candles))
	result := make([]models.Candle, 0, len(candles))

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
