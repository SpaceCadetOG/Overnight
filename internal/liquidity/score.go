package liquidity

func score(context Context) int {
	value := 0
	if context.Sequence == SequenceSSLToBSL || context.Sequence == SequenceBSLToSSL {
		value += 4
	}
	if context.DirectionalTarget {
		value += 3
	}
	if !context.OpposingPresent {
		value += 2
	}
	if context.ObstacleCount == 0 {
		value++
	}
	if value > 10 {
		return 10
	}
	return value
}
