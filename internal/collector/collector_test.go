package collector

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/store"
)

type memoryStore struct{ records map[string][]any }

func (s *memoryStore) Append(stream string, value any) error {
	if s.records == nil {
		s.records = map[string][]any{}
	}
	s.records[stream] = append(s.records[stream], value)
	return nil
}

func TestStreamClassification(t *testing.T) {
	cases := map[string]string{"order_book/0": "orderbook_events", "trade/0": "trade_flow", "ticker/0": "ticker_events", "market_stats/all": "market_stats"}
	for input, want := range cases {
		if got := streamName(input); got != want {
			t.Fatalf("%s=%s want %s", input, got, want)
		}
	}
}

func TestOrderBookSnapshotAndIncrementalReconstruction(t *testing.T) {
	c := New("", "", mustStore(t))
	snapshot := `{"channel":"order_book:0","type":"subscribed/order_book","order_book":{"asks":[{"price":"101","size":"2"}],"bids":[{"price":"99","size":"3"}],"begin_nonce":0,"nonce":10}}`
	if err := c.record([]byte(snapshot)); err != nil {
		t.Fatal(err)
	}
	delta := `{"channel":"order_book:0","type":"update/order_book","order_book":{"asks":[{"price":"101","size":"0"},{"price":"102","size":"4"}],"bids":[{"price":"99","size":"5"}],"begin_nonce":10,"nonce":12}}`
	if err := c.record([]byte(delta)); err != nil {
		t.Fatal(err)
	}
	book := c.books["order_book:0"]
	if _, exists := book.Asks["101"]; exists {
		t.Fatal("zero-size ask was not deleted")
	}
	if book.Asks["102"] != "4" || book.Bids["99"] != "5" || book.Nonce != 12 {
		t.Fatalf("unexpected reconstructed book: %+v", book)
	}
	status := c.Status.Snapshot()
	if status.BooksReady != 1 || status.Snapshots != 1 || status.NonceGaps != 0 {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestOrderBookGapForcesFreshSnapshot(t *testing.T) {
	c := New("", "", mustStore(t))
	for _, message := range []string{
		`{"channel":"order_book:0","type":"subscribed/order_book","order_book":{"asks":[],"bids":[],"begin_nonce":0,"nonce":10}}`,
		`{"channel":"order_book:0","type":"update/order_book","order_book":{"asks":[],"bids":[],"begin_nonce":11,"nonce":12}}`,
	} {
		err := c.record([]byte(message))
		if strings.Contains(message, `"begin_nonce":11`) {
			if err == nil || !strings.Contains(err.Error(), "nonce gap") {
				t.Fatalf("expected nonce gap, got %v", err)
			}
		} else if err != nil {
			t.Fatal(err)
		}
	}
	if _, exists := c.books["order_book:0"]; exists {
		t.Fatal("invalid book retained after gap")
	}
	if c.Status.Snapshot().NonceGaps != 1 {
		t.Fatalf("gap count=%d", c.Status.Snapshot().NonceGaps)
	}
}

func mustStore(t *testing.T) *store.JSONL {
	t.Helper()
	s, err := store.NewJSONL(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStatusJSONIncludesBookReadiness(t *testing.T) {
	b, err := json.Marshal(StatusView{BooksReady: 10, Snapshots: 10})
	if err != nil || !strings.Contains(string(b), `"books_ready":10`) {
		t.Fatalf("status json=%s err=%v", b, err)
	}
}

func TestRecordStreamUsesStableAssetSymbol(t *testing.T) {
	c := New("", "", mustStore(t))
	c.marketIDs["17"] = "SHIB"
	if got := c.recordStream("order_book:17"); got != "asset=SHIB/orderbook_events" {
		t.Fatalf("record stream=%s", got)
	}
}

func TestServerErrorForcesReconnect(t *testing.T) {
	c := New("", "", mustStore(t))
	err := c.record([]byte(`{"error":{"code":30009,"message":"Too Many Websocket Messages!"}}`))
	if err == nil || !strings.Contains(err.Error(), "30009") {
		t.Fatalf("expected websocket error, got %v", err)
	}
}

func TestFreshSnapshotReplacesPriorConnectionWithoutGap(t *testing.T) {
	c := New("", "", mustStore(t))
	for _, message := range []string{
		`{"channel":"order_book:1","type":"subscribed/order_book","order_book":{"asks":[{"price":"101","size":"1"}],"bids":[],"begin_nonce":0,"nonce":10}}`,
		`{"channel":"order_book:1","type":"subscribed/order_book","order_book":{"asks":[{"price":"102","size":"2"}],"bids":[],"begin_nonce":0,"nonce":20}}`,
	} {
		if err := c.record([]byte(message)); err != nil {
			t.Fatal(err)
		}
	}
	if c.Status.Snapshot().NonceGaps != 0 || c.books["order_book:1"].Nonce != 20 {
		t.Fatalf("snapshot did not replace old connection state: %+v", c.Status.Snapshot())
	}
}

func TestCrossedBookIsRejectedAndInvalidated(t *testing.T) {
	c := New("", "", mustStore(t))
	err := c.record([]byte(`{"channel":"order_book:1","type":"subscribed/order_book","order_book":{"asks":[{"price":"99","size":"1"}],"bids":[{"price":"100","size":"1"}],"begin_nonce":0,"nonce":10}}`))
	if err == nil || !strings.Contains(err.Error(), "crossed order book") {
		t.Fatalf("expected crossed-book rejection, got %v", err)
	}
	status := c.Status.Snapshot()
	if status.CrossedBooks != 1 || status.BooksReady != 0 {
		t.Fatalf("unexpected status: %+v", status)
	}
	if _, exists := c.books["order_book:1"]; exists {
		t.Fatal("crossed book remained ready")
	}
}

func TestNegativeAndMalformedLevelsAreRejected(t *testing.T) {
	for _, level := range []string{
		`{"price":"101","size":"-1"}`,
		`{"price":"bad","size":"1"}`,
		`{"price":"101","size":"bad"}`,
	} {
		c := New("", "", mustStore(t))
		message := `{"channel":"order_book:1","type":"subscribed/order_book","order_book":{"asks":[` + level + `],"bids":[],"begin_nonce":0,"nonce":10}}`
		if err := c.record([]byte(message)); err == nil || !strings.Contains(err.Error(), "invalid ask level") {
			t.Fatalf("level %s: expected rejection, got %v", level, err)
		}
		if status := c.Status.Snapshot(); status.InvalidLevels != 1 || status.BooksReady != 0 {
			t.Fatalf("level %s: unexpected status %+v", level, status)
		}
	}
}

func TestConfirmedLiquidationIsSeparatedAndEnriched(t *testing.T) {
	s := &memoryStore{}
	c := New("", "", s)
	c.marketIDs["1"] = "BTC"
	if err := c.record([]byte(`{"channel":"order_book:1","type":"subscribed/order_book","order_book":{"asks":[{"price":"101","size":"2"}],"bids":[{"price":"99","size":"3"}],"begin_nonce":0,"nonce":10}}`)); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().UnixMilli()
	msg := `{"channel":"trade:1","trades":[],"liquidation_trades":[{"market_id":1,"timestamp":` + strconv.FormatInt(now, 10) + `,"trade_id_str":"44","tx_hash":"abc","price":"100","size":"2","usd_amount":"200","is_maker_ask":false,"taker_position_size_before":"3","taker_position_sign_changed":true,"type":"liquidation"}]}`
	if err := c.record([]byte(msg)); err != nil {
		t.Fatal(err)
	}
	rows := s.records["asset=BTC/confirmed_liquidations"]
	if len(rows) != 1 {
		t.Fatalf("confirmed rows=%d", len(rows))
	}
	record := rows[0].(map[string]any)
	if record["confirmed"] != true || record["liquidated_position_side"] != "LONG" || record["aggressor_side"] != "SELL" {
		t.Fatalf("bad record: %+v", record)
	}
	book := record["book_at_event"].(bookSnapshot)
	if book.BestBid != 99 || book.BestAsk != 101 || book.Imbalance1 != .6 {
		t.Fatalf("bad book: %+v", book)
	}
	if len(s.records["asset=BTC/inferred_liquidation_cascades"]) != 0 {
		t.Fatal("confirmed event leaked into inferred dataset")
	}
}
