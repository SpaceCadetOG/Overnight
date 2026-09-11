// Package lighter converts public Lighter market messages into Oracle events.
// It contains no authenticated account or order-submission behavior.
package lighter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/oracle/model"
)

type Context struct {
	Asset          string
	ReceivedAt     time.Time
	ConnectionID   string
	FirstSequence  uint64
	DefaultQuality model.Quality
}

// Normalize returns one or more ordered events. A trade-channel message may
// contain many ordinary and liquidation trades, each with its own event.
func Normalize(raw []byte, ctx Context) ([]model.Envelope, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var message map[string]any
	if err := decoder.Decode(&message); err != nil {
		return nil, fmt.Errorf("decode Lighter message: %w", err)
	}
	if _, failed := message["error"]; failed {
		return nil, fmt.Errorf("Lighter message contains an error")
	}
	if ctx.FirstSequence == 0 {
		return nil, fmt.Errorf("first Oracle sequence must be positive")
	}
	if ctx.DefaultQuality == "" {
		ctx.DefaultQuality = model.QualityUnavailable
	}
	channel := stringValue(message["channel"])
	typ := stringValue(message["type"])
	sequence := ctx.FirstSequence
	makeEvent := func(stream model.Stream, venueSequence int64, quality model.Quality, payload any, exchangeTime *time.Time) (model.Envelope, error) {
		event, err := model.New(ctx.Asset, stream, sequence, venueSequence, ctx.ReceivedAt, ctx.ConnectionID, quality, payload)
		if err == nil {
			event.ExchangeTimestamp = exchangeTime
			sequence++
		}
		return event, err
	}

	switch {
	case strings.Contains(channel, "order_book"):
		book, ok := message["order_book"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("order-book payload is missing")
		}
		payload, nonce, err := normalizeBook(book)
		if err != nil {
			return nil, err
		}
		stream := model.StreamBookDelta
		quality := ctx.DefaultQuality
		if typ == "subscribed/order_book" {
			stream = model.StreamBookSnapshot
			quality = model.QualityCertified
		}
		event, err := makeEvent(stream, nonce, quality, payload, timestampFrom(message, book))
		if err != nil {
			return nil, err
		}
		return []model.Envelope{event}, nil
	case strings.Contains(channel, "trade"):
		return normalizeTrades(message, ctx, makeEvent)
	case strings.Contains(channel, "ticker") || strings.Contains(channel, "market_stats"):
		body, err := json.Marshal(message)
		if err != nil {
			return nil, err
		}
		event, err := makeEvent(model.StreamTicker, 0, ctx.DefaultQuality, model.Ticker{MarketID: marketID(channel), Fields: body}, timestampFrom(message))
		if err != nil {
			return nil, err
		}
		return []model.Envelope{event}, nil
	default:
		return nil, fmt.Errorf("unsupported Lighter channel %q", channel)
	}
}

type eventFactory func(model.Stream, int64, model.Quality, any, *time.Time) (model.Envelope, error)

func normalizeTrades(message map[string]any, ctx Context, makeEvent eventFactory) ([]model.Envelope, error) {
	out := []model.Envelope{}
	for _, spec := range []struct {
		field  string
		stream model.Stream
	}{
		{"trades", model.StreamTrade},
		{"liquidation_trades", model.StreamConfirmedLiquidation},
	} {
		items, _ := message[spec.field].([]any)
		for _, item := range items {
			row, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("malformed %s row", spec.field)
			}
			makerAsk := boolValue(row["is_maker_ask"])
			aggressor := "SELL"
			if makerAsk {
				aggressor = "BUY"
			}
			at := timestampFrom(row)
			var payload any = model.Trade{TradeID: firstString(row, "trade_id_str", "trade_id"), TransactionHash: stringValue(row["tx_hash"]), Price: decimal(row["price"]), Size: decimal(row["size"]), USDValue: decimal(row["usd_amount"]), AggressorSide: aggressor, MakerIsAsk: makerAsk}
			if spec.stream == model.StreamConfirmedLiquidation {
				positionSide := "SHORT"
				if aggressor == "SELL" {
					positionSide = "LONG"
				}
				payload = model.Liquidation{TradeID: firstString(row, "trade_id_str", "trade_id"), TransactionHash: stringValue(row["tx_hash"]), Price: decimal(row["price"]), Size: decimal(row["size"]), USDValue: decimal(row["usd_amount"]), AggressorSide: aggressor, LiquidatedPositionSide: positionSide, Confirmed: true, MethodVersion: "lighter-public-liquidation-v1"}
			}
			event, err := makeEvent(spec.stream, int64Value(row["trade_id"]), ctx.DefaultQuality, payload, at)
			if err != nil {
				return nil, err
			}
			event.TransactionTimestamp = at
			out = append(out, event)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("trade message contained no events")
	}
	return out, nil
}

func normalizeBook(raw map[string]any) (model.Book, int64, error) {
	nonce := int64Value(raw["nonce"])
	if nonce <= 0 {
		return model.Book{}, 0, fmt.Errorf("book nonce must be positive")
	}
	bids, err := normalizeLevels(raw["bids"])
	if err != nil {
		return model.Book{}, 0, fmt.Errorf("normalize bids: %w", err)
	}
	asks, err := normalizeLevels(raw["asks"])
	if err != nil {
		return model.Book{}, 0, fmt.Errorf("normalize asks: %w", err)
	}
	depth := len(bids)
	if len(asks) > depth {
		depth = len(asks)
	}
	return model.Book{BeginNonce: int64Value(raw["begin_nonce"]), Nonce: nonce, Depth: depth, Bids: bids, Asks: asks}, nonce, nil
}

func normalizeLevels(raw any) ([]model.Level, error) {
	items, ok := raw.([]any)
	if !ok && raw != nil {
		return nil, fmt.Errorf("levels must be an array")
	}
	levels := make([]model.Level, 0, len(items))
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("malformed level")
		}
		price, size := decimal(row["price"]), decimal(row["size"])
		if price == "" || size == "" {
			return nil, fmt.Errorf("level price and size are required")
		}
		if value, err := strconv.ParseFloat(price, 64); err != nil || value <= 0 {
			return nil, fmt.Errorf("invalid price %q", price)
		}
		if value, err := strconv.ParseFloat(size, 64); err != nil || value < 0 {
			return nil, fmt.Errorf("invalid size %q", size)
		}
		levels = append(levels, model.Level{Price: price, Size: size})
	}
	return levels, nil
}

func timestampFrom(objects ...map[string]any) *time.Time {
	for _, object := range objects {
		for _, key := range []string{"timestamp", "transaction_timestamp", "event_timestamp"} {
			value := object[key]
			if value == nil {
				continue
			}
			if millis := int64Value(value); millis > 0 {
				at := time.UnixMilli(millis).UTC()
				return &at
			}
			if parsed, err := time.Parse(time.RFC3339Nano, stringValue(value)); err == nil {
				parsed = parsed.UTC()
				return &parsed
			}
		}
	}
	return nil
}

func marketID(channel string) string {
	parts := strings.FieldsFunc(channel, func(r rune) bool { return r == '/' || r == ':' })
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

func firstString(row map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(row[key]); value != "" {
			return value
		}
	}
	return ""
}

func decimal(value any) string {
	switch value := value.(type) {
	case json.Number:
		return value.String()
	case string:
		return value
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	default:
		return ""
	}
}

func stringValue(value any) string {
	switch value := value.(type) {
	case string:
		return value
	case json.Number:
		return value.String()
	default:
		return ""
	}
}

func int64Value(value any) int64 {
	raw := decimal(value)
	result, _ := strconv.ParseInt(raw, 10, 64)
	return result
}

func boolValue(value any) bool {
	result, _ := value.(bool)
	return result
}
