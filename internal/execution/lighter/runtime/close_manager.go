package runtime

import (
	"fmt"

	lighter "github.com/ogtrading/overnight-strategy/internal/execution/lighter"
	wsruntime "github.com/ogtrading/overnight-strategy/internal/execution/lighter/ws/runtime"
)

type CloseManager struct {
	Executor  *lighter.Executor
	Positions *wsruntime.Sync
	Markets   *wsruntime.MarketManager
}

func (c *CloseManager) ClosePosition(symbol string) error {

	position, ok := c.Positions.Get(symbol)

	if !ok {
		return fmt.Errorf("no websocket position for %s", symbol)
	}

	if position.Size == 0 {
		return fmt.Errorf("%s already flat", symbol)
	}

	price, ok := c.Markets.Get(symbol)

	if !ok {
		return fmt.Errorf("no market price for %s", symbol)
	}

	_, err := c.Executor.Close(
		symbol,
		position.Side,
		position.Size,
		price,
	)

	return err
}
