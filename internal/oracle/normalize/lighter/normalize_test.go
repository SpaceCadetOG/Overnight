package lighter

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/oracle/model"
)

func testContext() Context {
	return Context{Asset: "BTC", ReceivedAt: time.Date(2026, 9, 11, 12, 0, 1, 0, time.UTC), ConnectionID: "conn-test", FirstSequence: 100, DefaultQuality: model.QualityCertified}
}

func TestNormalizeBookSnapshotPreservesVenueSequenceAndDecimals(t *testing.T) {
	raw := []byte(`{"channel":"order_book:1","type":"subscribed/order_book","order_book":{"asks":[{"price":"77351.100","size":"0.28462000"}],"bids":[{"price":"77350.900","size":"0.00082000"}],"begin_nonce":0,"nonce":991}}`)
	events, err := Normalize(raw, testContext())
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Stream != model.StreamBookSnapshot || events[0].VenueSequence != 991 || events[0].OracleSequence != 100 || events[0].Quality != model.QualityCertified {
		t.Fatalf("unexpected event: %+v", events)
	}
	var book model.Book
	if err := json.Unmarshal(events[0].Payload, &book); err != nil {
		t.Fatal(err)
	}
	if book.Asks[0].Price != "77351.100" || book.Bids[0].Size != "0.00082000" {
		t.Fatalf("book decimals changed: %+v", book)
	}
}

func TestNormalizeTradeBatchProducesSequencedIndependentEvents(t *testing.T) {
	raw := []byte(`{"channel":"trade:1","trades":[{"trade_id":7,"trade_id_str":"7","timestamp":1789128000000,"tx_hash":"abc","price":"100.10","size":"2.50","usd_amount":"250.25","is_maker_ask":true},{"trade_id":8,"trade_id_str":"8","timestamp":1789128000001,"price":"100.20","size":"1.25","is_maker_ask":false}],"liquidation_trades":[{"trade_id":9,"trade_id_str":"9","timestamp":1789128000002,"price":"100.00","size":"4.00","usd_amount":"400","is_maker_ask":false}]}`)
	events, err := Normalize(raw, testContext())
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("events=%d", len(events))
	}
	for index, event := range events {
		if event.OracleSequence != uint64(100+index) || event.TransactionTimestamp == nil {
			t.Fatalf("event %d not correctly sequenced/timestamped: %+v", index, event)
		}
	}
	var trade model.Trade
	if err := json.Unmarshal(events[0].Payload, &trade); err != nil {
		t.Fatal(err)
	}
	if trade.AggressorSide != "BUY" || !trade.MakerIsAsk {
		t.Fatalf("aggressor interpretation=%+v", trade)
	}
	var liquidation model.Liquidation
	if err := json.Unmarshal(events[2].Payload, &liquidation); err != nil {
		t.Fatal(err)
	}
	if !liquidation.Confirmed || liquidation.LiquidatedPositionSide != "LONG" {
		t.Fatalf("liquidation=%+v", liquidation)
	}
}

func TestNormalizeRejectsMalformedAndUnknownMessages(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte(`not-json`),
		[]byte(`{"channel":"unknown"}`),
		[]byte(`{"channel":"order_book:1","type":"update/order_book","order_book":{"nonce":0,"asks":[],"bids":[]}}`),
		[]byte(`{"channel":"trade:1","trades":[]}`),
	} {
		if _, err := Normalize(raw, testContext()); err == nil {
			t.Fatalf("accepted malformed message: %s", raw)
		}
	}
}
