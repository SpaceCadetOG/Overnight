package oracleapi

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestExternalLiveOracle is skipped in ordinary unit tests. Operations can run
// it against a loopback or SSH-tunneled Oracle before production activation.
func TestExternalLiveOracle(t *testing.T) {
	endpoint := os.Getenv("ORACLE_LIVE_SMOKE_URL")
	if endpoint == "" {
		t.Skip("ORACLE_LIVE_SMOKE_URL is not set")
	}
	conn, _, err := websocket.DefaultDialer.Dial(endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	books, synchronized := 0, false
	requireBookState := os.Getenv("ORACLE_LIVE_REQUIRE_BOOK_STATE") == "1"
	for {
		_, body, err := conn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		var message struct {
			Type     string `json:"type"`
			Books    int    `json:"books"`
			Snapshot struct {
				Asset          string `json:"asset"`
				Depth          int    `json:"depth"`
				OracleSequence uint64 `json:"oracle_sequence"`
				Bids           []any  `json:"bids"`
				Asks           []any  `json:"asks"`
				Quality        string `json:"quality"`
			} `json:"snapshot"`
		}
		if err := json.Unmarshal(body, &message); err != nil {
			t.Fatal(err)
		}
		if message.Type == "book_snapshot" {
			books++
		}
		if message.Type == "synchronized" {
			if books != message.Books || books == 0 {
				t.Fatalf("book snapshots=%d synchronization=%d", books, message.Books)
			}
			synchronized = true
			if !requireBookState {
				t.Logf("synchronized books=%d", books)
				return
			}
		}
		if synchronized && message.Type == "book_state" {
			if message.Snapshot.Asset == "" || message.Snapshot.Depth == 0 || message.Snapshot.OracleSequence == 0 || len(message.Snapshot.Bids) == 0 || len(message.Snapshot.Asks) == 0 || message.Snapshot.Quality != "CERTIFIED" {
				t.Fatalf("invalid authoritative book_state: %s", body)
			}
			t.Logf("book_state asset=%s depth=%d bids=%d asks=%d oracle_sequence=%d quality=%s", message.Snapshot.Asset, message.Snapshot.Depth, len(message.Snapshot.Bids), len(message.Snapshot.Asks), message.Snapshot.OracleSequence, message.Snapshot.Quality)
			return
		}
	}
}
