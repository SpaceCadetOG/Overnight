package oracleapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func fixture(t *testing.T) *Server {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "date=2026-09-08")
	if err := os.MkdirAll(filepath.Join(dir, "asset=BTC"), 0o750); err != nil {
		t.Fatal(err)
	}
	event := zstdBody(t, `{"received_at":"2026-09-08T12:00:00Z","connection_id":"conn-1","event":{"channel":"order_book:1","type":"subscribed/order_book","order_book":{"begin_nonce":0,"nonce":10,"bids":[{"price":"59999.10","size":"1.250"}],"asks":[{"price":"60000.20","size":"2.500"}]}}}`+"\n")
	bookPath := filepath.Join(dir, "asset=BTC", "orderbook_events.jsonl.zst")
	if err := os.WriteFile(bookPath, event, 0o640); err != nil {
		t.Fatal(err)
	}
	trades := zstdBody(t, `{"received_at":"2026-09-08T12:01:00Z","connection_id":"conn-1","event":{"channel":"trade:1","type":"update/trade","trades":[{"trade_id":"41","timestamp":1788868860000,"price":"60001.10","size":"0.00120","usd_amount":"72.00132","is_maker_ask":true},{"trade_id":"42","timestamp":1788868861000,"price":"60002.20","size":"0.00230","usd_amount":"138.00506","is_maker_ask":false}],"liquidation_trades":[]}}`+"\n")
	tradePath := filepath.Join(dir, "asset=BTC", "trade_flow.jsonl.zst")
	if err := os.WriteFile(tradePath, trades, 0o640); err != nil {
		t.Fatal(err)
	}
	certificate := []byte(`{"classification":"CERTIFIED","assets":[{"symbol":"BTC","windows":[{"started_at":"2026-09-08T12:00:00Z","ended_at":"2026-09-08T13:00:00Z","certified":true}]}]}`)
	if err := os.WriteFile(filepath.Join(dir, "RECORDER_CERTIFICATE.json"), certificate, 0o640); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{SchemaVersion: 4, PackageID: "lighter-2026-09-08", Date: "2026-09-08", Assets: []string{"BTC"}, CertificationClass: "CERTIFIED", Files: []File{
		{Path: "asset=BTC/orderbook_events.jsonl.zst", Compressed: int64(len(event)), Records: 1, SHA256: digest(event)},
		{Path: "asset=BTC/trade_flow.jsonl.zst", Compressed: int64(len(trades)), Records: 1, SHA256: digest(trades)},
	}}
	body, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(dir, "MANIFEST.json"), body, 0o640); err != nil {
		t.Fatal(err)
	}
	server, err := New(root, "test", "abc")
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func zstdBody(t *testing.T, value string) []byte {
	t.Helper()
	var output bytes.Buffer
	encoder, err := zstd.NewWriter(&output)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encoder.Write([]byte(value)); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func digest(value []byte) string { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }

func TestCatalogManifestCoverageAndQuality(t *testing.T) {
	h := fixture(t).Handler()
	for _, path := range []string{"/v1/datasets?asset=BTC", "/v1/datasets/lighter-2026-09-08/manifest", "/v1/coverage?asset=BTC", "/v1/quality/windows?package_id=lighter-2026-09-08"} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, path, nil))
		if r.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, r.Code, r.Body.String())
		}
	}
}

func TestDownloadIsManifestAllowlistedAndSupportsRange(t *testing.T) {
	h := fixture(t).Handler()
	r := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/downloads/lighter-2026-09-08/asset=BTC/orderbook_events.jsonl.zst", nil)
	req.Header.Set("Range", "bytes=0-2")
	h.ServeHTTP(r, req)
	expected := zstdBody(t, `{"received_at":"2026-09-08T12:00:00Z","connection_id":"conn-1","event":{"channel":"order_book:1","type":"subscribed/order_book","order_book":{"begin_nonce":0,"nonce":10,"bids":[{"price":"59999.10","size":"1.250"}],"asks":[{"price":"60000.20","size":"2.500"}]}}}`+"\n")
	if r.Code != http.StatusPartialContent || !bytes.Equal(r.Body.Bytes(), expected[:3]) {
		t.Fatalf("status=%d body=%q", r.Code, r.Body.String())
	}
	r = httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/downloads/lighter-2026-09-08/../../etc/passwd", nil))
	if r.Code == http.StatusOK || strings.Contains(r.Body.String(), "root:") {
		t.Fatalf("traversal exposed content status=%d", r.Code)
	}
}

func TestHealthIdentifiesReadOnlyOracle(t *testing.T) {
	r := httptest.NewRecorder()
	fixture(t).Handler().ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), `"mode":"read-only"`) {
		t.Fatalf("body=%s", r.Body.String())
	}
}

func TestHistoricalTapeIsNormalizedFilteredAndPaginated(t *testing.T) {
	h := fixture(t).Handler()
	path := "/v1/trades?asset=BTC&from=2026-09-08T12:00:00Z&to=2026-09-08T13:00:00Z&limit=1"
	first := httptest.NewRecorder()
	h.ServeHTTP(first, httptest.NewRequest(http.MethodGet, path, nil))
	if first.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", first.Code, first.Body.String())
	}
	var page struct {
		Events []struct {
			Stream  string          `json:"stream"`
			Payload json.RawMessage `json:"payload"`
		} `json:"events"`
		Next  string `json:"next_cursor"`
		Count int    `json:"count"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Count != 1 || page.Next == "" || len(page.Events) != 1 || page.Events[0].Stream != "trade" {
		t.Fatalf("page=%+v", page)
	}
	if !strings.Contains(string(page.Events[0].Payload), `"price":"60001.10"`) || !strings.Contains(string(page.Events[0].Payload), `"size":"0.00120"`) {
		t.Fatalf("decimal strings changed: %s", page.Events[0].Payload)
	}
	second := httptest.NewRecorder()
	h.ServeHTTP(second, httptest.NewRequest(http.MethodGet, path+"&cursor="+page.Next, nil))
	if second.Code != http.StatusOK {
		t.Fatalf("second status=%d body=%s", second.Code, second.Body.String())
	}
	if strings.Contains(second.Body.String(), `"trade_id":"41"`) || !strings.Contains(second.Body.String(), `"trade_id":"42"`) {
		t.Fatalf("bad second page: %s", second.Body.String())
	}
}

func TestMarketEventsExposeBookSnapshot(t *testing.T) {
	h := fixture(t).Handler()
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/events?asset=BTC&stream=book_snapshot,book_delta&from=2026-09-08T12:00:00Z&to=2026-09-08T13:00:00Z", nil))
	if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), `"stream":"book_snapshot"`) || !strings.Contains(r.Body.String(), `"price":"59999.10"`) {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}
}

func TestHistoricalQueryBounds(t *testing.T) {
	h := fixture(t).Handler()
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/trades?asset=BTC&from=2026-09-08T00:00:00Z&to=2026-09-10T00:00:00Z", nil))
	if r.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}
}
