package store

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBufferedDailyJSONLDrainsOnClose(t *testing.T) {
	root := t.TempDir()
	location, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewBufferedDailyJSONL(root, location, time.Hour, 32)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if err := store.Append("asset=BTC/oracle_events", map[string]int{"sequence": i}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	day := "date=" + time.Now().In(location).Format("2006-01-02")
	file, err := os.Open(filepath.Join(root, day, "asset=BTC", "oracle_events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		var record map[string]int
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		if record["sequence"] != count {
			t.Fatalf("sequence=%d want=%d", record["sequence"], count)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 10 {
		t.Fatalf("records=%d want=10", count)
	}
}

func TestBufferedDailyJSONLRejectsUnsafeStream(t *testing.T) {
	store, err := NewBufferedDailyJSONL(t.TempDir(), time.UTC, time.Millisecond, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Append("../escape", map[string]bool{"bad": true}); err == nil {
		t.Fatal("expected unsafe stream rejection")
	}
}
