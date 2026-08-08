package runtime

import (
	"github.com/ogtrading/overnight-strategy/internal/execution/lighter/ws"
)

type MarketFeed struct {
	Client  *ws.Client
	Manager *MarketManager
}

func NewMarketFeed(
	client *ws.Client,
	manager *MarketManager,
) *MarketFeed {

	return &MarketFeed{
		Client:  client,
		Manager: manager,
	}
}

func (m *MarketFeed) Subscribe(
	symbol string,
) error {

	return m.Client.Send(
		ws.MarketStats(symbol),
	)
}

func (m *MarketFeed) ReadOnce() error {

	data, err := m.Client.Read()

	if err != nil {
		return err
	}

	price, err := ws.ParseMarketPrice(data)

	if err != nil {
		return err
	}

	m.Manager.Update(*price)

	return nil
}
