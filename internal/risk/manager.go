package risk

import "fmt"

type Manager struct {
	Config Config

	CurrentPositionUSD float64
	DailyLoss          float64
	TradesToday        int
}

func NewManager(cfg Config) *Manager {
	return &Manager{
		Config: cfg,
	}
}

func (r *Manager) Check(req OrderRequest) error {

	if req.USD > r.Config.MaxPositionUSD {
		return fmt.Errorf(
			"position size exceeds limit: %.2f > %.2f",
			req.USD,
			r.Config.MaxPositionUSD,
		)
	}

	if r.DailyLoss >= r.Config.MaxDailyLoss {
		return fmt.Errorf(
			"daily loss limit reached",
		)
	}

	if r.TradesToday >= r.Config.MaxTrades {
		return fmt.Errorf(
			"maximum trades reached",
		)
	}

	return nil
}
