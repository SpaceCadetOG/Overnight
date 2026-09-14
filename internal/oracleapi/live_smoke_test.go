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
	books := 0
	for {
		_, body, err := conn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		var message struct {
			Type  string `json:"type"`
			Books int    `json:"books"`
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
			return
		}
	}
}
