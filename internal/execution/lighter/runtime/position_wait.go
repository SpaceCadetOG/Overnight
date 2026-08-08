package runtime

import (
	"fmt"
	"time"

	wsruntime "github.com/ogtrading/overnight-strategy/internal/execution/lighter/ws/runtime"
)

func WaitForFlat(
	sync *wsruntime.Sync,
	symbol string,
	timeout time.Duration,
) error {

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {

		position, ok := sync.Get(symbol)

		if ok && position.Size == 0 {
			return nil
		}

		time.Sleep(
			250 * time.Millisecond,
		)
	}

	return fmt.Errorf(
		"timeout waiting for %s flat",
		symbol,
	)
}
