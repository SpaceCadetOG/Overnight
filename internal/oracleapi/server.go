package oracleapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	root      string
	version   string
	commit    string
	startedAt time.Time
}

func New(root, version, commit string) (*Server, error) {
	if root == "" {
		return nil, errors.New("archive root is required")
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("archive root unavailable: %w", err)
	}
	return &Server{root: root, version: version, commit: commit, startedAt: time.Now().UTC()}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /v1/version", s.health)
	mux.HandleFunc("GET /v1/datasets", s.datasets)
	mux.HandleFunc("GET /v1/datasets/{package}/manifest", s.manifest)
	mux.HandleFunc("GET /v1/coverage", s.coverage)
	mux.HandleFunc("GET /v1/quality/windows", s.quality)
	mux.HandleFunc("GET /v1/downloads/{package}/{path...}", s.download)
	return securityHeaders(mux)
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
	writeJSON(w, http.StatusOK, map[string]any{"service": "Market Data Oracle", "mode": "read-only", "version": s.version, "commit": s.commit, "started_at": s.startedAt})
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
