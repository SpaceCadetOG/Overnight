package execution

import (
	"testing"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

func TestXAGLifecycleUsesSharedBreakevenRule(t *testing.T) {
	start := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	order := Order{Symbol: "XAG", Side: "BUY", Price: 63.851, Stop: 63.830, TP1: 63.864, TP2: 63.893, ExpiresAt: start.Add(11 * time.Hour).Unix()}
	candles := []models.Candle{
		{OpenTime: start, Low: 63.840, High: 63.870, Close: 63.865},
		{OpenTime: start.Add(5 * time.Minute), Low: 63.850, High: 63.860, Close: 63.852},
	}
	incremental := PaperTrade{SchemaVersion: 1, LifecycleVersion: LifecycleVersion, Order: order, State: Waiting}
	var err error
	for _, candle := range candles {
		incremental, _, err = AdvancePaper(incremental, candle)
		if err != nil {
			t.Fatal(err)
		}
	}
	batch, err := Simulate(order, candles)
	if err != nil {
		t.Fatal(err)
	}
	if incremental.Outcome != "TP1_THEN_BE" || batch.Outcome != incremental.Outcome {
		t.Fatalf("incremental=%s batch=%s", incremental.Outcome, batch.Outcome)
	}
}

func TestXAGExecutablePrecisionPreservesFrozenGeometry(t *testing.T) {
	expiry := time.Date(2026, 8, 10, 21, 0, 0, 0, time.UTC)
	spec := MarketSpec{Symbol: "XAG", Status: "active", TickSize: .001, QuantityStep: .001, MinimumBase: .001, MinimumNotional: 10}
	for _, order := range []Order{
		{Symbol: "XAG", Side: "BUY", Price: 63.8514, Quantity: 23.93, Stop: 63.8296, TP1: 63.8642, TP2: 63.8934, ExpiresAt: expiry.Unix()},
		{Symbol: "XAG", Side: "SELL", Price: 64.0644, Quantity: 23.93, Stop: 64.1104, TP1: 63.9746, TP2: 63.7826, ExpiresAt: expiry.Unix()},
	} {
		normalized := spec.Normalize(order)
		if err := spec.Validate(normalized); err != nil {
			t.Fatalf("%s XAG geometry rejected after normalization: %v (%+v)", order.Side, err, normalized)
		}
	}
}

func TestPaperAndLiveObservationsProduceSameLifecycleActions(t *testing.T) {
	at := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	plan := LifecyclePlan{StrategyOrderID: "strategy-1", Symbol: "BTC", Side: "BUY", Entry: 100, Stop: 99, TP1: 101, TP2: 103, ExpiresAt: at.Add(11 * time.Hour), TP1Fraction: .5, MoveToBEAfterTP1: true}
	waiting := LifecycleState{Phase: LifecycleWaiting}
	paperEntry, err := EvaluateOvernightLifecycle(plan, waiting, LifecycleInput{At: at, Low: 99.5, High: 100.5, HasPriceRange: true})
	if err != nil {
		t.Fatal(err)
	}
	liveEntry, err := EvaluateOvernightLifecycle(plan, waiting, LifecycleInput{At: at, EntryFilled: true})
	if err != nil {
		t.Fatal(err)
	}
	assertActionsEqual(t, paperEntry.Actions, liveEntry.Actions)

	paperTP1, err := EvaluateOvernightLifecycle(plan, paperEntry.State, LifecycleInput{At: at.Add(time.Minute), Low: 100.2, High: 101.2, HasPriceRange: true})
	if err != nil {
		t.Fatal(err)
	}
	liveTP1, err := EvaluateOvernightLifecycle(plan, liveEntry.State, LifecycleInput{At: at.Add(time.Minute), TP1Filled: true})
	if err != nil {
		t.Fatal(err)
	}
	assertActionsEqual(t, paperTP1.Actions, liveTP1.Actions)
	if paperTP1.State.ActiveStop != plan.Entry || liveTP1.State.ActiveStop != plan.Entry {
		t.Fatalf("paper stop=%v live stop=%v", paperTP1.State.ActiveStop, liveTP1.State.ActiveStop)
	}
}

func TestLifecyclePlanIsNotReinterpreted(t *testing.T) {
	expiry := time.Date(2026, 8, 10, 21, 0, 0, 0, time.UTC)
	order := Order{Symbol: "ETH", Side: "SELL", Price: 2000, Stop: 2010, TP1: 1990, TP2: 1970, ExpiresAt: expiry.Unix()}
	plan := PlanFromOrder("same-strategy-order", order)
	if plan.StrategyOrderID != "same-strategy-order" || plan.Side != order.Side || plan.Entry != order.Price || plan.Stop != order.Stop || plan.TP1 != order.TP1 || plan.TP2 != order.TP2 || !plan.ExpiresAt.Equal(expiry) {
		t.Fatalf("plan drifted from order: %#v", plan)
	}
}

func TestOpenPositionExpiryRejectsZeroMark(t *testing.T) {
	at := time.Date(2026, 8, 10, 21, 0, 1, 0, time.UTC)
	plan := LifecyclePlan{StrategyOrderID: "strategy-1", Symbol: "BTC", Side: "BUY", Entry: 100, Stop: 99, TP1: 101, TP2: 103, ExpiresAt: at.Add(-time.Second), TP1Fraction: .5, MoveToBEAfterTP1: true}
	state := LifecycleState{Phase: LifecycleInitial, FillPrice: 100, ActiveStop: 99}
	decision, err := EvaluateOvernightLifecycle(plan, state, LifecycleInput{At: at, Expired: true})
	if err == nil || decision.State.Phase == LifecycleClosed {
		t.Fatalf("zero-mark expiry must be rejected without closing: decision=%+v err=%v", decision, err)
	}
}

func TestExpiredWaitingEntryDoesNotRequireMark(t *testing.T) {
	at := time.Date(2026, 8, 10, 21, 0, 1, 0, time.UTC)
	plan := LifecyclePlan{StrategyOrderID: "strategy-1", Symbol: "BTC", Side: "BUY", Entry: 100, Stop: 99, TP1: 101, TP2: 103, ExpiresAt: at.Add(-time.Second), TP1Fraction: .5, MoveToBEAfterTP1: true}
	decision, err := EvaluateOvernightLifecycle(plan, LifecycleState{Phase: LifecycleWaiting}, LifecycleInput{At: at, Expired: true})
	if err != nil || decision.State.Phase != LifecycleNoFill || decision.Outcome != "NO_FILL" {
		t.Fatalf("waiting expiry=%+v err=%v", decision, err)
	}
}

func TestAuthenticatedFlatReconcilesLocalOpenState(t *testing.T) {
	at := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	plan := LifecyclePlan{StrategyOrderID: "strategy-1", Symbol: "BTC", Side: "BUY", Entry: 100, Stop: 99, TP1: 101, TP2: 103, ExpiresAt: at.Add(time.Hour), TP1Fraction: .5, MoveToBEAfterTP1: true}
	state := LifecycleState{Phase: LifecycleInitial, FillPrice: 100, ActiveStop: 99}
	decision, err := EvaluateOvernightLifecycle(plan, state, LifecycleInput{At: at, Mark: 100.5, PositionClosed: true})
	if err != nil || decision.State.Phase != LifecycleClosed || decision.Outcome != "RECONCILED_FLAT" || len(decision.Actions) != 1 || decision.Actions[0] != ActionReconcileClosed {
		t.Fatalf("authenticated-flat decision=%+v err=%v", decision, err)
	}
}

func assertActionsEqual(t *testing.T, left, right []LifecycleAction) {
	t.Helper()
	if len(left) != len(right) {
		t.Fatalf("actions %v != %v", left, right)
	}
	for i := range left {
		if left[i] != right[i] {
			t.Fatalf("actions %v != %v", left, right)
		}
	}
}
