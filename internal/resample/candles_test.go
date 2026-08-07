package resample

import (
	"testing"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

func TestOneMinuteToFiveMinute(t *testing.T) {
	start := time.Date(
		2026,
		time.July,
		26,
		12,
		0,
		0,
		0,
		time.UTC,
	)

	input := make([]models.Candle, 0, 5)

	for index := 0; index < 5; index++ {
		openTime := start.Add(time.Duration(index) * time.Minute)
		price := 100.0 + float64(index)

		input = append(input, models.Candle{
			OpenTime:  openTime,
			CloseTime: openTime.Add(time.Minute - time.Millisecond),
			Open:      price,
			High:      price + 2,
			Low:       price - 1,
			Close:     price + 1,
			Volume:    10,
		})
	}

	output, err := Candles(input, 5*time.Minute)
	if err != nil {
		t.Fatalf("resample candles: %v", err)
	}

	if len(output) != 1 {
		t.Fatalf("expected 1 candle, got %d", len(output))
	}

	got := output[0]

	if got.Open != 100 {
		t.Fatalf("unexpected open %.2f", got.Open)
	}

	if got.High != 106 {
		t.Fatalf("unexpected high %.2f", got.High)
	}

	if got.Low != 99 {
		t.Fatalf("unexpected low %.2f", got.Low)
	}

	if got.Close != 105 {
		t.Fatalf("unexpected close %.2f", got.Close)
	}

	if got.Volume != 50 {
		t.Fatalf("unexpected volume %.2f", got.Volume)
	}
}
