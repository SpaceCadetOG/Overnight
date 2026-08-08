package lighter

import (
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	lighterclient "github.com/elliottech/lighter-go/client"
	lighterhttp "github.com/elliottech/lighter-go/client/http"
	lightertypes "github.com/elliottech/lighter-go/types"
	lightertx "github.com/elliottech/lighter-go/types/txtypes"

	adapter "github.com/ogtrading/overnight-strategy/internal/adapters/lighter"
	"github.com/ogtrading/overnight-strategy/internal/execution"
	"github.com/ogtrading/overnight-strategy/internal/execution/lighter/ws"
	"github.com/ogtrading/overnight-strategy/internal/execution/lighter/ws/runtime"
)

type submittedOrder struct {
	Symbol string
	Index  int64
}

type Executor struct {
	txClient     *lighterclient.TxClient
	adapter      *adapter.Client
	accountIndex int64
	apiKeyIndex  uint8
	orderManager *runtime.OrderManager
	nextIndex    atomic.Int64
	mu           sync.RWMutex
	submitted    map[string]submittedOrder
}

func NewExecutor(baseURL, privateKey string, accountIndex int64, apiKeyIndex uint8, chainID uint32, orderManager *runtime.OrderManager) (*Executor, error) {
	txClient, err := lighterclient.CreateClient(lighterhttp.NewClient(baseURL), privateKey, chainID, apiKeyIndex, accountIndex)
	if err != nil {
		return nil, err
	}
	executor := &Executor{txClient: txClient, adapter: adapter.NewClient(baseURL), accountIndex: accountIndex, apiKeyIndex: apiKeyIndex, orderManager: orderManager, submitted: map[string]submittedOrder{}}
	executor.nextIndex.Store(time.Now().UnixMilli())
	return executor, nil
}

func (e *Executor) Submit(req execution.OrderRequest) (execution.OrderResponse, error) {
	return e.submit(req, true)
}

// SubmitControlledTest is isolated from strategy automation and may bypass only
// the daily entry window. Symbol, kill-switch, risk, idempotency, precision and
// account reconciliation gates remain enforced.
func (e *Executor) SubmitControlledTest(req execution.OrderRequest) (execution.OrderResponse, error) {
	return e.submit(req, false)
}

func (e *Executor) submit(req execution.OrderRequest, enforceWindow bool) (execution.OrderResponse, error) {
	if _, err := MarketFor(req.Symbol); err != nil {
		return execution.OrderResponse{}, err
	}
	if !req.ReduceOnly {
		if err := execution.GateFromEnvironment(execution.Live).Authorize(req.Symbol, time.Now()); err != nil {
			return execution.OrderResponse{}, err
		}
		location, err := time.LoadLocation("America/Chicago")
		if err != nil {
			return execution.OrderResponse{}, err
		}
		now := time.Now()
		if enforceWindow && !execution.WithinOrderWindow(now, now.In(location), location) {
			return execution.OrderResponse{}, fmt.Errorf("new %s entry outside 05:00-05:05 CT order window", req.Symbol)
		}
		if req.ClientOrderIndex <= 0 {
			return execution.OrderResponse{}, fmt.Errorf("deterministic client order index is required for restart-safe entry submission")
		}
		if req.RiskUSD <= 0 || req.RiskLimitUSD <= 0 || req.RiskUSD > req.RiskLimitUSD+1e-9 {
			return execution.OrderResponse{}, fmt.Errorf("entry risk %.2f exceeds or omits limit %.2f", req.RiskUSD, req.RiskLimitUSD)
		}
		if err := e.preflightAccount(req.Symbol); err != nil {
			return execution.OrderResponse{}, err
		}
	}
	index := req.ClientOrderIndex
	if index <= 0 {
		index = e.nextIndex.Add(1)
	}
	expiry := req.ExpiresAt
	if expiry.IsZero() && req.OrderType != lightertx.MarketOrder {
		expiry = time.Now().Add(6 * time.Minute)
	}
	txReq, err := BuildCreateOrder(OrderRequest{Symbol: req.Symbol, Side: Side(req.Side), Price: req.Price, Quantity: req.Size, ClientOrderIndex: index, Expiry: expiry, Type: req.OrderType, ReduceOnly: req.ReduceOnly, TriggerPrice: req.TriggerPrice})
	if err != nil {
		return execution.OrderResponse{}, err
	}
	tx, err := e.txClient.GetCreateOrderTransaction(txReq, nil)
	if err != nil {
		return execution.OrderResponse{}, err
	}
	orderID, err := e.send(tx)
	if err != nil {
		return execution.OrderResponse{}, err
	}
	e.mu.Lock()
	e.submitted[orderID] = submittedOrder{Symbol: req.Symbol, Index: index}
	e.mu.Unlock()
	if e.orderManager != nil {
		e.orderManager.Update(ws.OrderSnapshot{OrderID: orderID, ClientOrderID: fmt.Sprint(index), Symbol: req.Symbol, Status: ws.OrderSubmitted, Side: req.Side, Price: req.Price, Size: req.Size, Timestamp: time.Now().UTC()})
	}
	return execution.OrderResponse{OrderID: orderID, Status: "SUBMITTED", Mode: execution.Live}, nil
}

func (e *Executor) preflightAccount(symbol string) error {
	auth, err := e.txClient.GetAuthToken(time.Now().Add(7 * time.Hour))
	if err != nil {
		return fmt.Errorf("preflight auth: %w", err)
	}
	e.adapter.SetAuth(auth)
	account, err := e.adapter.GetAccount(e.accountIndex)
	if err != nil {
		return fmt.Errorf("preflight account reconciliation: %w", err)
	}
	open := 0
	for _, position := range account.Positions {
		if position.Size == 0 {
			continue
		}
		if position.Symbol == symbol {
			return fmt.Errorf("%s already has an open position", symbol)
		}
		if position.Symbol == "BTC" || position.Symbol == "ETH" {
			open++
		}
	}
	if open >= execution.DefaultRiskLimits().MaxOpenPositions {
		return fmt.Errorf("maximum live positions already open")
	}
	return nil
}

func (e *Executor) Cancel(orderID string) error {
	e.mu.RLock()
	submitted, ok := e.submitted[orderID]
	e.mu.RUnlock()
	if !ok {
		return fmt.Errorf("unknown locally submitted order %s", orderID)
	}
	req, err := BuildCancelOrder(submitted.Symbol, submitted.Index)
	if err != nil {
		return err
	}
	tx, err := e.txClient.GetCancelOrderTransaction(req, nil)
	if err != nil {
		return err
	}
	if _, err := e.send(tx); err != nil {
		return err
	}
	if e.orderManager != nil {
		e.orderManager.Update(ws.OrderSnapshot{OrderID: orderID, ClientOrderID: fmt.Sprint(submitted.Index), Symbol: submitted.Symbol, Status: ws.OrderCanceled, Timestamp: time.Now().UTC()})
	}
	return nil
}

func (e *Executor) GetPosition(symbol string) float64 { return 0 }

func (e *Executor) Close(symbol, side string, size, price float64) (execution.OrderResponse, error) {
	closeSide := Buy
	if side == "LONG" {
		closeSide = Sell
	}
	return e.Submit(execution.OrderRequest{Symbol: symbol, Side: string(closeSide), Size: size, Price: price, ReduceOnly: true, OrderType: lightertx.MarketOrder})
}

func (e *Executor) CancelIndexed(symbol string, index int64) error {
	req, err := BuildCancelOrder(symbol, index)
	if err != nil {
		return err
	}
	tx, err := e.txClient.GetCancelOrderTransaction(req, nil)
	if err != nil {
		return err
	}
	_, err = e.send(tx)
	return err
}

func (e *Executor) send(tx lightertx.TxInfo) (string, error) {
	auth, err := e.txClient.GetAuthToken(time.Now().Add(7 * time.Hour))
	if err != nil {
		return "", err
	}
	e.adapter.SetAuth(auth)
	txInfo, err := tx.GetTxInfo()
	if err != nil {
		return "", err
	}
	values := url.Values{}
	values.Set("tx_type", fmt.Sprintf("%d", tx.GetTxType()))
	values.Set("tx_info", txInfo)
	values.Set("price_protection", "false")
	responseHash, err := e.adapter.SendTx(values)
	if err != nil {
		return "", err
	}
	if responseHash != "" {
		return responseHash, nil
	}
	if signedHash := tx.GetTxHash(); signedHash != "" {
		return signedHash, nil
	}
	return "", fmt.Errorf("Lighter accepted transaction but no transaction hash was available")
}

// Compile-time guards for the SDK request types used by this executor.
var _ = lightertypes.CancelOrderTxReq{}
