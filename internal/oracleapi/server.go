package oracleapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type File struct {
	Path       string `json:"path"`
	Compressed int64  `json:"compressed_bytes"`
	Records    uint64 `json:"records"`
	SHA256     string `json:"sha256"`
}

type Manifest struct {
	SchemaVersion      int               `json:"schema_version"`
	PackageID          string            `json:"package_id"`
	CollectorVersion   string            `json:"collector_version"`
	CollectorCommit    string            `json:"collector_commit"`
	Exchange           string            `json:"exchange"`
	Date               string            `json:"date"`
	Timezone           string            `json:"timezone"`
	GeneratedAt        time.Time         `json:"generated_at"`
	Files              []File            `json:"files"`
	Records            uint64            `json:"records"`
	RawBytes           int64             `json:"raw_bytes"`
	NonceGaps          uint64            `json:"nonce_gaps"`
	WSErrors           uint64            `json:"websocket_errors"`
	Reconnects         uint64            `json:"reconnects"`
	FirstEvent         time.Time         `json:"first_event"`
	LastEvent          time.Time         `json:"last_event"`
	Assets             []string          `json:"assets"`
	Missing            []string          `json:"missing_assets,omitempty"`
	Complete           bool              `json:"complete"`
	RecorderCertified  bool              `json:"recorder_certified"`
	CertificationClass string            `json:"certification_class,omitempty"`
	CertifiedWindows   int               `json:"certified_windows,omitempty"`
	TotalWindows       int               `json:"total_windows,omitempty"`
	UncertainIntervals uint64            `json:"uncertain_intervals,omitempty"`
	UncertainMillis    int64             `json:"uncertain_interval_ms,omitempty"`
	LongestUncertainMS int64             `json:"longest_uncertain_interval_ms,omitempty"`
	DisconnectReasons  map[string]uint64 `json:"disconnect_reasons,omitempty"`
}

type Server struct {
	root       string
	version    string
	commit     string
	startedAt  time.Time
	querySlot  chan struct{}
	checksumMu sync.Mutex
	checksums  map[string]cachedChecksum
	liveProxy  *httputil.ReverseProxy
}

type cachedChecksum struct {
	Size    int64
	ModTime time.Time
	Digest  string
}

func New(root, version, commit string) (*Server, error) {
	if root == "" {
		return nil, errors.New("archive root is required")
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("archive root unavailable: %w", err)
	}
	backend, _ := url.Parse("http://127.0.0.1:8082")
	proxy := httputil.NewSingleHostReverseProxy(backend)
	proxy.FlushInterval = -1
	return &Server{root: root, version: version, commit: commit, startedAt: time.Now().UTC(), querySlot: make(chan struct{}, 1), checksums: map[string]cachedChecksum{}, liveProxy: proxy}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /v1/health", s.health)
	mux.HandleFunc("GET /v1/readiness", s.readiness)
	mux.HandleFunc("GET /v1/version", s.health)
	mux.HandleFunc("GET /v1/datasets", s.datasets)
	mux.HandleFunc("GET /v1/datasets/{package}/manifest", s.manifest)
	mux.HandleFunc("GET /v1/coverage", s.coverage)
	mux.HandleFunc("GET /v1/quality/windows", s.quality)
	mux.HandleFunc("GET /v1/downloads/{package}/{path...}", s.download)
	mux.HandleFunc("GET /v1/trades", s.trades)
	mux.HandleFunc("GET /v1/events", s.events)
	mux.HandleFunc("GET /v1/candles", s.candles)
	mux.HandleFunc("GET /v1/profiles", s.profiles)
	mux.HandleFunc("GET /v1/footprints", s.footprints)
	mux.HandleFunc("GET /v1/order-flow", s.orderFlow)
	mux.HandleFunc("GET /v1/session-definitions", s.sessionDefinitions)
	mux.HandleFunc("GET /v1/sessions", s.sessions)
	mux.HandleFunc("GET /v1/levels", s.levels)
	mux.HandleFunc("GET /v1/zones", s.zones)
	mux.HandleFunc("GET /v1/books/{asset}", s.live)
	mux.HandleFunc("GET /v1/books/{asset}/at", s.bookAt)
	mux.HandleFunc("GET /v1/market/{asset}", s.live)
	mux.HandleFunc("GET /v1/instruments", s.live)
	mux.HandleFunc("GET /v1/live", s.live)
	mux.HandleFunc("GET /v1/replay", s.replay)
	return securityHeaders(mux)
}

func (s *Server) verifyFile(path, expected string) error {
	if expected == "" {
		return errors.New("manifest checksum is missing")
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	s.checksumMu.Lock()
	if cached, ok := s.checksums[path]; ok && cached.Size == info.Size() && cached.ModTime.Equal(info.ModTime()) && strings.EqualFold(cached.Digest, expected) {
		s.checksumMu.Unlock()
		return nil
	}
	s.checksumMu.Unlock()
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(digest, expected) {
		return fmt.Errorf("archive checksum mismatch")
	}
	s.checksumMu.Lock()
	s.checksums[path] = cachedChecksum{Size: info.Size(), ModTime: info.ModTime(), Digest: digest}
	s.checksumMu.Unlock()
	return nil
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"service": "Market Data Oracle", "mode": "read-only", "version": s.version, "commit": s.commit, "started_at": s.startedAt, "live_backend": "http://127.0.0.1:8082"})
}

func (s *Server) live(w http.ResponseWriter, r *http.Request) {
	if s.liveProxy == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("live Oracle backend unavailable"))
		return
	}
	s.liveProxy.ServeHTTP(w, r)
}

func (s *Server) readiness(w http.ResponseWriter, _ *http.Request) {
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	response, err := client.Get("http://127.0.0.1:8082/healthz")
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ready": false, "reason": "collector_unreachable"})
		return
	}
	defer response.Body.Close()
	var status struct {
		Connected         bool      `json:"connected"`
		BooksReady        int       `json:"books_ready"`
		OracleBooksReady  int       `json:"oracle_books_ready"`
		OracleParityReady int       `json:"oracle_parity_ready"`
		OracleLastError   string    `json:"oracle_last_error"`
		LastEvent         time.Time `json:"last_event"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&status) != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ready": false, "reason": "collector_unhealthy"})
		return
	}
	age := time.Since(status.LastEvent)
	ready := status.Connected && status.BooksReady == 12 && status.OracleBooksReady == 12 && status.OracleParityReady == 12 && status.OracleLastError == "" && age >= 0 && age <= 10*time.Second
	code := http.StatusOK
	if !ready {
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, map[string]any{"ready": ready, "collector_connected": status.Connected, "books_ready": status.BooksReady, "oracle_books_ready": status.OracleBooksReady, "oracle_parity_ready": status.OracleParityReady, "oracle_last_error": status.OracleLastError, "last_event": status.LastEvent, "event_age_ms": age.Milliseconds()})
}

func (s *Server) datasets(w http.ResponseWriter, r *http.Request) {
	items, err := s.catalog()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	from, to, asset := r.URL.Query().Get("from"), r.URL.Query().Get("to"), strings.ToUpper(r.URL.Query().Get("asset"))
	out := make([]Manifest, 0, len(items))
	for _, item := range items {
		if from != "" && item.Date < from || to != "" && item.Date > to || asset != "" && !contains(item.Assets, asset) {
			continue
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"datasets": out, "count": len(out)})
}

func (s *Server) manifest(w http.ResponseWriter, r *http.Request) {
	m, _, err := s.find(r.PathValue("package"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) coverage(w http.ResponseWriter, r *http.Request) {
	items, err := s.catalog()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	asset := strings.ToUpper(r.URL.Query().Get("asset"))
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	type row struct {
		PackageID          string    `json:"package_id"`
		Date               string    `json:"date"`
		FirstEvent         time.Time `json:"first_event"`
		LastEvent          time.Time `json:"last_event"`
		Certification      string    `json:"certification"`
		UncertainIntervals uint64    `json:"uncertain_intervals"`
		UncertainMillis    int64     `json:"uncertain_interval_ms"`
	}
	rows := []row{}
	for _, item := range items {
		if from != "" && item.Date < from || to != "" && item.Date > to || asset != "" && !contains(item.Assets, asset) {
			continue
		}
		classification := item.CertificationClass
		if classification == "" {
			if item.RecorderCertified {
				classification = "CERTIFIED"
			} else {
				classification = "LEGACY_UNCERTIFIED"
			}
		}
		rows = append(rows, row{item.PackageID, item.Date, item.FirstEvent, item.LastEvent, classification, item.UncertainIntervals, item.UncertainMillis})
	}
	writeJSON(w, http.StatusOK, map[string]any{"asset": asset, "coverage": rows, "count": len(rows)})
}

func (s *Server) quality(w http.ResponseWriter, r *http.Request) {
	packageID := r.URL.Query().Get("package_id")
	if packageID == "" {
		writeError(w, http.StatusBadRequest, errors.New("package_id is required"))
		return
	}
	_, dir, err := s.find(packageID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	body, err := os.ReadFile(filepath.Join(dir, "RECORDER_CERTIFICATE.json"))
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("quality certificate unavailable"))
		return
	}
	var certificate any
	if json.Unmarshal(body, &certificate) != nil {
		writeError(w, http.StatusInternalServerError, errors.New("quality certificate is invalid"))
		return
	}
	writeJSON(w, http.StatusOK, certificate)
}

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	m, dir, err := s.find(r.PathValue("package"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	rel := filepath.Clean(r.PathValue("path"))
	if rel == "." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}
	allowed := rel == "MANIFEST.json" || rel == "SHA256SUMS"
	for _, file := range m.Files {
		if filepath.Clean(file.Path) == rel {
			allowed = true
			break
		}
	}
	if !allowed {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(dir, rel)
	file, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(rel)))
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	http.ServeContent(w, r, filepath.Base(rel), info.ModTime(), file)
}

func (s *Server) catalog() ([]Manifest, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	items := []Manifest{}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "date=") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(s.root, entry.Name(), "MANIFEST.json"))
		if err != nil {
			continue
		}
		var item Manifest
		if json.Unmarshal(body, &item) == nil && item.PackageID != "" {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Date > items[j].Date })
	return items, nil
}

func (s *Server) find(packageID string) (Manifest, string, error) {
	if packageID == "" || strings.ContainsAny(packageID, `/\\`) {
		return Manifest{}, "", errors.New("dataset not found")
	}
	items, err := s.catalog()
	if err != nil {
		return Manifest{}, "", err
	}
	for _, item := range items {
		if item.PackageID == packageID {
			return item, filepath.Join(s.root, "date="+item.Date), nil
		}
	}
	return Manifest{}, "", errors.New("dataset not found")
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if strings.EqualFold(value, wanted) {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
