package live

import (
	"github.com/ogtrading/overnight-strategy/internal/models"
	"testing"
	"time"
)

func TestResearchAssetCannotCreateIntent(t *testing.T) {
	plan := models.TradePlan{Date: time.Now(), Direction: models.BiasLong, Entry: 10, Stop: 9, TP1: 11, TP2: 12, Valid: true}
	if _, err := BuildIntent("XAU", plan, 0.2); err == nil {
		t.Fatal("XAU received order intent")
	}
}

func TestBasketRisk(t *testing.T) {
	plan := models.TradePlan{Date: time.Now(), Direction: models.BiasLong, Entry: 10, Stop: 9, TP1: 11, TP2: 12, Valid: true}
	intents := []Intent{}
	for _, symbol := range []string{"BTC", "ETH", "ZEC", "BNB", "SOL"} {
		intent, err := BuildIntent(symbol, plan, 0.2)
		if err != nil {
			t.Fatal(err)
		}
		intents = append(intents, intent)
	}
	if err := ValidateBasket(intents, DefaultRiskPolicy()); err != nil {
		t.Fatal(err)
	}
}

func TestIntentStateMachineRejectsUnsafeJump(t *testing.T) {
	intent := Intent{State: Created}
	if _, err := Transition(intent, Submitted); err == nil {
		t.Fatal("created intent jumped directly to submitted")
	}
	if _, err := Transition(intent, DryRun); err != nil {
		t.Fatal(err)
	}
}
