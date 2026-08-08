package lighter

import (
	"testing"

	lightertypes "github.com/elliottech/lighter-go/types"
)

func TestCreateOrderRequestShape(t *testing.T) {

	req := &lightertypes.CreateOrderTxReq{
		MarketIndex:      0,
		ClientOrderIndex: 1,
		BaseAmount:       1000,
		Price:            60000,
		IsAsk:            0,
		Type:             0,
		TimeInForce:      1,
		ReduceOnly:       0,
		TriggerPrice:     0,
		OrderExpiry:      0,
	}

	if req.MarketIndex != 0 {
		t.Fatal("market index mismatch")
	}

	if req.Price != 60000 {
		t.Fatal("price mismatch")
	}
}
