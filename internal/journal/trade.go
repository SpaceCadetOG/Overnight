package journal

import (
	"math"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/execution"
	"github.com/ogtrading/overnight-strategy/internal/live"
)

// TradeRecord is the consolidated, machine-readable forensic record. It stores
// market and trade facts only; subjective emotion/discipline fields are excluded.
type TradeRecord struct {
	SchemaVersion    int                  `json:"schema_version"`
	SessionID        string               `json:"session_id"`
	OpportunityID    string               `json:"opportunity_id"`
	StrategyOrderID  string               `json:"strategy_order_id"`
	ExchangeOrderID  string               `json:"exchange_order_id,omitempty"`
	RunID            string               `json:"run_id"`
	ID               string               `json:"trade_id"`
	CaseID           string               `json:"case_id,omitempty"`
	RecordedAt       time.Time            `json:"recorded_at"`
	SessionDate      time.Time            `json:"session_date"`
	Symbol           string               `json:"symbol"`
	ExchangeSymbol   string               `json:"exchange_symbol"`
	Mode             string               `json:"mode"`
	Classification   string               `json:"classification"`
	StrategyVersion  string               `json:"strategy_version"`
	Market           live.MarketSnapshot  `json:"market"`
	Order            execution.Order      `json:"order"`
	State            execution.PaperState `json:"state"`
	Outcome          string               `json:"outcome"`
	PlannedEntry     float64              `json:"planned_entry"`
	ActualFill       float64              `json:"actual_fill,omitempty"`
	EntrySlippage    float64              `json:"entry_slippage,omitempty"`
	EntrySlippageBPS float64              `json:"entry_slippage_bps,omitempty"`
	ExitPrice        float64              `json:"exit_price,omitempty"`
	RMultiple        float64              `json:"r_multiple"`
	MFE              float64              `json:"mfe_points"`
	MAE              float64              `json:"mae_points"`
	MFER             float64              `json:"mfe_r"`
	MAER             float64              `json:"mae_r"`
	TP1Hit           bool                 `json:"tp1_hit"`
}

type LiveExecution struct {
	OrderID    string
	State      string
	Outcome    string
	ActualFill float64
	ExitPrice  float64
	RMultiple  float64
	MFE        float64
	MAE        float64
	TP1Hit     bool
}

func FromPaper(snapshot live.MarketSnapshot, exchangeSymbol, version string, trade execution.PaperTrade) TradeRecord {
	risk := math.Abs(trade.Order.Price - trade.Order.Stop)
	record := TradeRecord{
		SchemaVersion: 1, ID: snapshot.SessionDate.Format("20060102") + "-" + snapshot.Symbol + "-PAPER",
		RecordedAt: time.Now().UTC(), SessionDate: snapshot.SessionDate, Symbol: snapshot.Symbol,
		ExchangeSymbol: exchangeSymbol, Mode: "PAPER_EXECUTION", Classification: snapshot.Classification,
		StrategyVersion: version, Market: snapshot, Order: trade.Order, State: trade.State,
		Outcome: trade.Outcome, PlannedEntry: trade.Order.Price, ActualFill: trade.FillPrice,
		ExitPrice: trade.ExitPrice, RMultiple: trade.RMultiple, MFE: trade.MFE, MAE: trade.MAE, TP1Hit: trade.TP1Hit,
	}
	if trade.FillPrice > 0 && trade.Order.Price > 0 {
		record.EntrySlippage = math.Abs(trade.FillPrice - trade.Order.Price)
		record.EntrySlippageBPS = record.EntrySlippage / trade.Order.Price * 10000
	}
	if risk > 0 {
		record.MFER, record.MAER = trade.MFE/risk, trade.MAE/risk
	}
	return record
}

// FromLive normalizes authenticated exchange execution facts into the same
// schema used by the paper control. Callers supply only reconciled fill facts.
func FromLive(snapshot live.MarketSnapshot, exchangeSymbol, version string, order execution.Order, facts LiveExecution) TradeRecord {
	risk := math.Abs(order.Price - order.Stop)
	record := TradeRecord{
		SchemaVersion: 1, ID: snapshot.SessionDate.Format("20060102") + "-" + snapshot.Symbol + "-LIVE-" + facts.OrderID,
		RecordedAt: time.Now().UTC(), SessionDate: snapshot.SessionDate, Symbol: snapshot.Symbol,
		ExchangeSymbol: exchangeSymbol, Mode: "LIVE_EXECUTION", Classification: snapshot.Classification,
		StrategyVersion: version, Market: snapshot, Order: order, State: execution.PaperState(facts.State),
		Outcome: facts.Outcome, PlannedEntry: order.Price, ActualFill: facts.ActualFill,
		ExitPrice: facts.ExitPrice, RMultiple: facts.RMultiple, MFE: facts.MFE, MAE: facts.MAE, TP1Hit: facts.TP1Hit,
	}
	record.ExchangeOrderID = facts.OrderID
	if facts.ActualFill > 0 && order.Price > 0 {
		record.EntrySlippage = math.Abs(facts.ActualFill - order.Price)
		record.EntrySlippageBPS = record.EntrySlippage / order.Price * 10000
	}
	if risk > 0 {
		record.MFER, record.MAER = facts.MFE/risk, facts.MAE/risk
	}
	return record
}
