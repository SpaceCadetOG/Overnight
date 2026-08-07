package models

import "time"

type TradeOutcome string

const (
	OutcomeNoFill       TradeOutcome = "NO_FILL"
	OutcomeStopped      TradeOutcome = "STOPPED"
	OutcomeTP1Breakeven TradeOutcome = "TP1_BE"
	OutcomeTP2          TradeOutcome = "TP2"
	OutcomeTimeExit     TradeOutcome = "TIME_EXIT"
	OutcomeInvalid      TradeOutcome = "INVALID"
)

type TradeResult struct {
	Date      time.Time
	Direction Bias
	Outcome   TradeOutcome

	// Planned strategy levels.
	Entry float64
	Stop  float64
	TP1   float64
	TP2   float64

	EntrySource string
	StopSource  string
	TP1Source   string

	// Simulated execution prices.
	FillPrice    float64
	TP1FillPrice float64
	ExitPrice    float64

	Filled   bool
	FillTime time.Time

	TP1Hit  bool
	TP1Time time.Time

	ExitTime time.Time

	InitialRisk float64

	// GrossR excludes fees. RealizedR includes all fees.
	GrossR    float64
	FeeR      float64
	RealizedR float64

	EntrySlippageBps float64
	ExitSlippageBps  float64
	TotalFees        float64

	// Execution research metrics.
	MFER float64
	MAER float64

	HighestAfterFill float64
	LowestAfterFill  float64

	WindowHigh float64
	WindowLow  float64

	// For NO_FILL sessions, the minimum price distance by which the
	// order was missed. Zero means the entry was touched.
	MissedEntryDistance float64

	MinutesToFill  int
	MinutesInTrade int

	Notes string
}
