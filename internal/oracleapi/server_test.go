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
	"time"

	"github.com/gorilla/websocket"
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
	liquidity := zstdBody(t, `{"schema_version":"oracle-liquidity-observation-v1","asset":"BTC","window_start":"2026-09-08T12:00:59Z","window_end":"2026-09-08T12:01:02Z","connection_id":"conn-1","book_quality":"CERTIFIED","observation_type":"DISPLAYED_BOOK_CHANGE","changes":[[1788868860000,"ASK","60001.10","1.00120","1.00000","-0.00120","REMOVED",11,12],[1788868860500,"ASK","60001.10","1.00000","1.00120","0.00120","ADDED",12,13],[1788868861000,"BID","59990.00","2.00230","2.00000","-0.00230","REMOVED",13,14]]}`+"\n")
	liquidityPath := filepath.Join(dir, "asset=BTC", "liquidity_observations.jsonl.zst")
	if err := os.WriteFile(liquidityPath, liquidity, 0o640); err != nil {
		t.Fatal(err)
	}
	marketStats := zstdBody(t, `{"received_at":"2026-09-08T12:01:00Z","event":{"timestamp":1788868860000,"market_stats":{"1":{"symbol":"BTC","open_interest":"123.45","mark_price":"60001.50"}}}}`+"\n")
	marketStatsPath := filepath.Join(dir, "market_stats.jsonl.zst")
	if err := os.WriteFile(marketStatsPath, marketStats, 0o640); err != nil {
		t.Fatal(err)
	}
	certificate := []byte(`{"classification":"CERTIFIED","assets":[{"symbol":"BTC","windows":[{"started_at":"2026-09-08T12:00:00Z","ended_at":"2026-09-08T13:00:00Z","certified":true}]}]}`)
	if err := os.WriteFile(filepath.Join(dir, "RECORDER_CERTIFICATE.json"), certificate, 0o640); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{SchemaVersion: 4, PackageID: "lighter-2026-09-08", Date: "2026-09-08", Assets: []string{"BTC"}, CertificationClass: "CERTIFIED", Files: []File{
		{Path: "asset=BTC/orderbook_events.jsonl.zst", Compressed: int64(len(event)), Records: 1, SHA256: digest(event)},
		{Path: "asset=BTC/trade_flow.jsonl.zst", Compressed: int64(len(trades)), Records: 1, SHA256: digest(trades)},
		{Path: "asset=BTC/liquidity_observations.jsonl.zst", Compressed: int64(len(liquidity)), Records: 1, SHA256: digest(liquidity)},
		{Path: "market_stats.jsonl.zst", Compressed: int64(len(marketStats)), Records: 1, SHA256: digest(marketStats)},
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

func TestHostedDashboardIsReadOnlyAndLinksOracleAPIs(t *testing.T) {
	r := httptest.NewRecorder()
	fixture(t).Handler().ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/dashboard", nil))
	if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), "Market Data Oracle") || !strings.Contains(r.Body.String(), "/v1/profiles") || !strings.Contains(r.Body.String(), "/v1/heatmap") {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}
	if !strings.Contains(r.Header().Get("Content-Security-Policy"), "default-src 'none'") {
		t.Fatalf("missing restrictive CSP: %s", r.Header().Get("Content-Security-Policy"))
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

func TestTradeDerivedAnalytics(t *testing.T) {
	h := fixture(t).Handler()
	base := "?asset=BTC&from=2026-09-08T12:00:00Z&to=2026-09-08T13:00:00Z"
	tests := []struct {
		path string
		want []string
	}{
		{"/v1/candles" + base + "&interval=1m", []string{`"type":"CANDLES"`, `"open":"60001.1"`, `"close":"60002.2"`, `"trades":2`}},
		{"/v1/profiles" + base + "&value_area=0.70", []string{`"type":"VOLUME_PROFILE"`, `"poc":"60002.2"`, `"vwap":`, `"distribution"`}},
		{"/v1/footprints" + base + "&interval=1m", []string{`"type":"FOOTPRINT"`, `"levels"`, `"buy_volume":"0.0012"`, `"sell_volume":"0.0023"`}},
		{"/v1/order-flow" + base, []string{`"type":"ORDER_FLOW"`, `"trades":2`, `"delta":"-0.0011"`, `"packages":["lighter-2026-09-08"]`}},
		{"/v1/levels" + base, []string{`"schema_version":"oracle-market-structure-v1"`, `"type":"POC"`, `"type":"VWAP"`, `"session_definition_version":"global-sessions-v1"`}},
		{"/v1/zones" + base + "&tolerance_bps=10", []string{`"zone_model":"profile-cluster-v1"`, `"zones"`, `"sources"`, `"strength"`}},
	}
	for _, test := range tests {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, test.path, nil))
		if r.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", test.path, r.Code, r.Body.String())
		}
		for _, want := range test.want {
			if !strings.Contains(r.Body.String(), want) {
				t.Errorf("%s missing %s body=%s", test.path, want, r.Body.String())
			}
		}
	}
}

func TestStreamingProfileAccumulatorMatchesExactProfile(t *testing.T) {
	trades := []analyticTrade{
		{At: time.Unix(1, 0), Price: 100, Size: 2, Notional: 200, Buy: true},
		{At: time.Unix(2, 0), Price: 101, Size: 3, Notional: 303, Buy: false},
		{At: time.Unix(3, 0), Price: 100, Size: 1, Notional: 100, Buy: true},
	}
	want := buildProfile(trades, 0.70)
	acc := newProfileAccumulator()
	for _, trade := range trades {
		acc.Add(trade)
	}
	got := acc.Profile(0.70)
	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(got)
	if !bytes.Equal(wantJSON, gotJSON) {
		t.Fatalf("streaming profile differs\nwant=%s\ngot=%s", wantJSON, gotJSON)
	}
}

func TestPersistentMicrostructureAPIs(t *testing.T) {
	h := fixture(t).Handler()
	base := "?asset=BTC&from=2026-09-08T12:00:00Z&to=2026-09-08T13:00:00Z"
	tests := []struct {
		path string
		want []string
	}{
		{"/v1/liquidity" + base, []string{`"model":"displayed-liquidity-correlation-v1"`, `"correlation":"LIKELY_EXECUTED"`, `"correlation":"LIKELY_CANCELLED"`, `"disclaimer"`}},
		{"/v1/heatmap" + base, []string{`"schema_version":"oracle-heatmap-v1"`, `"price":"60001.10"`, `"added":"0.0012"`}},
		{"/v1/open-interest" + base, []string{`"schema_version":"oracle-open-interest-v1"`, `"open_interest":"123.45"`, `"mark_price":"60001.50"`}},
	}
	for _, test := range tests {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, test.path, nil))
		if r.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", test.path, r.Code, r.Body.String())
		}
		for _, want := range test.want {
			if !strings.Contains(r.Body.String(), want) {
				t.Errorf("%s missing %s body=%s", test.path, want, r.Body.String())
			}
		}
	}
}

func TestCanonicalSessionsAreVersionedAndDSTSafe(t *testing.T) {
	h := fixture(t).Handler()
	for _, path := range []string{
		"/v1/session-definitions",
		"/v1/sessions?type=US&at=2026-03-09T14:00:00Z",
		"/v1/sessions?type=LONDON&at=2026-03-30T12:00:00Z",
		"/v1/sessions?type=24H&at=2026-09-14T19:00:00Z",
	} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, path, nil))
		if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), sessionDefinitionVersion) {
			t.Fatalf("%s status=%d body=%s", path, r.Code, r.Body.String())
		}
	}
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/sessions?type=US&at=2026-03-09T14:00:00Z", nil))
	if !strings.Contains(r.Body.String(), `"utc_start":"2026-03-09T13:30:00Z"`) || !strings.Contains(r.Body.String(), `"utc_offset_seconds":-14400`) {
		t.Fatalf("New York DST boundary is wrong: %s", r.Body.String())
	}
}

func TestHistoricalPointInTimeBook(t *testing.T) {
	r := httptest.NewRecorder()
	fixture(t).Handler().ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/books/BTC/at?at=2026-09-08T12:05:00Z", nil))
	if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), `"schema_version":"oracle-book-at-v1"`) || !strings.Contains(r.Body.String(), `"price":"59999.10"`) || !strings.Contains(r.Body.String(), `"quality":["CERTIFIED"]`) {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}
}

func TestDeterministicReplayWebSocket(t *testing.T) {
	server := httptest.NewServer(fixture(t).Handler())
	defer server.Close()
	endpoint := "ws" + strings.TrimPrefix(server.URL, "http") + "/v1/replay?asset=BTC&stream=trade&from=2026-09-08T12:00:00Z&to=2026-09-08T13:00:00Z&speed=10000"
	conn, _, err := websocket.DefaultDialer.Dial(endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	want := []string{"replay_start", "replay_event", "replay_event", "replay_end"}
	for _, expected := range want {
		_, body, err := conn.ReadMessage()
		if err != nil || !strings.Contains(string(body), `"type":"`+expected+`"`) {
			t.Fatalf("expected %s body=%s err=%v", expected, body, err)
		}
	}
}
