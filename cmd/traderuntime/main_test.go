package main

import (
	"errors"
	"testing"
)

func TestRuntimeErrorSummaryHidesHTTPBody(t *testing.T) {
	key, message := runtimeErrorSummary(errors.New("BTC paper: Lighter /api/v1/candles returned HTTP 503: <html><body>Service Temporarily Unavailable</body></html>"))
	if key != "lighter-http-503" || message != "Lighter API temporarily unavailable (HTTP 503)." {
		t.Fatalf("key=%q message=%q", key, message)
	}
}

func TestRuntimeErrorSummaryBoundsUnknownErrors(t *testing.T) {
	_, message := runtimeErrorSummary(errors.New(string(make([]byte, 500))))
	if len(message) > 180 {
		t.Fatalf("message was not bounded: %d", len(message))
	}
}
