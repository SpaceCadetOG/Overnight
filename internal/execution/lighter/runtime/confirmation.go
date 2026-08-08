package runtime

import (
	"fmt"

	wsruntime "github.com/ogtrading/overnight-strategy/internal/execution/lighter/ws/runtime"
)

func ConfirmFlat(
	positions *wsruntime.Sync,
	symbol string,
) error {

	position, ok := positions.Get(symbol)

	if !ok {
		return fmt.Errorf("position unavailable")
	}

	if position.Size != 0 {
		return fmt.Errorf(
			"still open: %s size %.8f",
			symbol,
			position.Size,
		)
	}

	return nil
}
