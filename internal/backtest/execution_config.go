package backtest

import (
	"fmt"
	"strings"
)

type ExecutionMode string

const (
	ExecutionIdeal     ExecutionMode = "ideal"
	ExecutionRealistic ExecutionMode = "realistic"
)

type ExecutionConfig struct {
	Mode ExecutionMode

	EntrySlippageBps float64
	StopSlippageBps  float64
	TPSlippageBps    float64
	TimeSlippageBps  float64

	MakerFeeBps float64
	TakerFeeBps float64
}

func IdealExecutionConfig() ExecutionConfig {
	return ExecutionConfig{
		Mode: ExecutionIdeal,
	}
}

func RealisticExecutionConfig() ExecutionConfig {
	return ExecutionConfig{
		Mode: ExecutionRealistic,

		EntrySlippageBps: 1.0,
		StopSlippageBps:  3.0,
		TPSlippageBps:    1.0,
		TimeSlippageBps:  1.0,

		MakerFeeBps: 1.0,
		TakerFeeBps: 3.5,
	}
}

func ParseExecutionConfig(value string) (ExecutionConfig, error) {
	switch ExecutionMode(strings.ToLower(strings.TrimSpace(value))) {
	case ExecutionIdeal:
		return IdealExecutionConfig(), nil

	case ExecutionRealistic:
		return RealisticExecutionConfig(), nil

	default:
		return ExecutionConfig{}, fmt.Errorf(
			"unsupported execution mode %q: expected ideal or realistic",
			value,
		)
	}
}

func (config ExecutionConfig) Validate() error {
	values := map[string]float64{
		"entry slippage": config.EntrySlippageBps,
		"stop slippage":  config.StopSlippageBps,
		"TP slippage":    config.TPSlippageBps,
		"time slippage":  config.TimeSlippageBps,
		"maker fee":      config.MakerFeeBps,
		"taker fee":      config.TakerFeeBps,
	}

	for name, value := range values {
		if value < 0 {
			return fmt.Errorf("%s cannot be negative", name)
		}
	}

	return nil
}
