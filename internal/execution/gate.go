package execution

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Mode string

const (
	Paper Mode = "PAPER_EXECUTION"
	Live  Mode = "LIVE_EXECUTION"
)

type Gate struct {
	Mode           Mode
	KillSwitch     bool
	AllowedSymbols map[string]bool
}

func GateFromEnvironment(mode Mode) Gate {
	return Gate{Mode: mode, KillSwitch: strings.EqualFold(strings.TrimSpace(os.Getenv("KILL_SWITCH")), "true"), AllowedSymbols: map[string]bool{"BTC": true, "ETH": true}}
}

func (g Gate) Authorize(symbol string, now time.Time) error {
	if g.KillSwitch {
		return fmt.Errorf("KILL_SWITCH=true blocks new orders")
	}
	if g.Mode == Paper {
		return nil
	}
	if g.Mode != Live {
		return fmt.Errorf("unknown execution mode %s", g.Mode)
	}
	if !g.AllowedSymbols[symbol] {
		return fmt.Errorf("%s is outside staged live rollout", symbol)
	}
	return nil
}
