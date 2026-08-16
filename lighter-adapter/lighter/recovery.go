package lighter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const recoveryStateVersion = 1

var ErrDuplicateOrderIntent = errors.New("order intent already reserved")

type SubmissionState string

const (
	SubmissionReserved  SubmissionState = "RESERVED"
	SubmissionSubmitted SubmissionState = "SUBMITTED"
	SubmissionFailed    SubmissionState = "FAILED"
	SubmissionUnknown   SubmissionState = "UNKNOWN"
)

type OrderMapping struct {
	IntentKey          string           `json:"intent_key"`
	ClientOrderIndex   int64            `json:"client_order_index"`
	ExchangeOrderIndex int64            `json:"exchange_order_index,omitempty"`
	Symbol             string           `json:"symbol"`
	MarketIndex        int16            `json:"market_index"`
	SubmissionState    SubmissionState  `json:"submission_state"`
	TxHash             string           `json:"tx_hash,omitempty"`
	LastOrder          *ReconciledOrder `json:"last_order,omitempty"`
	CreatedAt          int64            `json:"created_at"`
	UpdatedAt          int64            `json:"updated_at"`
}

type RecoveryState struct {
	Version           int                      `json:"version"`
	AccountIndex      int64                    `json:"account_index"`
	APIKeyIndex       uint8                    `json:"api_key_index"`
	LastObservedNonce int64                    `json:"last_observed_nonce"`
	Orders            map[string]*OrderMapping `json:"orders"`
	PositionSnapshot  *PositionSnapshot        `json:"position_snapshot,omitempty"`
	UpdatedAt         int64                    `json:"updated_at"`
}

type RecoveryStore struct {
	path  string
	mu    sync.Mutex
	state RecoveryState
}

func OpenRecoveryStore(path string, accountIndex int64, apiKeyIndex uint8) (*RecoveryStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("recovery state path is required")
	}
	if accountIndex <= 0 {
		return nil, errors.New("account index must be positive")
	}

	store := &RecoveryStore{path: path}
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		store.state = RecoveryState{
			Version: recoveryStateVersion, AccountIndex: accountIndex, APIKeyIndex: apiKeyIndex,
			Orders: make(map[string]*OrderMapping),
		}
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read recovery state: %w", err)
	}
	if err := json.Unmarshal(body, &store.state); err != nil {
		return nil, fmt.Errorf("decode recovery state: %w", err)
	}
	if store.state.Version != recoveryStateVersion {
		return nil, fmt.Errorf("unsupported recovery state version %d", store.state.Version)
	}
	if store.state.AccountIndex != accountIndex || store.state.APIKeyIndex != apiKeyIndex {
		return nil, fmt.Errorf("recovery state belongs to account=%d api_key=%d", store.state.AccountIndex, store.state.APIKeyIndex)
	}
	if store.state.Orders == nil {
		store.state.Orders = make(map[string]*OrderMapping)
	}
	return store, nil
}

func (s *RecoveryStore) persistLocked() error {
	s.state.UpdatedAt = time.Now().UnixMilli()
	body, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode recovery state: %w", err)
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create recovery directory: %w", err)
	}
	temporary, err := os.CreateTemp(dir, ".recovery-*.tmp")
	if err != nil {
		return fmt.Errorf("create recovery temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure recovery temporary file: %w", err)
	}
	if _, err := temporary.Write(body); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write recovery state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync recovery state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close recovery state: %w", err)
	}
	if err := os.Rename(temporaryName, s.path); err != nil {
		return fmt.Errorf("replace recovery state: %w", err)
	}
	removeTemporary = false
	return nil
}

func (s *RecoveryStore) Snapshot() RecoveryState {
	s.mu.Lock()
	defer s.mu.Unlock()
	body, _ := json.Marshal(s.state)
	var clone RecoveryState
	_ = json.Unmarshal(body, &clone)
	return clone
}

func (s *RecoveryStore) RecordObservedNonce(next int64) error {
	if next < 0 {
		return errors.New("observed nonce cannot be negative")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.LastObservedNonce = next
	return s.persistLocked()
}

// ReserveOrder durably claims an intent key before any transaction is signed.
// A repeated key always returns the original mapping and ErrDuplicateOrderIntent.
func (s *RecoveryStore) ReserveOrder(intentKey string, clientOrderIndex int64, symbol string, marketIndex int16) (*OrderMapping, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	intentKey = strings.TrimSpace(intentKey)
	if intentKey == "" {
		return nil, errors.New("intent key is required")
	}
	if clientOrderIndex <= 0 {
		return nil, errors.New("client order index must be positive")
	}
	if existing, ok := s.state.Orders[intentKey]; ok {
		copy := *existing
		return &copy, ErrDuplicateOrderIntent
	}
	for _, existing := range s.state.Orders {
		if existing.ClientOrderIndex == clientOrderIndex {
			return nil, fmt.Errorf("client_order_index=%d is already mapped to intent %q", clientOrderIndex, existing.IntentKey)
		}
	}
	now := time.Now().UnixMilli()
	mapping := &OrderMapping{
		IntentKey: intentKey, ClientOrderIndex: clientOrderIndex, Symbol: normalizeSymbol(symbol),
		MarketIndex: marketIndex, SubmissionState: SubmissionReserved, CreatedAt: now, UpdatedAt: now,
	}
	s.state.Orders[intentKey] = mapping
	if err := s.persistLocked(); err != nil {
		delete(s.state.Orders, intentKey)
		return nil, err
	}
	copy := *mapping
	return &copy, nil
}

func (s *RecoveryStore) MarkSubmitted(intentKey, txHash string) error {
	return s.updateMapping(intentKey, func(mapping *OrderMapping) {
		mapping.SubmissionState = SubmissionSubmitted
		mapping.TxHash = txHash
	})
}

func (s *RecoveryStore) MarkSubmissionFailed(intentKey string) error {
	return s.updateMapping(intentKey, func(mapping *OrderMapping) {
		mapping.SubmissionState = SubmissionFailed
	})
}

func (s *RecoveryStore) MarkSubmissionUnknown(intentKey string) error {
	return s.updateMapping(intentKey, func(mapping *OrderMapping) {
		mapping.SubmissionState = SubmissionUnknown
	})
}

func (s *RecoveryStore) ReopenFailedIntent(intentKey string) (*OrderMapping, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	mapping, ok := s.state.Orders[intentKey]
	if !ok {
		return nil, fmt.Errorf("order intent %q is not reserved", intentKey)
	}
	if mapping.SubmissionState != SubmissionFailed {
		copy := *mapping
		return &copy, fmt.Errorf("order intent %q is %s, not safely retryable", intentKey, mapping.SubmissionState)
	}
	mapping.SubmissionState = SubmissionReserved
	mapping.UpdatedAt = time.Now().UnixMilli()
	if err := s.persistLocked(); err != nil {
		mapping.SubmissionState = SubmissionFailed
		return nil, err
	}
	copy := *mapping
	return &copy, nil
}

func (s *RecoveryStore) MarkReconciledSubmitted(intentKey string, order *ReconciledOrder) error {
	if order == nil || order.State == OrderStateUnknown {
		return errors.New("known reconciled order is required")
	}
	return s.updateMapping(intentKey, func(mapping *OrderMapping) {
		mapping.SubmissionState = SubmissionSubmitted
		mapping.ExchangeOrderIndex = order.ExchangeOrderIndex
		copy := *order
		mapping.LastOrder = &copy
	})
}

func (s *RecoveryStore) updateMapping(intentKey string, update func(*OrderMapping)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	mapping, ok := s.state.Orders[intentKey]
	if !ok {
		return fmt.Errorf("order intent %q is not reserved", intentKey)
	}
	update(mapping)
	mapping.UpdatedAt = time.Now().UnixMilli()
	return s.persistLocked()
}

type NonceCoordinator struct {
	mu   sync.Mutex
	next int64
}

func NewNonceCoordinator(next int64) (*NonceCoordinator, error) {
	if next < 0 {
		return nil, errors.New("next nonce cannot be negative")
	}
	return &NonceCoordinator{next: next}, nil
}

func (n *NonceCoordinator) Take() int64 {
	n.mu.Lock()
	defer n.mu.Unlock()
	current := n.next
	n.next++
	return current
}

func (n *NonceCoordinator) Peek() int64 {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.next
}

// Resync replaces local nonce state with exchange truth. Call only when no
// signed transaction is in flight.
func (n *NonceCoordinator) Resync(ctx context.Context, manager *Manager) (int64, error) {
	next, err := manager.NextNonce(ctx)
	if err != nil {
		return 0, err
	}
	n.mu.Lock()
	n.next = next
	n.mu.Unlock()
	return next, nil
}

type RecoveryReport struct {
	RecoveredAt           int64                 `json:"recovered_at"`
	ExchangeNextNonce     int64                 `json:"exchange_next_nonce"`
	ActiveOrders          []Order               `json:"active_orders"`
	UntrackedActiveOrders []Order               `json:"untracked_active_orders"`
	TrackedOrders         []*ReconciledOrder    `json:"tracked_orders"`
	Positions             *PositionSnapshot     `json:"positions"`
	PositionDiscrepancies []PositionDiscrepancy `json:"position_discrepancies,omitempty"`
}

// Recover reconstructs exchange state without submitting, cancelling, or
// otherwise mutating exchange state.
func (s *RecoveryStore) Recover(ctx context.Context, manager *Manager) (*RecoveryReport, *NonceCoordinator, error) {
	if manager.AccountIndex != s.state.AccountIndex || manager.APIKeyIndex != s.state.APIKeyIndex {
		return nil, nil, errors.New("manager identity does not match recovery store")
	}
	nextNonce, err := manager.NextNonce(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("recover nonce: %w", err)
	}
	nonces, err := NewNonceCoordinator(nextNonce)
	if err != nil {
		return nil, nil, err
	}
	active, err := manager.ActiveOrders(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("recover active orders: %w", err)
	}
	positions, err := manager.PositionSnapshot(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("recover positions: %w", err)
	}

	s.mu.Lock()
	intentKeys := make([]string, 0, len(s.state.Orders))
	trackedIndexes := make(map[int64]bool, len(s.state.Orders))
	var priorPositions *PositionSnapshot
	if s.state.PositionSnapshot != nil {
		copy := *s.state.PositionSnapshot
		copy.Positions = append([]CanonicalPosition(nil), s.state.PositionSnapshot.Positions...)
		priorPositions = &copy
	}
	for key, mapping := range s.state.Orders {
		intentKeys = append(intentKeys, key)
		trackedIndexes[mapping.ClientOrderIndex] = true
	}
	s.mu.Unlock()
	sort.Strings(intentKeys)

	tracked := make([]*ReconciledOrder, 0, len(intentKeys))
	for _, key := range intentKeys {
		s.mu.Lock()
		clientOrderIndex := s.state.Orders[key].ClientOrderIndex
		s.mu.Unlock()
		order, err := manager.ReconcileOrder(ctx, clientOrderIndex)
		if err != nil {
			return nil, nil, fmt.Errorf("recover intent %q: %w", key, err)
		}
		tracked = append(tracked, order)
		s.mu.Lock()
		mapping := s.state.Orders[key]
		mapping.LastOrder = order
		mapping.ExchangeOrderIndex = order.ExchangeOrderIndex
		mapping.UpdatedAt = time.Now().UnixMilli()
		s.mu.Unlock()
	}

	untracked := make([]Order, 0)
	for _, order := range active {
		if !trackedIndexes[order.ClientOrderIndex] {
			untracked = append(untracked, order)
		}
	}
	var positionDiscrepancies []PositionDiscrepancy
	if priorPositions != nil {
		expected := make([]PositionExpectation, 0, len(priorPositions.Positions))
		for _, position := range priorPositions.Positions {
			expected = append(expected, PositionExpectation{Symbol: position.Symbol, Side: position.Side, Size: position.Size})
		}
		positionDiscrepancies, err = ComparePositions(*positions, expected)
		if err != nil {
			return nil, nil, fmt.Errorf("compare recovered positions: %w", err)
		}
	}

	s.mu.Lock()
	s.state.LastObservedNonce = nextNonce
	s.state.PositionSnapshot = positions
	err = s.persistLocked()
	s.mu.Unlock()
	if err != nil {
		return nil, nil, err
	}

	return &RecoveryReport{
		RecoveredAt: time.Now().UnixMilli(), ExchangeNextNonce: nextNonce, ActiveOrders: active,
		UntrackedActiveOrders: untracked, TrackedOrders: tracked, Positions: positions,
		PositionDiscrepancies: positionDiscrepancies,
	}, nonces, nil
}
