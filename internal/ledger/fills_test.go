package ledger

import (
	"testing"
	"time"
)

func TestVenueFillParsingAndAccounting(t *testing.T) {
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	entry, err := ParseVenueFill(map[string]any{"trade_id": "1", "market_id": 1, "size": "2", "price": "100", "ask_id": 10, "bid_id": 11, "ask_account_id": 9, "bid_account_id": 7, "timestamp": now.UnixMilli(), "taker_fee": "0.10"}, 7, now)
	if err != nil {
		t.Fatal(err)
	}
	exit, err := ParseVenueFill(map[string]any{"trade_id": "2", "market_id": 1, "size": "2", "price": "102", "ask_id": 12, "bid_id": 13, "ask_account_id": 7, "bid_account_id": 9, "timestamp": now.Add(time.Minute).UnixMilli(), "taker_fee": "0.10"}, 7, now)
	if err != nil {
		t.Fatal(err)
	}
	a, err := ComputeAccounting([]VenueFill{entry, exit}, map[string]string{"11": "ENTRY", "12": "EXIT"}, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Complete || a.GrossPnL != 4 || a.Fees != .2 || a.NetPnL != 3.8 || a.RMultiple != 1.9 {
		t.Fatalf("accounting=%+v", a)
	}
}
