package lighter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestPrivateStream(t *testing.T, buffer int) *PrivateStream {
	t.Helper()
	manager := &Manager{BaseURL: "https://example.invalid", AccountIndex: 77}
	stream, err := NewPrivateStream(manager, &stubExecution{}, PrivateStreamConfig{EventBuffer: buffer})
	if err != nil {
		t.Fatal(err)
	}
	return stream
}

func TestPrivateStreamAppliesCanonicalEvents(t *testing.T) {
	stream := newTestPrivateStream(t, 20)
	err := stream.applyEnvelope(wsEnvelope{
		Type: "update/account_all_orders", Channel: "account_all_orders:77", Timestamp: 100,
		Orders:       map[string][]Order{"1": {{ClientOrderIndex: 10, OrderIndex: 20, MarketIndex: 1, Status: "canceled-post-only"}}},
		Trades:       []byte(`{"1":[{"trade_id":30,"market_id":1,"size":"0.01","price":"60000"}]}`),
		Positions:    map[string]Position{"1": {MarketID: 1, Symbol: "BTC", Sign: 1, Size: "0.01"}},
		Stats:        &AccountStats{AccountStatsValues: AccountStatsValues{Collateral: "1000", AvailableBalance: "800"}},
		Transactions: []AccountTransaction{{SequenceIndex: 5, Nonce: 8}},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := stream.Snapshot()
	if snapshot.Orders[10].State != OrderStateCancelled {
		t.Fatalf("order=%+v", snapshot.Orders[10])
	}
	if snapshot.Trades[30].Price != "60000" || snapshot.Positions[1].Side != PositionSideLong {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	if snapshot.Account == nil || snapshot.Account.Collateral != "1000" || snapshot.LastTxSequence != 5 {
		t.Fatalf("account/sequence snapshot=%+v", snapshot)
	}
}

func TestPrivateStreamDetectsTransactionSequenceGap(t *testing.T) {
	stream := newTestPrivateStream(t, 10)
	if err := stream.applyEnvelope(wsEnvelope{Channel: "account_tx:77", Transactions: []AccountTransaction{{SequenceIndex: 10}}}); err != nil {
		t.Fatal(err)
	}
	err := stream.applyEnvelope(wsEnvelope{Channel: "account_tx:77", Transactions: []AccountTransaction{{SequenceIndex: 12}}})
	if err == nil {
		t.Fatal("expected transaction sequence gap")
	}
	if !stream.Snapshot().NeedsRESTRecovery {
		t.Fatal("sequence gap did not require REST recovery")
	}
}

func TestPrivateStreamIgnoresStaleChannelEvent(t *testing.T) {
	stream := newTestPrivateStream(t, 10)
	if err := stream.applyEnvelope(wsEnvelope{
		Channel: "account_all_orders:77", Timestamp: 20,
		Orders: map[string][]Order{"1": {{ClientOrderIndex: 1, Status: "open"}}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := stream.applyEnvelope(wsEnvelope{
		Channel: "account_all_orders:77", Timestamp: 19,
		Orders: map[string][]Order{"1": {{ClientOrderIndex: 1, Status: "filled"}}},
	}); err != nil {
		t.Fatal(err)
	}
	if stream.Snapshot().Orders[1].State != OrderStateOpen {
		t.Fatal("stale event overwrote current order")
	}
}

type fakeStreamConn struct {
	mu     sync.Mutex
	writes []map[string]string
	closed bool
}

func (c *fakeStreamConn) ReadJSON(any) error { return io.EOF }
func (c *fakeStreamConn) WriteJSON(value any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	message := value.(map[string]string)
	copy := make(map[string]string, len(message))
	for key, item := range message {
		copy[key] = item
	}
	c.writes = append(c.writes, copy)
	return nil
}
func (c *fakeStreamConn) WriteControl(int, []byte, time.Time) error { return nil }
func (c *fakeStreamConn) SetReadDeadline(time.Time) error           { return nil }
func (c *fakeStreamConn) SetPongHandler(func(string) error)         {}
func (c *fakeStreamConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return nil
}

func TestPrivateStreamForceReconnectClosesActiveConnection(t *testing.T) {
	stream := newTestPrivateStream(t, 10)
	connection := &fakeStreamConn{}
	stream.mu.Lock()
	stream.active = connection
	stream.snapshot.Connected = true
	stream.mu.Unlock()
	if err := stream.ForceReconnect(); err != nil {
		t.Fatal(err)
	}
	connection.mu.Lock()
	closed := connection.closed
	connection.mu.Unlock()
	if !closed {
		t.Fatal("active connection was not closed")
	}
}

type fakeStreamDialer struct{ connections atomic.Int64 }

func (d *fakeStreamDialer) DialContext(context.Context, string, http.Header) (streamConn, error) {
	d.connections.Add(1)
	return &fakeStreamConn{}, nil
}

type recoveryOnlyExecution struct{ reconciliations atomic.Int64 }

func (e *recoveryOnlyExecution) PlaceOrder(context.Context, PlaceOrderRequest) (*OrderSubmission, error) {
	panic("not used")
}
func (e *recoveryOnlyExecution) CancelOrder(context.Context, int64) error { panic("not used") }
func (e *recoveryOnlyExecution) CancelAll(context.Context) error          { panic("not used") }
func (e *recoveryOnlyExecution) GetActiveOrders(context.Context) ([]Order, error) {
	panic("not used")
}
func (e *recoveryOnlyExecution) GetOrderStatus(context.Context, int64) (*ReconciledOrder, error) {
	panic("not used")
}
func (e *recoveryOnlyExecution) GetPositions(context.Context) (*PositionSnapshot, error) {
	panic("not used")
}
func (e *recoveryOnlyExecution) Reconcile(context.Context) (*RecoveryReport, error) {
	e.reconciliations.Add(1)
	return &RecoveryReport{Positions: &PositionSnapshot{}}, nil
}

func TestPrivateStreamReconcilesBeforeEveryReconnect(t *testing.T) {
	execution := &recoveryOnlyExecution{}
	manager := &Manager{
		BaseURL: "https://example.invalid", AccountIndex: 77,
		authTokenFunc: func() (string, error) { return "auth-token", nil },
	}
	stream, err := NewPrivateStream(manager, execution, PrivateStreamConfig{
		URL: "wss://example.invalid/stream", PingInterval: time.Second,
		ReconnectMin: time.Millisecond, ReconnectMax: 2 * time.Millisecond, EventBuffer: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	dialer := &fakeStreamDialer{}
	stream.dialer = dialer
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	err = stream.Run(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("run error=%v", err)
	}
	if execution.reconciliations.Load() < 2 || dialer.connections.Load() < 2 {
		t.Fatalf("reconciliations=%d connections=%d", execution.reconciliations.Load(), dialer.connections.Load())
	}
}

func TestPrivateStreamSubscriptionsAuthenticateProtectedChannels(t *testing.T) {
	stream := newTestPrivateStream(t, 10)
	connection := &fakeStreamConn{}
	if err := stream.subscribe(connection, "secret-token"); err != nil {
		t.Fatal(err)
	}
	connection.mu.Lock()
	defer connection.mu.Unlock()
	if len(connection.writes) != 5 {
		t.Fatalf("subscriptions=%+v", connection.writes)
	}
	for _, message := range connection.writes {
		if message["auth"] != "secret-token" {
			t.Fatalf("protected channel lacks auth: %+v", message)
		}
	}
}

func TestPrivateStreamConfigEnforcesKeepalive(t *testing.T) {
	manager := &Manager{BaseURL: "https://mainnet.zklighter.elliot.ai"}
	config, err := (PrivateStreamConfig{}).normalized(manager)
	if err != nil {
		t.Fatal(err)
	}
	if config.URL != "wss://mainnet.zklighter.elliot.ai/stream" || config.PingInterval >= 2*time.Minute {
		t.Fatalf("config=%+v", config)
	}
	if _, err := (PrivateStreamConfig{PingInterval: 2 * time.Minute}).normalized(manager); err == nil {
		t.Fatal("expected invalid keepalive interval")
	}
}
