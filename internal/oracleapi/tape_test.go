package oracleapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestLegacyIndexWithoutConnectionIDProducesCandles(t *testing.T) {
	server := fixture(t)
	dir := filepath.Join(server.root, "date=2026-09-08")
	manifestPath := filepath.Join(dir, "MANIFEST.json")
	var manifest Manifest
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.SchemaVersion = 2
	manifest.CertificationClass = ""
	legacy, _ := json.Marshal(manifest)
	if err := os.WriteFile(manifestPath, legacy, 0o640); err != nil {
		t.Fatal(err)
	}
	certificate := filepath.Join(dir, "RECORDER_CERTIFICATE.json")
	if err := os.Remove(certificate); err != nil {
		t.Fatal(err)
	}

	tradePath := filepath.Join(dir, "asset=BTC", "trade_flow.jsonl.zst")
	trades := zstdBody(t, `{"received_at":"2026-09-08T12:01:00Z","event":{"channel":"trade:1","type":"update/trade","trades":[{"trade_id":"41","timestamp":1788868860000,"price":"60001.10","size":"0.00120","usd_amount":"72.00132","is_maker_ask":true}],"liquidation_trades":[]}}`+"\n")
	if err := os.WriteFile(tradePath, trades, 0o640); err != nil {
		t.Fatal(err)
	}
	manifest.Files[1].Compressed = int64(len(trades))
	manifest.Files[1].Records = 1
	manifest.Files[1].SHA256 = digest(trades)
	legacy, _ = json.Marshal(manifest)
	if err := os.WriteFile(manifestPath, legacy, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := BuildIndexes(server.root, server.indexRoot, manifest.PackageID); err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRecorder()
	path := "/v1/candles?asset=BTC&from=2026-09-08T12:01:00Z&to=2026-09-08T12:02:00Z&interval=1m&allow_uncertified=true"
	server.Handler().ServeHTTP(r, httptest.NewRequest(http.MethodGet, path, nil))
	if r.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}
	var response struct {
		Candles []candle `json:"candles"`
		Quality []string `json:"quality"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Candles) != 1 || response.Candles[0].Start != time.Date(2026, 9, 8, 12, 1, 0, 0, time.UTC) {
		t.Fatalf("unexpected candles: %+v", response.Candles)
	}
	if len(response.Quality) != 1 || response.Quality[0] != "CHECKSUM_VERIFIED_UNCERTIFIED" {
		t.Fatalf("legacy quality was not preserved: %v", response.Quality)
	}
}

func TestLegacyQualityReportsObservedConnectionWindowsWithoutCertification(t *testing.T) {
	server := fixture(t)
	dir := filepath.Join(server.root, "date=2026-09-08")
	manifestPath := filepath.Join(dir, "MANIFEST.json")
	var manifest Manifest
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.SchemaVersion = 2
	manifest.CertificationClass = ""
	manifest.FirstEvent = time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	manifest.LastEvent = time.Date(2026, 9, 8, 12, 5, 0, 0, time.UTC)
	events := zstdBody(t, `{"channel":"connected","event":{"type":"connected"},"received_at":"2026-09-08T12:02:00Z"}`+"\n")
	reconnects := zstdBody(t, `{"recorded_at":"2026-09-08T12:01:00Z","error":"test"}`+"\n")
	for _, file := range []struct {
		name string
		body []byte
	}{
		{"collector_events.jsonl.zst", events},
		{"collector_reconnects.jsonl.zst", reconnects},
	} {
		if err := os.WriteFile(filepath.Join(dir, file.name), file.body, 0o640); err != nil {
			t.Fatal(err)
		}
		manifest.Files = append(manifest.Files, File{Path: file.name, Compressed: int64(len(file.body)), Records: 1, SHA256: digest(file.body)})
	}
	legacy, _ := json.Marshal(manifest)
	if err := os.WriteFile(manifestPath, legacy, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "RECORDER_CERTIFICATE.json")); err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRecorder()
	server.Handler().ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/quality/windows?package_id="+manifest.PackageID, nil))
	if r.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}
	var response struct {
		Classification string `json:"classification"`
		CoverageBasis  string `json:"coverage_basis"`
		Assets         []struct {
			Symbol  string           `json:"symbol"`
			Windows []observedWindow `json:"windows"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Classification != "LEGACY_UNCERTIFIED" || response.CoverageBasis != "collector_connection_logs" || len(response.Assets) == 0 || len(response.Assets[0].Windows) != 2 {
		t.Fatalf("unexpected legacy coverage: %+v", response)
	}
	for _, window := range response.Assets[0].Windows {
		if window.Certified || !window.Connected {
			t.Fatalf("legacy window was mislabeled: %+v", window)
		}
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
