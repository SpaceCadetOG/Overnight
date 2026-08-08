package mlshadow

import (
	"reflect"
	"testing"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

type capture struct {
	stream string
	value  any
}

func (c *capture) Append(s string, v any) error { c.stream = s; c.value = v; return nil }

func TestPredictionAlwaysUsesSeparateShadowStream(t *testing.T) {
	c := &capture{}
	p := Prediction{ModelVersion: "m1", OpportunityID: "opp", Target: "fill"}
	if err := Write(c, p); err != nil {
		t.Fatal(err)
	}
	got := c.value.(Prediction)
	if c.stream != PredictionStream || !got.ShadowOnly {
		t.Fatalf("stream=%s prediction=%+v", c.stream, got)
	}
}

func TestMLPredictionCannotModifyFrozenPlan(t *testing.T) {
	plan := models.TradePlan{Entry: 100, Stop: 99, TP1: 101, TP2: 102, Valid: true}
	before := plan
	_ = Prediction{ModelVersion: "m1", OpportunityID: "opp", Target: "expected_r", Value: -10, ShadowOnly: true}
	if !reflect.DeepEqual(plan, before) {
		t.Fatal("ML output modified baseline plan")
	}
}

func TestFeatureSnapshotRejectsLookAhead(t *testing.T) {
	now := time.Now()
	f := FeatureSnapshot{OpportunityID: "opp", FeatureDefinitionVersion: "v1", AsOfTime: now, AvailableAt: now, SourceWindowEnd: now.Add(time.Second)}
	if f.Validate() == nil {
		t.Fatal("look-ahead feature accepted")
	}
}
