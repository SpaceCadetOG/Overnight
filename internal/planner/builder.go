package planner

import "fmt"

type MarketMap struct {
	Symbol string

	High float64
	Low  float64

	Fib382 float64
	Fib500 float64
	Fib618 float64

	Side Side
}

func Build(m MarketMap) (TradePlan, error) {

	if m.Symbol == "" {
		return TradePlan{}, fmt.Errorf("missing symbol")
	}

	if m.Fib382 == 0 || m.Fib500 == 0 || m.Fib618 == 0 {
		return TradePlan{}, fmt.Errorf("missing fibonacci levels")
	}

	plan := TradePlan{
		Symbol: m.Symbol,
		Side:   m.Side,
		Valid:  true,
	}

	// Frozen baseline:
	// Entry = midpoint Fib382/Fib500
	plan.Entry = (m.Fib382 + m.Fib500) / 2

	// TP1 = Fib618
	plan.TP1 = m.Fib618

	if m.Side == Long {

		// PROFILE_FIB stop placeholder.
		// Actual profile stop already validated elsewhere.
		plan.Stop = m.Fib382

		// TP2 overnight high
		plan.TP2 = m.High

	}

	if m.Side == Short {

		plan.Stop = m.Fib382

		// TP2 overnight low
		plan.TP2 = m.Low

	}

	return plan, nil
}
