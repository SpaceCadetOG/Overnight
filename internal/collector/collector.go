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
	"github.com/ogtrading/overnight-strategy/internal/store"
	"github.com/ogtrading/overnight-strategy/internal/universe"
)

type Status struct {
	mu         sync.RWMutex
	Connected  bool      `json:"connected"`
	LastEvent  time.Time `json:"last_event"`
	LastError  string    `json:"last_error,omitempty"`
	Events     uint64    `json:"events"`
	NonceGaps  uint64    `json:"nonce_gaps"`
	Reconnects uint64    `json:"reconnects"`
}

type StatusView struct {
	Connected  bool      `json:"connected"`
	LastEvent  time.Time `json:"last_event"`
	LastError  string    `json:"last_error,omitempty"`
	Events     uint64    `json:"events"`
	NonceGaps  uint64    `json:"nonce_gaps"`
	Reconnects uint64    `json:"reconnects"`
}

func (s *Status) Snapshot() StatusView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return StatusView{Connected: s.Connected, LastEvent: s.LastEvent, LastError: s.LastError, Events: s.Events, NonceGaps: s.NonceGaps, Reconnects: s.Reconnects}
}

type Collector struct {
	BaseURL   string
	WSURL     string
	Store     *store.JSONL
	Status    *Status
	lastNonce map[string]int64
}

func New(baseURL, wsURL string, output *store.JSONL) *Collector {
	if strings.TrimSpace(wsURL) == "" {
		wsURL = "wss://mainnet.zklighter.elliot.ai/stream"
	}
	return &Collector{BaseURL: baseURL, WSURL: wsURL, Store: output, Status: &Status{}, lastNonce: map[string]int64{}}
}

func (c *Collector) Run(ctx context.Context) error {
	if c.Store == nil {
		return fmt.Errorf("event store is required")
	}
	for ctx.Err() == nil {
		err := c.runOnce(ctx)
		c.Status.mu.Lock()
		c.Status.Connected = false
		if err != nil && err != context.Canceled {
			c.Status.LastError = err.Error()
		}
		c.Status.Reconnects++
		c.Status.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return ctx.Err()
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
	c.Status.mu.Unlock()
	if err := conn.WriteJSON(map[string]any{"type": "subscribe", "channel": "market_stats/all"}); err != nil {
		return err
	}
	for _, asset := range universe.All() {
		market, ok := marketMap[asset.Symbol]
		if !ok {
			return fmt.Errorf("market %s unavailable", asset.Symbol)
		}
		for _, prefix := range []string{"ticker/", "trade/", "order_book/"} {
			if err := conn.WriteJSON(map[string]any{"type": "subscribe", "channel": prefix + strconv.Itoa(int(market.MarketID))}); err != nil {
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
	channel := fmt.Sprint(envelope["channel"])
	if channel == "<nil>" || channel == "" {
		channel = fmt.Sprint(envelope["type"])
	}
	// Lighter's top-level nonce is shared across unrelated channels. Order-book
	// reconstruction continuity is expressed by the nested begin_nonce/nonce
	// range, so only that range is checked per market.
	if strings.Contains(channel, "order_book") {
		if book, ok := envelope["order_book"].(map[string]any); ok {
			begin, end := int64(number(book["begin_nonce"])), int64(number(book["nonce"]))
			if end > 0 {
				if last := c.lastNonce[channel]; last > 0 && begin > last+1 {
					c.Status.mu.Lock()
					c.Status.NonceGaps++
					c.Status.mu.Unlock()
					_ = c.Store.Append("collector_gaps", map[string]any{"at": time.Now().UTC(), "channel": channel, "previous": last, "begin_nonce": begin, "end_nonce": end})
				}
				c.lastNonce[channel] = end
			}
		}
	}
	stream := streamName(channel)
	record := map[string]any{"received_at": time.Now().UTC(), "channel": channel, "event": envelope}
	if err := c.Store.Append(stream, record); err != nil {
		return err
	}
	c.Status.mu.Lock()
	c.Status.Events++
	c.Status.LastEvent = time.Now().UTC()
	c.Status.mu.Unlock()
	return nil
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
