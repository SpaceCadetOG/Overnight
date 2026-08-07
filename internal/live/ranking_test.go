package live

import (
	"github.com/ogtrading/overnight-strategy/internal/models"
	"testing"
)

func TestRankingDoesNotGrantAuthority(t *testing.T) {
	rows := []RecordedResult{{Symbol: "XAU", Result: models.TradeResult{Outcome: models.OutcomeTP2, Filled: true, RealizedR: 2}}, {Symbol: "BTC", Result: models.TradeResult{Outcome: models.OutcomeStopped, Filled: true, RealizedR: -1}}}
	ranks := RankResults(rows)
	if ranks[0].Symbol != "XAU" || ranks[0].ExecutionAuthority {
		t.Fatalf("ranks=%+v", ranks)
	}
}
