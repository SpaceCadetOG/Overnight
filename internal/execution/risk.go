package execution

import "fmt"

type RiskLimits struct {
	PerTradePercent  float64
	MaxBasketPercent float64
	MaxOpenPositions int
}

func DefaultRiskLimits() RiskLimits {
	return RiskLimits{PerTradePercent: 0.5, MaxBasketPercent: 2.0, MaxOpenPositions: 2}
}

func (r RiskLimits) Budget(equity float64, requested int) (perTrade, basket float64, err error) {
	if equity <= 0 {
		return 0, 0, fmt.Errorf("positive account equity is required")
	}
	if requested <= 0 || requested > r.MaxOpenPositions {
		return 0, 0, fmt.Errorf("requested positions %d exceeds limit %d", requested, r.MaxOpenPositions)
	}
	perTrade = equity * r.PerTradePercent / 100
	basket = equity * r.MaxBasketPercent / 100
	if perTrade*float64(requested) > basket {
		perTrade = basket / float64(requested)
	}
	return perTrade, basket, nil
}
