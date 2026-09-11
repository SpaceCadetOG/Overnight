// Package book owns deterministic L2 reconstruction for the Market Data Oracle.
package book

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/oracle/model"
)

var ErrUnavailable = errors.New("Oracle book is unavailable until a fresh snapshot is applied")

type State struct {
	mu            sync.RWMutex
	asset         string
	bids          map[string]string
	asks          map[string]string
	nonce         int64
	sequence      uint64
	quality       model.Quality
	ready         bool
	invalidReason string
}

type Snapshot struct {
	Asset          string        `json:"asset"`
	At             time.Time     `json:"timestamp"`
	ConnectionID   string        `json:"connection_id"`
	VenueNonce     int64         `json:"venue_nonce"`
	OracleSequence uint64        `json:"oracle_sequence"`
	Depth          int           `json:"depth"`
	Bids           []model.Level `json:"bids"`
	Asks           []model.Level `json:"asks"`
	Quality        model.Quality `json:"quality"`
	SchemaVersion  string        `json:"schema_version"`
	Checksum       string        `json:"checksum"`
}

func New(asset string) *State {
	return &State{asset: asset, bids: map[string]string{}, asks: map[string]string{}, quality: model.QualityUnavailable}
}

func (s *State) ApplySnapshot(value model.Book, oracleSequence uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if value.Nonce <= 0 || oracleSequence == 0 {
		return s.invalidateLocked("invalid snapshot sequence")
	}
	bids, asks := map[string]string{}, map[string]string{}
	if err := apply(bids, value.Bids); err != nil {
		return s.invalidateLocked("invalid snapshot bids: " + err.Error())
	}
	if err := apply(asks, value.Asks); err != nil {
		return s.invalidateLocked("invalid snapshot asks: " + err.Error())
	}
	if crossed(bids, asks) {
		return s.invalidateLocked("crossed snapshot")
	}
	s.bids, s.asks, s.nonce, s.sequence = bids, asks, value.Nonce, oracleSequence
	s.quality, s.ready, s.invalidReason = model.QualityCertified, true, ""
	return nil
}

func (s *State) ApplyDelta(value model.Book, oracleSequence uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ready {
		return ErrUnavailable
	}
	if value.BeginNonce != s.nonce || value.Nonce <= s.nonce || oracleSequence != s.sequence+1 {
		return s.invalidateLocked(fmt.Sprintf("sequence gap: nonce=%d begin=%d end=%d oracle=%d next=%d", s.nonce, value.BeginNonce, value.Nonce, oracleSequence, s.sequence+1))
	}
	bids, asks := clone(s.bids), clone(s.asks)
	if err := apply(bids, value.Bids); err != nil {
		return s.invalidateLocked("invalid delta bids: " + err.Error())
	}
	if err := apply(asks, value.Asks); err != nil {
		return s.invalidateLocked("invalid delta asks: " + err.Error())
	}
	if crossed(bids, asks) {
		return s.invalidateLocked("crossed delta")
	}
	s.bids, s.asks, s.nonce, s.sequence = bids, asks, value.Nonce, oracleSequence
	return nil
}

func (s *State) Invalidate(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.invalidateLocked(reason)
}

func (s *State) Quality() (model.Quality, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.quality, s.invalidReason
}

func (s *State) Snapshot(depth int, at time.Time, connectionID string) (Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.ready {
		return Snapshot{}, ErrUnavailable
	}
	if depth <= 0 {
		depth = max(len(s.bids), len(s.asks))
	}
	out := Snapshot{Asset: s.asset, At: at.UTC(), ConnectionID: connectionID, VenueNonce: s.nonce, OracleSequence: s.sequence, Depth: depth, Bids: sorted(s.bids, true, depth), Asks: sorted(s.asks, false, depth), Quality: s.quality, SchemaVersion: "oracle-book-checkpoint-v1"}
	out.Checksum = checksum(out)
	return out, nil
}

func Restore(value Snapshot) (*State, error) {
	if value.SchemaVersion != "oracle-book-checkpoint-v1" || value.Checksum == "" || checksum(value) != value.Checksum {
		return nil, errors.New("invalid book checkpoint checksum or schema")
	}
	s := New(value.Asset)
	if err := s.ApplySnapshot(model.Book{Nonce: value.VenueNonce, Bids: value.Bids, Asks: value.Asks}, value.OracleSequence); err != nil {
		return nil, err
	}
	s.quality = value.Quality
	return s, nil
}

func (s *State) invalidateLocked(reason string) error {
	s.ready = false
	s.quality = model.QualityUnavailable
	s.invalidReason = reason
	return fmt.Errorf("%w: %s", ErrUnavailable, reason)
}

func apply(side map[string]string, levels []model.Level) error {
	for _, level := range levels {
		price, size := new(big.Rat), new(big.Rat)
		if _, ok := price.SetString(level.Price); !ok || price.Sign() <= 0 {
			return fmt.Errorf("invalid price %q", level.Price)
		}
		if _, ok := size.SetString(level.Size); !ok || size.Sign() < 0 {
			return fmt.Errorf("invalid size %q", level.Size)
		}
		if size.Sign() == 0 {
			delete(side, level.Price)
		} else {
			side[level.Price] = level.Size
		}
	}
	return nil
}

func crossed(bids, asks map[string]string) bool {
	bestBids, bestAsks := sorted(bids, true, 1), sorted(asks, false, 1)
	if len(bestBids) == 0 || len(bestAsks) == 0 {
		return false
	}
	bid, ask := new(big.Rat), new(big.Rat)
	bid.SetString(bestBids[0].Price)
	ask.SetString(bestAsks[0].Price)
	return bid.Cmp(ask) >= 0
}

func sorted(side map[string]string, descending bool, depth int) []model.Level {
	prices := make([]string, 0, len(side))
	for price := range side {
		prices = append(prices, price)
	}
	sort.Slice(prices, func(i, j int) bool {
		a, b := new(big.Rat), new(big.Rat)
		a.SetString(prices[i])
		b.SetString(prices[j])
		comparison := a.Cmp(b)
		if descending {
			return comparison > 0
		}
		return comparison < 0
	})
	if depth < len(prices) {
		prices = prices[:depth]
	}
	out := make([]model.Level, 0, len(prices))
	for _, price := range prices {
		out = append(out, model.Level{Price: price, Size: side[price]})
	}
	return out
}

func clone(source map[string]string) map[string]string {
	out := make(map[string]string, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func checksum(value Snapshot) string {
	value.Checksum = ""
	body, _ := json.Marshal(value)
	digest := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
