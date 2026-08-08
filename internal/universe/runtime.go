package universe

func Runtime(symbol string) Destination {
	destination, err := Resolve(symbol)
	if err != nil {
		return PaperExecutor
	}
	return destination
}
