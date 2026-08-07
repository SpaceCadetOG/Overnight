package research

type FeatureAnalysis struct {
	Reports []FeatureReport
}

type FeatureReport struct {
	Name  string
	True  FeatureBucket
	False FeatureBucket
}

type FeatureBucket struct {
	ValidPlans int
	Filled     int
	NoFill     int

	// Filled-trade outcomes
	Wins      int
	Losses    int
	Breakeven int

	// Raw totals
	TotalR      float64
	TotalMFE    float64
	TotalMAE    float64
	GrossProfit float64
	GrossLoss   float64

	// Derived metrics
	FillRate      float64
	FilledWinRate float64

	AverageRPerPlan   float64
	AverageRPerFilled float64
	AverageMFE        float64
	AverageMAE        float64
	ProfitFactor      float64
}
