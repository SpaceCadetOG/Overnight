package liquidity

import "time"

type Side string

const (
	BuySide  Side = "BUY_SIDE"
	SellSide Side = "SELL_SIDE"
)

type Kind string

const (
	SessionHigh       Kind = "SESSION_HIGH"
	SessionLow        Kind = "SESSION_LOW"
	SwingHigh         Kind = "SWING_HIGH"
	SwingLow          Kind = "SWING_LOW"
	EqualHigh         Kind = "EQUAL_HIGH"
	EqualLow          Kind = "EQUAL_LOW"
	PreviousHigh      Kind = "PREVIOUS_DAY_HIGH"
	PreviousLow       Kind = "PREVIOUS_DAY_LOW"
	InternalLiquidity Kind = "INTERNAL_LIQUIDITY"
	ExternalLiquidity Kind = "EXTERNAL_LIQUIDITY"
)

type Level struct {
	Kind      Kind
	Side      Side
	Price     float64
	FormedAt  time.Time
	LastTime  time.Time
	Taken     bool
	TakenAt   time.Time
	Touches   int
	DistanceR float64
	External  bool
	Strength  int
}

type Sequence string

const (
	SequenceNone     Sequence = "NONE"
	SequenceSSLToBSL Sequence = "SSL_TO_BSL"
	SequenceBSLToSSL Sequence = "BSL_TO_SSL"
)

type Event string

const (
	EventNone  Event = "NONE"
	EventGrab  Event = "GRAB"
	EventSweep Event = "SWEEP"
	EventRun   Event = "RUN"
)

type Context struct {
	Levels                       []Level
	NearestAbove                 *Level
	NearestBelow                 *Level
	BuySideTaken                 bool
	SellSideTaken                bool
	OpposingPresent              bool
	DirectionalTarget            bool
	Sequence                     Sequence
	PathScore                    int
	Event                        Event
	ObstacleCount                int
	ValueLocation                ValueLocationState
	Path                         LiquidityPath
	ValueTransition              ValueTransitionState
	LiquidityConsumedBeforeEntry bool
	TargetAvailable              bool
	TargetAvailability           TargetAvailabilityState
	TP1ObstacleCount             int
	TP2ObstacleCount             int
	InternalTakenBeforeEntry     bool
	ExternalTarget               bool
	InternalToExternal           bool
	EntryLiquidity               EntryLiquidityType
	InternalTargetPrice          float64
	ExternalTargetPrice          float64
}

type ValueLocationState string

const (
	AboveValue             ValueLocationState = "ABOVE_VALUE"
	InsideValue            ValueLocationState = "INSIDE_VALUE"
	BelowValueAcceptance   ValueLocationState = "BELOW_VALUE_ACCEPTANCE"
	BelowValueSweepReclaim ValueLocationState = "BELOW_VALUE_SWEEP_RECLAIM"
)

type LiquidityPath struct {
	TargetLiquidity   float64
	OpposingLiquidity float64
	TargetDistance    float64
	OpposingDistance  float64
	CleanPath         bool
}

type ValueTransitionState string

const (
	ValueAcceptance   ValueTransitionState = "VALUE_ACCEPTANCE"
	ValueRejection    ValueTransitionState = "VALUE_REJECTION"
	ValueRotation     ValueTransitionState = "VALUE_ROTATION"
	ValueContinuation ValueTransitionState = "VALUE_CONTINUATION"
)

type TargetAvailabilityState string

const (
	TargetAvailableState TargetAvailabilityState = "AVAILABLE"
	TargetConsumedState  TargetAvailabilityState = "CONSUMED"
	TargetAbsentState    TargetAvailabilityState = "ABSENT"
)

type EntryLiquidityType string

const EntryLiquidityMaxDistanceR = 1.0

const (
	EntryInternalSSL   EntryLiquidityType = "INTERNAL_SSL"
	EntryInternalBSL   EntryLiquidityType = "INTERNAL_BSL"
	EntryExternalSSL   EntryLiquidityType = "EXTERNAL_SSL"
	EntryExternalBSL   EntryLiquidityType = "EXTERNAL_BSL"
	EntryLiquidityNone EntryLiquidityType = "NONE"
)
