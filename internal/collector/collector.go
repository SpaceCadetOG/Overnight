package collector

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ogtrading/overnight-strategy/internal/marketdata/lighter"
	"github.com/ogtrading/overnight-strategy/internal/oracle/live"
	"github.com/ogtrading/overnight-strategy/internal/oracle/model"
	"github.com/ogtrading/overnight-strategy/internal/universe"
)

type Status struct {
	mu                    sync.RWMutex
	Connected             bool      `json:"connected"`
	LastEvent             time.Time `json:"last_event"`
	LastError             string    `json:"last_error,omitempty"`
	Events                uint64    `json:"events"`
	NonceGaps             uint64    `json:"nonce_gaps"`
	Reconnects            uint64    `json:"reconnects"`
	BooksReady            int       `json:"books_ready"`
	Snapshots             uint64    `json:"snapshots"`
	CrossedBooks          uint64    `json:"crossed_books"`
	InvalidLevels         uint64    `json:"invalid_levels"`
	ConfirmedLiquidations uint64    `json:"confirmed_liquidations"`
	InferredCascades      uint64    `json:"inferred_liquidation_cascades"`
	ConnectionID          string    `json:"connection_id,omitempty"`
	ConnectionStartedAt   time.Time `json:"connection_started_at,omitempty"`
	LastRecoveryAt        time.Time `json:"last_recovery_at,omitempty"`
	OracleEvents          uint64    `json:"oracle_events"`
	OracleBooksReady      int       `json:"oracle_books_ready"`
	OracleParityReady     int       `json:"oracle_parity_ready"`
	OracleCheckpoints     uint64    `json:"oracle_checkpoints"`
	OracleParityFailures  uint64    `json:"oracle_parity_failures"`
	OracleSubscriberDrops uint64    `json:"oracle_subscriber_drops"`
	OracleLastError       string    `json:"oracle_last_error,omitempty"`
}

type StatusView struct {
	Connected             bool      `json:"connected"`
	LastEvent             time.Time `json:"last_event"`
	LastError             string    `json:"last_error,omitempty"`
	Events                uint64    `json:"events"`
	NonceGaps             uint64    `json:"nonce_gaps"`
	Reconnects            uint64    `json:"reconnects"`
	BooksReady            int       `json:"books_ready"`
	Snapshots             uint64    `json:"snapshots"`
	CrossedBooks          uint64    `json:"crossed_books"`
	InvalidLevels         uint64    `json:"invalid_levels"`
	ConfirmedLiquidations uint64    `json:"confirmed_liquidations"`
	InferredCascades      uint64    `json:"inferred_liquidation_cascades"`
	ConnectionID          string    `json:"connection_id,omitempty"`
	ConnectionStartedAt   time.Time `json:"connection_started_at,omitempty"`
	LastRecoveryAt        time.Time `json:"last_recovery_at,omitempty"`
	OracleEvents          uint64    `json:"oracle_events"`
	OracleBooksReady      int       `json:"oracle_books_ready"`
	OracleParityReady     int       `json:"oracle_parity_ready"`
	OracleCheckpoints     uint64    `json:"oracle_checkpoints"`
	OracleParityFailures  uint64    `json:"oracle_parity_failures"`
	OracleSubscriberDrops uint64    `json:"oracle_subscriber_drops"`
	OracleLastError       string    `json:"oracle_last_error,omitempty"`
}

func (s *Status) Snapshot() StatusView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return StatusView{Connected: s.Connected, LastEvent: s.LastEvent, LastError: s.LastError, Events: s.Events, NonceGaps: s.NonceGaps, Reconnects: s.Reconnects, BooksReady: s.BooksReady, Snapshots: s.Snapshots, CrossedBooks: s.CrossedBooks, InvalidLevels: s.InvalidLevels, ConfirmedLiquidations: s.ConfirmedLiquidations, InferredCascades: s.InferredCascades, ConnectionID: s.ConnectionID, ConnectionStartedAt: s.ConnectionStartedAt, LastRecoveryAt: s.LastRecoveryAt, OracleEvents: s.OracleEvents, OracleBooksReady: s.OracleBooksReady, OracleParityReady: s.OracleParityReady, OracleCheckpoints: s.OracleCheckpoints, OracleParityFailures: s.OracleParityFailures, OracleSubscriberDrops: s.OracleSubscriberDrops, OracleLastError: s.OracleLastError}
}

type Collector struct {
	BaseURL        string
	WSURL          string
	Store          interface{ Append(string, any) error }
	Status         *Status
	lastNonce      map[string]int64
	books          map[string]*orderBook
	marketIDs      map[string]string
	lastCheckpoint map[string]time.Time
	flow           *liquidationCorrelator
	connectionID   string
	connectionLast time.Time
	recoveryLogged bool
	oracle         *oraclePipeline
	oracleParityOK map[string]bool
	instrumentMu   sync.RWMutex
	instruments    map[string]oracleInstrument
}

type oracleInstrument struct {
	Asset           string          `json:"asset"`
	Venue           string          `json:"venue"`
	VenueSymbol     string          `json:"venue_symbol"`
	MarketID        int16           `json:"market_id"`
	Status          string          `json:"status"`
	MarketType      string          `json:"market_type"`
	MinimumSize     json.RawMessage `json:"minimum_size"`
	MinimumNotional json.RawMessage `json:"minimum_notional"`
	PriceDecimals   int             `json:"price_decimals"`
	SizeDecimals    int             `json:"size_decimals"`
}

type orderBook struct {
	Asks  map[string]string
	Bids  map[string]string
	Nonce int64
}

func New(baseURL, wsURL string, output interface{ Append(string, any) error }) *Collector {
	return NewWithOracleEventStore(baseURL, wsURL, output, output)
}

// NewWithOracleEventStore keeps high-volume normalized event persistence off
// the WebSocket processing path while retaining the durable primary store for
// checkpoints, observations, and existing recorder streams.
func NewWithOracleEventStore(baseURL, wsURL string, output, oracleEvents interface{ Append(string, any) error }) *Collector {
	if strings.TrimSpace(wsURL) == "" {
		// Lighter exposes the same public market-data feed through a read-only
		// route for IPs in restricted regions. The collector never submits orders.
		wsURL = "wss://mainnet.zklighter.elliot.ai/stream?readonly=true"
	} else if !strings.Contains(wsURL, "readonly=") {
		separator := "?"
		if strings.Contains(wsURL, "?") {
			separator = "&"
		}
		wsURL += separator + "readonly=true"
	}
	return &Collector{BaseURL: baseURL, WSURL: wsURL, Store: output, Status: &Status{}, lastNonce: map[string]int64{}, books: map[string]*orderBook{}, marketIDs: map[string]string{}, lastCheckpoint: map[string]time.Time{}, flow: newLiquidationCorrelator(), oracle: newOraclePipelineWithEventStore(output, oracleEvents), oracleParityOK: map[string]bool{}, instruments: map[string]oracleInstrument{}}
}

func (c *Collector) Run(ctx context.Context) error {
	if c.Store == nil {
		return fmt.Errorf("event store is required")
	}
	consecutiveFailures := 0
	for ctx.Err() == nil {
		before := c.Status.Snapshot().Events
		err := c.runOnce(ctx)
		outageStarted := time.Now().UTC()
		statusBefore := c.Status.Snapshot()
		c.Status.mu.Lock()
		c.Status.Connected = false
		if err != nil && err != context.Canceled {
			c.Status.LastError = err.Error()
		}
		c.Status.Reconnects++
		reconnects := c.Status.Reconnects
		c.Status.mu.Unlock()
		if c.Status.Snapshot().Events > before {
			consecutiveFailures = 0
		} else {
			consecutiveFailures++
		}
		delay := 2 * time.Second
		for i := 0; i < consecutiveFailures && delay < 30*time.Second; i++ {
			delay *= 2
		}
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		outageEnded := time.Now().UTC()
		uncertainStarted := statusBefore.LastEvent
		if uncertainStarted.IsZero() || uncertainStarted.After(outageStarted) {
			uncertainStarted = outageStarted
		}
		_ = c.Store.Append("collector_reconnects", map[string]any{"schema_version": 2, "recorded_at": outageEnded, "connection_id": statusBefore.ConnectionID, "connection_started_at": statusBefore.ConnectionStartedAt, "last_event_at": statusBefore.LastEvent, "disconnect_detected_at": outageStarted, "reconnect_started_at": outageStarted, "reconnect_attempt_ended_at": outageEnded, "uncertain_interval_started_at": uncertainStarted, "uncertain_interval_ended_at": outageEnded, "uncertain_interval_ms": outageEnded.Sub(uncertainStarted).Milliseconds(), "retry_delay_ms": outageEnded.Sub(outageStarted).Milliseconds(), "reconnect": reconnects, "reason_category": disconnectCategory(err), "error": errorString(err), "books_ready_after_disconnect": 0, "requires_fresh_snapshots": true})
	}
	return ctx.Err()
}

func disconnectCategory(err error) string {
	if err == nil {
		return "CONNECTION_CLOSED"
	}
	value := strings.ToLower(err.Error())
	switch {
	case strings.Contains(value, "context canceled"):
		return "SHUTDOWN"
	case strings.Contains(value, "nonce gap"):
		return "SEQUENCE_GAP"
	case strings.Contains(value, "timeout"):
		return "READ_TIMEOUT"
	case strings.Contains(value, "reset by peer"):
		return "PEER_RESET"
	case strings.Contains(value, "close 1000"):
		return "NORMAL_CLOSE"
	case strings.Contains(value, "websocket error"):
		return "VENUE_ERROR"
	default:
		return "TRANSPORT_ERROR"
	}
}

func newConnectionID(at time.Time) string {
	var random [8]byte
	_, _ = rand.Read(random[:])
	return fmt.Sprintf("conn_%d_%x", at.UnixMilli(), random[:])
}

func errorString(err error) string {
	if err == nil {
		return "connection_closed"
	}
	return err.Error()
}

func (c *Collector) runOnce(ctx context.Context) error {
	marketMap, err := lighter.New(c.BaseURL, nil).MarketMap(ctx)
	if err != nil {
		return err
	}
	headers := http.Header{"Origin": []string{"https://lighter.xyz"}, "User-Agent": []string{"overnight-strategy-collector/1.0"}}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, c.WSURL, headers)
	if err != nil {
		return err
	}
	defer conn.Close()
	c.Status.mu.Lock()
	c.Status.Connected = true
	c.Status.LastError = ""
	c.Status.BooksReady = 0
	c.Status.OracleBooksReady = 0
	c.Status.OracleParityReady = 0
	c.Status.OracleLastError = ""
	c.Status.mu.Unlock()
	c.books = map[string]*orderBook{}
	c.lastNonce = map[string]int64{}
	c.lastCheckpoint = map[string]time.Time{}
	c.oracle.resetBooks()
	c.oracleParityOK = map[string]bool{}
	connectionStarted := time.Now().UTC()
	c.connectionID = newConnectionID(connectionStarted)
	c.connectionLast = time.Time{}
	c.recoveryLogged = false
	c.Status.mu.Lock()
	c.Status.ConnectionID = c.connectionID
	c.Status.ConnectionStartedAt = connectionStarted
	c.Status.mu.Unlock()
	_ = c.Store.Append("collector_connections", map[string]any{"schema_version": 2, "recorded_at": connectionStarted, "connection_id": c.connectionID, "connection_started_at": connectionStarted, "state": "RESYNCING", "books_ready": 0})
	const subscriptionDelay = 350 * time.Millisecond // below Lighter's 200 client messages/minute limit
	subscribe := func(channel string) error {
		if err := conn.WriteJSON(map[string]any{"type": "subscribe", "channel": channel}); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(subscriptionDelay):
			return nil
		}
	}
	if err := subscribe("market_stats/all"); err != nil {
		return err
	}
	for _, asset := range universe.All() {
		market, ok := marketMap[asset.MarketSymbol()]
		if !ok {
			return fmt.Errorf("market %s (%s) unavailable", asset.Symbol, asset.MarketSymbol())
		}
		c.marketIDs[strconv.Itoa(int(market.MarketID))] = asset.Symbol
		c.instrumentMu.Lock()
		c.instruments[asset.Symbol] = oracleInstrument{Asset: asset.Symbol, Venue: "lighter", VenueSymbol: market.Symbol, MarketID: market.MarketID, Status: market.Status, MarketType: market.MarketType, MinimumSize: market.MinBaseAmount, MinimumNotional: market.MinQuoteAmount, PriceDecimals: market.PriceDecimals, SizeDecimals: market.SizeDecimals}
		c.instrumentMu.Unlock()
		for _, prefix := range []string{"ticker/", "trade/", "order_book/"} {
			if err := subscribe(prefix + strconv.Itoa(int(market.MarketID))); err != nil {
				return err
			}
		}
	}
	go func() { <-ctx.Done(); _ = conn.Close() }()
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		if err := c.record(message); err != nil {
			return err
		}
	}
}

func (c *Collector) record(message []byte) error {
	var envelope map[string]any
	if err := json.Unmarshal(message, &envelope); err != nil {
		return err
	}
	if failure, ok := envelope["error"].(map[string]any); ok {
		return fmt.Errorf("Lighter websocket error code=%v message=%v", failure["code"], failure["message"])
	}
	channel := fmt.Sprint(envelope["channel"])
	if channel == "<nil>" || channel == "" {
		channel = fmt.Sprint(envelope["type"])
	}
	stream := c.recordStream(channel)
	receivedAt := time.Now().UTC()
	record := map[string]any{"schema_version": 2, "received_at": receivedAt, "connection_id": c.connectionID, "connection_started_at": c.Status.Snapshot().ConnectionStartedAt, "channel": channel, "event": envelope}
	if err := c.Store.Append(stream, record); err != nil {
		return err
	}

	// Lighter's top-level nonce is shared across unrelated channels. For each
	// order book, a subscription starts with a complete snapshot and every delta
	// must begin at the preceding nonce. A gap invalidates reconstruction, so the
	// connection is closed and resubscribed to obtain a fresh snapshot.
	if strings.Contains(channel, "order_book") {
		if book, ok := envelope["order_book"].(map[string]any); ok {
			begin, end := int64(number(book["begin_nonce"])), int64(number(book["nonce"]))
			snapshot := fmt.Sprint(envelope["type"]) == "subscribed/order_book"
			if end > 0 {
				if last := c.lastNonce[channel]; !snapshot && last > 0 && begin != last {
					c.Status.mu.Lock()
					c.Status.NonceGaps++
					c.Status.mu.Unlock()
					_ = c.Store.Append("collector_gaps", map[string]any{"at": time.Now().UTC(), "channel": channel, "previous": last, "begin_nonce": begin, "end_nonce": end})
					c.invalidateBook(channel)
					return fmt.Errorf("order-book nonce gap on %s: previous=%d begin=%d end=%d", channel, last, begin, end)
				}
				if err := c.applyOrderBook(channel, envelope["type"], book, end); err != nil {
					return err
				}
				c.lastNonce[channel] = end
			}
		}
	}
	asset := c.assetForChannel(channel)
	oracleEvents, oracleCheckpoints, err := c.oracle.accept(message, asset, receivedAt, c.connectionID)
	if err != nil {
		c.Status.mu.Lock()
		c.Status.OracleParityFailures++
		c.Status.OracleLastError = err.Error()
		if asset != "" {
			c.oracleParityOK[asset] = false
		}
		c.Status.OracleParityReady = c.oracleParityCount()
		c.Status.mu.Unlock()
		// The Oracle remains a shadow consumer until parity is proven. It must
		// never interrupt authoritative raw recording.
		oracleEvents, oracleCheckpoints = 0, 0
	} else if strings.Contains(channel, "order_book") && asset != "" {
		c.Status.mu.Lock()
		if !c.oracleParity(channel, asset) {
			c.Status.OracleParityFailures++
			c.Status.OracleLastError = "book parity mismatch for " + asset
			c.oracleParityOK[asset] = false
		} else {
			c.oracleParityOK[asset] = true
			if c.oracleParityCount() == len(universe.All()) {
				c.Status.OracleLastError = ""
			}
		}
		c.Status.OracleParityReady = c.oracleParityCount()
		c.Status.mu.Unlock()
	}
	if strings.Contains(channel, "trade") {
		if err := c.recordLiquidationResearch(channel, envelope, record["received_at"].(time.Time)); err != nil {
			return err
		}
	}
	if err := c.flushLiquidationWindows(record["received_at"].(time.Time)); err != nil {
		return err
	}
	c.Status.mu.Lock()
	c.Status.Events++
	c.Status.LastEvent = receivedAt
	c.Status.OracleEvents += uint64(oracleEvents)
	c.Status.OracleCheckpoints += uint64(oracleCheckpoints)
	c.Status.OracleBooksReady = c.oracle.hub.ReadyBooks()
	c.Status.OracleSubscriberDrops = c.oracle.hub.Dropped()
	c.Status.mu.Unlock()
	c.connectionLast = receivedAt
	return nil
}

func (c *Collector) oracleParityCount() int {
	ready := 0
	for _, ok := range c.oracleParityOK {
		if ok {
			ready++
		}
	}
	return ready
}

func (c *Collector) oracleParity(channel, asset string) bool {
	legacy := c.books[channel]
	shadow, ok := c.oracle.hub.Book(asset)
	if legacy == nil || !ok || len(shadow.Bids) == 0 || len(shadow.Asks) == 0 {
		return false
	}
	legacyBid, legacyAsk := bestPrices(legacy)
	shadowBid, bidErr := strconv.ParseFloat(shadow.Bids[0].Price, 64)
	shadowAsk, askErr := strconv.ParseFloat(shadow.Asks[0].Price, 64)
	return bidErr == nil && askErr == nil && legacyBid == shadowBid && legacyAsk == shadowAsk && levelsMatch(legacy.Bids, shadow.Bids) && levelsMatch(legacy.Asks, shadow.Asks)
}

func levelsMatch(legacy map[string]string, published []model.Level) bool {
	if len(published) == 0 || len(published) > len(legacy) {
		return false
	}
	for _, level := range published {
		if legacy[level.Price] != level.Size {
			return false
		}
	}
	return true
}

func (c *Collector) assetForChannel(channel string) string {
	parts := strings.FieldsFunc(channel, func(r rune) bool { return r == ':' || r == '/' })
	if len(parts) == 2 {
		return c.marketIDs[parts[1]]
	}
	return ""
}

func (c *Collector) recordStream(channel string) string {
	stream := streamName(channel)
	parts := strings.FieldsFunc(channel, func(r rune) bool { return r == ':' || r == '/' })
	if len(parts) == 2 {
		if symbol := c.marketIDs[parts[1]]; symbol != "" {
			return "asset=" + symbol + "/" + stream
		}
	}
	return stream
}

func (c *Collector) applyOrderBook(channel string, eventType any, payload map[string]any, nonce int64) error {
	snapshot := fmt.Sprint(eventType) == "subscribed/order_book"
	book := c.books[channel]
	if snapshot {
		book = &orderBook{Asks: map[string]string{}, Bids: map[string]string{}}
		c.books[channel] = book
		c.Status.mu.Lock()
		c.Status.Snapshots++
		c.Status.mu.Unlock()
	} else if book == nil {
		return fmt.Errorf("order-book delta before snapshot on %s", channel)
	}
	if err := applyLevels(book.Asks, payload["asks"]); err != nil {
		c.Status.mu.Lock()
		c.Status.InvalidLevels++
		c.Status.mu.Unlock()
		c.invalidateBook(channel)
		return fmt.Errorf("invalid ask level on %s: %w", channel, err)
	}
	if err := applyLevels(book.Bids, payload["bids"]); err != nil {
		c.Status.mu.Lock()
		c.Status.InvalidLevels++
		c.Status.mu.Unlock()
		c.invalidateBook(channel)
		return fmt.Errorf("invalid bid level on %s: %w", channel, err)
	}
	bestBid, bestAsk := bestPrices(book)
	if bestBid > 0 && bestAsk > 0 && bestBid >= bestAsk {
		c.Status.mu.Lock()
		c.Status.CrossedBooks++
		c.Status.mu.Unlock()
		c.invalidateBook(channel)
		return fmt.Errorf("crossed order book on %s: bid=%g ask=%g", channel, bestBid, bestAsk)
	}
	book.Nonce = nonce
	c.Status.mu.Lock()
	c.Status.BooksReady = len(c.books)
	c.Status.mu.Unlock()
	now := time.Now().UTC()
	if snapshot || now.Sub(c.lastCheckpoint[channel]) >= time.Minute {
		parts := strings.FieldsFunc(channel, func(r rune) bool { return r == ':' || r == '/' })
		symbol := ""
		if len(parts) == 2 {
			symbol = c.marketIDs[parts[1]]
		}
		stream := "reconstructed_book_checkpoints"
		if symbol != "" {
			stream = "asset=" + symbol + "/" + stream
		}
		if err := c.Store.Append(stream, map[string]any{"schema_version": 2, "recorded_at": now, "connection_id": c.connectionID, "channel": channel, "symbol": symbol, "nonce": nonce, "best_bid": bestBid, "best_ask": bestAsk, "bid_levels": len(book.Bids), "ask_levels": len(book.Asks)}); err != nil {
			return err
		}
		c.lastCheckpoint[channel] = now
	}
	if snapshot && !c.recoveryLogged && c.Status.Snapshot().BooksReady == len(universe.All()) {
		c.recoveryLogged = true
		c.Status.mu.Lock()
		c.Status.LastRecoveryAt = now
		c.Status.mu.Unlock()
		_ = c.Store.Append("collector_recovery_windows", map[string]any{"schema_version": 1, "recorded_at": now, "connection_id": c.connectionID, "connection_started_at": c.Status.Snapshot().ConnectionStartedAt, "all_books_ready_at": now, "recovery_duration_ms": now.Sub(c.Status.Snapshot().ConnectionStartedAt).Milliseconds(), "books_ready": len(universe.All()), "fresh_snapshots_verified": true})
	}
	return nil
}

func applyLevels(side map[string]string, raw any) error {
	levels, _ := raw.([]any)
	for _, item := range levels {
		level, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("malformed level")
		}
		price, size := fmt.Sprint(level["price"]), fmt.Sprint(level["size"])
		if price == "" || price == "<nil>" {
			return fmt.Errorf("missing price")
		}
		priceValue, priceErr := strconv.ParseFloat(price, 64)
		quantity, quantityErr := strconv.ParseFloat(size, 64)
		if priceErr != nil || priceValue <= 0 || quantityErr != nil || quantity < 0 {
			return fmt.Errorf("price=%q size=%q", price, size)
		}
		if quantity == 0 {
			delete(side, price)
			continue
		}
		side[price] = size
	}
	return nil
}

func (c *Collector) invalidateBook(channel string) {
	delete(c.books, channel)
	delete(c.lastNonce, channel)
	c.Status.mu.Lock()
	c.Status.BooksReady = len(c.books)
	c.Status.mu.Unlock()
}

func bestPrices(book *orderBook) (float64, float64) {
	var bestBid, bestAsk float64
	for raw := range book.Bids {
		price, _ := strconv.ParseFloat(raw, 64)
		if price > bestBid {
			bestBid = price
		}
	}
	for raw := range book.Asks {
		price, _ := strconv.ParseFloat(raw, 64)
		if price > 0 && (bestAsk == 0 || price < bestAsk) {
			bestAsk = price
		}
	}
	return bestBid, bestAsk
}

func (c *Collector) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		status := c.Status.Snapshot()
		code := http.StatusOK
		if !status.Connected {
			code = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(status)
	})
	mux.HandleFunc("GET /v1/books/{asset}", c.oracle.hub.ServeBook)
	mux.HandleFunc("GET /v1/market/{asset}", c.oracle.hub.ServeMarket)
	mux.HandleFunc("GET /v1/live", c.oracle.hub.ServeWS)
	mux.HandleFunc("GET /v1/order-flow", c.serveLiveOrderFlow)
	mux.HandleFunc("GET /v1/instruments", c.serveInstruments)
	return mux
}

func (c *Collector) serveLiveOrderFlow(w http.ResponseWriter, r *http.Request) {
	asset := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("asset")))
	if asset == "" {
		http.Error(w, "asset is required", http.StatusBadRequest)
		return
	}
	window := 5 * time.Minute
	if raw := r.URL.Query().Get("window"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 || parsed > 4*time.Hour {
			http.Error(w, "window must be between 1ns and 4h", http.StatusBadRequest)
			return
		}
		window = parsed
	}
	now := time.Now().UTC()
	from := now.Add(-window)
	events, oldest := c.oracle.hub.Recent(asset, map[model.Stream]bool{model.StreamTrade: true}, from)
	buy, sell := 0.0, 0.0
	quality := model.QualityCertified
	cutoff := time.Time{}
	trades := 0
	for _, event := range events {
		var trade model.Trade
		if json.Unmarshal(event.Payload, &trade) != nil {
			continue
		}
		size, err := strconv.ParseFloat(trade.Size, 64)
		if err != nil || size <= 0 {
			continue
		}
		if strings.EqualFold(trade.AggressorSide, "BUY") {
			buy += size
		} else {
			sell += size
		}
		trades++
		cutoff = event.OracleReceivedAt
		if event.Quality != model.QualityCertified {
			quality = event.Quality
		}
	}
	coverage := "COMPLETE_WINDOW"
	if oldest.IsZero() || oldest.After(from) {
		coverage = "DEVELOPING_PARTIAL"
	}
	lag := int64(0)
	if !cutoff.IsZero() {
		lag = now.Sub(cutoff).Milliseconds()
	}
	response := map[string]any{
		"schema_version": "oracle-live-order-flow-v1", "asset": asset,
		"from": from, "to": now, "information_cutoff": cutoff,
		"coverage_start": oldest, "coverage_state": coverage,
		"trades": trades, "buy_volume": decimalFloat(buy), "sell_volume": decimalFloat(sell),
		"total_volume": decimalFloat(buy + sell), "delta": decimalFloat(buy - sell), "cvd": decimalFloat(buy - sell),
		"lag_ms": lag, "quality": quality,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func decimalFloat(value float64) string {
	formatted := strings.TrimRight(strings.TrimRight(strconv.FormatFloat(value, 'f', 8, 64), "0"), ".")
	if formatted == "-0" || formatted == "" {
		return "0"
	}
	return formatted
}

func (c *Collector) serveInstruments(w http.ResponseWriter, _ *http.Request) {
	c.instrumentMu.RLock()
	defer c.instrumentMu.RUnlock()
	items := make([]oracleInstrument, 0, len(c.instruments))
	for _, asset := range universe.All() {
		if item, ok := c.instruments[asset.Symbol]; ok {
			items = append(items, item)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"schema_version": "oracle-instruments-v1", "instruments": items, "count": len(items)})
}

func (c *Collector) OracleHub() *live.Hub { return c.oracle.hub }

func streamName(channel string) string {
	switch {
	case strings.Contains(channel, "order_book"):
		return "orderbook_events"
	case strings.Contains(channel, "trade"):
		return "trade_flow"
	case strings.Contains(channel, "ticker"):
		return "ticker_events"
	case strings.Contains(channel, "market_stats"):
		return "market_stats"
	default:
		return "collector_events"
	}
}

func number(value any) float64 {
	switch value := value.(type) {
	case float64:
		return value
	case string:
		result, _ := strconv.ParseFloat(value, 64)
		return result
	default:
		return 0
	}
}
