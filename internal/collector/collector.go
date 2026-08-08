package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ogtrading/overnight-strategy/internal/marketdata/lighter"
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
	ConfirmedLiquidations uint64    `json:"confirmed_liquidations"`
	InferredCascades      uint64    `json:"inferred_liquidation_cascades"`
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
	ConfirmedLiquidations uint64    `json:"confirmed_liquidations"`
	InferredCascades      uint64    `json:"inferred_liquidation_cascades"`
}

func (s *Status) Snapshot() StatusView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return StatusView{Connected: s.Connected, LastEvent: s.LastEvent, LastError: s.LastError, Events: s.Events, NonceGaps: s.NonceGaps, Reconnects: s.Reconnects, BooksReady: s.BooksReady, Snapshots: s.Snapshots, ConfirmedLiquidations: s.ConfirmedLiquidations, InferredCascades: s.InferredCascades}
}

type Collector struct {
	BaseURL   string
	WSURL     string
	Store     interface{ Append(string, any) error }
	Status    *Status
	lastNonce map[string]int64
	books     map[string]*orderBook
	marketIDs map[string]string
	flow      *liquidationCorrelator
}

type orderBook struct {
	Asks  map[string]string
	Bids  map[string]string
	Nonce int64
}

func New(baseURL, wsURL string, output interface{ Append(string, any) error }) *Collector {
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
	return &Collector{BaseURL: baseURL, WSURL: wsURL, Store: output, Status: &Status{}, lastNonce: map[string]int64{}, books: map[string]*orderBook{}, marketIDs: map[string]string{}, flow: newLiquidationCorrelator()}
}

func (c *Collector) Run(ctx context.Context) error {
	if c.Store == nil {
		return fmt.Errorf("event store is required")
	}
	consecutiveFailures := 0
	for ctx.Err() == nil {
		before := c.Status.Snapshot().Events
		err := c.runOnce(ctx)
		c.Status.mu.Lock()
		c.Status.Connected = false
		if err != nil && err != context.Canceled {
			c.Status.LastError = err.Error()
		}
		c.Status.Reconnects++
		reconnects := c.Status.Reconnects
		c.Status.mu.Unlock()
		_ = c.Store.Append("collector_reconnects", map[string]any{"recorded_at": time.Now().UTC(), "reconnect": reconnects, "error": errorString(err)})
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
	}
	return ctx.Err()
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
	c.Status.mu.Unlock()
	c.books = map[string]*orderBook{}
	c.lastNonce = map[string]int64{}
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
	record := map[string]any{"received_at": time.Now().UTC(), "channel": channel, "event": envelope}
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
					delete(c.books, channel)
					delete(c.lastNonce, channel)
					return fmt.Errorf("order-book nonce gap on %s: previous=%d begin=%d end=%d", channel, last, begin, end)
				}
				if err := c.applyOrderBook(channel, envelope["type"], book, end); err != nil {
					return err
				}
				c.lastNonce[channel] = end
			}
		}
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
	c.Status.LastEvent = time.Now().UTC()
	c.Status.mu.Unlock()
	return nil
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
	applyLevels(book.Asks, payload["asks"])
	applyLevels(book.Bids, payload["bids"])
	book.Nonce = nonce
	c.Status.mu.Lock()
	c.Status.BooksReady = len(c.books)
	c.Status.mu.Unlock()
	return nil
}

func applyLevels(side map[string]string, raw any) {
	levels, _ := raw.([]any)
	for _, item := range levels {
		level, _ := item.(map[string]any)
		price, size := fmt.Sprint(level["price"]), fmt.Sprint(level["size"])
		if price == "" || price == "<nil>" {
			continue
		}
		quantity, err := strconv.ParseFloat(size, 64)
		if err == nil && quantity == 0 {
			delete(side, price)
			continue
		}
		side[price] = size
	}
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
	return mux
}

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
