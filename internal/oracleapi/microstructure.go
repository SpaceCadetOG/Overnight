package oracleapi

import (
	"bufio"
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
)

const microstructureModel = "displayed-liquidity-correlation-v1"

type liquidityObservation struct {
	At, WindowStart                 time.Time
	Side, Price, Previous, New, Act string
	Delta                           float64
	ConnectionID, Quality           string
	VenueSequence, OracleSequence   uint64
}

type liquidityBatch struct {
	WindowStart  time.Time         `json:"window_start"`
	WindowEnd    time.Time         `json:"window_end"`
	ConnectionID string            `json:"connection_id"`
	BookQuality  string            `json:"book_quality"`
	Changes      []json.RawMessage `json:"changes"`
}

type liquidityEvidence struct {
	At             time.Time `json:"at"`
	Side           string    `json:"side"`
	Price          string    `json:"price"`
	PreviousSize   string    `json:"previous_size"`
	NewSize        string    `json:"new_size"`
	SizeDelta      string    `json:"size_delta"`
	Action         string    `json:"action"`
	Correlation    string    `json:"correlation"`
	Confidence     string    `json:"confidence"`
	MatchedVolume  string    `json:"matched_trade_volume"`
	Replenishments int       `json:"replenishments"`
	Absorption     bool      `json:"absorption_candidate"`
	BookQuality    string    `json:"book_quality"`
	ConnectionID   string    `json:"connection_id"`
}

func (s *Server) liquidity(w http.ResponseWriter, r *http.Request) {
	in, err := parseAnalyticsInput(r)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	if in.To.Sub(in.From) > maxQueryRange {
		writeError(w, 400, errors.New("liquidity queries are limited to 24h; request adjacent pages"))
		return
	}
	observations, packages, quality, err := s.loadLiquidity(r, in)
	if err != nil {
		writeError(w, statusForArchiveError(err), err)
		return
	}
	trades, _, status, err := s.analyticTrades(r, in)
	if err != nil {
		writeError(w, status, err)
		return
	}
	window := 1500 * time.Millisecond
	if raw := r.URL.Query().Get("correlation_window_ms"); raw != "" {
		ms, parseErr := strconv.Atoi(raw)
		if parseErr != nil || ms < 1 || ms > 10000 {
			writeError(w, 400, errors.New("correlation_window_ms must be between 1 and 10000"))
			return
		}
		window = time.Duration(ms) * time.Millisecond
	}
	evidence := correlateLiquidity(observations, trades, window)
	writeJSON(w, 200, map[string]any{"schema_version": "oracle-liquidity-analysis-v1", "model": microstructureModel, "asset": in.Asset, "from": in.From, "to": in.To, "information_cutoff": in.To, "events": evidence, "count": len(evidence), "packages": packages, "quality": quality, "disclaimer": "Executed and cancelled labels are correlation inferences from displayed-book changes and trades, not exchange order lifecycle facts."})
}

func (s *Server) heatmap(w http.ResponseWriter, r *http.Request) {
	in, err := parseAnalyticsInput(r)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	if in.To.Sub(in.From) > maxQueryRange {
		writeError(w, 400, errors.New("heatmap queries are limited to 24h; request adjacent pages"))
		return
	}
	observations, packages, quality, err := s.loadLiquidity(r, in)
	if err != nil {
		writeError(w, statusForArchiveError(err), err)
		return
	}
	type bin struct {
		Side, Price    string
		Added, Removed float64
		Updates        int
		First, Last    time.Time
		LastSize       string
	}
	bins := map[string]*bin{}
	for _, event := range observations {
		key := event.Side + ":" + event.Price
		b := bins[key]
		if b == nil {
			b = &bin{Side: event.Side, Price: event.Price, First: event.At}
			bins[key] = b
		}
		b.Last = event.At
		b.LastSize = event.New
		b.Updates++
		if event.Delta > 0 {
			b.Added += event.Delta
		} else {
			b.Removed += -event.Delta
		}
	}
	out := make([]map[string]any, 0, len(bins))
	for _, b := range bins {
		out = append(out, map[string]any{"side": b.Side, "price": b.Price, "added": decimal(b.Added), "removed": decimal(b.Removed), "net": decimal(b.Added - b.Removed), "last_size": b.LastSize, "updates": b.Updates, "first_observed": b.First, "last_observed": b.Last})
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := strconv.ParseFloat(out[i]["price"].(string), 64)
		b, _ := strconv.ParseFloat(out[j]["price"].(string), 64)
		return a < b
	})
	writeJSON(w, 200, map[string]any{"schema_version": "oracle-heatmap-v1", "model": "displayed-liquidity-heatmap-v1", "asset": in.Asset, "from": in.From, "to": in.To, "information_cutoff": in.To, "levels": out, "packages": packages, "quality": quality})
}

func (s *Server) openInterest(w http.ResponseWriter, r *http.Request) {
	in, err := parseAnalyticsInput(r)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	if in.To.Sub(in.From) > maxQueryRange {
		writeError(w, 400, errors.New("open-interest queries are limited to 24h; request adjacent pages"))
		return
	}
	points, packages, quality, err := s.loadOpenInterest(r, in)
	if err != nil {
		writeError(w, statusForArchiveError(err), err)
		return
	}
	writeJSON(w, 200, map[string]any{"schema_version": "oracle-open-interest-v1", "asset": in.Asset, "from": in.From, "to": in.To, "information_cutoff": in.To, "points": points, "count": len(points), "packages": packages, "quality": quality})
}

func (s *Server) loadLiquidity(r *http.Request, in analyticsInput) ([]liquidityObservation, []string, []string, error) {
	var out []liquidityObservation
	packages, quality, err := s.scanArchiveFiles(r, in, "asset="+in.Asset+"/liquidity_observations.jsonl.zst", func(line []byte) error {
		var batch liquidityBatch
		if err := json.Unmarshal(line, &batch); err != nil {
			return err
		}
		for _, raw := range batch.Changes {
			var values []json.RawMessage
			if json.Unmarshal(raw, &values) != nil || len(values) < 7 {
				continue
			}
			var ms int64
			var side, price, previous, next, delta, action string
			_ = json.Unmarshal(values[0], &ms)
			_ = json.Unmarshal(values[1], &side)
			_ = json.Unmarshal(values[2], &price)
			_ = json.Unmarshal(values[3], &previous)
			_ = json.Unmarshal(values[4], &next)
			_ = json.Unmarshal(values[5], &delta)
			_ = json.Unmarshal(values[6], &action)
			at := time.UnixMilli(ms).UTC()
			if at.Before(in.From) || !at.Before(in.To) {
				continue
			}
			d, _ := strconv.ParseFloat(delta, 64)
			var venue, oracle uint64
			if len(values) > 7 {
				_ = json.Unmarshal(values[7], &venue)
			}
			if len(values) > 8 {
				_ = json.Unmarshal(values[8], &oracle)
			}
			out = append(out, liquidityObservation{At: at, WindowStart: batch.WindowStart, Side: side, Price: price, Previous: previous, New: next, Delta: d, Act: action, ConnectionID: batch.ConnectionID, Quality: batch.BookQuality, VenueSequence: venue, OracleSequence: oracle})
		}
		return nil
	})
	return out, packages, quality, err
}

func (s *Server) scanArchiveFiles(r *http.Request, in analyticsInput, rel string, consume func([]byte) error) ([]string, []string, error) {
	if packages, qualities, used, err := s.scanIndexedArchiveFiles(r, in, rel, consume); used {
		return packages, qualities, err
	}
	dates, err := chicagoDates(in.From, in.To)
	if err != nil {
		return nil, nil, err
	}
	var packages, qualities []string
	for _, date := range dates {
		m, dir, err := s.find("lighter-" + date)
		if err != nil {
			return packages, qualities, fmt.Errorf("archive for Chicago date %s is unavailable", date)
		}
		q, _, err := usableQuality(m, in.Allow)
		if err != nil {
			return packages, qualities, err
		}
		spec, ok := findManifestFile(m, rel)
		if !ok {
			continue
		}
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err = s.verifyFile(path, spec.SHA256); err != nil {
			return packages, qualities, err
		}
		f, err := os.Open(path)
		if err != nil {
			return packages, qualities, err
		}
		zr, err := zstd.NewReader(f)
		if err != nil {
			f.Close()
			return packages, qualities, err
		}
		scanner := bufio.NewScanner(zr)
		scanner.Buffer(make([]byte, 64<<10), 32<<20)
		for scanner.Scan() {
			if err := r.Context().Err(); err != nil {
				zr.Close()
				f.Close()
				return packages, qualities, err
			}
			if err := consume(scanner.Bytes()); err != nil {
				zr.Close()
				f.Close()
				return packages, qualities, err
			}
		}
		err = scanner.Err()
		zr.Close()
		f.Close()
		if err != nil {
			return packages, qualities, err
		}
		packages = append(packages, m.PackageID)
		qualities = appendUnique(qualities, string(q))
	}
	if len(packages) == 0 {
		return packages, qualities, errors.New("requested stream is unavailable in matching archives")
	}
	return packages, qualities, nil
}

func correlateLiquidity(events []liquidityObservation, trades []analyticTrade, window time.Duration) []liquidityEvidence {
	replenishments := map[string]int{}
	out := make([]liquidityEvidence, 0, len(events))
	for _, event := range events {
		key := event.Side + ":" + event.Price
		if event.Delta > 0 {
			replenishments[key]++
		}
		matched := 0.0
		price, _ := strconv.ParseFloat(event.Price, 64)
		for _, trade := range trades {
			if trade.At.Before(event.At.Add(-window)) || trade.At.After(event.At.Add(window)) {
				continue
			}
			if price > 0 && abs(trade.Price-price)/price <= 0.00001 {
				matched += trade.Size
			}
		}
		classification, confidence := "OBSERVED_CHANGE", "NONE"
		removed := -event.Delta
		if removed > 0 {
			if matched >= removed*0.5 {
				classification, confidence = "LIKELY_EXECUTED", "MEDIUM"
			} else if matched == 0 {
				classification, confidence = "LIKELY_CANCELLED", "MEDIUM"
			} else {
				classification, confidence = "UNKNOWN", "LOW"
			}
		}
		absorption := removed > 0 && matched > 0 && replenishments[key] > 1
		out = append(out, liquidityEvidence{At: event.At, Side: event.Side, Price: event.Price, PreviousSize: event.Previous, NewSize: event.New, SizeDelta: decimal(event.Delta), Action: event.Act, Correlation: classification, Confidence: confidence, MatchedVolume: decimal(matched), Replenishments: replenishments[key], Absorption: absorption, BookQuality: event.Quality, ConnectionID: event.ConnectionID})
	}
	return out
}

func (s *Server) loadOpenInterest(r *http.Request, in analyticsInput) ([]map[string]any, []string, []string, error) {
	var out []map[string]any
	packages, quality, err := s.scanArchiveFiles(r, in, "market_stats.jsonl.zst", func(line []byte) error {
		var row struct {
			ReceivedAt time.Time `json:"received_at"`
			Event      struct {
				MarketStats map[string]json.RawMessage `json:"market_stats"`
				Timestamp   int64                      `json:"timestamp"`
			} `json:"event"`
		}
		if json.Unmarshal(line, &row) != nil {
			return nil
		}
		at := row.ReceivedAt
		if row.Event.Timestamp > 0 {
			at = time.UnixMilli(row.Event.Timestamp).UTC()
		}
		if at.Before(in.From) || !at.Before(in.To) {
			return nil
		}
		for id, raw := range row.Event.MarketStats {
			var stat struct {
				Symbol       string          `json:"symbol"`
				OpenInterest json.RawMessage `json:"open_interest"`
				MarkPrice    json.RawMessage `json:"mark_price"`
			}
			if json.Unmarshal(raw, &stat) != nil || !strings.EqualFold(stat.Symbol, in.Asset) {
				continue
			}
			var oi, mark any
			_ = json.Unmarshal(stat.OpenInterest, &oi)
			_ = json.Unmarshal(stat.MarkPrice, &mark)
			out = append(out, map[string]any{"at": at, "market_id": id, "open_interest": oi, "mark_price": mark})
		}
		return nil
	})
	return out, packages, quality, err
}

func statusForArchiveError(err error) int {
	if strings.Contains(err.Error(), "unavailable") || strings.Contains(err.Error(), "absent") {
		return 404
	}
	return 422
}
func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
