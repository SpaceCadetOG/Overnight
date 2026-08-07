package session

import (
	"testing"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

func TestBuildOvernightSessions(t *testing.T) {
	location, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatalf("load timezone: %v", err)
	}

	start := time.Date(
		2026,
		time.July,
		25,
		19,
		0,
		0,
		0,
		location,
	)

	candles := make([]models.Candle, 0, 120)

	for index := 0; index < 120; index++ {
		openLocal := start.Add(time.Duration(index) * 5 * time.Minute)

		openUTC := openLocal.UTC()

		price := 100.0 + float64(index)

		candles = append(candles, models.Candle{
			OpenTime:  openUTC,
			CloseTime: openUTC.Add(5*time.Minute - time.Millisecond),
			Open:      price,
			High:      price + 2,
			Low:       price - 1,
			Close:     price + 1,
			Volume:    10,
		})
	}

	sessions, err := BuildOvernightSessions(candles, location)
	if err != nil {
		t.Fatalf("build sessions: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	got := sessions[0]

	if len(got.Candles) != 120 {
		t.Fatalf(
			"expected 120 candles, got %d",
			len(got.Candles),
		)
	}

	if got.Start.Hour() != 19 || got.Start.Minute() != 0 {
		t.Fatalf("unexpected start: %s", got.Start)
	}

	lastOpen := got.Candles[len(got.Candles)-1].
		OpenTime.
		In(location)

	if lastOpen.Hour() != 4 || lastOpen.Minute() != 55 {
		t.Fatalf("unexpected final candle: %s", lastOpen)
	}

	if got.Open != 100 {
		t.Fatalf("unexpected open: %.2f", got.Open)
	}

	if got.High != 221 {
		t.Fatalf("unexpected high: %.2f", got.High)
	}

	if got.Low != 99 {
		t.Fatalf("unexpected low: %.2f", got.Low)
	}

	if got.Close != 220 {
		t.Fatalf("unexpected close: %.2f", got.Close)
	}

}

func TestIncompleteSessionIsRejected(t *testing.T) {
	location, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatalf("load timezone: %v", err)
	}

	start := time.Date(
		2026,
		time.July,
		25,
		19,
		0,
		0,
		0,
		location,
	)

	candles := make([]models.Candle, 0, 119)

	for index := 0; index < 119; index++ {
		openLocal := start.Add(time.Duration(index) * 5 * time.Minute)
		openUTC := openLocal.UTC()

		candles = append(candles, models.Candle{
			OpenTime:  openUTC,
			CloseTime: openUTC.Add(5*time.Minute - time.Millisecond),
			Open:      100,
			High:      101,
			Low:       99,
			Close:     100,
			Volume:    10,
		})
	}

	sessions, err := BuildOvernightSessions(candles, location)
	if err != nil {
		t.Fatalf("build sessions: %v", err)
	}

	if len(sessions) != 0 {
		t.Fatalf(
			"expected incomplete session to be rejected, got %d",
			len(sessions),
		)
	}
}

func TestDaytimeCandlesAreIgnored(t *testing.T) {
	location, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatalf("load timezone: %v", err)
	}

	local := time.Date(
		2026,
		time.July,
		26,
		12,
		0,
		0,
		0,
		location,
	)

	candles := []models.Candle{
		{
			OpenTime:  local.UTC(),
			CloseTime: local.Add(5*time.Minute - time.Millisecond).UTC(),
			Open:      100,
			High:      101,
			Low:       99,
			Close:     100,
			Volume:    10,
		},
	}

	sessions, err := BuildOvernightSessions(candles, location)
	if err != nil {
		t.Fatalf("build sessions: %v", err)
	}

	if len(sessions) != 0 {
		t.Fatalf("expected no sessions, got %d", len(sessions))
	}
}
