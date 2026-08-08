package lighter

import (
	"testing"
	"time"

	lightertx "github.com/elliottech/lighter-go/types/txtypes"
)

func TestBuildLiveEntryPayloads(t *testing.T) {
	expiry := time.Date(2026, 8, 10, 10, 5, 0, 0, time.UTC)
	cases := []struct {
		symbol          string
		price, quantity float64
		market          int16
		encodedPrice    uint32
		encodedSize     int64
	}{
		{"BTC", 64907.1, 0.00123, 1, 649071, 123},
		{"ETH", 1916.25, 0.0123, 0, 191625, 123},
	}
	for _, tc := range cases {
		t.Run(tc.symbol, func(t *testing.T) {
			tx, err := BuildCreateOrder(OrderRequest{Symbol: tc.symbol, Side: Buy, Price: tc.price, Quantity: tc.quantity, ClientOrderIndex: 10, Expiry: expiry, Type: lightertx.LimitOrder})
			if err != nil {
				t.Fatal(err)
			}
			if tx.MarketIndex != tc.market || tx.Price != tc.encodedPrice || tx.BaseAmount != tc.encodedSize {
				t.Fatalf("wrong encoding: %+v", tx)
			}
			if tx.IsAsk != 0 || tx.ReduceOnly != 0 || tx.TimeInForce != lightertx.GoodTillTime {
				t.Fatalf("wrong entry flags: %+v", tx)
			}
		})
	}
}

func TestBuildProtectiveAndCancelPayloads(t *testing.T) {
	expiry := time.Date(2026, 8, 10, 21, 0, 0, 0, time.UTC)
	stop, err := BuildCreateOrder(OrderRequest{Symbol: "ETH", Side: Sell, Price: 1900, Quantity: .01, ClientOrderIndex: 11, Expiry: expiry, Type: lightertx.StopLossOrder, ReduceOnly: true, TriggerPrice: 1900})
	if err != nil {
		t.Fatal(err)
	}
	if stop.MarketIndex != 0 || stop.IsAsk != 1 || stop.ReduceOnly != 1 || stop.TriggerPrice != 190000 || stop.TimeInForce != lightertx.ImmediateOrCancel {
		t.Fatalf("wrong stop: %+v", stop)
	}
	cancel, err := BuildCancelOrder("BTC", 99)
	if err != nil {
		t.Fatal(err)
	}
	if cancel.MarketIndex != 1 || cancel.Index != 99 {
		t.Fatalf("wrong cancel: %+v", cancel)
	}
}

func TestBuildReduceOnlyMarketClose(t *testing.T) {
	tx, err := BuildCreateOrder(OrderRequest{Symbol: "BTC", Side: Sell, Price: 64000, Quantity: .001, ClientOrderIndex: 12, Type: lightertx.MarketOrder, ReduceOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if tx.Type != lightertx.MarketOrder || tx.TimeInForce != lightertx.ImmediateOrCancel || tx.OrderExpiry != 0 || tx.ReduceOnly != 1 {
		t.Fatalf("wrong market close: %+v", tx)
	}
}

func TestRejectUnknownMarket(t *testing.T) {
	_, err := BuildCreateOrder(OrderRequest{Symbol: "DOGE"})
	if err == nil {
		t.Fatal("expected unsupported market error")
	}
}
