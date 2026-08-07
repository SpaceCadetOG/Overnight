package auction

import "math"

// EqualWithinTolerance reports whether two finite values are equal within
// the supplied absolute tolerance.
func EqualWithinTolerance(a, b, tolerance float64) bool {
	if math.IsNaN(a) || math.IsNaN(b) ||
		math.IsInf(a, 0) || math.IsInf(b, 0) {
		return false
	}

	if tolerance < 0 {
		tolerance = math.Abs(tolerance)
	}

	return math.Abs(a-b) <= tolerance
}

// RelativePosition reports whether value is below, equal to, or above
// reference. Non-finite inputs produce PositionUnknown.
func RelativePosition(value, reference, tolerance float64) Position {
	if math.IsNaN(value) || math.IsNaN(reference) ||
		math.IsInf(value, 0) || math.IsInf(reference, 0) {
		return PositionUnknown
	}

	if EqualWithinTolerance(value, reference, tolerance) {
		return PositionEqual
	}

	if value < reference {
		return PositionBelow
	}

	return PositionAbove
}

// Between reports whether value lies inclusively between endpointA and
// endpointB. Endpoint order does not matter, so it works for both long
// and short trade paths.
func Between(value, endpointA, endpointB, tolerance float64) bool {
	if math.IsNaN(value) || math.IsNaN(endpointA) || math.IsNaN(endpointB) ||
		math.IsInf(value, 0) || math.IsInf(endpointA, 0) ||
		math.IsInf(endpointB, 0) {
		return false
	}

	if tolerance < 0 {
		tolerance = math.Abs(tolerance)
	}

	lower := math.Min(endpointA, endpointB) - tolerance
	upper := math.Max(endpointA, endpointB) + tolerance

	return value >= lower && value <= upper
}
