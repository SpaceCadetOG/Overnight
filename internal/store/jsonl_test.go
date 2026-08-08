package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestJSONLRoundTrip(t *testing.T) {
	root := t.TempDir()
	store, err := NewJSONL(root)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		ID int `json:"id"`
	}
	if err := store.Append("events", row{ID: 7}); err != nil {
		t.Fatal(err)
	}
	rows, err := ReadAll[row](root, "events")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != 7 {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestDailyJSONLPartitionsByChicagoDateAndNestedStream(t *testing.T) {
	location, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	s, err := NewDailyJSONL(root, location)
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Date(2026, 8, 9, 4, 59, 0, 0, time.UTC) }
	if err := s.Append("asset=BTC/orderbook_events", map[string]int{"nonce": 7}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "date=2026-08-08", "asset=BTC", "orderbook_events.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("daily stream missing: %v", err)
	}
}
