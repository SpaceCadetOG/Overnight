package oracleapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTapeRejectsUnboundedRange(t *testing.T) {
	server := &Server{querySlot: make(chan struct{}, 1)}
	req := httptest.NewRequest(http.MethodGet, "/v1/tape?asset=BTC&from=2026-09-10T14:00:00Z&to=2026-09-10T14:16:00Z", nil)
	w := httptest.NewRecorder()
	server.tape(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "15m") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestTapeUsesHourlyIndexAndPreservesRecordedOrder(t *testing.T) {
	server := fixture(t)
	if err := BuildIndexes(server.root, server.indexRoot, "lighter-2026-09-08"); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(server.root, "date=2026-09-08", "asset=BTC", "trade_flow.jsonl.zst")
	if err := os.Rename(source, source+".offline"); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRecorder()
	server.Handler().ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/tape?asset=BTC&from=2026-09-08T12:00:00Z&to=2026-09-08T12:02:00Z", nil))
	if r.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}
	var body struct {
		Schema string      `json:"schema_version"`
		Mode   string      `json:"query_mode"`
		Cutoff string      `json:"information_cutoff"`
		Trades []tapeTrade `json:"trades"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Schema != tapeSchema || body.Mode != "INDEXED_HOURLY" || len(body.Trades) != 2 {
		t.Fatalf("unexpected tape: %+v", body)
	}
	if body.Trades[0].Price != "60001.1" || body.Trades[1].Price != "60002.2" || body.Trades[0].At.After(body.Trades[1].At) {
		t.Fatalf("bad ordering: %+v", body.Trades)
	}
}
