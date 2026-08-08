package execution

import (
	"fmt"

	"github.com/ogtrading/overnight-strategy/internal/universe"
)

type Router struct {
	live  Executor
	paper Executor
}

func NewRouter(live Executor, paper Executor) *Router {
	return &Router{
		live:  live,
		paper: paper,
	}
}

func (r *Router) Executor(symbol string) (Executor, Mode, error) {

	destination, err := universe.Resolve(symbol)
	if err != nil {
		return nil, "", fmt.Errorf("unsupported symbol %s: %w", symbol, err)
	}
	if destination == universe.LiveExecutor {
		return r.live, Live, nil
	}
	return r.paper, Paper, nil
}
