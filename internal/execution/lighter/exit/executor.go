package exit

import (
	"fmt"
	"math"
	"time"

	lightertypes "github.com/elliottech/lighter-go/types"

	lighter "github.com/ogtrading/overnight-strategy/internal/execution/lighter"
)

type Action string

const (
	TP1  Action = "TP1"
	TP2  Action = "TP2"
	STOP Action = "STOP"
)

type Request struct {
	Symbol string

	PositionSide string

	Size int64

	Price uint32

	Action Action
}

func BuildClose(
	req Request,
) (*lightertypes.CreateOrderTxReq, error) {

	var side lighter.Side

	switch req.PositionSide {

	case "LONG":
		side = lighter.Sell

	case "SHORT":
		side = lighter.Buy

	default:
		return nil, fmt.Errorf("invalid position side %s", req.PositionSide)
	}

	market, err := lighter.MarketFor(req.Symbol)
	if err != nil {
		return nil, err
	}
	return lighter.BuildCloseOrder(
		req.Symbol,
		side,
		float64(req.Size)/math.Pow10(market.SizeDecimals),
		float64(req.Price)/math.Pow10(market.PriceDecimals),
		time.Now().UnixMilli(),
		time.Now().Add(30*time.Second),
	)
}
