// Package live provides the bounded, in-memory distribution layer for
// normalized Oracle events. Durable history remains in the recorder store.
package live

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ogtrading/overnight-strategy/internal/oracle/book"
	"github.com/ogtrading/overnight-strategy/internal/oracle/model"
)

type subscription struct {
	assets  map[string]bool
	streams map[model.Stream]bool
	queue   chan model.Envelope
}

// Hub retains only current full-depth books and bounded subscriber queues.
// It is not the durable system of record.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[*subscription]struct{}
	books       map[string]book.Snapshot
	latest      map[string]map[model.Stream]model.Envelope
	dropped     uint64
	recent      []model.Envelope
	recentLimit int
}

// BookView is an Oracle-authoritative, bounded DOM projection. Clients render
// it directly and must not reconstruct a book from public venue deltas.
type BookView struct {
	SchemaVersion  string        `json:"schema_version"`
	Asset          string        `json:"asset"`
	At             time.Time     `json:"timestamp"`
	ConnectionID   string        `json:"connection_id"`
	VenueNonce     int64         `json:"venue_nonce"`
	OracleSequence uint64        `json:"oracle_sequence"`
	Depth          int           `json:"depth"`
	Bids           []model.Level `json:"bids"`
	Asks           []model.Level `json:"asks"`
	Quality        model.Quality `json:"quality"`
}

// OrderFlowView is a low-latency trade-derived view over the bounded live
// event buffer. CoverageStart makes partial developing coverage explicit.
type OrderFlowView struct {
	SchemaVersion string        `json:"schema_version"`
	Asset         string        `json:"asset"`
	From          time.Time     `json:"from"`
	To            time.Time     `json:"to"`
	CoverageStart time.Time     `json:"coverage_start,omitempty"`
	CoverageState string        `json:"coverage_state"`
	Trades        int           `json:"trades"`
	BuyVolume     string        `json:"buy_volume"`
	SellVolume    string        `json:"sell_volume"`
	Delta         string        `json:"delta"`
	CVD           string        `json:"cvd"`
	LagMS         int64         `json:"lag_ms"`
	Quality       model.Quality `json:"quality"`
}

func New() *Hub {
	return &Hub{subscribers: map[*subscription]struct{}{}, books: map[string]book.Snapshot{}, latest: map[string]map[model.Stream]model.Envelope{}, recentLimit: 120000}
}

func (h *Hub) SetBook(value book.Snapshot) {
	h.mu.Lock()
	h.books[value.Asset] = value
	h.mu.Unlock()
}

func (h *Hub) ResetBooks() {
	h.mu.Lock()
	h.books = map[string]book.Snapshot{}
	for subscriber := range h.subscribers {
		close(subscriber.queue)
		delete(h.subscribers, subscriber)
	}
	h.mu.Unlock()
}

func (h *Hub) Book(asset string) (book.Snapshot, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	value, ok := h.books[strings.ToUpper(asset)]
	return value, ok
}

func (h *Hub) ReadyBooks() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.books)
}

func (h *Hub) Dropped() uint64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.dropped
}

// Recent returns a copy of matching events so callers can derive bounded live
// analytics without holding the hub lock or accessing execution state.
func (h *Hub) Recent(asset string, streams map[model.Stream]bool, since time.Time) ([]model.Envelope, time.Time) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	asset = strings.ToUpper(asset)
	oldest := time.Time{}
	if len(h.recent) > 0 {
		oldest = h.recent[0].OracleReceivedAt
	}
	out := make([]model.Envelope, 0)
	for _, event := range h.recent {
		if event.Asset != asset || (!since.IsZero() && event.OracleReceivedAt.Before(since)) {
			continue
		}
		if len(streams) > 0 && !streams[event.Stream] {
			continue
		}
		out = append(out, event)
	}
	return out, oldest
}

func (h *Hub) Publish(event model.Envelope) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.recent = append(h.recent, event)
	if h.latest[event.Asset] == nil {
		h.latest[event.Asset] = map[model.Stream]model.Envelope{}
	}
	h.latest[event.Asset][event.Stream] = event
	if overflow := len(h.recent) - h.recentLimit; overflow > 0 {
		copy(h.recent, h.recent[overflow:])
		h.recent = h.recent[:h.recentLimit]
	}
	for subscriber := range h.subscribers {
		if len(subscriber.assets) > 0 && !subscriber.assets[event.Asset] {
			continue
		}
		if len(subscriber.streams) > 0 && !subscriber.streams[event.Stream] {
			continue
		}
		select {
		case subscriber.queue <- event:
		default:
			h.dropped++
			close(subscriber.queue)
			delete(h.subscribers, subscriber)
		}
	}
}

func (h *Hub) ServeBook(w http.ResponseWriter, r *http.Request) {
	asset := strings.ToUpper(r.PathValue("asset"))
	value, ok := h.Book(asset)
	if !ok {
		http.Error(w, "book unavailable until a fresh snapshot is synchronized", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func (h *Hub) ServeMarket(w http.ResponseWriter, r *http.Request) {
	asset := strings.ToUpper(r.PathValue("asset"))
	h.mu.RLock()
	value, bookReady := h.books[asset]
	latest := map[model.Stream]model.Envelope{}
	for stream, event := range h.latest[asset] {
		latest[stream] = event
	}
	h.mu.RUnlock()
	if !bookReady && len(latest) == 0 {
		http.Error(w, "market unavailable", http.StatusServiceUnavailable)
		return
	}
	asOf := value.At
	for _, event := range latest {
		if event.OracleReceivedAt.After(asOf) {
			asOf = event.OracleReceivedAt
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"schema_version": "oracle-market-snapshot-v1", "asset": asset, "as_of": asOf, "book_ready": bookReady, "book": value, "latest": latest})
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 64 * 1024,
	CheckOrigin: func(r *http.Request) bool {
		host := r.Host
		return strings.HasPrefix(host, "127.0.0.1:") || strings.HasPrefix(host, "localhost:")
	},
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	var since time.Time
	if raw := r.URL.Query().Get("since"); raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			http.Error(w, "since must be RFC3339", http.StatusBadRequest)
			return
		}
		since = parsed.UTC()
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	depth := parseDepth(r.URL.Query().Get("depth"))
	subscriber := &subscription{assets: parseAssets(r.URL.Query().Get("assets")), streams: parseStreams(r.URL.Query().Get("streams")), queue: make(chan model.Envelope, 4096)}
	h.mu.Lock()
	h.subscribers[subscriber] = struct{}{}
	initial := make([]book.Snapshot, 0, len(h.books))
	for asset, snapshot := range h.books {
		if len(subscriber.assets) == 0 || subscriber.assets[asset] {
			initial = append(initial, snapshot)
		}
	}
	backfill := make([]model.Envelope, 0)
	for _, event := range h.recent {
		if since.IsZero() || event.OracleReceivedAt.Before(since) {
			continue
		}
		if len(subscriber.assets) > 0 && !subscriber.assets[event.Asset] {
			continue
		}
		if len(subscriber.streams) > 0 && !subscriber.streams[event.Stream] {
			continue
		}
		backfill = append(backfill, event)
	}
	oldest := time.Time{}
	if len(h.recent) > 0 {
		oldest = h.recent[0].OracleReceivedAt
	}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		if _, ok := h.subscribers[subscriber]; ok {
			delete(h.subscribers, subscriber)
			close(subscriber.queue)
		}
		h.mu.Unlock()
	}()
	sort.Slice(initial, func(i, j int) bool { return initial[i].Asset < initial[j].Asset })
	if err := conn.WriteJSON(map[string]any{"type": "backfill_start", "events": len(backfill), "oldest_available": oldest, "truncated": !since.IsZero() && !oldest.IsZero() && since.Before(oldest)}); err != nil {
		return
	}
	for _, event := range backfill {
		if err := conn.WriteJSON(map[string]any{"type": "backfill_event", "event": event}); err != nil {
			return
		}
	}
	for _, snapshot := range initial {
		if err := conn.WriteJSON(map[string]any{"type": "book_snapshot", "snapshot": bookView(snapshot, depth)}); err != nil {
			return
		}
	}
	if err := conn.WriteJSON(map[string]any{"type": "synchronized", "books": len(initial), "at": time.Now().UTC()}); err != nil {
		return
	}
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case event, ok := <-subscriber.queue:
			if !ok {
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteJSON(map[string]any{"type": "event", "event": event}); err != nil {
				return
			}
			if event.Stream == model.StreamBookSnapshot || event.Stream == model.StreamBookDelta {
				if snapshot, exists := h.Book(event.Asset); exists {
					if err := conn.WriteJSON(map[string]any{"type": "book_state", "snapshot": bookView(snapshot, depth)}); err != nil {
						return
					}
				}
			}
		case at := <-heartbeat.C:
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteJSON(map[string]any{"type": "heartbeat", "at": at.UTC(), "books": h.ReadyBooks()}); err != nil {
				return
			}
		}
	}
}

func parseDepth(raw string) int {
	depth, err := strconv.Atoi(raw)
	if err != nil || depth <= 0 {
		return 25
	}
	if depth > 200 {
		return 200
	}
	return depth
}

func bookView(snapshot book.Snapshot, depth int) BookView {
	bids, asks := snapshot.Bids, snapshot.Asks
	if len(bids) > depth {
		bids = bids[:depth]
	}
	if len(asks) > depth {
		asks = asks[:depth]
	}
	return BookView{SchemaVersion: "oracle-book-view-v1", Asset: snapshot.Asset, At: snapshot.At, ConnectionID: snapshot.ConnectionID, VenueNonce: snapshot.VenueNonce, OracleSequence: snapshot.OracleSequence, Depth: depth, Bids: bids, Asks: asks, Quality: snapshot.Quality}
}

func parseAssets(value string) map[string]bool {
	out := map[string]bool{}
	for _, item := range strings.Split(value, ",") {
		if item = strings.ToUpper(strings.TrimSpace(item)); item != "" {
			out[item] = true
		}
	}
	return out
}

func parseStreams(value string) map[model.Stream]bool {
	out := map[model.Stream]bool{}
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out[model.Stream(item)] = true
		}
	}
	return out
}
