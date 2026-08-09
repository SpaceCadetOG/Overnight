package execution

import (
	"github.com/ogtrading/overnight-strategy/internal/models"
	"testing"
	"time"
)

func TestAdvancePaperTP1ThenBreakeven(t *testing.T) {
	start := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	trade := PaperTrade{Order: Order{Symbol: "BTC", Side: "BUY", Price: 100, Stop: 99, TP1: 101, TP2: 103, ExpiresAt: start.Add(11 * time.Hour).Unix()}, State: Waiting}
	var changed bool
	var err error
	trade, changed, err = AdvancePaper(trade, models.Candle{OpenTime: start, Low: 99.5, High: 101.2})
	if err != nil || !changed || trade.State != PaperTP1 {
		t.Fatalf("first candle: %#v %v", trade, err)
	}
	trade, changed, err = AdvancePaper(trade, models.Candle{OpenTime: start.Add(5 * time.Minute), Low: 99.9, High: 100.5})
	if err != nil || !changed || trade.Outcome != "TP1_THEN_BE" || trade.ExitPrice != 100 {
		t.Fatalf("BE candle: %#v %v", trade, err)
	}
}

func TestAdvancePaperIsIdempotent(t *testing.T) {
	at := time.Now().UTC().Truncate(time.Minute)
	trade := PaperTrade{Order: Order{Side: "BUY", Price: 100, Stop: 99, TP1: 101, TP2: 102, ExpiresAt: at.Add(time.Hour).Unix()}, State: Waiting}
	c := models.Candle{OpenTime: at, Low: 99.5, High: 100.5}
	trade, _, _ = AdvancePaper(trade, c)
	again, changed, _ := AdvancePaper(trade, c)
	if changed || again.UpdatedAt != trade.UpdatedAt {
		t.Fatal("duplicate candle changed state")
	}
}
