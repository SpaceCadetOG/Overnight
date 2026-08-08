package exit

type Executor interface {
	Close(
		symbol string,
		side string,
		size int64,
		price uint32,
	) (string, error)
}
