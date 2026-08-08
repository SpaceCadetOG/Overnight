package collector

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

type flowTrade struct {
	At         time.Time
	Price, USD float64
	Side       string
}

type liquidationCorrelator struct {
	trades        map[string][]flowTrade
	lastInference map[string]time.Time
	pending       []*pendingCorrelation
	seenConfirmed map[string]bool
}

type pendingCorrelation struct {
	ID, Symbol, Dataset, Source string
	EventAt, ReceivedAt         time.Time
	EventPrice                  float64
	Completed                   map[time.Duration]bool
}

func newLiquidationCorrelator() *liquidationCorrelator {
	return &liquidationCorrelator{trades: map[string][]flowTrade{}, lastInference: map[string]time.Time{}, seenConfirmed: map[string]bool{}}
}

type bookSnapshot struct {
	Nonce                                                              int64 `json:"nonce"`
	BestBid, BestAsk, Spread, Mid, Microprice                          float64
	BidDepth1, AskDepth1, BidDepth5, AskDepth5, BidDepth10, AskDepth10 float64
	Imbalance1, Imbalance5, Imbalance10                                float64
}

func (c *Collector) recordLiquidationResearch(channel string, envelope map[string]any, received time.Time) error {
	marketID := channelMarketID(channel)
	symbol := c.marketIDs[marketID]
	if symbol == "" {
		return nil
	}
	latest := time.Time{}
	for _, raw := range objectList(envelope["trades"]) {
		trade := parseFlowTrade(raw)
		if trade.At.IsZero() {
			continue
		}
		c.flow.trades[symbol] = append(c.flow.trades[symbol], trade)
		if trade.At.After(latest) {
			latest = trade.At
		}
	}
	if !latest.IsZero() {
		c.pruneFlow(symbol, latest)
	}
	book := c.bookForMarket(marketID)
	for _, raw := range objectList(envelope["liquidation_trades"]) {
		trade := parseFlowTrade(raw)
		if trade.At.IsZero() {
			continue
		}
		eventID := symbol + ":" + fmt.Sprint(raw["trade_id_str"])
		if c.flow.seenConfirmed[eventID] {
			continue
		}
		c.flow.seenConfirmed[eventID] = true
		if trade.At.After(latest) {
			latest = trade.At
		}
		record := map[string]any{
			"schema_version": "liquidation-v1", "confirmed": true,
			"source": "lighter_public_trade_liquidation", "symbol": symbol,
			"market_id": int64(number(raw["market_id"])), "exchange_timestamp": trade.At,
			"received_at": received, "trade_id": fmt.Sprint(raw["trade_id_str"]),
			"transaction_hash": fmt.Sprint(raw["tx_hash"]), "price": trade.Price,
			"size": number(raw["size"]), "usd_value": trade.USD,
			"aggressor_side": trade.Side, "liquidated_position_side": liquidatedSide(raw),
			"position_sign_changed": raw["taker_position_sign_changed"],
			"book_at_event":         book, "pre_event_tape": c.tapeWindows(symbol, trade.At),
			"correlation_windows": []string{"-5s:+5s", "-30s:+30s", "-5m:+5m"},
		}
		if err := c.Store.Append("asset="+symbol+"/confirmed_liquidations", record); err != nil {
			return err
		}
		c.flow.pending = append(c.flow.pending, &pendingCorrelation{ID: fmt.Sprint(raw["trade_id_str"]), Symbol: symbol, Dataset: "confirmed_liquidations", Source: "lighter_public_trade_liquidation", EventAt: trade.At, ReceivedAt: received, EventPrice: trade.Price, Completed: map[time.Duration]bool{}})
		c.Status.mu.Lock()
		c.Status.ConfirmedLiquidations++
		c.Status.mu.Unlock()
	}
	if !latest.IsZero() {
		return c.detectInferredCascade(symbol, marketID, latest, received, book)
	}
	return nil
}

func (c *Collector) detectInferredCascade(symbol, marketID string, at, received time.Time, book bookSnapshot) error {
	if at.Sub(c.flow.lastInference[symbol]) < 30*time.Second {
		return nil
	}
	window := selectTrades(c.flow.trades[symbol], at.Add(-5*time.Second), at)
	if len(window) < 8 {
		return nil
	}
	var buys, sells float64
	for _, t := range window {
		if t.Side == "BUY" {
			buys += t.USD
		} else {
			sells += t.USD
		}
	}
	total := buys + sells
	if total < 1000 {
		return nil
	}
	dominance := math.Max(buys, sells) / total
	displacementBPS := math.Abs(window[len(window)-1].Price-window[0].Price) / window[0].Price * 10000
	if dominance < .85 || displacementBPS < 8 {
		return nil
	}
	side := "SELL"
	if buys > sells {
		side = "BUY"
	}
	record := map[string]any{
		"schema_version": "liquidation-v1", "confirmed": false,
		"source": "order_flow_inference_v1", "symbol": symbol, "market_id": marketID,
		"exchange_timestamp": at, "received_at": received, "direction": side,
		"window": "5s", "trade_count": len(window), "aggressive_buy_usd": buys,
		"aggressive_sell_usd": sells, "dominance": dominance,
		"price_displacement_bps": displacementBPS, "book_at_detection": book,
		"thresholds":          map[string]any{"minimum_trades": 8, "minimum_usd": 1000, "minimum_dominance": .85, "minimum_displacement_bps": 8},
		"correlation_windows": []string{"-5s:+5s", "-30s:+30s", "-5m:+5m"},
	}
	if err := c.Store.Append("asset="+symbol+"/inferred_liquidation_cascades", record); err != nil {
		return err
	}
	c.flow.pending = append(c.flow.pending, &pendingCorrelation{ID: fmt.Sprintf("%s-%d", symbol, at.UnixMilli()), Symbol: symbol, Dataset: "inferred_liquidation_cascades", Source: "order_flow_inference_v1", EventAt: at, ReceivedAt: received, EventPrice: window[len(window)-1].Price, Completed: map[time.Duration]bool{}})
	c.flow.lastInference[symbol] = at
	c.Status.mu.Lock()
	c.Status.InferredCascades++
	c.Status.mu.Unlock()
	return nil
}

func (c *Collector) flushLiquidationWindows(now time.Time) error {
	keep := c.flow.pending[:0]
	for _, event := range c.flow.pending {
		for _, horizon := range []time.Duration{5 * time.Second, 30 * time.Second, 5 * time.Minute} {
			if event.Completed[horizon] || now.Before(event.ReceivedAt.Add(horizon)) {
				continue
			}
			trades := selectTrades(c.flow.trades[event.Symbol], event.EventAt.Add(-horizon), event.EventAt.Add(horizon))
			var buy, sell float64
			endPrice := event.EventPrice
			for _, trade := range trades {
				if trade.Side == "BUY" {
					buy += trade.USD
				} else {
					sell += trade.USD
				}
				if !trade.At.Before(event.EventAt) {
					endPrice = trade.Price
				}
			}
			displacement := 0.0
			if event.EventPrice > 0 {
				displacement = (endPrice - event.EventPrice) / event.EventPrice * 10000
			}
			continuation := false
			if buy > sell {
				continuation = displacement > 0
			} else if sell > buy {
				continuation = displacement < 0
			}
			record := map[string]any{"schema_version": "liquidation-window-v1", "event_id": event.ID, "symbol": event.Symbol, "parent_dataset": event.Dataset, "source": event.Source, "confirmed": event.Dataset == "confirmed_liquidations", "event_timestamp": event.EventAt, "window_seconds": int(horizon.Seconds()), "completed_at": now, "trade_count": len(trades), "aggressive_buy_usd": buy, "aggressive_sell_usd": sell, "delta_usd": buy - sell, "event_price": event.EventPrice, "end_price": endPrice, "price_displacement_bps": displacement, "continuation": continuation}
			if err := c.Store.Append("asset="+event.Symbol+"/liquidation_context_windows", record); err != nil {
				return err
			}
			event.Completed[horizon] = true
		}
		if !event.Completed[5*time.Minute] {
			keep = append(keep, event)
		}
	}
	c.flow.pending = keep
	return nil
}

func (c *Collector) pruneFlow(symbol string, now time.Time) {
	cut := now.Add(-6 * time.Minute)
	items := c.flow.trades[symbol]
	i := 0
	for i < len(items) && items[i].At.Before(cut) {
		i++
	}
	c.flow.trades[symbol] = append([]flowTrade(nil), items[i:]...)
}

func (c *Collector) tapeWindows(symbol string, at time.Time) map[string]any {
	out := map[string]any{}
	for _, d := range []time.Duration{5 * time.Second, 30 * time.Second, 5 * time.Minute} {
		items := selectTrades(c.flow.trades[symbol], at.Add(-d), at)
		var buy, sell float64
		for _, t := range items {
			if t.Side == "BUY" {
				buy += t.USD
			} else {
				sell += t.USD
			}
		}
		out[d.String()] = map[string]any{"trade_count": len(items), "aggressive_buy_usd": buy, "aggressive_sell_usd": sell, "delta_usd": buy - sell}
	}
	return out
}

func (c *Collector) bookForMarket(marketID string) bookSnapshot {
	for _, prefix := range []string{"order_book:", "order_book/"} {
		if b := c.books[prefix+marketID]; b != nil {
			return summarizeBook(b)
		}
	}
	return bookSnapshot{}
}

type level struct{ price, size float64 }

func summarizeBook(book *orderBook) bookSnapshot {
	bids, asks := levels(book.Bids, true), levels(book.Asks, false)
	var s bookSnapshot
	s.Nonce = book.Nonce
	if len(bids) > 0 {
		s.BestBid = bids[0].price
	}
	if len(asks) > 0 {
		s.BestAsk = asks[0].price
	}
	if s.BestBid > 0 && s.BestAsk > 0 {
		s.Spread = s.BestAsk - s.BestBid
		s.Mid = (s.BestAsk + s.BestBid) / 2
	}
	s.BidDepth1 = depth(bids, 1)
	s.AskDepth1 = depth(asks, 1)
	s.BidDepth5 = depth(bids, 5)
	s.AskDepth5 = depth(asks, 5)
	s.BidDepth10 = depth(bids, 10)
	s.AskDepth10 = depth(asks, 10)
	s.Imbalance1 = imbalance(s.BidDepth1, s.AskDepth1)
	s.Imbalance5 = imbalance(s.BidDepth5, s.AskDepth5)
	s.Imbalance10 = imbalance(s.BidDepth10, s.AskDepth10)
	if s.BidDepth1+s.AskDepth1 > 0 {
		s.Microprice = (s.BestAsk*s.BidDepth1 + s.BestBid*s.AskDepth1) / (s.BidDepth1 + s.AskDepth1)
	}
	return s
}
func levels(side map[string]string, desc bool) []level {
	out := make([]level, 0, len(side))
	for p, q := range side {
		pf, e1 := strconv.ParseFloat(p, 64)
		qf, e2 := strconv.ParseFloat(q, 64)
		if e1 == nil && e2 == nil && qf > 0 {
			out = append(out, level{pf, qf})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if desc {
			return out[i].price > out[j].price
		}
		return out[i].price < out[j].price
	})
	return out
}
func depth(v []level, n int) float64 {
	if len(v) < n {
		n = len(v)
	}
	var x float64
	for _, l := range v[:n] {
		x += l.size
	}
	return x
}
func imbalance(b, a float64) float64 {
	if b+a == 0 {
		return 0
	}
	return b / (b + a)
}
func objectList(v any) []map[string]any {
	raw, _ := v.([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, x := range raw {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}
func parseFlowTrade(raw map[string]any) flowTrade {
	ms := int64(number(raw["timestamp"]))
	side := "SELL"
	if makerAsk, _ := raw["is_maker_ask"].(bool); makerAsk {
		side = "BUY"
	}
	return flowTrade{At: time.UnixMilli(ms).UTC(), Price: number(raw["price"]), USD: number(raw["usd_amount"]), Side: side}
}
func selectTrades(v []flowTrade, from, to time.Time) []flowTrade {
	out := []flowTrade{}
	for _, t := range v {
		if !t.At.Before(from) && !t.At.After(to) {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}
func channelMarketID(channel string) string {
	p := strings.FieldsFunc(channel, func(r rune) bool { return r == ':' || r == '/' })
	if len(p) == 2 {
		return p[1]
	}
	return ""
}
func liquidatedSide(raw map[string]any) string {
	p := number(raw["taker_position_size_before"])
	if p > 0 {
		return "LONG"
	}
	if p < 0 {
		return "SHORT"
	}
	return "UNKNOWN"
}
