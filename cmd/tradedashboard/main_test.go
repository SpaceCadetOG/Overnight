package main

import (
	"errors"
	"testing"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/execution"
	"github.com/ogtrading/overnight-strategy/internal/journal"
	"github.com/ogtrading/overnight-strategy/internal/store"
)

func TestShortErrorHidesLighterHTMLResponse(t *testing.T) {
	got := shortError(errors.New("Lighter /api/v1/account returned HTTP 503: <html><body>temporary unavailable</body></html>"))
	if got != "Lighter API temporarily unavailable (HTTP 503)" {
		t.Fatalf("got %q", got)
	}
}

func TestDisplayStatusFallsBackToLifecycleState(t *testing.T) {
	tests := []struct {
		state execution.PaperState
		want  string
	}{
		{execution.Waiting, "WAIT FILL"},
		{execution.PaperFilled, "FILLED"},
		{execution.PaperTP1, "TP1/RUN"},
		{execution.PaperClosed, "CLOSED"},
		{execution.PaperNoFill, "NO FILL"},
	}
	for _, test := range tests {
		if got := displayStatus(journal.TradeRecord{State: test.state}); got != test.want {
			t.Fatalf("state %s: got %s want %s", test.state, got, test.want)
		}
	}
}

func TestRuntimeStateOverlaysIncompleteJournal(t *testing.T) {
	root := t.TempDir()
	events, err := store.NewJSONL(root)
	if err != nil {
		t.Fatal(err)
	}
	location, _ := time.LoadLocation("America/Chicago")
	session := time.Now().In(location)
	state := execution.PaperTrade{SessionDate: session, TradeID: "paper-btc", Order: execution.Order{Symbol: "BTC", Side: "BUY", Price: 100, Stop: 99, TP1: 101, TP2: 102}, State: execution.PaperFilled, Outcome: "OPEN", FillPrice: 100, UpdatedAt: time.Now().UTC()}
	if err := events.Append("paper_runtime_states", state); err != nil {
		t.Fatal(err)
	}
	records := map[string]journal.TradeRecord{"BTC": {Symbol: "BTC", RecordedAt: time.Now().UTC().Add(time.Second)}}
	overlayPaperRuntime(root, records, session, location)
	if records["BTC"].State != execution.PaperFilled || displayStatus(records["BTC"]) != "OPEN" || !activeTrade(records["BTC"]) {
		t.Fatalf("runtime state not reflected: %#v", records["BTC"])
	}
}
