package main

import (
	"errors"
	"os"
	"path/filepath"
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

func TestPaperMarkToMarketTracksRunner(t *testing.T) {
	record := journal.TradeRecord{State: execution.PaperTP1, TP1Hit: true, Order: execution.Order{Side: "BUY", Price: 100, Stop: 99, TP1: 101, TP2: 103}}
	r, pnl, remaining, next := paperMarkToMarket(record, 101.5, .50)
	if r != 1.25 || pnl != .625 || remaining != 50 || next != "TP2 OR BREAKEVEN" {
		t.Fatalf("r=%v pnl=%v remaining=%v next=%q", r, pnl, remaining, next)
	}
}

func TestReadRecorderMarksUsesLastTicker(t *testing.T) {
	root := t.TempDir()
	location, _ := time.LoadLocation("America/Chicago")
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	dir := filepath.Join(root, "date="+now.In(location).Format("2006-01-02"), "asset=BTC")
	if err := os.MkdirAll(dir, 0750); err != nil {
		t.Fatal(err)
	}
	data := "{\"received_at\":\"2026-08-09T11:59:59Z\",\"event\":{\"ticker\":{\"a\":{\"price\":\"101\"},\"b\":{\"price\":\"99\"}}}}\n{\"partial\":"
	if err := os.WriteFile(filepath.Join(dir, "ticker_events.jsonl"), []byte(data), 0640); err != nil {
		t.Fatal(err)
	}
	marks := readRecorderMarks(root, now, location)
	if marks["BTC"].Price != 100 {
		t.Fatalf("mark=%v", marks["BTC"].Price)
	}
}

func TestCurrentRecordsResetsAtChicagoMidnight(t *testing.T) {
	location, _ := time.LoadLocation("America/Chicago")
	now := time.Date(2026, 8, 10, 0, 1, 0, 0, location)
	records := []journal.TradeRecord{
		{Symbol: "BTC", SessionDate: now.Add(-2 * time.Minute), RecordedAt: now.Add(-2 * time.Minute), RMultiple: 5},
		{Symbol: "ETH", SessionDate: now, RecordedAt: now, RMultiple: 1},
	}
	got := currentRecords(records, now, location)
	if len(got) != 1 || got["ETH"].RMultiple != 1 {
		t.Fatalf("daily records did not reset at Chicago midnight: %#v", got)
	}
}

func TestPriorWeeklyRResetsMondayAndUsesLatestResult(t *testing.T) {
	location, _ := time.LoadLocation("America/Chicago")
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, location) // Wednesday
	records := []journal.TradeRecord{
		{Symbol: "BTC", SessionDate: time.Date(2026, 8, 9, 12, 0, 0, 0, location), RecordedAt: now.Add(-72 * time.Hour), RMultiple: 10}, // Sunday: excluded
		{Symbol: "BTC", SessionDate: time.Date(2026, 8, 10, 12, 0, 0, 0, location), RecordedAt: now.Add(-48 * time.Hour), RMultiple: -1},
		{Symbol: "BTC", SessionDate: time.Date(2026, 8, 10, 12, 0, 0, 0, location), RecordedAt: now.Add(-47 * time.Hour), RMultiple: 2}, // latest Monday value
		{Symbol: "ETH", SessionDate: time.Date(2026, 8, 11, 12, 0, 0, 0, location), RecordedAt: now.Add(-24 * time.Hour), RMultiple: 1},
		{Symbol: "SOL", SessionDate: now, RecordedAt: now, RMultiple: 8},                                            // today: caller adds live daily R
		{Symbol: "ETH", Mode: "LIVE_EXECUTION", SessionDate: now.AddDate(0, 0, -1), RecordedAt: now, RMultiple: 20}, // live: excluded
	}
	if got := priorWeeklyR(records, now, location); got != 3 {
		t.Fatalf("priorWeeklyR=%v, want 3", got)
	}
}
