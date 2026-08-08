package lighter

import "math"

func ToBaseAmount(
	quantity float64,
	decimals int,
) int64 {

	scale := math.Pow10(decimals)

	return int64(
		math.Round(quantity * scale),
	)
}
