package mlshadow

import (
	"github.com/ogtrading/overnight-strategy/internal/execution"
	"reflect"
	"testing"
	"time"
)

func TestShadowPredictionCannotMutateOrAuthorizeExecution(t *testing.T) {
	order := execution.Order{Symbol: "BTC", Side: "BUY", Price: 100, Stop: 99, TP1: 101, TP2: 102}
	before := order
	c := &capture{}
	if err := Write(c, Prediction{ModelVersion: "hostile", OpportunityID: "opp", Target: "replace_order", Value: 999}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, before) {
		t.Fatal("prediction mutated order")
	}
	gate := execution.Gate{Mode: execution.Live, AllowedSymbols: map[string]bool{"BTC": true, "ETH": true}}
	if gate.Authorize("SOL", time.Now()) == nil {
		t.Fatal("shadow prediction expanded live universe")
	}
}
