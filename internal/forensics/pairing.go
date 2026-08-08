package forensics

import (
	"fmt"
	"github.com/ogtrading/overnight-strategy/internal/journal"
)

type PaperLivePair struct {
	OpportunityID string              `json:"opportunity_id"`
	PaperTradeID  string              `json:"paper_trade_id"`
	LiveTradeID   string              `json:"live_trade_id"`
	Paper         journal.TradeRecord `json:"paper"`
	Live          journal.TradeRecord `json:"live"`
}

func Pair(paper, live journal.TradeRecord) (PaperLivePair, error) {
	if paper.OpportunityID == "" || live.OpportunityID == "" {
		return PaperLivePair{}, fmt.Errorf("opportunity IDs are required")
	}
	if paper.OpportunityID != live.OpportunityID {
		return PaperLivePair{}, fmt.Errorf("opportunity mismatch")
	}
	if paper.ID == live.ID {
		return PaperLivePair{}, fmt.Errorf("paper/live trade IDs must differ")
	}
	return PaperLivePair{OpportunityID: paper.OpportunityID, PaperTradeID: paper.ID, LiveTradeID: live.ID, Paper: paper, Live: live}, nil
}
