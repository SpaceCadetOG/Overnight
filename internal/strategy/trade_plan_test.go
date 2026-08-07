package strategy

import (
	"math"
	"testing"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

func TestDefaultLongTradePlan(t *testing.T) {
	session := models.Session{
		Bias:   models.BiasLong,
		Fib382: 97,
		Fib500: 101,
		Fib618: 105,
		VAL:    98,
		POC:    103,
		High:   115,
	}

	plan := BuildTradePlanWithConfig(
		session,
		DefaultPlanConfig(),
	)

	if !plan.Valid {
		t.Fatalf("expected valid plan: %s", plan.InvalidReason)
	}

	if plan.Entry != 99 {
		t.Fatalf("expected midpoint entry 99, got %.2f", plan.Entry)
	}

	if plan.TP1 != 105 {
		t.Fatalf("expected Fib 61.8 TP1, got %.2f", plan.TP1)
	}

	if plan.Stop >= plan.Entry {
		t.Fatalf("expected stop below entry, got %.2f", plan.Stop)
	}
}

func TestFib382EntryMethod(t *testing.T) {
	session := models.Session{
		Bias:   models.BiasLong,
		Fib382: 97,
		Fib500: 101,
		Fib618: 105,
		VAL:    96,
		High:   110,
	}

	config := DefaultPlanConfig()
	config.EntryMethod = EntryFib382

	plan := BuildTradePlanWithConfig(session, config)

	if !plan.Valid {
		t.Fatalf("expected valid plan: %s", plan.InvalidReason)
	}

	if plan.Entry != 97 {
		t.Fatalf("expected Fib382 entry, got %.2f", plan.Entry)
	}
}

func TestFib500EntryMethod(t *testing.T) {
	session := models.Session{
		Bias:   models.BiasLong,
		Fib382: 97,
		Fib500: 101,
		Fib618: 105,
		VAL:    96,
		High:   110,
	}

	config := DefaultPlanConfig()
	config.EntryMethod = EntryFib500

	plan := BuildTradePlanWithConfig(session, config)

	if !plan.Valid {
		t.Fatalf("expected valid plan: %s", plan.InvalidReason)
	}

	if plan.Entry != 101 {
		t.Fatalf("expected Fib500 entry, got %.2f", plan.Entry)
	}
}

func TestNearestValidPOCTarget(t *testing.T) {
	session := models.Session{
		Bias:   models.BiasLong,
		Fib382: 97,
		Fib500: 101,
		Fib618: 108,
		VAL:    98,
		POC:    104,
		High:   115,
	}

	config := DefaultPlanConfig()
	config.TP1Method = TP1NearestValid
	config.MinimumPOCRR = 0.50

	plan := BuildTradePlanWithConfig(session, config)

	if !plan.Valid {
		t.Fatalf("expected valid plan: %s", plan.InvalidReason)
	}

	if plan.TP1Source != string(TP1POC) {
		t.Fatalf("expected POC target, got %s", plan.TP1Source)
	}
}

func TestTradePlanCalculatesRiskReward(t *testing.T) {
	session := models.Session{
		Bias:   models.BiasLong,
		Fib382: 99,
		Fib500: 101,
		Fib618: 104,
		VAL:    99,
		High:   110,
	}

	config := DefaultPlanConfig()
	config.StopBufferBPS = 0

	plan := BuildTradePlanWithConfig(session, config)

	if !plan.Valid {
		t.Fatalf("expected valid plan: %s", plan.InvalidReason)
	}

	if math.Abs(plan.RR1-4) > 0.000001 {
		t.Fatalf("expected RR1 4, got %.6f", plan.RR1)
	}

	if math.Abs(plan.RR2-10) > 0.000001 {
		t.Fatalf("expected RR2 10, got %.6f", plan.RR2)
	}
}
