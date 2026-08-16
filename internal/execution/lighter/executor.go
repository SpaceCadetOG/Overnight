package lighter

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	lightertx "github.com/elliottech/lighter-go/types/txtypes"
	adapter "github.com/ogtrading/lighter-adapter/lighter"
	"github.com/ogtrading/overnight-strategy/internal/execution"
)

type Config struct {
	BaseURL      string
	WSURL        string
	PrivateKey   string
	AccountIndex int64
	APIKeyIndex  uint8
	ChainID      uint32
	StateRoot    string
	Risk         adapter.RiskConfig
}

type Executor struct {
	manager   *adapter.Manager
	execution *adapter.RiskManagedExecution
	store     *adapter.RecoveryStore
	stream    *adapter.PrivateStream
}

func NewExecutor(ctx context.Context, cfg Config) (*Executor, error) {
	manager, err := adapter.NewManager(adapter.Config{BaseURL: cfg.BaseURL, ChainID: cfg.ChainID, AccountIndex: cfg.AccountIndex, APIKeyIndex: cfg.APIKeyIndex, PrivateKey: cfg.PrivateKey})
	if err != nil {
		return nil, err
	}
	stateRoot := strings.TrimSpace(cfg.StateRoot)
	if stateRoot == "" {
		return nil, errors.New("Lighter execution state root is required")
	}
	store, err := adapter.OpenRecoveryStore(filepath.Join(stateRoot, "lighter-recovery.json"), cfg.AccountIndex, cfg.APIKeyIndex)
	if err != nil {
		return nil, err
	}
	engine, _, err := adapter.RecoverExecutionEngine(ctx, manager, store)
	if err != nil {
		return nil, err
	}
	portfolio, err := adapter.NewPortfolioManager(engine)
	if err != nil {
		return nil, err
	}
	risk, err := adapter.NewRiskManager(cfg.Risk, filepath.Join(stateRoot, "lighter-risk.json"))
	if err != nil {
		return nil, err
	}
	riskExecution, err := adapter.NewRiskManagedExecution(engine, portfolio, risk)
	if err != nil {
		return nil, err
	}
	stream, err := adapter.NewPrivateStream(manager, riskExecution, adapter.PrivateStreamConfig{URL: cfg.WSURL})
	if err != nil {
		return nil, err
	}
	return &Executor{manager: manager, execution: riskExecution, store: store, stream: stream}, nil
}

func (e *Executor) StartPrivateStream(ctx context.Context) {
	go func() {
		_ = e.stream.Run(ctx)
	}()
}

func (e *Executor) ValidateProtection(ctx context.Context, symbol string, quantity, stop, tp1Quantity, tp1, runnerQuantity, tp2 float64) error {
	market, err := e.manager.MarketBySymbol(ctx, symbol)
	if err != nil {
		return err
	}
	for _, order := range []struct {
		name     string
		quantity float64
		price    float64
	}{{"stop", quantity, stop}, {"TP1", tp1Quantity, tp1}, {"TP2", runnerQuantity, tp2}} {
		if _, err := market.EncodeOrder(order.quantity, order.price); err != nil {
			minimum, minErr := market.MinimumQuantityAtPrices(order.price)
			if minErr != nil {
				return fmt.Errorf("%s protection invalid: %w", order.name, err)
			}
			return fmt.Errorf("%s protection quantity %.8f is not executable (minimum %.8f): %w", order.name, order.quantity, minimum, err)
		}
	}
	return nil
}

func translateRequest(req execution.OrderRequest) (adapter.PlaceOrderRequest, error) {
	if strings.TrimSpace(req.IntentKey) == "" {
		return adapter.PlaceOrderRequest{}, errors.New("deterministic intent key is required")
	}
	side := adapter.Side(strings.ToUpper(req.Side))
	if side != adapter.SideBuy && side != adapter.SideSell {
		return adapter.PlaceOrderRequest{}, fmt.Errorf("invalid side %q", req.Side)
	}
	result := adapter.PlaceOrderRequest{
		IntentKey: req.IntentKey, ClientOrderIndex: req.ClientOrderIndex, Symbol: req.Symbol, Side: side,
		Quantity: req.Size, Price: req.Price, ReduceOnly: req.ReduceOnly, ExpiresAt: req.ExpiresAt,
		TimeInForce: adapter.TimeInForceGoodTill, StopPrice: req.StopPrice,
	}
	switch req.OrderType {
	case lightertx.LimitOrder:
		result.Type = adapter.ExecutionOrderLimit
	case lightertx.MarketOrder:
		result.Type, result.TimeInForce = adapter.ExecutionOrderMarket, adapter.TimeInForceIOC
	case lightertx.StopLossOrder:
		result.Type, result.TimeInForce, result.TriggerPrice = adapter.ExecutionOrderStopLoss, adapter.TimeInForceIOC, req.TriggerPrice
	case lightertx.TakeProfitOrder:
		result.Type, result.TimeInForce, result.TriggerPrice = adapter.ExecutionOrderTakeProfit, adapter.TimeInForceIOC, req.TriggerPrice
	case lightertx.StopLossLimitOrder:
		result.Type, result.TriggerPrice = adapter.ExecutionOrderStopLossLimit, req.TriggerPrice
	case lightertx.TakeProfitLimitOrder:
		result.Type, result.TriggerPrice = adapter.ExecutionOrderTakeProfitLimit, req.TriggerPrice
	default:
		return adapter.PlaceOrderRequest{}, fmt.Errorf("unsupported Lighter order type %d", req.OrderType)
	}
	return result, nil
}

func (e *Executor) Submit(req execution.OrderRequest) (execution.OrderResponse, error) {
	if !req.ReduceOnly {
		if err := execution.GateFromEnvironment(execution.Live).Authorize(req.Symbol, time.Now()); err != nil {
			return execution.OrderResponse{}, err
		}
		location, err := time.LoadLocation("America/Chicago")
		if err != nil {
			return execution.OrderResponse{}, err
		}
		now := time.Now()
		if !execution.WithinOrderWindow(now, now.In(location), location) {
			return execution.OrderResponse{}, fmt.Errorf("new %s entry outside 05:00-05:05 CT order window", req.Symbol)
		}
		if req.RiskUSD <= 0 || req.RiskLimitUSD <= 0 || req.RiskUSD > req.RiskLimitUSD+1e-9 {
			return execution.OrderResponse{}, fmt.Errorf("entry risk %.2f exceeds or omits limit %.2f", req.RiskUSD, req.RiskLimitUSD)
		}
	}
	request, err := translateRequest(req)
	if err != nil {
		return execution.OrderResponse{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	submission, err := e.execution.PlaceOrder(ctx, request)
	if err != nil {
		return execution.OrderResponse{}, err
	}
	return execution.OrderResponse{OrderID: submission.TxHash, Status: "SUBMITTED", Mode: execution.Live}, nil
}

func (e *Executor) SubmitControlledTest(req execution.OrderRequest) (execution.OrderResponse, error) {
	// Controlled tests retain every production gate except the entry-time window.
	if !req.ReduceOnly {
		if err := execution.GateFromEnvironment(execution.Live).Authorize(req.Symbol, time.Now()); err != nil {
			return execution.OrderResponse{}, err
		}
		if req.RiskUSD <= 0 || req.RiskLimitUSD <= 0 || req.RiskUSD > req.RiskLimitUSD+1e-9 {
			return execution.OrderResponse{}, fmt.Errorf("entry risk %.2f exceeds or omits limit %.2f", req.RiskUSD, req.RiskLimitUSD)
		}
	}
	request, err := translateRequest(req)
	if err != nil {
		return execution.OrderResponse{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	submission, err := e.execution.PlaceOrder(ctx, request)
	if err != nil {
		return execution.OrderResponse{}, err
	}
	return execution.OrderResponse{OrderID: submission.TxHash, Status: "SUBMITTED", Mode: execution.Live}, nil
}

func (e *Executor) Cancel(orderID string) error {
	for _, mapping := range e.store.Snapshot().Orders {
		if mapping.TxHash == orderID {
			return e.CancelIndexed(mapping.Symbol, mapping.ClientOrderIndex)
		}
	}
	return fmt.Errorf("unknown persisted order %s", orderID)
}

func (e *Executor) CancelIndexed(_ string, index int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return e.execution.CancelOrder(ctx, index)
}

func (e *Executor) GetPosition(symbol string) float64 {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	snapshot, err := e.execution.GetPositions(ctx)
	if err != nil {
		return 0
	}
	position, ok := snapshot.Position(symbol)
	if !ok {
		return 0
	}
	size, _ := strconv.ParseFloat(position.Size, 64)
	if position.Side == adapter.PositionSideShort {
		return -size
	}
	return size
}

func (e *Executor) Close(symbol, side string, size, price float64) (execution.OrderResponse, error) {
	index, err := execution.ClientOrderIndex(fmt.Sprintf("manual:%s:%s:%d", symbol, side, time.Now().UnixNano()))
	if err != nil {
		return execution.OrderResponse{}, err
	}
	return e.CloseIndexed(symbol, side, size, price, index, fmt.Sprintf("manual:%d", index))
}

func (e *Executor) CloseIndexed(symbol, side string, size, price float64, index int64, intentKey string) (execution.OrderResponse, error) {
	closeSide := "BUY"
	if side == "LONG" {
		closeSide = "SELL"
	}
	return e.Submit(execution.OrderRequest{IntentKey: intentKey, Symbol: symbol, Side: closeSide, Size: size, Price: price, ReduceOnly: true, OrderType: lightertx.MarketOrder, ClientOrderIndex: index})
}

var _ execution.Executor = (*Executor)(nil)
