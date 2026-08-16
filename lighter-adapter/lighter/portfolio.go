package lighter

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"
)

type PortfolioSnapshot struct {
	AccountIndex        int64               `json:"account_index"`
	Collateral          string              `json:"collateral"`
	AvailableCollateral string              `json:"available_collateral"`
	GrossExposure       string              `json:"gross_exposure"`
	Positions           []CanonicalPosition `json:"positions"`
	ActiveOrders        []Order             `json:"active_orders"`
	IsFlat              bool                `json:"is_flat"`
	CapturedAt          int64               `json:"captured_at"`
	ExchangeTime        int64               `json:"exchange_time"`
}

type PortfolioManager struct {
	execution Execution
}

func NewPortfolioManager(execution Execution) (*PortfolioManager, error) {
	if execution == nil {
		return nil, fmt.Errorf("execution interface is required")
	}
	return &PortfolioManager{execution: execution}, nil
}

func absoluteDecimal(value string) (*big.Rat, error) {
	number, ok := decimal(value)
	if !ok {
		return nil, fmt.Errorf("invalid decimal %q", value)
	}
	return new(big.Rat).Abs(number), nil
}

func canonicalDecimal(value *big.Rat) string {
	if value == nil || value.Sign() == 0 {
		return "0"
	}
	return value.FloatString(18)
}

func orderRemainingExposure(order Order) (*big.Rat, error) {
	remaining, err := absoluteDecimal(order.RemainingBaseAmount)
	if err != nil {
		return nil, fmt.Errorf("order client_order_index=%d remaining amount: %w", order.ClientOrderIndex, err)
	}
	price, err := absoluteDecimal(order.Price)
	if err != nil {
		return nil, fmt.Errorf("order client_order_index=%d price: %w", order.ClientOrderIndex, err)
	}
	return new(big.Rat).Mul(remaining, price), nil
}

func (p *PortfolioManager) Snapshot(ctx context.Context) (*PortfolioSnapshot, error) {
	positions, err := p.execution.GetPositions(ctx)
	if err != nil {
		return nil, fmt.Errorf("portfolio positions: %w", err)
	}
	orders, err := p.execution.GetActiveOrders(ctx)
	if err != nil {
		return nil, fmt.Errorf("portfolio orders: %w", err)
	}

	gross := new(big.Rat)
	flat := true
	for _, position := range positions.Positions {
		value, err := absoluteDecimal(position.PositionValue)
		if err != nil {
			return nil, fmt.Errorf("position %s value: %w", position.Symbol, err)
		}
		gross.Add(gross, value)
		if !PositionIsFlat(position) {
			flat = false
		}
	}
	for _, order := range orders {
		value, err := orderRemainingExposure(order)
		if err != nil {
			return nil, err
		}
		gross.Add(gross, value)
	}
	if len(orders) != 0 {
		flat = false
	}

	return &PortfolioSnapshot{
		AccountIndex: positions.AccountIndex, Collateral: positions.Collateral,
		AvailableCollateral: positions.AvailableBalance, GrossExposure: canonicalDecimal(gross),
		Positions: positions.Positions, ActiveOrders: orders, IsFlat: flat,
		CapturedAt: time.Now().UnixMilli(), ExchangeTime: positions.TransactionTime,
	}, nil
}

func (p PortfolioSnapshot) Position(symbol string) (CanonicalPosition, bool) {
	wanted := normalizeSymbol(symbol)
	for _, position := range p.Positions {
		if normalizeSymbol(position.Symbol) == wanted {
			return position, true
		}
	}
	return CanonicalPosition{Symbol: wanted, Side: PositionSideFlat, Size: "0"}, false
}

func (p PortfolioSnapshot) SymbolExposure(symbol string, marketID int16) (*big.Rat, error) {
	wanted := normalizeSymbol(symbol)
	exposure := new(big.Rat)
	for _, position := range p.Positions {
		if normalizeSymbol(position.Symbol) != wanted {
			continue
		}
		value, err := absoluteDecimal(position.PositionValue)
		if err != nil {
			return nil, err
		}
		exposure.Add(exposure, value)
	}
	for _, order := range p.ActiveOrders {
		if order.MarketIndex != marketID {
			continue
		}
		value, err := orderRemainingExposure(order)
		if err != nil {
			return nil, err
		}
		exposure.Add(exposure, value)
	}
	return exposure, nil
}

type FlatVerification struct {
	Flat          bool     `json:"flat"`
	OpenPositions []string `json:"open_positions,omitempty"`
	ActiveOrders  []int64  `json:"active_orders,omitempty"`
}

func (p PortfolioSnapshot) VerifyFlat() FlatVerification {
	result := FlatVerification{Flat: true}
	for _, position := range p.Positions {
		if !PositionIsFlat(position) {
			result.Flat = false
			result.OpenPositions = append(result.OpenPositions, normalizeSymbol(position.Symbol))
		}
	}
	for _, order := range p.ActiveOrders {
		result.Flat = false
		result.ActiveOrders = append(result.ActiveOrders, order.ClientOrderIndex)
	}
	sort.Strings(result.OpenPositions)
	return result
}

func ratFromFloat(name string, value float64) (*big.Rat, error) {
	return parsePositiveRat(name, strconv.FormatFloat(value, 'f', -1, 64))
}

func normalizeConfiguredSymbol(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}
