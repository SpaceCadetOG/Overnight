package research

import "time"

type Report struct {
	Summary         PerformanceStats
	Directions      map[string]PerformanceStats
	Weekdays        map[time.Weekday]PerformanceStats
	Durations       DurationStats
	Outcomes        OutcomeReport
	MAEDistribution []DistributionBucket
	MFEDistribution []DistributionBucket
}

type PerformanceStats struct {
	Trades       int
	Wins         int
	Losses       int
	Breakeven    int
	TotalR       float64
	AverageR     float64
	WinRate      float64
	ProfitFactor float64
	AverageMFE   float64
	AverageMAE   float64
	MaxDrawdown  float64
}

type DurationStats struct {
	All     DurationGroup
	Winners DurationGroup
	Losers  DurationGroup
}

type DurationGroup struct {
	Trades         int
	AverageMinutes float64
	MedianMinutes  float64
	P90Minutes     float64
	MinimumMinutes int
	MaximumMinutes int
}

type DistributionBucket struct {
	Label      string
	LowerBound float64
	UpperBound float64
	Count      int
	Percent    float64
}
