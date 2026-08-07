package binance

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseKline(t *testing.T) {
	raw := []any{
		int64(1721952000000),
		"65000.10",
		"65120.50",
		"64950.25",
		"65080.75",
		"123.456",
		int64(1721952299999),
		"0",
		0,
		"0",
		"0",
		"0",
	}

	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	var row []json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	candle, err := parseKline(row)
	if err != nil {
		t.Fatalf("parse kline: %v", err)
	}

	if candle.Open != 65000.10 {
		t.Fatalf("unexpected open: %.2f", candle.Open)
	}

	if candle.High != 65120.50 {
		t.Fatalf("unexpected high: %.2f", candle.High)
	}

	if candle.Low != 64950.25 {
		t.Fatalf("unexpected low: %.2f", candle.Low)
	}

	if candle.Close != 65080.75 {
		t.Fatalf("unexpected close: %.2f", candle.Close)
	}

	if candle.Volume != 123.456 {
		t.Fatalf("unexpected volume: %.3f", candle.Volume)
	}

	expectedOpen := time.UnixMilli(1721952000000).UTC()
	if !candle.OpenTime.Equal(expectedOpen) {
		t.Fatalf(
			"unexpected open time: got %s want %s",
			candle.OpenTime,
			expectedOpen,
		)
	}
}
