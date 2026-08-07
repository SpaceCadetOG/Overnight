package live

import "fmt"

type Reconciliation struct {
	ExpectedActive []string `json:"expected_active"`
	ExchangeActive []string `json:"exchange_active"`
	Missing        []string `json:"missing"`
	Unexpected     []string `json:"unexpected"`
	Matched        bool     `json:"matched"`
}

func Reconcile(intents []Intent, exchangeSymbols []string) Reconciliation {
	expectedSet, exchangeSet := map[string]bool{}, map[string]bool{}
	for _, intent := range intents {
		switch intent.State {
		case Submitted, Open, Partial, Filled, TP1, Runner:
			expectedSet[intent.Symbol] = true
		}
	}
	for _, symbol := range exchangeSymbols {
		if symbol != "" {
			exchangeSet[symbol] = true
		}
	}
	result := Reconciliation{}
	for symbol := range expectedSet {
		result.ExpectedActive = append(result.ExpectedActive, symbol)
		if !exchangeSet[symbol] {
			result.Missing = append(result.Missing, symbol)
		}
	}
	for symbol := range exchangeSet {
		result.ExchangeActive = append(result.ExchangeActive, symbol)
		if !expectedSet[symbol] {
			result.Unexpected = append(result.Unexpected, symbol)
		}
	}
	result.Matched = len(result.Missing) == 0 && len(result.Unexpected) == 0
	return result
}

func (r Reconciliation) Validate() error {
	if !r.Matched {
		return fmt.Errorf("reconciliation mismatch: missing=%v unexpected=%v", r.Missing, r.Unexpected)
	}
	return nil
}
