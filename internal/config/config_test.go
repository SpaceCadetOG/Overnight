package config

import "testing"

func TestDefaultConfigIsValid(t *testing.T) {
	cfg := Default()

	if err := cfg.Validate(); err != nil {
		t.Fatalf("default configuration should be valid: %v", err)
	}
}

func TestTargetAllocationsMustTotalOne(t *testing.T) {
	cfg := Default()
	cfg.TP1Fraction = 0.60
	cfg.TP2Fraction = 0.60

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid target allocation error")
	}
}

func TestOrderCancellationMustBeAfterPlacement(t *testing.T) {
	cfg := Default()
	cfg.OrderCancelHour = cfg.OrderPlaceHour

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid cancellation time error")
	}
}
