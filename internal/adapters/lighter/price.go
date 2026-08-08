package lighter

import (
	"math"
	"strconv"
)

func PriceConvert(price string, decimals int) (int64, error) {

	p, err := strconv.ParseFloat(price, 64)

	if err != nil {
		return 0, err
	}

	multiplier := math.Pow10(decimals)

	return int64(math.Round(p * multiplier)), nil
}
