package oracleapi

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/ogtrading/overnight-strategy/internal/oracle/model"
	oraclelighter "github.com/ogtrading/overnight-strategy/internal/oracle/normalize/lighter"
)

const (
	defaultQueryLimit = 1000
	maxQueryLimit     = 5000
	maxQueryRange     = 24 * time.Hour
)

type queryCursor struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

type queryResponse struct {
	Events     []model.Envelope `json:"events"`
	Count      int              `json:"count"`
	NextCursor string           `json:"next_cursor,omitempty"`
	Packages   []string         `json:"packages"`
	Quality    []string         `json:"quality"`
	Excluded   int              `json:"excluded_events"`
}

type eventQuery struct {
	Asset            string
	Streams          map[model.Stream]bool
	From, To         time.Time
	Limit            int
	After            queryCursor
	AllowUncertified bool
}

type archiveRow struct {
	ReceivedAt   time.Time       `json:"received_at"`
	ConnectionID string          `json:"connection_id"`
	Event        json.RawMessage `json:"event"`
}

type qualityCertificate struct {
	Assets []struct {
		Symbol  string `json:"symbol"`
		Windows []struct {
			StartedAt time.Time `json:"started_at"`
			EndedAt   time.Time `json:"ended_at"`
			Certified bool      `json:"certified"`
		} `json:"windows"`
	} `json:"assets"`
}

type certifiedWindow struct{ Start, End time.Time }

func (s *Server) trades(w http.ResponseWriter, r *http.Request) {
	s.serveQuery(w, r, map[model.Stream]bool{model.StreamTrade: true, model.StreamConfirmedLiquidation: true})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	streams, err := parseStreams(r.URL.Query().Get("stream"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.serveQuery(w, r, streams)
}

func (s *Server) serveQuery(w http.ResponseWriter, r *http.Request, forced map[model.Stream]bool) {
	query, err := parseEventQuery(r, forced)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	select {
	case s.querySlot <- struct{}{}:
		defer func() { <-s.querySlot }()
	default:
		w.Header().Set("Retry-After", "5")
		writeError(w, http.StatusTooManyRequests, errors.New("historical query already running"))
		return
	}
	result, status, err := s.runQuery(r, query)
	if err != nil {
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func parseEventQuery(r *http.Request, forced map[model.Stream]bool) (eventQuery, error) {
	q := r.URL.Query()
	asset := strings.ToUpper(strings.TrimSpace(q.Get("asset")))
	if asset == "" || strings.ContainsAny(asset, `/\\`) {
		return eventQuery{}, errors.New("asset is required")
	}
	from, err := time.Parse(time.RFC3339Nano, q.Get("from"))
	if err != nil {
		return eventQuery{}, errors.New("from must be RFC3339")
	}
	to, err := time.Parse(time.RFC3339Nano, q.Get("to"))
	if err != nil {
		return eventQuery{}, errors.New("to must be RFC3339")
	}
	if !to.After(from) || to.Sub(from) > maxQueryRange {
		return eventQuery{}, errors.New("time range must be positive and no greater than 24h")
	}
	limit := defaultQueryLimit
	if raw := q.Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil {
			return eventQuery{}, errors.New("limit must be an integer")
		}
	}
	if limit < 1 || limit > maxQueryLimit {
		return eventQuery{}, fmt.Errorf("limit must be between 1 and %d", maxQueryLimit)
	}
	streams := forced
	if streams == nil {
		streams, err = parseStreams(q.Get("stream"))
		if err != nil {
			return eventQuery{}, err
		}
	}
	after, err := decodeCursor(q.Get("cursor"))
	if err != nil {
		return eventQuery{}, err
	}
	return eventQuery{Asset: asset, Streams: streams, From: from.UTC(), To: to.UTC(), Limit: limit, After: after, AllowUncertified: q.Get("allow_uncertified") == "true"}, nil
}

func parseStreams(raw string) (map[model.Stream]bool, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("stream is required")
	}
	result := map[model.Stream]bool{}
	for _, item := range strings.Split(raw, ",") {
		stream := model.Stream(strings.TrimSpace(item))
		switch stream {
		case model.StreamTrade, model.StreamConfirmedLiquidation, model.StreamBookSnapshot, model.StreamBookDelta, model.StreamTicker:
			result[stream] = true
		default:
			return nil, fmt.Errorf("unsupported stream %q", stream)
		}
	}
	return result, nil
}

func (s *Server) runQuery(r *http.Request, query eventQuery) (queryResponse, int, error) {
	dates, err := chicagoDates(query.From, query.To)
	if err != nil {
		return queryResponse{}, 500, err
	}
	result := queryResponse{Events: []model.Envelope{}, Packages: []string{}, Quality: []string{}}
	for _, date := range dates {
		manifest, dir, err := s.find("lighter-" + date)
		if err != nil {
			return result, http.StatusNotFound, fmt.Errorf("archive for Chicago date %s is unavailable", date)
		}
		quality, status, err := usableQuality(manifest, query.AllowUncertified)
		if err != nil {
			return result, status, err
		}
		if !contains(manifest.Assets, query.Asset) {
			return result, 404, fmt.Errorf("asset %s is absent from %s", query.Asset, manifest.PackageID)
		}
		windows, err := loadCertifiedWindows(dir, query.Asset)
		if err != nil && quality != model.QualityChecksumVerifiedUncertified {
			return result, 422, err
		}
		if quality == model.QualityCertifiedWithExcludedIntervals && len(windows) == 0 {
			quality = model.QualityChecksumVerifiedUncertified
		}
		result.Packages = append(result.Packages, manifest.PackageID)
		result.Quality = appendUnique(result.Quality, string(quality))
		paths := streamPaths(query.Asset, query.Streams)
		for _, rel := range paths {
			fileSpec, ok := findManifestFile(manifest, rel)
			if !ok {
				continue
			}
			path := filepath.Join(dir, filepath.FromSlash(rel))
			if err := s.verifyFile(path, fileSpec.SHA256); err != nil {
				return result, 422, fmt.Errorf("%s: %w", rel, err)
			}
			full, excluded, err := scanEvents(r, path, manifest.Date, query, quality, windows, result.Events)
			result.Excluded += excluded
			if err != nil {
				return result, 500, err
			}
			result.Events = full
		}
	}
	sort.SliceStable(result.Events, func(i, j int) bool {
		left, right := eventTime(result.Events[i]), eventTime(result.Events[j])
		if left.Equal(right) {
			return result.Events[i].EventID < result.Events[j].EventID
		}
		return left.Before(right)
	})
	if len(result.Events) > query.Limit {
		result.Events = result.Events[:query.Limit]
	}
	result.Count = len(result.Events)
	if result.Count == query.Limit {
		last := result.Events[result.Count-1]
		result.NextCursor = encodeCursor(queryCursor{At: eventTime(last), ID: last.EventID})
	}
	return result, 200, nil
}

func scanEvents(r *http.Request, path, packageDate string, query eventQuery, quality model.Quality, windows []certifiedWindow, existing []model.Envelope) ([]model.Envelope, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return existing, 0, err
	}
	defer file.Close()
	decoder, err := zstd.NewReader(file)
	if err != nil {
		return existing, 0, err
	}
	defer decoder.Close()
	scanner := bufio.NewScanner(decoder)
	scanner.Buffer(make([]byte, 64<<10), 32<<20)
	base, err := sequenceBase(packageDate)
	if err != nil {
		return existing, 0, err
	}
	sequence, excluded := uint64(1), 0
	streamSequence := map[model.Stream]uint64{}
	for scanner.Scan() {
		if err := r.Context().Err(); err != nil {
			return existing, excluded, err
		}
		var row archiveRow
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return existing, excluded, err
		}
		events, err := oraclelighter.Normalize(row.Event, oraclelighter.Context{Asset: query.Asset, ReceivedAt: row.ReceivedAt, ConnectionID: row.ConnectionID, FirstSequence: sequence, DefaultQuality: quality})
		if err != nil {
			continue
		}
		sequence += uint64(len(events))
		for _, event := range events {
			streamSequence[event.Stream]++
			event.OracleSequence = base + streamSequence[event.Stream]
			event.EventID = event.DeterministicID()
			if !query.Streams[event.Stream] {
				continue
			}
			at := eventTime(event)
			if at.Before(query.From) || !at.Before(query.To) {
				continue
			}
			if len(windows) > 0 && !inCertifiedWindow(event.OracleReceivedAt, windows) {
				excluded++
				continue
			}
			if !afterCursor(event, query.After) {
				continue
			}
			existing = append(existing, event)
		}
	}
	return existing, excluded, scanner.Err()
}

func sequenceBase(date string) (uint64, error) {
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil {
		return 0, fmt.Errorf("invalid package date %q", date)
	}
	value, err := strconv.ParseUint(parsed.Format("20060102"), 10, 64)
	if err != nil {
		return 0, err
	}
	return value * 1_000_000_000, nil
}

func streamPaths(asset string, streams map[model.Stream]bool) []string {
	set := map[string]bool{}
	for stream := range streams {
		switch stream {
		case model.StreamTrade, model.StreamConfirmedLiquidation:
			set["asset="+asset+"/trade_flow.jsonl.zst"] = true
		case model.StreamBookSnapshot, model.StreamBookDelta:
			set["asset="+asset+"/orderbook_events.jsonl.zst"] = true
		case model.StreamTicker:
			set["asset="+asset+"/ticker_events.jsonl.zst"] = true
		}
	}
	paths := make([]string, 0, len(set))
	for path := range set {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func usableQuality(m Manifest, allow bool) (model.Quality, int, error) {
	switch m.CertificationClass {
	case "CERTIFIED":
		return model.QualityCertified, 200, nil
	case "CERTIFIED_WITH_EXCLUDED_INTERVALS":
		return model.QualityCertifiedWithExcludedIntervals, 200, nil
	case "QUARANTINED":
		return model.QualityQuarantined, 422, fmt.Errorf("package %s is quarantined", m.PackageID)
	case "":
		if allow {
			return model.QualityChecksumVerifiedUncertified, 200, nil
		}
		return model.QualityUnavailable, 422, fmt.Errorf("package %s is legacy uncertified; set allow_uncertified=true for exploratory use", m.PackageID)
	default:
		return model.QualityUnavailable, 422, fmt.Errorf("unsupported certification class %q", m.CertificationClass)
	}
}

func chicagoDates(from, to time.Time) ([]string, error) {
	loc, err := time.LoadLocation("America/Chicago")
	if err != nil {
		return nil, err
	}
	first := from.In(loc)
	last := to.Add(-time.Nanosecond).In(loc)
	day := time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, loc)
	end := time.Date(last.Year(), last.Month(), last.Day(), 0, 0, 0, 0, loc)
	var out []string
	for !day.After(end) {
		out = append(out, day.Format("2006-01-02"))
		day = day.AddDate(0, 0, 1)
	}
	return out, nil
}

func loadCertifiedWindows(dir, asset string) ([]certifiedWindow, error) {
	body, err := os.ReadFile(filepath.Join(dir, "RECORDER_CERTIFICATE.json"))
	if err != nil {
		return nil, errors.New("quality certificate unavailable")
	}
	var cert qualityCertificate
	if err := json.Unmarshal(body, &cert); err != nil {
		return nil, errors.New("quality certificate invalid")
	}
	for _, item := range cert.Assets {
		if strings.EqualFold(item.Symbol, asset) {
			var out []certifiedWindow
			for _, w := range item.Windows {
				if w.Certified {
					out = append(out, certifiedWindow{w.StartedAt, w.EndedAt})
				}
			}
			return out, nil
		}
	}
	return nil, fmt.Errorf("quality certificate has no asset %s", asset)
}

func findManifestFile(m Manifest, path string) (File, bool) {
	for _, f := range m.Files {
		if filepath.ToSlash(f.Path) == path {
			return f, true
		}
	}
	return File{}, false
}
func inCertifiedWindow(at time.Time, windows []certifiedWindow) bool {
	for _, w := range windows {
		if !at.Before(w.Start) && !at.After(w.End) {
			return true
		}
	}
	return false
}
func eventTime(e model.Envelope) time.Time {
	if e.ExchangeTimestamp != nil {
		return e.ExchangeTimestamp.UTC()
	}
	return e.OracleReceivedAt.UTC()
}
func appendUnique(values []string, value string) []string {
	for _, v := range values {
		if v == value {
			return values
		}
	}
	return append(values, value)
}
func afterCursor(e model.Envelope, c queryCursor) bool {
	if c.At.IsZero() {
		return true
	}
	at := eventTime(e)
	return at.After(c.At) || (at.Equal(c.At) && e.EventID > c.ID)
}
func encodeCursor(c queryCursor) string {
	body, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(body)
}
func decodeCursor(raw string) (queryCursor, error) {
	if raw == "" {
		return queryCursor{}, nil
	}
	body, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return queryCursor{}, errors.New("invalid cursor")
	}
	var c queryCursor
	if json.Unmarshal(body, &c) != nil || c.At.IsZero() || c.ID == "" {
		return queryCursor{}, errors.New("invalid cursor")
	}
	return c, nil
}
