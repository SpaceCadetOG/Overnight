package oracleapi

import (
	"testing"
	"time"
)

func minute(m int) time.Time {
	return time.Date(2026, 8, 9, 0, m, 0, 0, time.UTC)
}

// TestBuildCandlesMergesOutOfOrderTrades reproduces a recorder reconnect
// catch-up batch: a later trade from minute 0 arrives after minute 1's
// trades have already started. Without a stable sort first, buildCandles
// opened a second, conflicting candle for minute 0 instead of merging into
// the one that already existed.
func TestBuildCandlesMergesOutOfOrderTrades(t *testing.T) {
	trades := []analyticTrade{
		{At: minute(0).Add(10 * time.Second), Price: 100, Size: 1, Buy: true},
		{At: minute(0).Add(20 * time.Second), Price: 101, Size: 1, Buy: true},
		{At: minute(1).Add(5 * time.Second), Price: 102, Size: 1, Buy: false},
		// Late-arriving catch-up trade for minute 0, received after minute 1
		// has already started streaming.
		{At: minute(0).Add(40 * time.Second), Price: 99, Size: 2, Buy: false},
	}
	candles := buildCandles(trades, time.Minute)
	if len(candles) != 2 {
		t.Fatalf("expected exactly one candle per minute, got %d: %+v", len(candles), candles)
	}
	if !candles[0].Start.Equal(minute(0)) || !candles[1].Start.Equal(minute(1)) {
		t.Fatalf("unexpected candle order: %+v", candles)
	}
	first := candles[0]
	if first.Open != "100" || first.Close != "99" || first.High != "101" || first.Low != "99" {
		t.Fatalf("minute 0 candle not aggregated in chronological order: %+v", first)
	}
	if first.Trades != 3 {
		t.Fatalf("expected 3 trades folded into minute 0, got %d", first.Trades)
	}
	if first.Volume != "4" {
		t.Fatalf("expected minute 0 volume 1+1+2=4, got %s", first.Volume)
	}
}

func TestBuildCandlesPreservesCVDAcrossOutOfOrderTrades(t *testing.T) {
	trades := []analyticTrade{
		{At: minute(0), Price: 100, Size: 1, Buy: true},
		{At: minute(1), Price: 101, Size: 1, Buy: true},
		// Out-of-order sell for minute 0 arrives last; CVD must still reflect
		// true chronological order (buy at minute 0 happened before it).
		{At: minute(0).Add(30 * time.Second), Price: 100, Size: 1, Buy: false},
	}
	candles := buildCandles(trades, time.Minute)
	if len(candles) != 2 {
		t.Fatalf("expected 2 candles, got %d: %+v", len(candles), candles)
	}
	// Minute 0: +1 (buy) then -1 (sell) => CVD 0. Minute 1: +1 (buy) => CVD 1.
	if candles[0].CVD != "0" {
		t.Fatalf("minute 0 CVD: want 0, got %s", candles[0].CVD)
	}
	if candles[1].CVD != "1" {
		t.Fatalf("minute 1 CVD: want 1, got %s", candles[1].CVD)
	}
}
