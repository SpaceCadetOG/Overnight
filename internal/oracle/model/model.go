// Package model defines the venue-independent Market Data Oracle contract.
// It intentionally has no dependency on account, signing, or execution code.
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const SchemaVersion = "oracle-event-v1"

type Stream string

const (
	StreamInstrument           Stream = "instrument"
	StreamTrade                Stream = "trade"
	StreamBookSnapshot         Stream = "book_snapshot"
	StreamBookDelta            Stream = "book_delta"
	StreamTicker               Stream = "ticker"
	StreamConfirmedLiquidation Stream = "confirmed_liquidation"
	StreamInferredLiquidation  Stream = "inferred_liquidation"
	StreamConnection           Stream = "connection"
	StreamQuality              Stream = "quality"
	StreamHeartbeat            Stream = "heartbeat"
)

type Quality string

const (
	QualityCertified                      Quality = "CERTIFIED"
	QualityCertifiedWithExcludedIntervals Quality = "CERTIFIED_WITH_EXCLUDED_INTERVALS"
	QualityChecksumVerifiedUncertified    Quality = "CHECKSUM_VERIFIED_UNCERTIFIED"
	QualityQuarantined                    Quality = "QUARANTINED"
	QualityUnavailable                    Quality = "UNAVAILABLE"
)

// Envelope is identical on the live feed, in immutable normalized storage,
// and during replay. OracleSequence is assigned independently per asset and
// stream; VenueSequence preserves a Lighter nonce when one exists.
type Envelope struct {
	EventID              string          `json:"event_id"`
	Asset                string          `json:"asset"`
	Venue                string          `json:"venue"`
	Stream               Stream          `json:"stream"`
	OracleSequence       uint64          `json:"oracle_sequence"`
	VenueSequence        int64           `json:"venue_sequence,omitempty"`
	ExchangeTimestamp    *time.Time      `json:"exchange_timestamp,omitempty"`
	TransactionTimestamp *time.Time      `json:"transaction_timestamp,omitempty"`
	OracleReceivedAt     time.Time       `json:"oracle_received_at"`
	ConnectionID         string          `json:"connection_id"`
	SchemaVersion        string          `json:"schema_version"`
	Quality              Quality         `json:"quality"`
	Payload              json.RawMessage `json:"payload"`
}

type Level struct {
	Price string `json:"price"`
	Size  string `json:"size"`
}

type Book struct {
	BeginNonce int64   `json:"begin_nonce"`
	Nonce      int64   `json:"nonce"`
	Depth      int     `json:"depth"`
	Bids       []Level `json:"bids"`
	Asks       []Level `json:"asks"`
}

type Trade struct {
	TradeID         string `json:"trade_id"`
	TransactionHash string `json:"transaction_hash,omitempty"`
	Price           string `json:"price"`
	Size            string `json:"size"`
	USDValue        string `json:"usd_value,omitempty"`
	AggressorSide   string `json:"aggressor_side"`
	MakerIsAsk      bool   `json:"maker_is_ask"`
}

type Ticker struct {
	MarketID string          `json:"market_id,omitempty"`
	Fields   json.RawMessage `json:"fields"`
}

type Liquidation struct {
	TradeID                string `json:"trade_id"`
	TransactionHash        string `json:"transaction_hash,omitempty"`
	Price                  string `json:"price"`
	Size                   string `json:"size"`
	USDValue               string `json:"usd_value,omitempty"`
	AggressorSide          string `json:"aggressor_side"`
	LiquidatedPositionSide string `json:"liquidated_position_side"`
	Confirmed              bool   `json:"confirmed"`
	MethodVersion          string `json:"method_version"`
}

func New(asset string, stream Stream, oracleSequence uint64, venueSequence int64, received time.Time, connectionID string, quality Quality, payload any) (Envelope, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("marshal Oracle payload: %w", err)
	}
	e := Envelope{Asset: strings.ToUpper(strings.TrimSpace(asset)), Venue: "lighter", Stream: stream, OracleSequence: oracleSequence, VenueSequence: venueSequence, OracleReceivedAt: received.UTC(), ConnectionID: strings.TrimSpace(connectionID), SchemaVersion: SchemaVersion, Quality: quality, Payload: body}
	if err := e.Validate(); err != nil {
		return Envelope{}, err
	}
	e.EventID = e.DeterministicID()
	return e, nil
}

func (e Envelope) Validate() error {
	if e.Asset == "" || e.Asset != strings.ToUpper(e.Asset) {
		return errors.New("asset is required and must be uppercase")
	}
	if !validStream(e.Stream) {
		return fmt.Errorf("unsupported stream %q", e.Stream)
	}
	if e.OracleSequence == 0 {
		return errors.New("oracle_sequence must be positive")
	}
	if e.OracleReceivedAt.IsZero() {
		return errors.New("oracle_received_at is required")
	}
	if e.ConnectionID == "" {
		return errors.New("connection_id is required")
	}
	if e.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schema_version %q", e.SchemaVersion)
	}
	if !validQuality(e.Quality) {
		return fmt.Errorf("unsupported quality %q", e.Quality)
	}
	if len(e.Payload) == 0 || !json.Valid(e.Payload) {
		return errors.New("payload must be valid JSON")
	}
	return nil
}

func (e Envelope) DeterministicID() string {
	canonical := struct {
		Asset, Venue, Stream, Connection, Schema string
		OracleSequence                           uint64
		VenueSequence                            int64
		Payload                                  json.RawMessage
	}{e.Asset, e.Venue, string(e.Stream), e.ConnectionID, e.SchemaVersion, e.OracleSequence, e.VenueSequence, e.Payload}
	body, _ := json.Marshal(canonical)
	digest := sha256.Sum256(body)
	return "evt_" + hex.EncodeToString(digest[:16])
}

func validStream(value Stream) bool {
	switch value {
	case StreamInstrument, StreamTrade, StreamBookSnapshot, StreamBookDelta, StreamTicker, StreamConfirmedLiquidation, StreamInferredLiquidation, StreamConnection, StreamQuality, StreamHeartbeat:
		return true
	default:
		return false
	}
}

func validQuality(value Quality) bool {
	switch value {
	case QualityCertified, QualityCertifiedWithExcludedIntervals, QualityChecksumVerifiedUncertified, QualityQuarantined, QualityUnavailable:
		return true
	default:
		return false
	}
}
