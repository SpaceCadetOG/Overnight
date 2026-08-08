package ws

import (
	"encoding/json"
	"strconv"
	"time"
)

type OrderState string

const (
	OrderCreated   OrderState = "CREATED"
	OrderSubmitted OrderState = "SUBMITTED"
	OrderOpen      OrderState = "OPEN"
	OrderPartial   OrderState = "PARTIAL"
	OrderFilled    OrderState = "FILLED"
	OrderCanceled  OrderState = "CANCELED"
	OrderRejected  OrderState = "REJECTED"
)

type OrderSnapshot struct {
	OrderID       string
	ClientOrderID string
	Symbol        string
	Status        OrderState
	Side          string
	Price         float64
	Size          float64
	FilledSize    float64
	Timestamp     time.Time
}

type orderEvent struct {
	Channel string                  `json:"channel"`
	Orders  map[string]orderPayload `json:"orders"`
	Type    string                  `json:"type"`
}

type orderPayload struct {
	OrderID string `json:"order_id"`

	ClientOrderID string `json:"client_order_index"`

	Symbol string `json:"symbol"`

	Status string `json:"status"`

	IsAsk int `json:"is_ask"`

	Price string `json:"price"`

	InitialBaseAmount string `json:"initial_base_amount"`

	FilledBaseAmount string `json:"filled_base_amount"`
}

func ParseOrderEvent(
	data []byte,
) ([]OrderSnapshot, error) {

	var event orderEvent

	err := json.Unmarshal(
		data,
		&event,
	)

	if err != nil {
		return nil, err
	}

	out := make([]OrderSnapshot, 0)

	for _, o := range event.Orders {

		price, _ := strconv.ParseFloat(
			o.Price,
			64,
		)

		size, _ := strconv.ParseFloat(
			o.InitialBaseAmount,
			64,
		)

		filled, _ := strconv.ParseFloat(
			o.FilledBaseAmount,
			64,
		)

		side := "BUY"

		if o.IsAsk == 1 {
			side = "SELL"
		}

		out = append(
			out,
			OrderSnapshot{

				OrderID: o.OrderID,

				ClientOrderID: o.ClientOrderID,

				Symbol: o.Symbol,

				Status: OrderState(o.Status),

				Side: side,

				Price: price,

				Size: size,

				FilledSize: filled,

				Timestamp: time.Now().UTC(),
			},
		)
	}

	return out, nil
}
