package session

import (
	"fmt"
	"sort"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

const expectedFiveMinuteCandles = 120

// BuildOvernightSessions groups candles into sessions running from
// 19:00 through 05:00 in the supplied timezone.
//
// The final included five-minute candle opens at 04:55.
// The 05:00 candle belongs to the order-execution period, not the range.
func BuildOvernightSessions(
	candles []models.Candle,
	location *time.Location,
) ([]models.Session, error) {
	if location == nil {
		return nil, fmt.Errorf("location is required")
	}

	if len(candles) == 0 {
		return nil, nil
	}

	sorted := append([]models.Candle(nil), candles...)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].OpenTime.Before(sorted[j].OpenTime)
	})

	grouped := make(map[string][]models.Candle)

	for _, candle := range sorted {
		local := candle.OpenTime.In(location)

		if !isInsideOvernightSession(local) {
			continue
		}

		sessionDate := overnightSessionDate(local)

		key := sessionDate.Format("2006-01-02")
		grouped[key] = append(grouped[key], candle)
	}

	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	sessions := make([]models.Session, 0, len(keys))

	for _, key := range keys {
		sessionCandles := grouped[key]

		sort.Slice(sessionCandles, func(i, j int) bool {
			return sessionCandles[i].OpenTime.Before(
				sessionCandles[j].OpenTime,
			)
		})

		if len(sessionCandles) != expectedFiveMinuteCandles {
			continue
		}

		firstLocal := sessionCandles[0].OpenTime.In(location)
		lastLocal := sessionCandles[len(sessionCandles)-1].
			OpenTime.
			In(location)

		if firstLocal.Hour() != 19 || firstLocal.Minute() != 0 {
			continue
		}

		if lastLocal.Hour() != 4 || lastLocal.Minute() != 55 {
			continue
		}

		sessionValue := buildSession(sessionCandles, location)
		sessions = append(sessions, sessionValue)
	}

	return sessions, nil
}

// isInsideOvernightSession includes 19:00–23:59 and 00:00–04:59.
func isInsideOvernightSession(local time.Time) bool {
	hour := local.Hour()

	return hour >= 19 || hour < 5
}

// overnightSessionDate returns the morning date on which the session ends.
//
// Example:
//
//	2026-07-25 19:00 -> session date 2026-07-26
//	2026-07-26 04:55 -> session date 2026-07-26
func overnightSessionDate(local time.Time) time.Time {
	if local.Hour() >= 19 {
		local = local.AddDate(0, 0, 1)
	}

	return time.Date(
		local.Year(),
		local.Month(),
		local.Day(),
		0,
		0,
		0,
		0,
		local.Location(),
	)
}

func buildSession(
	candles []models.Candle,
	location *time.Location,
) models.Session {
	first := candles[0]
	last := candles[len(candles)-1]

	high := first.High
	low := first.Low

	for _, candle := range candles[1:] {
		if candle.High > high {
			high = candle.High
		}

		if candle.Low < low {
			low = candle.Low
		}
	}

	startLocal := first.OpenTime.In(location)

	sessionDate := overnightSessionDate(startLocal)

	return models.Session{
		Date:    sessionDate,
		Start:   first.OpenTime.In(location),
		End:     last.CloseTime.In(location),
		Candles: candles,
		Open:    first.Open,
		High:    high,
		Low:     low,
		Close:   last.Close,
		Bias:    models.BiasNone,
	}
}
