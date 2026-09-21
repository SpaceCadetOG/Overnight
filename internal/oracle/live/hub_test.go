package live

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ogtrading/overnight-strategy/internal/oracle/book"
	"github.com/ogtrading/overnight-strategy/internal/oracle/model"
)

func TestWebSocketBackfillSnapshotAndLiveSequence(t *testing.T) {
	hub := New()
	state := book.New("BTC")
	if err := state.ApplySnapshot(model.Book{Nonce: 10, Bids: []model.Level{{Price: "99", Size: "2"}}, Asks: []model.Level{{Price: "101", Size: "3"}}}, 1); err != nil {
		t.Fatal(err)
	}
	snapshot, err := state.Snapshot(0, time.Now(), "conn-1")
	if err != nil {
		t.Fatal(err)
	}
	hub.SetBook(snapshot)
	before := time.Now().UTC().Add(-time.Second)
	trade, err := model.New("BTC", model.StreamTrade, 1, 7, time.Now(), "conn-1", model.QualityCertified, model.Trade{TradeID: "7", Price: "100", Size: "1", AggressorSide: "BUY"})
	if err != nil {
		t.Fatal(err)
	}
	hub.Publish(trade)

	server := httptest.NewServer(hubHandler(hub))
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/v1/live?assets=BTC&since=" + before.Format(time.RFC3339Nano)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	for _, expected := range []string{"backfill_start", "backfill_event", "book_snapshot", "synchronized"} {
		_, body, err := conn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		var message map[string]any
		if err := json.Unmarshal(body, &message); err != nil {
			t.Fatal(err)
		}
		if message["type"] != expected {
			t.Fatalf("type=%v want=%s body=%s", message["type"], expected, body)
		}
	}

	next, _ := model.New("BTC", model.StreamTrade, 2, 8, time.Now(), "conn-1", model.QualityCertified, model.Trade{TradeID: "8", Price: "100.5", Size: "1", AggressorSide: "SELL"})
	hub.Publish(next)
	_, body, err := conn.ReadMessage()
	if err != nil || !strings.Contains(string(body), `"type":"event"`) || !strings.Contains(string(body), `"trade_id":"8"`) {
		t.Fatalf("live message=%s err=%v", body, err)
	}
}

func TestBookDeltaPublishesAuthoritativeBoundedBookState(t *testing.T) {
	hub := New()
	state := book.New("BTC")
	if err := state.ApplySnapshot(model.Book{Nonce: 10, Bids: []model.Level{{Price: "99", Size: "2"}, {Price: "98", Size: "4"}}, Asks: []model.Level{{Price: "101", Size: "3"}, {Price: "102", Size: "5"}}}, 1); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := state.Snapshot(0, time.Now(), "conn-1")
	hub.SetBook(snapshot)
	server := httptest.NewServer(hubHandler(hub))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/live?assets=BTC&streams=book_delta&depth=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for i := 0; i < 3; i++ {
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Fatal(err)
		}
	}
	if err := state.ApplyDelta(model.Book{BeginNonce: 10, Nonce: 11, Bids: []model.Level{{Price: "99", Size: "3"}}}, 2); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = state.Snapshot(0, time.Now(), "conn-1")
	hub.SetBook(snapshot)
	delta, _ := model.New("BTC", model.StreamBookDelta, 2, 11, time.Now(), "conn-1", model.QualityCertified, model.Book{BeginNonce: 10, Nonce: 11, Bids: []model.Level{{Price: "99", Size: "3"}}})
	hub.Publish(delta)
	_, _, _ = conn.ReadMessage()
	_, body, err := conn.ReadMessage()
	if err != nil || !strings.Contains(string(body), `"type":"book_state"`) || !strings.Contains(string(body), `"depth":1`) || strings.Contains(string(body), `"price":"98"`) {
		t.Fatalf("book state=%s err=%v", body, err)
	}
}

func TestSynchronizedIsEmittedExactlyOncePerConnection(t *testing.T) {
	hub := New()
	state := book.New("BTC")
	if err := state.ApplySnapshot(model.Book{Nonce: 10, Bids: []model.Level{{Price: "99", Size: "2"}}, Asks: []model.Level{{Price: "101", Size: "3"}}}, 1); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := state.Snapshot(0, time.Now(), "conn-1")
	hub.SetBook(snapshot)
	server := httptest.NewServer(hubHandler(hub))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/live?assets=BTC&streams=book_delta", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	synchronized := 0
	for i := 0; i < 3; i++ {
		_, body, err := conn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), `"type":"synchronized"`) {
			synchronized++
		}
	}

	if err := state.ApplyDelta(model.Book{BeginNonce: 10, Nonce: 11, Bids: []model.Level{{Price: "99", Size: "3"}}}, 2); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = state.Snapshot(0, time.Now(), "conn-1")
	hub.SetBook(snapshot)
	delta, _ := model.New("BTC", model.StreamBookDelta, 2, 11, time.Now(), "conn-1", model.QualityCertified, model.Book{BeginNonce: 10, Nonce: 11, Bids: []model.Level{{Price: "99", Size: "3"}}})
	hub.Publish(delta)
	for i := 0; i < 2; i++ {
		_, body, err := conn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), `"type":"synchronized"`) {
			synchronized++
		}
	}
	if synchronized != 1 {
		t.Fatalf("synchronized messages=%d want=1", synchronized)
	}
}

func hubHandler(hub *Hub) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/live", hub.ServeWS)
	return mux
}
