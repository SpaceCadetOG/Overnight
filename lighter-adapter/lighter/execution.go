package lighter

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/elliottech/lighter-go/types"
	"github.com/elliottech/lighter-go/types/txtypes"
)

type Execution interface {
	PlaceOrder(context.Context, PlaceOrderRequest) (*OrderSubmission, error)
	CancelOrder(context.Context, int64) error
	CancelAll(context.Context) error
	GetActiveOrders(context.Context) ([]Order, error)
	GetOrderStatus(context.Context, int64) (*ReconciledOrder, error)
	GetPositions(context.Context) (*PositionSnapshot, error)
	Reconcile(context.Context) (*RecoveryReport, error)
}

type Side string

const (
	SideBuy  Side = "BUY"
	SideSell Side = "SELL"
)

type ExecutionOrderType string

const (
	ExecutionOrderLimit           ExecutionOrderType = "LIMIT"
	ExecutionOrderMarket          ExecutionOrderType = "MARKET"
	ExecutionOrderStopLoss        ExecutionOrderType = "STOP_LOSS"
	ExecutionOrderTakeProfit      ExecutionOrderType = "TAKE_PROFIT"
	ExecutionOrderStopLossLimit   ExecutionOrderType = "STOP_LOSS_LIMIT"
	ExecutionOrderTakeProfitLimit ExecutionOrderType = "TAKE_PROFIT_LIMIT"
)

type TimeInForce string

const (
	TimeInForceIOC      TimeInForce = "IOC"
	TimeInForceGoodTill TimeInForce = "GOOD_TILL_TIME"
	TimeInForcePostOnly TimeInForce = "POST_ONLY"
)

type PlaceOrderRequest struct {
	IntentKey        string             `json:"intent_key"`
	ClientOrderIndex int64              `json:"client_order_index,omitempty"`
	Symbol           string             `json:"symbol"`
	Side             Side               `json:"side"`
	Quantity         float64            `json:"quantity"`
	Price            float64            `json:"price"`
	Type             ExecutionOrderType `json:"type"`
	TimeInForce      TimeInForce        `json:"time_in_force"`
	ReduceOnly       bool               `json:"reduce_only"`
	ExpiresAt        time.Time          `json:"expires_at,omitempty"`
	TriggerPrice     float64            `json:"trigger_price,omitempty"`
	StopPrice        float64            `json:"stop_price,omitempty"`
}

type OrderSubmission struct {
	IntentKey          string  `json:"intent_key"`
	ClientOrderIndex   int64   `json:"client_order_index"`
	MarketIndex        int16   `json:"market_index"`
	TxHash             string  `json:"tx_hash"`
	ExchangeOrderIndex int64   `json:"exchange_order_index,omitempty"`
	Nonce              int64   `json:"nonce"`
	EncodedBaseAmount  int64   `json:"encoded_base_amount"`
	EncodedPrice       uint32  `json:"encoded_price"`
	RequestedQuantity  float64 `json:"requested_quantity"`
	RequestedPrice     float64 `json:"requested_price"`
}

type ErrorKind string

const (
	ErrorValidation ErrorKind = "VALIDATION"
	ErrorConflict   ErrorKind = "CONFLICT"
	ErrorNotFound   ErrorKind = "NOT_FOUND"
	ErrorAuth       ErrorKind = "AUTH"
	ErrorRateLimit  ErrorKind = "RATE_LIMIT"
	ErrorTimeout    ErrorKind = "TIMEOUT"
	ErrorExchange   ErrorKind = "EXCHANGE"
	ErrorInternal   ErrorKind = "INTERNAL"
)

type ExecutionError struct {
	Kind      ErrorKind
	Operation string
	Retryable bool
	Err       error
}

func (e *ExecutionError) Error() string {
	return fmt.Sprintf("%s %s: %v", e.Operation, e.Kind, e.Err)
}

func (e *ExecutionError) Unwrap() error { return e.Err }

func classifyExecutionError(operation string, err error) *ExecutionError {
	if err == nil {
		return nil
	}
	var netError net.Error
	text := strings.ToLower(err.Error())
	classified := &ExecutionError{Kind: ErrorExchange, Operation: operation, Err: err}
	switch {
	case errors.Is(err, ErrRiskRejected):
		classified.Kind = ErrorValidation
	case errors.Is(err, ErrDuplicateOrderIntent), strings.Contains(text, "already"):
		classified.Kind = ErrorConflict
	case errors.As(err, &netError) && netError.Timeout(), errors.Is(err, context.DeadlineExceeded):
		classified.Kind, classified.Retryable = ErrorTimeout, true
	case strings.Contains(text, "429"), strings.Contains(text, "rate limit"):
		classified.Kind, classified.Retryable = ErrorRateLimit, true
	case strings.Contains(text, "http 5"):
		classified.Kind, classified.Retryable = ErrorExchange, true
	case strings.Contains(text, "401"), strings.Contains(text, "403"), strings.Contains(text, "auth token"):
		classified.Kind = ErrorAuth
	case strings.Contains(text, "not found"):
		classified.Kind = ErrorNotFound
	case strings.Contains(text, "required"), strings.Contains(text, "invalid"), strings.Contains(text, "must be"), strings.Contains(text, "below"):
		classified.Kind = ErrorValidation
	}
	return classified
}

type ExecutionEngine struct {
	manager    *Manager
	store      *RecoveryStore
	nonces     *NonceCoordinator
	nonceMu    sync.RWMutex
	mutationMu sync.Mutex

	clientIDMu   sync.Mutex
	nextClientID int64
	readAttempts int
}

func RecoverExecutionEngine(ctx context.Context, manager *Manager, store *RecoveryStore) (*ExecutionEngine, *RecoveryReport, error) {
	report, nonces, err := store.Recover(ctx, manager)
	if err != nil {
		return nil, nil, classifyExecutionError("initialize execution", err)
	}
	nextClientID := time.Now().UnixMilli()
	for _, mapping := range store.Snapshot().Orders {
		if mapping.ClientOrderIndex >= nextClientID {
			nextClientID = mapping.ClientOrderIndex + 1
		}
	}
	return &ExecutionEngine{
		manager: manager, store: store, nonces: nonces, nextClientID: nextClientID, readAttempts: 3,
	}, report, nil
}

func (e *ExecutionEngine) allocateClientOrderIndex() int64 {
	e.clientIDMu.Lock()
	defer e.clientIDMu.Unlock()
	id := e.nextClientID
	e.nextClientID++
	return id
}

func (e *ExecutionEngine) takeNonce() int64 {
	e.nonceMu.RLock()
	defer e.nonceMu.RUnlock()
	return e.nonces.Take()
}

func (e *ExecutionEngine) resyncNonce(ctx context.Context, operationErr error) error {
	e.nonceMu.RLock()
	next, err := e.nonces.Resync(ctx, e.manager)
	e.nonceMu.RUnlock()
	if err == nil {
		err = e.store.RecordObservedNonce(next)
	}
	if err != nil {
		return fmt.Errorf("%v; nonce resynchronization failed: %w", operationErr, err)
	}
	return operationErr
}

func encodeOrderOptions(request PlaceOrderRequest) (uint8, uint8, int64, error) {
	var orderType uint8
	switch request.Type {
	case ExecutionOrderLimit:
		orderType = txtypes.LimitOrder
	case ExecutionOrderMarket:
		orderType = txtypes.MarketOrder
	case ExecutionOrderStopLoss:
		orderType = txtypes.StopLossOrder
	case ExecutionOrderTakeProfit:
		orderType = txtypes.TakeProfitOrder
	case ExecutionOrderStopLossLimit:
		orderType = txtypes.StopLossLimitOrder
	case ExecutionOrderTakeProfitLimit:
		orderType = txtypes.TakeProfitLimitOrder
	default:
		return 0, 0, 0, fmt.Errorf("invalid order type %q", request.Type)
	}

	var tif uint8
	switch request.TimeInForce {
	case TimeInForceIOC:
		tif = txtypes.ImmediateOrCancel
	case TimeInForceGoodTill:
		tif = txtypes.GoodTillTime
	case TimeInForcePostOnly:
		tif = txtypes.PostOnly
	default:
		return 0, 0, 0, fmt.Errorf("invalid time in force %q", request.TimeInForce)
	}
	marketTrigger := request.Type == ExecutionOrderStopLoss || request.Type == ExecutionOrderTakeProfit
	limitTrigger := request.Type == ExecutionOrderStopLossLimit || request.Type == ExecutionOrderTakeProfitLimit
	if (request.Type == ExecutionOrderMarket || marketTrigger) && request.TimeInForce != TimeInForceIOC {
		return 0, 0, 0, errors.New("market and market-trigger orders must use IOC")
	}
	if limitTrigger && request.TimeInForce == TimeInForceIOC {
		return 0, 0, 0, errors.New("trigger-limit orders must use GOOD_TILL_TIME or POST_ONLY")
	}
	expiry := int64(0)
	triggerOrder := marketTrigger || limitTrigger
	if request.TimeInForce != TimeInForceIOC || triggerOrder {
		if request.ExpiresAt.IsZero() || !request.ExpiresAt.After(time.Now()) {
			return 0, 0, 0, errors.New("limit and trigger orders require a future expiry")
		}
		expiry = request.ExpiresAt.UnixMilli()
	}
	return orderType, tif, expiry, nil
}

func (e *ExecutionEngine) PlaceOrder(ctx context.Context, request PlaceOrderRequest) (*OrderSubmission, error) {
	e.mutationMu.Lock()
	defer e.mutationMu.Unlock()
	if strings.TrimSpace(request.IntentKey) == "" {
		return nil, classifyExecutionError("place order", errors.New("intent key is required"))
	}
	if request.Side != SideBuy && request.Side != SideSell {
		return nil, classifyExecutionError("place order", fmt.Errorf("invalid side %q", request.Side))
	}
	orderType, tif, expiry, err := encodeOrderOptions(request)
	if err != nil {
		return nil, classifyExecutionError("place order", err)
	}
	market, err := e.manager.MarketBySymbol(ctx, request.Symbol)
	if err != nil {
		return nil, classifyExecutionError("place order", err)
	}
	encoded, err := market.EncodeOrder(request.Quantity, request.Price)
	if err != nil {
		return nil, classifyExecutionError("place order", err)
	}
	triggerPrice := uint32(0)
	if request.Type == ExecutionOrderStopLoss || request.Type == ExecutionOrderTakeProfit || request.Type == ExecutionOrderStopLossLimit || request.Type == ExecutionOrderTakeProfitLimit {
		if request.TriggerPrice <= 0 {
			return nil, classifyExecutionError("place order", errors.New("trigger price is required"))
		}
		triggerPrice, err = market.EncodePrice(request.TriggerPrice)
		if err != nil {
			return nil, classifyExecutionError("place order", fmt.Errorf("encode trigger price: %w", err))
		}
	}
	clientOrderIndex := request.ClientOrderIndex
	if clientOrderIndex == 0 {
		clientOrderIndex = e.allocateClientOrderIndex()
	}
	mapping, err := e.store.ReserveOrder(request.IntentKey, clientOrderIndex, market.Symbol, market.MarketID)
	if err != nil {
		if errors.Is(err, ErrDuplicateOrderIntent) && mapping != nil && mapping.SubmissionState == SubmissionSubmitted {
			return submissionFromMapping(mapping), nil
		}
		if errors.Is(err, ErrDuplicateOrderIntent) && mapping != nil && mapping.SubmissionState == SubmissionFailed {
			mapping, err = e.store.ReopenFailedIntent(request.IntentKey)
			if err == nil {
				clientOrderIndex = mapping.ClientOrderIndex
			}
		}
		if errors.Is(err, ErrDuplicateOrderIntent) && mapping != nil && mapping.SubmissionState == SubmissionUnknown {
			reconciled, reconcileErr := e.manager.ReconcileOrder(ctx, mapping.ClientOrderIndex)
			if reconcileErr != nil {
				return nil, classifyExecutionError("resolve ambiguous order", reconcileErr)
			}
			if reconciled.State != OrderStateUnknown {
				if persistErr := e.store.MarkReconciledSubmitted(request.IntentKey, reconciled); persistErr != nil {
					return nil, classifyExecutionError("resolve ambiguous order", persistErr)
				}
				return submissionFromMapping(e.store.Snapshot().Orders[request.IntentKey]), nil
			}
			return nil, classifyExecutionError("resolve ambiguous order", fmt.Errorf("%w: client_order_index=%d remains exchange-unknown", ErrDuplicateOrderIntent, mapping.ClientOrderIndex))
		}
		if err == nil {
			// A known pre-send failure was durably reopened above.
		} else if mapping != nil {
			return nil, classifyExecutionError("place order", fmt.Errorf("%w: client_order_index=%d", err, mapping.ClientOrderIndex))
		} else {
			return nil, classifyExecutionError("place order", err)
		}
	}

	if e.manager.TxClient == nil {
		_ = e.store.MarkSubmissionFailed(request.IntentKey)
		return nil, classifyExecutionError("place order", errors.New("transaction client is not configured"))
	}
	nonce := e.takeNonce()
	if err := e.store.MarkPrepared(request.IntentKey, nonce, encoded.BaseAmount, encoded.Price, request.Quantity, request.Price); err != nil {
		_ = e.store.MarkSubmissionFailed(request.IntentKey)
		return nil, classifyExecutionError("place order", fmt.Errorf("persist encoded request: %w", err))
	}
	isAsk := uint8(0)
	if request.Side == SideSell {
		isAsk = 1
	}
	reduceOnly := uint8(0)
	if request.ReduceOnly {
		reduceOnly = 1
	}
	tx, err := e.manager.TxClient.GetCreateOrderTransaction(&types.CreateOrderTxReq{
		MarketIndex: encoded.MarketIndex, ClientOrderIndex: clientOrderIndex, BaseAmount: encoded.BaseAmount,
		Price: encoded.Price, IsAsk: isAsk, Type: orderType, TimeInForce: tif,
		ReduceOnly: reduceOnly, TriggerPrice: triggerPrice, OrderExpiry: expiry,
	}, &types.TransactOpts{Nonce: &nonce})
	if err != nil {
		_ = e.store.MarkSubmissionFailed(request.IntentKey)
		return nil, classifyExecutionError("place order", e.resyncNonce(ctx, fmt.Errorf("build transaction: %w", err)))
	}
	if err := tx.Validate(); err != nil {
		_ = e.store.MarkSubmissionFailed(request.IntentKey)
		return nil, classifyExecutionError("place order", e.resyncNonce(ctx, fmt.Errorf("validate transaction: %w", err)))
	}
	txInfo, err := tx.GetTxInfo()
	if err != nil {
		_ = e.store.MarkSubmissionFailed(request.IntentKey)
		return nil, classifyExecutionError("place order", e.resyncNonce(ctx, fmt.Errorf("encode transaction: %w", err)))
	}
	response, err := e.manager.sendTx(ctx, tx.GetTxType(), txInfo)
	if err != nil {
		// A transport failure can occur after exchange acceptance. Preserve the
		// reservation as ambiguous and require reconciliation before any retry.
		_ = e.store.MarkSubmissionUnknown(request.IntentKey)
		return nil, classifyExecutionError("place order", e.resyncNonce(ctx, err))
	}
	if err := e.store.MarkSubmitted(request.IntentKey, response.TxHash); err != nil {
		return nil, classifyExecutionError("place order", fmt.Errorf("persist submission: %w", err))
	}
	return &OrderSubmission{
		IntentKey: request.IntentKey, ClientOrderIndex: clientOrderIndex,
		MarketIndex: encoded.MarketIndex, TxHash: response.TxHash, Nonce: nonce,
		EncodedBaseAmount: encoded.BaseAmount, EncodedPrice: encoded.Price,
		RequestedQuantity: request.Quantity, RequestedPrice: request.Price,
	}, nil
}

func submissionFromMapping(mapping *OrderMapping) *OrderSubmission {
	if mapping == nil {
		return nil
	}
	return &OrderSubmission{
		IntentKey: mapping.IntentKey, ClientOrderIndex: mapping.ClientOrderIndex,
		MarketIndex: mapping.MarketIndex, TxHash: mapping.TxHash,
		ExchangeOrderIndex: mapping.ExchangeOrderIndex, Nonce: mapping.Nonce,
		EncodedBaseAmount: mapping.EncodedBaseAmount, EncodedPrice: mapping.EncodedPrice,
		RequestedQuantity: mapping.RequestedQuantity, RequestedPrice: mapping.RequestedPrice,
	}
}

func (e *ExecutionEngine) CancelOrder(ctx context.Context, clientOrderIndex int64) error {
	e.mutationMu.Lock()
	defer e.mutationMu.Unlock()
	order, err := e.GetOrderStatus(ctx, clientOrderIndex)
	if err != nil {
		return err
	}
	if order.State != OrderStateOpen && order.State != OrderStatePartial {
		return classifyExecutionError("cancel order", fmt.Errorf("order state %s is not cancellable", order.State))
	}
	_, err = e.manager.cancelWithNonce(ctx, Order{
		OrderIndex: order.ExchangeOrderIndex, ClientOrderIndex: order.ClientOrderIndex, MarketIndex: order.MarketIndex,
	}, e.takeNonce())
	if err != nil {
		return classifyExecutionError("cancel order", e.resyncNonce(ctx, err))
	}
	return nil
}

func (e *ExecutionEngine) CancelAll(ctx context.Context) error {
	e.mutationMu.Lock()
	defer e.mutationMu.Unlock()
	orders, err := e.GetActiveOrders(ctx)
	if err != nil {
		return err
	}
	for _, order := range orders {
		if _, err := e.manager.cancelWithNonce(ctx, order, e.takeNonce()); err != nil {
			return classifyExecutionError("cancel all", e.resyncNonce(ctx, fmt.Errorf("client_order_index=%d: %w", order.ClientOrderIndex, err)))
		}
	}
	return nil
}

func retryRead[T any](ctx context.Context, attempts int, operation string, call func() (T, error)) (T, error) {
	var zero T
	var last error
	for attempt := 1; attempt <= attempts; attempt++ {
		value, err := call()
		if err == nil {
			return value, nil
		}
		classified := classifyExecutionError(operation, err)
		last = classified
		if !classified.Retryable || attempt == attempts {
			return zero, classified
		}
		timer := time.NewTimer(time.Duration(attempt) * 100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return zero, classifyExecutionError(operation, ctx.Err())
		case <-timer.C:
		}
	}
	return zero, last
}

func (e *ExecutionEngine) GetActiveOrders(ctx context.Context) ([]Order, error) {
	return retryRead(ctx, e.readAttempts, "get active orders", func() ([]Order, error) {
		return e.manager.ActiveOrders(ctx, nil)
	})
}

func (e *ExecutionEngine) GetOrderStatus(ctx context.Context, clientOrderIndex int64) (*ReconciledOrder, error) {
	return retryRead(ctx, e.readAttempts, "get order status", func() (*ReconciledOrder, error) {
		return e.manager.ReconcileOrder(ctx, clientOrderIndex)
	})
}

func (e *ExecutionEngine) GetPositions(ctx context.Context) (*PositionSnapshot, error) {
	return retryRead(ctx, e.readAttempts, "get positions", func() (*PositionSnapshot, error) {
		return e.manager.PositionSnapshot(ctx)
	})
}

func (e *ExecutionEngine) Reconcile(ctx context.Context) (*RecoveryReport, error) {
	e.mutationMu.Lock()
	defer e.mutationMu.Unlock()
	e.nonceMu.Lock()
	defer e.nonceMu.Unlock()
	report, nonces, err := e.store.Recover(ctx, e.manager)
	if err != nil {
		return nil, classifyExecutionError("reconcile", err)
	}
	e.nonces = nonces
	return report, nil
}

var _ Execution = (*ExecutionEngine)(nil)

// HTTPStatusRetryable documents the write-safe boundary for callers that
// need to classify raw HTTP failures. Mutating calls are never auto-retried.
func HTTPStatusRetryable(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}
