package execution

import "time"

type OrderRequest struct {
	IntentKey        string
	Symbol           string
	Side             string
	Price            float64
	Size             float64
	ExpiresAt        time.Time
	ReduceOnly       bool
	OrderType        uint8
	TriggerPrice     float64
	StopPrice        float64
	ClientOrderIndex int64
	RiskUSD          float64
	RiskLimitUSD     float64
}

type OrderResponse struct {
	OrderID            string
	Status             string
	Mode               Mode
	ClientOrderIndex   int64
	ExchangeOrderIndex int64
	MarketID           int16
	Nonce              int64
	EncodedBaseAmount  int64
	EncodedPrice       uint32
	RequestedQuantity  float64
	RequestedPrice     float64
}

type Executor interface {
	Submit(OrderRequest) (OrderResponse, error)
	Cancel(orderID string) error
	GetPosition(symbol string) float64

	Close(
		symbol string,
		side string,
		size float64,
		price float64,
	) (OrderResponse, error)
}
