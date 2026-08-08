package forensics

import (
	"testing"
	"time"
)

func TestPaperAndLiveShareOpportunityButNotTrade(t *testing.T) {
	date := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	paper := IDs(date, "BTC", "baseline-v1", "entry=100|stop=99", "PAPER", "paper-cycle1")
	live := IDs(date, "BTC", "baseline-v1", "entry=100|stop=99", "LIVE", "live-cycle1")
	if paper.OpportunityID != live.OpportunityID {
		t.Fatal("paper/live opportunity mismatch")
	}
	if paper.TradeID == live.TradeID {
		t.Fatal("paper/live trade IDs collided")
	}
}

func TestCaseProjectionAndQuality(t *testing.T) {
	ids := IDs(time.Now(), "BTC", "v1", "plan-1", "PAPER", "run")
	events := []Envelope{}
	for i, kind := range []EventType{PlanCreated, PlanValidated, OrderSubmitted, OrderFilled, PositionOpened, PositionClosed} {
		e, err := New(kind, time.Now(), uint64(i+1), ids, "BTC", "PAPER", "v1", map[string]any{"outcome": "TP2"})
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	c, err := BuildCase(events)
	if err != nil {
		t.Fatal(err)
	}
	if c.State != Closed || c.Outcome != "TP2" || len(c.DataQuality) != 0 {
		t.Fatalf("case=%+v", c)
	}
}

func TestCaseDetectsSequenceGap(t *testing.T) {
	ids := IDs(time.Now(), "ETH", "v1", "plan-1", "PAPER", "run")
	a, _ := New(PlanCreated, time.Now(), 1, ids, "ETH", "PAPER", "v1", nil)
	b, _ := New(OrderSubmitted, time.Now(), 3, ids, "ETH", "PAPER", "v1", nil)
	c, _ := BuildCase([]Envelope{a, b})
	if len(c.DataQuality) == 0 {
		t.Fatal("sequence gap accepted")
	}
}
