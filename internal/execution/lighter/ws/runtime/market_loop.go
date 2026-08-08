package runtime

import (
	"time"
)

func (m *MarketFeed) Run() error {

	for {

		err := m.ReadOnce()

		if err != nil {
			return err
		}

		time.Sleep(
			10 * time.Millisecond,
		)
	}
}
