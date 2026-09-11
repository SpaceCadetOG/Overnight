package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEnvelopeHasStableIdentityAndPreservesDecimals(t *testing.T) {
	received := time.Date(2026, 9, 11, 12, 0, 0, 123, time.UTC)
	payload := Trade{TradeID: "42", Price: "77350.90000000", Size: "0.00017000", AggressorSide: "BUY", MakerIsAsk: true}
	a, err := New("BTC", StreamTrade, 7, 42, received, "conn-1", QualityCertified, payload)
	if err != nil {
		t.Fatal(err)
	}
	b, err := New("BTC", StreamTrade, 7, 42, received, "conn-1", QualityCertified, payload)
	if err != nil {
		t.Fatal(err)
	}
	if a.EventID == "" || a.EventID != b.EventID {
		t.Fatalf("event identity is not stable: %q %q", a.EventID, b.EventID)
	}
	var decoded Trade
	if err := json.Unmarshal(a.Payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Price != payload.Price || decoded.Size != payload.Size {
		t.Fatalf("decimal values changed: %+v", decoded)
	}
}

func TestEnvelopeRejectsInvalidContractFields(t *testing.T) {
	event, err := New("btc", StreamTrade, 1, 0, time.Now(), "conn", QualityCertified, Trade{})
	if err != nil || event.Asset != "BTC" {
		t.Fatalf("asset was not normalized: asset=%q err=%v", event.Asset, err)
	}
	_, err = New("BTC", StreamTrade, 0, 0, time.Now(), "conn", QualityCertified, Trade{})
	if err == nil {
		t.Fatal("zero Oracle sequence was accepted")
	}
}
