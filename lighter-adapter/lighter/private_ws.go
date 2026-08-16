package lighter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Trade struct {
	TradeID         int64  `json:"trade_id"`
	TradeIDString   string `json:"trade_id_str"`
	TxHash          string `json:"tx_hash"`
	Type            string `json:"type"`
	MarketID        int16  `json:"market_id"`
	Size            string `json:"size"`
	Price           string `json:"price"`
	USDAmount       string `json:"usd_amount"`
	AskID           int64  `json:"ask_id"`
	BidID           int64  `json:"bid_id"`
	AskClientID     int64  `json:"ask_client_id"`
	BidClientID     int64  `json:"bid_client_id"`
	AskAccountID    int64  `json:"ask_account_id"`
	BidAccountID    int64  `json:"bid_account_id"`
	IsMakerAsk      bool   `json:"is_maker_ask"`
	BlockHeight     int64  `json:"block_height"`
	Timestamp       int64  `json:"timestamp"`
	TakerFee        int64  `json:"taker_fee"`
	MakerFee        int64  `json:"maker_fee"`
	TransactionTime int64  `json:"transaction_time"`
}

type AccountStatsValues struct {
	Collateral       string `json:"collateral"`
	PortfolioValue   string `json:"portfolio_value"`
	Leverage         string `json:"leverage"`
	AvailableBalance string `json:"available_balance"`
	MarginUsage      string `json:"margin_usage"`
	BuyingPower      string `json:"buying_power"`
}

type AccountStats struct {
	AccountStatsValues
	AccountTradingMode int                `json:"account_trading_mode"`
	CrossStats         AccountStatsValues `json:"cross_stats"`
	TotalStats         AccountStatsValues `json:"total_stats"`
	Timestamp          int64              `json:"timestamp"`
}

type AccountTransaction struct {
	Hash             string `json:"hash"`
	Type             int    `json:"type"`
	Info             string `json:"info"`
	EventInfo        string `json:"event_info"`
	Status           int    `json:"status"`
	TransactionIndex int64  `json:"transaction_index"`
	AccountIndex     int64  `json:"account_index"`
	Nonce            int64  `json:"nonce"`
	APIKeyIndex      int    `json:"api_key_index"`
	SequenceIndex    int64  `json:"sequence_index"`
	TransactionTime  int64  `json:"transaction_time"`
}

type StreamEventType string

const (
	StreamConnected    StreamEventType = "CONNECTED"
	StreamDisconnected StreamEventType = "DISCONNECTED"
	StreamReconciled   StreamEventType = "REST_RECONCILED"
	StreamOrder        StreamEventType = "ORDER"
	StreamTrade        StreamEventType = "TRADE"
	StreamPosition     StreamEventType = "POSITION"
	StreamAccount      StreamEventType = "ACCOUNT"
	StreamTransaction  StreamEventType = "TRANSACTION"
)

type StreamEvent struct {
	Type        StreamEventType     `json:"type"`
	Channel     string              `json:"channel,omitempty"`
	Generation  uint64              `json:"generation"`
	ReceivedAt  int64               `json:"received_at"`
	Order       *ReconciledOrder    `json:"order,omitempty"`
	Trade       *Trade              `json:"trade,omitempty"`
	Position    *CanonicalPosition  `json:"position,omitempty"`
	Account     *AccountStats       `json:"account,omitempty"`
	Transaction *AccountTransaction `json:"transaction,omitempty"`
	Recovery    *RecoveryReport     `json:"recovery,omitempty"`
	Err         string              `json:"error,omitempty"`
}

type PrivateStreamSnapshot struct {
	Connected         bool                        `json:"connected"`
	Generation        uint64                      `json:"generation"`
	NeedsRESTRecovery bool                        `json:"needs_rest_recovery"`
	Orders            map[int64]ReconciledOrder   `json:"orders"`
	Trades            map[int64]Trade             `json:"trades"`
	Positions         map[int16]CanonicalPosition `json:"positions"`
	Account           *AccountStats               `json:"account,omitempty"`
	LastChannelTime   map[string]int64            `json:"last_channel_time"`
	LastTxSequence    int64                       `json:"last_tx_sequence"`
}

type PrivateStreamConfig struct {
	URL          string
	PingInterval time.Duration
	ReconnectMin time.Duration
	ReconnectMax time.Duration
	EventBuffer  int
}

func (c PrivateStreamConfig) normalized(manager *Manager) (PrivateStreamConfig, error) {
	if c.URL == "" {
		base, err := url.Parse(manager.BaseURL)
		if err != nil {
			return c, err
		}
		switch base.Scheme {
		case "https":
			base.Scheme = "wss"
		case "http":
			base.Scheme = "ws"
		default:
			return c, fmt.Errorf("unsupported Lighter URL scheme %q", base.Scheme)
		}
		base.Path = "/stream"
		base.RawQuery = ""
		c.URL = base.String()
	}
	parsed, err := url.Parse(c.URL)
	if err != nil || (parsed.Scheme != "ws" && parsed.Scheme != "wss") {
		return c, fmt.Errorf("invalid WebSocket URL %q", c.URL)
	}
	if c.PingInterval == 0 {
		c.PingInterval = 30 * time.Second
	}
	if c.PingInterval <= 0 || c.PingInterval >= 2*time.Minute {
		return c, errors.New("ping interval must be positive and less than two minutes")
	}
	if c.ReconnectMin == 0 {
		c.ReconnectMin = 250 * time.Millisecond
	}
	if c.ReconnectMax == 0 {
		c.ReconnectMax = 10 * time.Second
	}
	if c.ReconnectMin <= 0 || c.ReconnectMax < c.ReconnectMin {
		return c, errors.New("invalid reconnect backoff")
	}
	if c.EventBuffer == 0 {
		c.EventBuffer = 256
	}
	if c.EventBuffer < 1 {
		return c, errors.New("event buffer must be positive")
	}
	return c, nil
}

type streamConn interface {
	ReadJSON(any) error
	WriteJSON(any) error
	WriteControl(int, []byte, time.Time) error
	SetReadDeadline(time.Time) error
	SetPongHandler(func(string) error)
	Close() error
}

type streamDialer interface {
	DialContext(context.Context, string, http.Header) (streamConn, error)
}

type gorillaStreamDialer struct{ dialer *websocket.Dialer }

func (d gorillaStreamDialer) DialContext(ctx context.Context, endpoint string, headers http.Header) (streamConn, error) {
	connection, response, err := d.dialer.DialContext(ctx, endpoint, headers)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	return connection, err
}

type PrivateStream struct {
	manager   *Manager
	execution Execution
	config    PrivateStreamConfig
	dialer    streamDialer
	events    chan StreamEvent

	mu       sync.RWMutex
	snapshot PrivateStreamSnapshot
	active   streamConn
}

func NewPrivateStream(manager *Manager, execution Execution, config PrivateStreamConfig) (*PrivateStream, error) {
	if manager == nil || execution == nil {
		return nil, errors.New("manager and execution are required")
	}
	config, err := config.normalized(manager)
	if err != nil {
		return nil, err
	}
	return &PrivateStream{
		manager: manager, execution: execution, config: config,
		dialer: gorillaStreamDialer{dialer: websocket.DefaultDialer}, events: make(chan StreamEvent, config.EventBuffer),
		snapshot: PrivateStreamSnapshot{
			NeedsRESTRecovery: true, Orders: make(map[int64]ReconciledOrder), Trades: make(map[int64]Trade),
			Positions: make(map[int16]CanonicalPosition), LastChannelTime: make(map[string]int64),
		},
	}, nil
}

func (s *PrivateStream) Events() <-chan StreamEvent { return s.events }

func (s *PrivateStream) Snapshot() PrivateStreamSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	body, _ := json.Marshal(s.snapshot)
	var clone PrivateStreamSnapshot
	_ = json.Unmarshal(body, &clone)
	return clone
}

// ForceReconnect closes the current socket. Run will mark the stream stale,
// perform exchange-authoritative REST recovery, and establish a new generation.
func (s *PrivateStream) ForceReconnect() error {
	s.mu.RLock()
	connection := s.active
	s.mu.RUnlock()
	if connection == nil {
		return errors.New("private stream is not connected")
	}
	return connection.Close()
}

func (s *PrivateStream) emit(event StreamEvent) error {
	event.ReceivedAt = time.Now().UnixMilli()
	s.mu.RLock()
	event.Generation = s.snapshot.Generation
	s.mu.RUnlock()
	select {
	case s.events <- event:
		return nil
	default:
		s.mu.Lock()
		s.snapshot.NeedsRESTRecovery = true
		s.mu.Unlock()
		return errors.New("private stream event buffer full")
	}
}

type wsEnvelope struct {
	Type         string               `json:"type"`
	Channel      string               `json:"channel"`
	Timestamp    int64                `json:"timestamp"`
	Orders       map[string][]Order   `json:"orders"`
	Positions    map[string]Position  `json:"positions"`
	Trades       json.RawMessage      `json:"trades"`
	Stats        *AccountStats        `json:"stats"`
	Transactions []AccountTransaction `json:"txs"`
}

func flattenTrades(raw json.RawMessage) ([]Trade, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var list []Trade
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}
	var byMarket map[string][]Trade
	if err := json.Unmarshal(raw, &byMarket); err != nil {
		return nil, fmt.Errorf("decode trades: %w", err)
	}
	for _, trades := range byMarket {
		list = append(list, trades...)
	}
	return list, nil
}

func envelopeTime(envelope wsEnvelope) int64 {
	if envelope.Timestamp != 0 {
		return envelope.Timestamp
	}
	var latest int64
	for _, orders := range envelope.Orders {
		for _, order := range orders {
			if order.TransactionTime > latest {
				latest = order.TransactionTime
			}
		}
	}
	return latest
}

func (s *PrivateStream) applyEnvelope(envelope wsEnvelope) error {
	channelTime := envelopeTime(envelope)
	s.mu.Lock()
	if previous := s.snapshot.LastChannelTime[envelope.Channel]; channelTime != 0 && channelTime <= previous {
		s.mu.Unlock()
		return nil // duplicate or stale event
	}
	if channelTime != 0 {
		s.snapshot.LastChannelTime[envelope.Channel] = channelTime
	}
	s.mu.Unlock()

	for _, orders := range envelope.Orders {
		for _, order := range orders {
			canonical := reconcile(order)
			s.mu.Lock()
			s.snapshot.Orders[order.ClientOrderIndex] = *canonical
			s.mu.Unlock()
			if err := s.emit(StreamEvent{Type: StreamOrder, Channel: envelope.Channel, Order: canonical}); err != nil {
				return err
			}
		}
	}
	trades, err := flattenTrades(envelope.Trades)
	if err != nil {
		return err
	}
	for i := range trades {
		trade := trades[i]
		s.mu.Lock()
		s.snapshot.Trades[trade.TradeID] = trade
		s.mu.Unlock()
		if err := s.emit(StreamEvent{Type: StreamTrade, Channel: envelope.Channel, Trade: &trade}); err != nil {
			return err
		}
	}
	for _, raw := range envelope.Positions {
		position, err := normalizePosition(raw)
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.snapshot.Positions[position.MarketID] = position
		s.mu.Unlock()
		if err := s.emit(StreamEvent{Type: StreamPosition, Channel: envelope.Channel, Position: &position}); err != nil {
			return err
		}
	}
	if envelope.Stats != nil {
		stats := *envelope.Stats
		stats.Timestamp = envelope.Timestamp
		s.mu.Lock()
		s.snapshot.Account = &stats
		s.mu.Unlock()
		if err := s.emit(StreamEvent{Type: StreamAccount, Channel: envelope.Channel, Account: &stats}); err != nil {
			return err
		}
	}
	if len(envelope.Transactions) != 0 {
		sort.Slice(envelope.Transactions, func(i, j int) bool {
			return envelope.Transactions[i].SequenceIndex < envelope.Transactions[j].SequenceIndex
		})
		for i := range envelope.Transactions {
			transaction := envelope.Transactions[i]
			s.mu.Lock()
			previous := s.snapshot.LastTxSequence
			if previous != 0 && transaction.SequenceIndex <= previous {
				s.mu.Unlock()
				continue
			}
			if previous != 0 && transaction.SequenceIndex > previous+1 {
				s.snapshot.NeedsRESTRecovery = true
				s.mu.Unlock()
				return fmt.Errorf("account transaction sequence gap: previous=%d next=%d", previous, transaction.SequenceIndex)
			}
			s.snapshot.LastTxSequence = transaction.SequenceIndex
			s.mu.Unlock()
			if err := s.emit(StreamEvent{Type: StreamTransaction, Channel: envelope.Channel, Transaction: &transaction}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *PrivateStream) applyRecovery(report *RecoveryReport) error {
	s.mu.Lock()
	s.snapshot.Orders = make(map[int64]ReconciledOrder)
	for _, order := range report.ActiveOrders {
		canonical := reconcile(order)
		s.snapshot.Orders[canonical.ClientOrderIndex] = *canonical
	}
	s.snapshot.Positions = make(map[int16]CanonicalPosition)
	if report.Positions != nil {
		for _, position := range report.Positions.Positions {
			s.snapshot.Positions[position.MarketID] = position
		}
	}
	s.snapshot.LastChannelTime = make(map[string]int64)
	s.snapshot.LastTxSequence = 0
	s.snapshot.NeedsRESTRecovery = false
	s.mu.Unlock()
	return s.emit(StreamEvent{Type: StreamReconciled, Recovery: report})
}

func subscription(channel, auth string) map[string]string {
	message := map[string]string{"type": "subscribe", "channel": channel}
	if auth != "" {
		message["auth"] = auth
	}
	return message
}

func (s *PrivateStream) subscribe(connection streamConn, auth string) error {
	account := strconv.FormatInt(s.manager.AccountIndex, 10)
	channels := []map[string]string{
		subscription("account_all_orders/"+account, auth),
		subscription("account_tx/"+account, auth),
		subscription("account_all_trades/"+account, auth),
		subscription("account_all_positions/"+account, auth),
		subscription("user_stats/"+account, auth),
	}
	for _, message := range channels {
		if err := connection.WriteJSON(message); err != nil {
			return fmt.Errorf("subscribe %s: %w", message["channel"], err)
		}
	}
	return nil
}

func (s *PrivateStream) serve(ctx context.Context, connection streamConn) error {
	readTimeout := 2 * s.config.PingInterval
	_ = connection.SetReadDeadline(time.Now().Add(readTimeout))
	connection.SetPongHandler(func(string) error {
		return connection.SetReadDeadline(time.Now().Add(readTimeout))
	})
	readResult := make(chan error, 1)
	go func() {
		for {
			var envelope wsEnvelope
			if err := connection.ReadJSON(&envelope); err != nil {
				readResult <- err
				return
			}
			if envelope.Type == "pong" {
				continue
			}
			if err := s.applyEnvelope(envelope); err != nil {
				readResult <- err
				return
			}
		}
	}()
	ticker := time.NewTicker(s.config.PingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readResult:
			return err
		case <-ticker.C:
			if err := connection.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return err
			}
		}
	}
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *PrivateStream) Run(ctx context.Context) error {
	backoff := s.config.ReconnectMin
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		report, err := s.execution.Reconcile(ctx)
		if err != nil {
			if waitErr := waitContext(ctx, backoff); waitErr != nil {
				return waitErr
			}
			backoff = min(backoff*2, s.config.ReconnectMax)
			continue
		}
		if err := s.applyRecovery(report); err != nil {
			return err
		}
		auth, err := s.manager.authToken()
		if err != nil {
			return err
		}
		connection, err := s.dialer.DialContext(ctx, s.config.URL, nil)
		if err != nil {
			if waitErr := waitContext(ctx, backoff); waitErr != nil {
				return waitErr
			}
			backoff = min(backoff*2, s.config.ReconnectMax)
			continue
		}
		if err := s.subscribe(connection, auth); err != nil {
			_ = connection.Close()
			if waitErr := waitContext(ctx, backoff); waitErr != nil {
				return waitErr
			}
			continue
		}
		s.mu.Lock()
		s.snapshot.Connected = true
		s.snapshot.Generation++
		s.active = connection
		s.mu.Unlock()
		if err := s.emit(StreamEvent{Type: StreamConnected}); err != nil {
			_ = connection.Close()
			return err
		}
		backoff = s.config.ReconnectMin
		serveErr := s.serve(ctx, connection)
		_ = connection.Close()
		s.mu.Lock()
		s.active = nil
		s.snapshot.Connected = false
		s.snapshot.NeedsRESTRecovery = true
		s.mu.Unlock()
		_ = s.emit(StreamEvent{Type: StreamDisconnected, Err: serveErr.Error()})
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := waitContext(ctx, backoff); err != nil {
			return err
		}
	}
}
