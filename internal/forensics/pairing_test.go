package forensics

import (
	"github.com/ogtrading/overnight-strategy/internal/journal"
	"testing"
)

func TestPairUsesOpportunityNotTimestamp(t *testing.T) {
	paper := journal.TradeRecord{ID: "paper", OpportunityID: "opp", Mode: "PAPER_EXECUTION"}
	live := journal.TradeRecord{ID: "live", OpportunityID: "opp", Mode: "LIVE_EXECUTION"}
	pair, err := Pair(paper, live)
	if err != nil {
		t.Fatal(err)
	}
	if pair.PaperTradeID != "paper" || pair.LiveTradeID != "live" {
		t.Fatalf("pair=%+v", pair)
	}
}
