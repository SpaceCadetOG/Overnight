package paper

type Executor struct {
	State State

	Position Position
}

func NewExecutor() *Executor {

	return &Executor{
		State: FLAT,
	}

}

func (e *Executor) Submit(plan TradePlan) {

	e.State = ORDER_CREATED

	e.Position = Position{
		Symbol:    plan.Symbol,
		Direction: plan.Direction,
		Size:      plan.Size,
		Entry:     plan.Entry,
	}

}

func (e *Executor) Fill() {

	e.State = OPEN

}

func (e *Executor) Close() {

	e.State = CLOSED

	e.Position = Position{}

}

func (e *Executor) Reset() {

	e.State = FLAT

}
