package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const SchemaVersion = 1

// VenueFill is immutable exchange evidence. Raw preserves fields not yet
// normalized so future schema migrations never require rewriting history.
type VenueFill struct {
	SchemaVersion int            `json:"schema_version"`
	FillID        string         `json:"fill_id"`
	VenueTradeID  string         `json:"venue_trade_id"`
	VenueOrderID  string         `json:"venue_order_id"`
	TransactionID string         `json:"transaction_id,omitempty"`
	MarketID      int64          `json:"market_id"`
	Side          string         `json:"side"`
	Quantity      float64        `json:"quantity"`
	Price         float64        `json:"price"`
	Fee           float64        `json:"fee"`
	FeeAsset      string         `json:"fee_asset"`
	ExchangeAt    time.Time      `json:"exchange_timestamp"`
	ReceivedAt    time.Time      `json:"received_at"`
	Raw           map[string]any `json:"raw"`
}

func ParseVenueFill(raw map[string]any, accountIndex int64, receivedAt time.Time) (VenueFill, error) {
	tradeID := text(raw, "trade_id_str", "trade_id")
	if tradeID == "" {
		return VenueFill{}, fmt.Errorf("venue trade id is required")
	}
	askAccount, bidAccount := integer(raw, "ask_account_id"), integer(raw, "bid_account_id")
	fill := VenueFill{SchemaVersion: SchemaVersion, VenueTradeID: tradeID, TransactionID: text(raw, "tx_hash"), MarketID: integer(raw, "market_id"), Quantity: number(raw, "size"), Price: number(raw, "price"), Fee: number(raw, "fee", "taker_fee", "maker_fee"), FeeAsset: text(raw, "fee_asset"), ReceivedAt: receivedAt.UTC(), Raw: raw}
	if fill.FeeAsset == "" {
		fill.FeeAsset = "USDC"
	}
	switch accountIndex {
	case askAccount:
		fill.Side = "SELL"
		fill.VenueOrderID = text(raw, "ask_id")
	case bidAccount:
		fill.Side = "BUY"
		fill.VenueOrderID = text(raw, "bid_id")
	default:
		return VenueFill{}, fmt.Errorf("trade %s does not belong to account %d", tradeID, accountIndex)
	}
	stamp := integer(raw, "timestamp")
	if stamp <= 0 {
		return VenueFill{}, fmt.Errorf("trade %s has invalid exchange timestamp", tradeID)
	}
	if stamp < 1e12 {
		fill.ExchangeAt = time.Unix(stamp, 0).UTC()
	} else {
		fill.ExchangeAt = time.UnixMilli(stamp).UTC()
	}
	if fill.VenueOrderID == "" || fill.Quantity <= 0 || fill.Price <= 0 {
		return VenueFill{}, fmt.Errorf("trade %s has invalid order/quantity/price", tradeID)
	}
	fill.FillID = "lfill_" + stable(tradeID, fill.VenueOrderID, fmt.Sprint(fill.MarketID))
	return fill, nil
}

type Accounting struct {
	GrossPnL, Fees, Funding, NetPnL, RMultiple, EntryQuantity, ExitQuantity, AverageEntry, AverageExit float64
	Complete                                                                                           bool
}

// ComputeAccounting derives results only from immutable fills. OrderPurpose
// maps venue order IDs to ENTRY or EXIT. No mark or synthetic price is used.
func ComputeAccounting(fills []VenueFill, orderPurpose map[string]string, plannedRiskUSD, funding float64) (Accounting, error) {
	var a Accounting
	var entryNotional, exitNotional float64
	direction := ""
	for _, f := range fills {
		purpose := strings.ToUpper(orderPurpose[f.VenueOrderID])
		if purpose == "" {
			continue
		}
		a.Fees += f.Fee
		if purpose == "ENTRY" {
			a.EntryQuantity += f.Quantity
			entryNotional += f.Quantity * f.Price
			if direction == "" {
				direction = f.Side
			}
		} else if purpose == "EXIT" {
			a.ExitQuantity += f.Quantity
			exitNotional += f.Quantity * f.Price
		}
	}
	if a.EntryQuantity <= 0 {
		return a, fmt.Errorf("no immutable entry fills")
	}
	a.AverageEntry = entryNotional / a.EntryQuantity
	if a.ExitQuantity > 0 {
		a.AverageExit = exitNotional / a.ExitQuantity
	}
	matched := math.Min(a.EntryQuantity, a.ExitQuantity)
	if matched > 0 {
		if direction == "BUY" {
			a.GrossPnL = matched * (a.AverageExit - a.AverageEntry)
		} else {
			a.GrossPnL = matched * (a.AverageEntry - a.AverageExit)
		}
	}
	a.Funding = funding
	a.NetPnL = a.GrossPnL - a.Fees + a.Funding
	if plannedRiskUSD > 0 {
		a.RMultiple = a.NetPnL / plannedRiskUSD
	}
	a.Complete = math.Abs(a.EntryQuantity-a.ExitQuantity) < 1e-9
	return a, nil
}

func stable(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(h[:12])
}
func text(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			s := strings.TrimSpace(fmt.Sprint(v))
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}
func number(m map[string]any, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch x := v.(type) {
			case float64:
				return x
			case json.Number:
				n, _ := x.Float64()
				return n
			default:
				n, _ := strconv.ParseFloat(fmt.Sprint(v), 64)
				return n
			}
		}
	}
	return 0
}
func integer(m map[string]any, key string) int64 { return int64(number(m, key)) }
