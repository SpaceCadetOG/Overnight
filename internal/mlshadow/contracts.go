package mlshadow

import (
	"fmt"
	"time"
)

const SchemaVersion = 1
const PredictionStream = "ml_shadow_predictions"

type DecisionPoint string

const (
	Plan        DecisionPoint = "PLAN"
	TMinus5M    DecisionPoint = "T_MINUS_5_MIN"
	TMinus1M    DecisionPoint = "T_MINUS_1_MIN"
	EntryTouch  DecisionPoint = "ENTRY_TOUCH"
	Fill        DecisionPoint = "FILL"
	PostFill30S DecisionPoint = "POST_FILL_30_SECONDS"
	PostFill1M  DecisionPoint = "POST_FILL_1_MINUTE"
	PostFill5M  DecisionPoint = "POST_FILL_5_MINUTES"
	TP1         DecisionPoint = "TP1"
	TP2         DecisionPoint = "TP2"
	Stop        DecisionPoint = "STOP"
	Expiry      DecisionPoint = "EXPIRY"
)

type FeatureSnapshot struct {
	SchemaVersion            int                `json:"schema_version"`
	EntityID                 string             `json:"entity_id"`
	OpportunityID            string             `json:"opportunity_id"`
	StrategyOrderID          string             `json:"strategy_order_id"`
	Symbol                   string             `json:"symbol"`
	AsOfTime                 time.Time          `json:"as_of_time"`
	AvailableAt              time.Time          `json:"available_at"`
	DecisionPoint            DecisionPoint      `json:"decision_point"`
	FeatureDefinitionVersion string             `json:"feature_definition_version"`
	SourceWindowStart        time.Time          `json:"source_window_start"`
	SourceWindowEnd          time.Time          `json:"source_window_end"`
	Features                 map[string]float64 `json:"features"`
	DataQuality              []string           `json:"data_quality,omitempty"`
}

func (f FeatureSnapshot) Validate() error {
	if f.OpportunityID == "" || f.FeatureDefinitionVersion == "" {
		return fmt.Errorf("feature identity/version required")
	}
	if f.SourceWindowEnd.After(f.AsOfTime) || f.AvailableAt.After(f.AsOfTime) {
		return fmt.Errorf("point-in-time leakage: source or availability is after as_of_time")
	}
	return nil
}

type Prediction struct {
	SchemaVersion        int                `json:"schema_version"`
	PredictionID         string             `json:"prediction_id"`
	ModelVersion         string             `json:"model_version"`
	FeatureSchemaVersion string             `json:"feature_schema_version"`
	StrategyVersion      string             `json:"strategy_version"`
	PredictionTimestamp  time.Time          `json:"prediction_timestamp"`
	SessionID            string             `json:"session_id"`
	OpportunityID        string             `json:"opportunity_id"`
	StrategyOrderID      string             `json:"strategy_order_id"`
	Symbol               string             `json:"symbol"`
	Target               string             `json:"target"`
	Value                float64            `json:"value"`
	Calibration          float64            `json:"calibration"`
	TopFeatures          map[string]float64 `json:"top_features,omitempty"`
	DataQuality          []string           `json:"data_quality,omitempty"`
	ShadowOnly           bool               `json:"shadow_only"`
}

func (p Prediction) Validate() error {
	if !p.ShadowOnly {
		return fmt.Errorf("all ML predictions must be shadow_only")
	}
	if p.ModelVersion == "" || p.OpportunityID == "" || p.Target == "" {
		return fmt.Errorf("prediction identity/version required")
	}
	return nil
}

func Write(output interface{ Append(string, any) error }, prediction Prediction) error {
	prediction.SchemaVersion = SchemaVersion
	prediction.ShadowOnly = true
	if err := prediction.Validate(); err != nil {
		return err
	}
	return output.Append(PredictionStream, prediction)
}
