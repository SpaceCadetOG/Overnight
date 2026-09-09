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

func fixture(t *testing.T) *Server {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "date=2026-09-08")
	if err := os.MkdirAll(filepath.Join(dir, "asset=BTC"), 0o750); err != nil {
		t.Fatal(err)
	}
	event := []byte("event\n")
	if err := os.WriteFile(filepath.Join(dir, "asset=BTC", "orderbook_events.jsonl.zst"), event, 0o640); err != nil {
		t.Fatal(err)
	}
	certificate := []byte(`{"classification":"CERTIFIED","assets":[{"symbol":"BTC","windows":[{"certified":true}]}]}`)
	if err := os.WriteFile(filepath.Join(dir, "RECORDER_CERTIFICATE.json"), certificate, 0o640); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{SchemaVersion: 4, PackageID: "lighter-2026-09-08", Date: "2026-09-08", Assets: []string{"BTC"}, CertificationClass: "CERTIFIED", Files: []File{{Path: "asset=BTC/orderbook_events.jsonl.zst", Compressed: int64(len(event)), Records: 1}}}
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
	if r.Code != http.StatusPartialContent || r.Body.String() != "eve" {
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
