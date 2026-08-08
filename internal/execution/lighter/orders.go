package lighter

import (
	"fmt"
	"math"
	"time"

	lightertypes "github.com/elliottech/lighter-go/types"
	lightertx "github.com/elliottech/lighter-go/types/txtypes"
)

type Side string

const (
	Buy  Side = "BUY"
	Sell Side = "SELL"
)

type Market struct {
	Symbol          string
	Index           int16
	PriceDecimals   int
	SizeDecimals    int
	MinimumBase     float64
	MinimumNotional float64
}

var markets = map[string]Market{
	"BTC": {Symbol: "BTC", Index: 1, PriceDecimals: 1, SizeDecimals: 5, MinimumBase: .0001, MinimumNotional: 10},
	"ETH": {Symbol: "ETH", Index: 0, PriceDecimals: 2, SizeDecimals: 4, MinimumBase: .005, MinimumNotional: 10},
}

type OrderRequest struct {
	Symbol           string
	Side             Side
	Price            float64
	Quantity         float64
	ClientOrderIndex int64
	Expiry           time.Time
	Type             uint8
	ReduceOnly       bool
	TriggerPrice     float64
}

func MarketFor(symbol string) (Market, error) {
	market, ok := markets[symbol]
	if !ok {
		return Market{}, fmt.Errorf("unsupported live market %s", symbol)
	}
	return market, nil
}

func BuildCreateOrder(order OrderRequest) (*lightertypes.CreateOrderTxReq, error) {
	market, err := MarketFor(order.Symbol)
	if err != nil {
		return nil, err
	}
	if order.Side != Buy && order.Side != Sell {
		return nil, fmt.Errorf("invalid side %s", order.Side)
	}
	if order.Price <= 0 || order.Quantity <= 0 {
		return nil, fmt.Errorf("price and quantity must be positive")
	}
	if order.Quantity+1e-12 < market.MinimumBase {
		return nil, fmt.Errorf("%s quantity %.8f below minimum %.8f", order.Symbol, order.Quantity, market.MinimumBase)
	}
	if order.Price*order.Quantity+1e-9 < market.MinimumNotional {
		return nil, fmt.Errorf("%s notional %.2f below minimum %.2f", order.Symbol, order.Price*order.Quantity, market.MinimumNotional)
	}
	if order.ClientOrderIndex <= 0 {
		return nil, fmt.Errorf("positive client order index is required")
	}
	if order.Expiry.IsZero() && order.Type != lightertx.MarketOrder {
		return nil, fmt.Errorf("order expiry is required")
	}
	tif := uint8(lightertx.GoodTillTime)
	if order.Type == lightertx.MarketOrder || order.Type == lightertx.StopLossOrder || order.Type == lightertx.TakeProfitOrder {
		tif = lightertx.ImmediateOrCancel
	}
	reduceOnly := uint8(0)
	if order.ReduceOnly {
		reduceOnly = 1
	}
	isAsk := uint8(0)
	if order.Side == Sell {
		isAsk = 1
	}
	tx := &lightertypes.CreateOrderTxReq{
		MarketIndex: market.Index, ClientOrderIndex: order.ClientOrderIndex,
		BaseAmount: ToBaseAmount(order.Quantity, market.SizeDecimals), Price: ToPrice(order.Price, market.PriceDecimals),
		IsAsk: isAsk, Type: order.Type, TimeInForce: tif, ReduceOnly: reduceOnly,
	}
	if order.Type != lightertx.MarketOrder {
		tx.OrderExpiry = order.Expiry.UnixMilli()
	}
	if order.TriggerPrice > 0 {
		tx.TriggerPrice = ToPrice(order.TriggerPrice, market.PriceDecimals)
	}
	return tx, nil
}

func BuildCloseOrder(symbol string, side Side, quantity, price float64, clientOrderIndex int64, expiry time.Time) (*lightertypes.CreateOrderTxReq, error) {
	return BuildCreateOrder(OrderRequest{Symbol: symbol, Side: side, Price: price, Quantity: quantity, ClientOrderIndex: clientOrderIndex, Expiry: expiry, Type: lightertx.LimitOrder, ReduceOnly: true})
}

func BuildCancelOrder(symbol string, orderIndex int64) (*lightertypes.CancelOrderTxReq, error) {
	market, err := MarketFor(symbol)
	if err != nil {
		return nil, err
	}
	if orderIndex <= 0 {
		return nil, fmt.Errorf("positive order index is required")
	}
	return &lightertypes.CancelOrderTxReq{MarketIndex: market.Index, Index: orderIndex}, nil
}

func ToPrice(price float64, decimals int) uint32 {
	return uint32(math.Round(price * math.Pow10(decimals)))
}
