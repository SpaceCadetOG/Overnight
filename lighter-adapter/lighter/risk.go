package lighter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var ErrRiskRejected = errors.New("risk rejected order")

type RiskConfig struct {
	AllowedSymbols         []string          `json:"allowed_symbols"`
	MaxOrderNotional       string            `json:"max_order_notional"`
	MaxPortfolioExposure   string            `json:"max_portfolio_exposure"`
	MaxSymbolExposure      map[string]string `json:"max_symbol_exposure"`
	MinAvailableCollateral string            `json:"min_available_collateral"`
	MaxDailyLoss           string            `json:"max_daily_loss"`
	MaxRiskFraction        string            `json:"max_risk_fraction"`
}

type RiskState struct {
	Version             int               `json:"version"`
	TradingDay          string            `json:"trading_day"`
	RealizedPnL         string            `json:"realized_pnl"`
	KillSwitch          bool              `json:"kill_switch"`
	KillReason          string            `json:"kill_reason,omitempty"`
	UpdatedAt           int64             `json:"updated_at"`
	ObservedRealizedPnL map[string]string `json:"observed_realized_pnl,omitempty"`
}

type RiskDecision struct {
	Approved          bool   `json:"approved"`
	Reason            string `json:"reason,omitempty"`
	OrderNotional     string `json:"order_notional"`
	ProjectedExposure string `json:"projected_exposure"`
	RiskAmount        string `json:"risk_amount,omitempty"`
	RiskBudget        string `json:"risk_budget,omitempty"`
}

type RiskManager struct {
	config    RiskConfig
	statePath string
	mu        sync.Mutex
	state     RiskState
}

func NewRiskManager(config RiskConfig, statePath string) (*RiskManager, error) {
	if strings.TrimSpace(statePath) == "" {
		return nil, errors.New("risk state path is required")
	}
	if len(config.AllowedSymbols) == 0 {
		return nil, errors.New("at least one allowed symbol is required")
	}
	for _, field := range []struct{ name, value string }{
		{"max order notional", config.MaxOrderNotional},
		{"max portfolio exposure", config.MaxPortfolioExposure},
		{"minimum available collateral", config.MinAvailableCollateral},
		{"max daily loss", config.MaxDailyLoss},
		{"max risk fraction", config.MaxRiskFraction},
	} {
		if _, err := parsePositiveRat(field.name, field.value); err != nil {
			return nil, err
		}
	}
	riskFraction, _ := parsePositiveRat("max risk fraction", config.MaxRiskFraction)
	if riskFraction.Cmp(big.NewRat(1, 1)) > 0 {
		return nil, errors.New("max risk fraction cannot exceed 1")
	}
	for symbol, limit := range config.MaxSymbolExposure {
		if normalizeConfiguredSymbol(symbol) == "" {
			return nil, errors.New("symbol exposure limit has empty symbol")
		}
		if _, err := parsePositiveRat("symbol exposure limit", limit); err != nil {
			return nil, err
		}
	}

	manager := &RiskManager{config: config, statePath: statePath}
	body, err := os.ReadFile(statePath)
	if errors.Is(err, os.ErrNotExist) {
		manager.state = RiskState{Version: 1, TradingDay: tradingDay(time.Now()), RealizedPnL: "0", ObservedRealizedPnL: make(map[string]string)}
		if err := manager.persistLocked(); err != nil {
			return nil, err
		}
		return manager, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read risk state: %w", err)
	}
	if err := json.Unmarshal(body, &manager.state); err != nil {
		return nil, fmt.Errorf("decode risk state: %w", err)
	}
	if manager.state.Version != 1 {
		return nil, fmt.Errorf("unsupported risk state version %d", manager.state.Version)
	}
	if manager.state.ObservedRealizedPnL == nil {
		manager.state.ObservedRealizedPnL = make(map[string]string)
	}
	return manager, nil
}

func tradingDay(now time.Time) string { return now.UTC().Format("2006-01-02") }

func (r *RiskManager) rolloverLocked(now time.Time) {
	day := tradingDay(now)
	if r.state.TradingDay == day {
		return
	}
	r.state.TradingDay = day
	r.state.RealizedPnL = "0"
	r.state.ObservedRealizedPnL = make(map[string]string)
	if r.state.KillReason == "daily loss limit reached" {
		r.state.KillSwitch = false
		r.state.KillReason = ""
	}
	// A manual kill switch remains engaged across day boundaries.
}

func (r *RiskManager) persistLocked() error {
	r.state.UpdatedAt = time.Now().UnixMilli()
	body, err := json.MarshalIndent(r.state, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(r.statePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, ".risk-*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, r.statePath)
}

func (r *RiskManager) State() RiskState {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rolloverLocked(time.Now())
	return r.state
}

func (r *RiskManager) EngageKillSwitch(reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state.KillSwitch = true
	r.state.KillReason = strings.TrimSpace(reason)
	if r.state.KillReason == "" {
		r.state.KillReason = "manual kill switch"
	}
	return r.persistLocked()
}

func (r *RiskManager) ResetKillSwitch() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rolloverLocked(time.Now())
	if r.state.KillReason == "daily loss limit reached" {
		return errors.New("daily loss kill switch cannot be reset during the same trading day")
	}
	r.state.KillSwitch = false
	r.state.KillReason = ""
	return r.persistLocked()
}

func (r *RiskManager) RecordRealizedPnL(change string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rolloverLocked(time.Now())
	current, ok := decimal(r.state.RealizedPnL)
	if !ok {
		return errors.New("persisted realized PnL is invalid")
	}
	delta, ok := decimal(change)
	if !ok {
		return fmt.Errorf("invalid realized PnL change %q", change)
	}
	current.Add(current, delta)
	r.state.RealizedPnL = canonicalDecimal(current)
	maxLoss, _ := parsePositiveRat("max daily loss", r.config.MaxDailyLoss)
	if current.Sign() < 0 && new(big.Rat).Abs(current).Cmp(maxLoss) >= 0 {
		r.state.KillSwitch = true
		r.state.KillReason = "daily loss limit reached"
	}
	return r.persistLocked()
}

// ObservePositions accounts only for changes from the last exchange-authoritative
// realized-PnL values. The first observation establishes a durable baseline.
func (r *RiskManager) ObservePositions(snapshot *PositionSnapshot) error {
	if snapshot == nil {
		return errors.New("position snapshot is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rolloverLocked(time.Now())
	if r.state.ObservedRealizedPnL == nil {
		r.state.ObservedRealizedPnL = make(map[string]string)
	}
	total, ok := decimal(r.state.RealizedPnL)
	if !ok {
		return errors.New("persisted realized PnL is invalid")
	}
	for _, position := range snapshot.Positions {
		symbol := normalizeConfiguredSymbol(position.Symbol)
		current, ok := decimal(position.RealizedPnL)
		if !ok {
			return fmt.Errorf("invalid %s realized PnL %q", symbol, position.RealizedPnL)
		}
		previousText, seen := r.state.ObservedRealizedPnL[symbol]
		if seen {
			previous, valid := decimal(previousText)
			if !valid {
				return fmt.Errorf("persisted %s realized PnL is invalid", symbol)
			}
			total.Add(total, new(big.Rat).Sub(current, previous))
		}
		r.state.ObservedRealizedPnL[symbol] = canonicalDecimal(current)
	}
	r.state.RealizedPnL = canonicalDecimal(total)
	maxLoss, _ := parsePositiveRat("max daily loss", r.config.MaxDailyLoss)
	if total.Sign() < 0 && new(big.Rat).Abs(total).Cmp(maxLoss) >= 0 {
		r.state.KillSwitch = true
		r.state.KillReason = "daily loss limit reached"
	}
	return r.persistLocked()
}

func containsSymbol(symbols []string, wanted string) bool {
	for _, symbol := range symbols {
		if normalizeConfiguredSymbol(symbol) == wanted {
			return true
		}
	}
	return false
}

func riskRejection(reason string) (*RiskDecision, error) {
	return &RiskDecision{Approved: false, Reason: reason}, fmt.Errorf("%w: %s", ErrRiskRejected, reason)
}

func (r *RiskManager) ValidateOrder(ctx context.Context, manager *Manager, portfolio PortfolioSnapshot, request PlaceOrderRequest) (*RiskDecision, error) {
	r.mu.Lock()
	r.rolloverLocked(time.Now())
	killSwitch := r.state.KillSwitch
	killReason := r.state.KillReason
	r.mu.Unlock()
	if killSwitch && !request.ReduceOnly {
		return riskRejection("kill switch engaged: " + killReason)
	}
	symbol := normalizeConfiguredSymbol(request.Symbol)
	if !containsSymbol(r.config.AllowedSymbols, symbol) {
		return riskRejection("symbol is not funded-enabled")
	}
	market, err := manager.MarketBySymbol(ctx, symbol)
	if err != nil {
		return nil, err
	}
	encoded, err := market.EncodeOrder(request.Quantity, request.Price)
	if err != nil {
		return riskRejection(err.Error())
	}
	orderNotional, err := ratFromFloat("order notional", encoded.Notional)
	if err != nil {
		return nil, err
	}
	if request.ReduceOnly {
		position, exists := portfolio.Position(symbol)
		if !exists || PositionIsFlat(position) {
			return riskRejection("reduce-only order has no open position")
		}
		if (position.Side == PositionSideLong && request.Side != SideSell) || (position.Side == PositionSideShort && request.Side != SideBuy) {
			return riskRejection("reduce-only side would increase exposure")
		}
		quantity, _ := ratFromFloat("quantity", request.Quantity)
		positionSize, ok := decimal(position.Size)
		if !ok || quantity.Cmp(positionSize) > 0 {
			return riskRejection("reduce-only quantity exceeds position")
		}
		return &RiskDecision{Approved: true, OrderNotional: canonicalDecimal(orderNotional), ProjectedExposure: portfolio.GrossExposure}, nil
	}
	if request.StopPrice <= 0 {
		return riskRejection("entry stop price is required")
	}
	if (request.Side == SideBuy && request.StopPrice >= request.Price) || (request.Side == SideSell && request.StopPrice <= request.Price) {
		return riskRejection("entry stop price is on the wrong side")
	}
	stop, err := ratFromFloat("stop price", request.StopPrice)
	if err != nil {
		return nil, err
	}
	entry, err := ratFromFloat("entry price", request.Price)
	if err != nil {
		return nil, err
	}
	distance := new(big.Rat).Sub(entry, stop)
	distance.Abs(distance)
	quantity, err := ratFromFloat("quantity", request.Quantity)
	if err != nil {
		return nil, err
	}
	riskAmount := new(big.Rat).Mul(distance, quantity)
	equity, ok := decimal(portfolio.Collateral)
	if !ok || equity.Sign() <= 0 {
		return nil, errors.New("portfolio collateral is invalid")
	}
	riskFraction, _ := parsePositiveRat("max risk fraction", r.config.MaxRiskFraction)
	riskBudget := new(big.Rat).Mul(equity, riskFraction)
	if riskAmount.Cmp(riskBudget) > 0 {
		return riskRejection("order exceeds maximum risk fraction")
	}
	maxOrder, _ := parsePositiveRat("max order notional", r.config.MaxOrderNotional)
	if orderNotional.Cmp(maxOrder) > 0 {
		return riskRejection("order exceeds maximum notional")
	}
	available, ok := decimal(portfolio.AvailableCollateral)
	if !ok {
		return nil, errors.New("portfolio available collateral is invalid")
	}
	minimumAvailable, _ := parsePositiveRat("minimum available collateral", r.config.MinAvailableCollateral)
	if available.Cmp(minimumAvailable) < 0 || available.Cmp(orderNotional) < 0 {
		return riskRejection("insufficient available collateral")
	}

	gross, ok := decimal(portfolio.GrossExposure)
	if !ok {
		return nil, errors.New("portfolio gross exposure is invalid")
	}
	projected := new(big.Rat).Add(gross, orderNotional)
	maxPortfolio, _ := parsePositiveRat("max portfolio exposure", r.config.MaxPortfolioExposure)
	if projected.Cmp(maxPortfolio) > 0 {
		return riskRejection("order exceeds maximum portfolio exposure")
	}
	symbolExposure, err := portfolio.SymbolExposure(symbol, market.MarketID)
	if err != nil {
		return nil, err
	}
	projectedSymbol := new(big.Rat).Add(symbolExposure, orderNotional)
	for configuredSymbol, value := range r.config.MaxSymbolExposure {
		if normalizeConfiguredSymbol(configuredSymbol) != symbol {
			continue
		}
		limit, _ := parsePositiveRat("symbol exposure limit", value)
		if projectedSymbol.Cmp(limit) > 0 {
			return riskRejection("order exceeds symbol exposure limit")
		}
	}
	return &RiskDecision{
		Approved: true, OrderNotional: canonicalDecimal(orderNotional), ProjectedExposure: canonicalDecimal(projected),
		RiskAmount: canonicalDecimal(riskAmount), RiskBudget: canonicalDecimal(riskBudget),
	}, nil
}

func (r *RiskManager) SizeForRisk(equity, entryPrice, stopPrice string) (string, error) {
	equityValue, err := parsePositiveRat("equity", equity)
	if err != nil {
		return "", err
	}
	entry, err := parsePositiveRat("entry price", entryPrice)
	if err != nil {
		return "", err
	}
	stop, err := parsePositiveRat("stop price", stopPrice)
	if err != nil {
		return "", err
	}
	distance := new(big.Rat).Sub(entry, stop)
	distance.Abs(distance)
	if distance.Sign() == 0 {
		return "", errors.New("entry and stop price cannot be equal")
	}
	fraction, _ := parsePositiveRat("max risk fraction", r.config.MaxRiskFraction)
	if fraction.Cmp(big.NewRat(1, 1)) > 0 {
		return "", errors.New("max risk fraction cannot exceed 1")
	}
	riskBudget := new(big.Rat).Mul(equityValue, fraction)
	return canonicalDecimal(new(big.Rat).Quo(riskBudget, distance)), nil
}

type RiskManagedExecution struct {
	engine    *ExecutionEngine
	portfolio *PortfolioManager
	risk      *RiskManager
}

func NewRiskManagedExecution(engine *ExecutionEngine, portfolio *PortfolioManager, risk *RiskManager) (*RiskManagedExecution, error) {
	if engine == nil || portfolio == nil || risk == nil {
		return nil, errors.New("engine, portfolio, and risk manager are required")
	}
	return &RiskManagedExecution{engine: engine, portfolio: portfolio, risk: risk}, nil
}

func (e *RiskManagedExecution) PlaceOrder(ctx context.Context, request PlaceOrderRequest) (*OrderSubmission, error) {
	snapshot, err := e.portfolio.Snapshot(ctx)
	if err != nil {
		return nil, classifyExecutionError("risk portfolio", err)
	}
	if _, err := e.risk.ValidateOrder(ctx, e.engine.manager, *snapshot, request); err != nil {
		return nil, classifyExecutionError("risk validation", err)
	}
	return e.engine.PlaceOrder(ctx, request)
}

func (e *RiskManagedExecution) CancelOrder(ctx context.Context, id int64) error {
	return e.engine.CancelOrder(ctx, id)
}
func (e *RiskManagedExecution) CancelAll(ctx context.Context) error { return e.engine.CancelAll(ctx) }
func (e *RiskManagedExecution) GetActiveOrders(ctx context.Context) ([]Order, error) {
	return e.engine.GetActiveOrders(ctx)
}
func (e *RiskManagedExecution) GetOrderStatus(ctx context.Context, id int64) (*ReconciledOrder, error) {
	return e.engine.GetOrderStatus(ctx, id)
}
func (e *RiskManagedExecution) GetPositions(ctx context.Context) (*PositionSnapshot, error) {
	snapshot, err := e.engine.GetPositions(ctx)
	if err == nil {
		err = e.risk.ObservePositions(snapshot)
	}
	return snapshot, err
}
func (e *RiskManagedExecution) Reconcile(ctx context.Context) (*RecoveryReport, error) {
	report, err := e.engine.Reconcile(ctx)
	if err == nil && report != nil {
		err = e.risk.ObservePositions(report.Positions)
	}
	return report, err
}

var _ Execution = (*RiskManagedExecution)(nil)
