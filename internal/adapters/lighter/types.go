package lighter

type Order struct {
	OrderIndex          int64  `json:"order_index"`
	ClientOrderIndex    int64  `json:"client_order_index"`
	OrderID             string `json:"order_id"`
	MarketIndex         int16  `json:"market_index"`
	InitialBaseAmount   string `json:"initial_base_amount"`
	RemainingBaseAmount string `json:"remaining_base_amount"`
	Price               string `json:"price"`
	Type                string `json:"type"`
	TimeInForce         string `json:"time_in_force"`
	Status              string `json:"status"`
}

type OrdersResponse struct {
	Code       int     `json:"code"`
	Message    string  `json:"message"`
	NextCursor string  `json:"next_cursor"`
	Orders     []Order `json:"orders"`
}
