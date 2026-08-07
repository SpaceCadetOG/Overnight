package auction

import "math"

func populateDistances(structure *AuctionStructure) {
	risk := math.Abs(structure.Entry - structure.Stop)

	// A zero-risk plan is invalid for normal execution, but the research
	// structure remains safe and deterministic. Distances remain zero
	// instead of becoming NaN or infinity.
	if risk <= DefaultTolerance {
		return
	}

	structure.EntryToPOCR = normalizedDistance(
		structure.Entry,
		structure.POC,
		risk,
	)

	structure.EntryToVWAPR = normalizedDistance(
		structure.Entry,
		structure.VWAP,
		risk,
	)

	structure.POCToTP1R = normalizedDistance(
		structure.POC,
		structure.TP1,
		risk,
	)

	structure.VAHToTP1R = normalizedDistance(
		structure.VAH,
		structure.TP1,
		risk,
	)

	structure.VALToEntryR = normalizedDistance(
		structure.VAL,
		structure.Entry,
		risk,
	)

	structure.Fib618ToPOCR = normalizedDistance(
		structure.Fib618,
		structure.POC,
		risk,
	)
}

func normalizedDistance(a, b, risk float64) float64 {
	if risk <= DefaultTolerance {
		return 0
	}

	return math.Abs(a-b) / risk
}
