package indicators

import (
	"math"
	"testing"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

func TestVWAP(t *testing.T) {
	now := time.Now()

	candles := []models.Candle{
		{
			OpenTime:  now,
			CloseTime: now.Add(time.Minute),
			Open:      100,
			High:      110,
			Low:       90,
			Close:     100,
			Volume:    10,
		},
		{
			OpenTime:  now.Add(time.Minute),
			CloseTime: now.Add(2 * time.Minute),
			Open:      110,
			High:      120,
			Low:       100,
			Close:     110,
			Volume:    20,
		},
	}

	got, err := VWAP(candles)
	if err != nil {
		t.Fatalf("VWAP returned error: %v", err)
	}

	expected := ((100.0 * 10) + (110.0 * 20)) / 30

	if math.Abs(got-expected) > 0.000001 {
		t.Fatalf("got %.6f want %.6f", got, expected)
	}
}
