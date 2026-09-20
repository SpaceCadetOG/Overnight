package collector

import (
	"testing"
	"time"
)

func TestOraclePipelineSequencesBooksIndependentlyFromFlow(t *testing.T) {
	store := &memoryStore{records: map[string][]any{}}
	pipeline := newOraclePipeline(store)
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)

	snapshot := []byte(`{"channel":"order_book:1","type":"subscribed/order_book","order_book":{"asks":[{"price":"101","size":"2"}],"bids":[{"price":"99","size":"3"}],"begin_nonce":0,"nonce":10}}`)
	if _, _, err := pipeline.accept(snapshot, "BTC", now, "conn-test"); err != nil {
		t.Fatal(err)
	}

	trade := []byte(`{"channel":"trade:1","trades":[{"trade_id":7,"trade_id_str":"7","timestamp":1789819200001,"price":"100","size":"1","is_maker_ask":true}]}`)
	if _, _, err := pipeline.accept(trade, "BTC", now.Add(time.Millisecond), "conn-test"); err != nil {
		t.Fatal(err)
	}

	delta := []byte(`{"channel":"order_book:1","type":"update/order_book","order_book":{"asks":[{"price":"101","size":"1"}],"bids":[],"begin_nonce":10,"nonce":11}}`)
	if _, _, err := pipeline.accept(delta, "BTC", now.Add(2*time.Millisecond), "conn-test"); err != nil {
		t.Fatalf("flow event manufactured a book gap: %v", err)
	}

	if got := pipeline.sequences["BTC:book"]; got != 2 {
		t.Fatalf("book sequence=%d want=2", got)
	}
	if got := pipeline.sequences["BTC:trade"]; got != 1 {
		t.Fatalf("trade sequence=%d want=1", got)
	}
	snapshotState, ok := pipeline.hub.Book("BTC")
	if !ok || snapshotState.OracleSequence != 2 || snapshotState.VenueNonce != 11 {
		t.Fatalf("authoritative book not advanced: ok=%v snapshot=%+v", ok, snapshotState)
	}
}
