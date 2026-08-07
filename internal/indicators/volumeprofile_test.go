package indicators

import (
	"math"
	"testing"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

func TestCalculateVolumeProfile(t *testing.T) {
	start := time.Date(
		2026,
		time.July,
		25,
		19,
		0,
		0,
		0,
		time.UTC,
	)

	candles := []models.Candle{
		{
			OpenTime:  start,
			CloseTime: start.Add(5 * time.Minute),
			Open:      100,
			High:      104,
			Low:       100,
			Close:     103,
			Volume:    100,
		},
		{
			OpenTime:  start.Add(5 * time.Minute),
			CloseTime: start.Add(10 * time.Minute),
			Open:      103,
			High:      106,
			Low:       102,
			Close:     105,
			Volume:    500,
		},
		{
			OpenTime:  start.Add(10 * time.Minute),
			CloseTime: start.Add(15 * time.Minute),
			Open:      105,
			High:      110,
			Low:       105,
			Close:     109,
			Volume:    100,
		},
	}

	profile, err := CalculateVolumeProfile(
		candles,
		10,
		DefaultValueArea,
	)
	if err != nil {
		t.Fatalf("calculate volume profile: %v", err)
	}

	if profile.POC < 102 || profile.POC > 106 {
		t.Fatalf(
			"expected POC near high-volume region, got %.4f",
			profile.POC,
		)
	}

	if profile.VAL > profile.POC {
		t.Fatalf(
			"VAL %.4f must not exceed POC %.4f",
			profile.VAL,
			profile.POC,
		)
	}

	if profile.VAH < profile.POC {
		t.Fatalf(
			"VAH %.4f must not be below POC %.4f",
			profile.VAH,
			profile.POC,
		)
	}

	if profile.VAH <= profile.VAL {
		t.Fatalf(
			"VAH %.4f must exceed VAL %.4f",
			profile.VAH,
			profile.VAL,
		)
	}

	var distributedVolume float64

	for _, bin := range profile.Bins {
		distributedVolume += bin.Volume
	}

	if math.Abs(distributedVolume-700) > 0.000001 {
		t.Fatalf(
			"distributed volume mismatch: got %.6f want 700",
			distributedVolume,
		)
	}

	if profile.ValueAreaVolume < profile.TotalVolume*DefaultValueArea {
		t.Fatalf(
			"value area volume %.6f is below requested amount %.6f",
			profile.ValueAreaVolume,
			profile.TotalVolume*DefaultValueArea,
		)
	}
}

func TestVolumeProfileRejectsInvalidValueArea(t *testing.T) {
	candles := []models.Candle{
		{
			OpenTime:  time.Now(),
			CloseTime: time.Now().Add(time.Minute),
			Open:      100,
			High:      101,
			Low:       99,
			Close:     100,
			Volume:    10,
		},
	}

	_, err := CalculateVolumeProfile(candles, 10, 1.5)

	if err == nil {
		t.Fatal("expected invalid value area error")
	}
}
