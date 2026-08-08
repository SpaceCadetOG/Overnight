package strategy

type Engine interface {
	Evaluate(
		context MarketContext,
	) Signal
}

type OvernightStrategy struct{}

func New() *OvernightStrategy {

	return &OvernightStrategy{}

}

func (s *OvernightStrategy) Evaluate(
	ctx MarketContext,
) Signal {

	return Signal{
		Valid:     false,
		Direction: None,
		Reason:    "no setup",
	}

}
