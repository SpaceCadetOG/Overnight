package runtime

import (
	"time"

	adapter "github.com/ogtrading/overnight-strategy/internal/adapters/lighter"
	"github.com/ogtrading/overnight-strategy/internal/execution/lighter/ws/runtime"
	"github.com/ogtrading/overnight-strategy/internal/reconcile"
)

type Coordinator struct {
	Client *adapter.Client

	AccountIndex int64

	Orders *runtime.OrderManager

	LastSync time.Time
}

func NewCoordinator(
	client *adapter.Client,
	accountIndex int64,
	orders *runtime.OrderManager,
) *Coordinator {

	return &Coordinator{
		Client:       client,
		AccountIndex: accountIndex,
		Orders:       orders,
	}
}

func (c *Coordinator) Reconcile() (*reconcile.Snapshot, error) {

	snapshot, err := reconcile.Build(
		c.Client,
		c.AccountIndex,
	)

	if err != nil {
		return nil, err
	}

	c.LastSync = time.Now().UTC()

	return snapshot, nil
}
