package lighter

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

type EncodedOrder struct {
	MarketIndex int16

	BaseAmount int64
	Price      uint32

	Quantity float64
	PriceUSD float64

	Notional float64
}

func (m Market) EncodePrice(price float64) (uint32, error) {
	if err := m.Validate(); err != nil {
		return 0, err
	}
	value, err := encodeDecimal("price", price, m.SupportedPriceDecimals, new(big.Int).SetUint64(math.MaxUint32))
	if err != nil {
		return 0, err
	}
	return uint32(value.Uint64()), nil
}

func decimalScale(decimals int) (*big.Int, error) {
	if decimals < 0 {
		return nil, fmt.Errorf(
			"decimal precision cannot be negative: %d",
			decimals,
		)
	}

	if decimals > 18 {
		return nil, fmt.Errorf(
			"decimal precision too large: %d",
			decimals,
		)
	}

	scale := new(big.Int).Exp(
		big.NewInt(10),
		big.NewInt(int64(decimals)),
		nil,
	)

	return scale, nil
}

func parsePositiveRat(
	name string,
	value string,
) (*big.Rat, error) {
	value = strings.TrimSpace(value)

	if value == "" {
		return nil, fmt.Errorf("%s is empty", name)
	}

	r, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, fmt.Errorf(
			"%s is invalid: %q",
			name,
			value,
		)
	}

	if r.Sign() <= 0 {
		return nil, fmt.Errorf(
			"%s must be greater than zero: %q",
			name,
			value,
		)
	}

	return r, nil
}

func encodeDecimal(
	name string,
	value float64,
	decimals int,
	max *big.Int,
) (*big.Int, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil, fmt.Errorf(
			"%s must be finite",
			name,
		)
	}

	if value <= 0 {
		return nil, fmt.Errorf(
			"%s must be greater than zero",
			name,
		)
	}

	scale, err := decimalScale(decimals)
	if err != nil {
		return nil, err
	}

	// Convert through a decimal string rather than binary float arithmetic.
	raw := strconv.FormatFloat(
		value,
		'f',
		decimals+6,
		64,
	)

	r, ok := new(big.Rat).SetString(raw)
	if !ok {
		return nil, fmt.Errorf(
			"cannot convert %s=%v",
			name,
			value,
		)
	}

	scaled := new(big.Rat).Mul(
		r,
		new(big.Rat).SetInt(scale),
	)

	if scaled.Denom().Cmp(big.NewInt(1)) != 0 {
		return nil, fmt.Errorf(
			"%s %.12f exceeds supported precision of %d decimals",
			name,
			value,
			decimals,
		)
	}

	n := new(big.Int).Set(scaled.Num())

	if n.Sign() <= 0 {
		return nil, fmt.Errorf(
			"encoded %s must be greater than zero",
			name,
		)
	}

	if n.Cmp(max) > 0 {
		return nil, fmt.Errorf(
			"encoded %s overflows destination integer",
			name,
		)
	}

	return n, nil
}

func (m Market) EncodeOrder(
	quantity float64,
	price float64,
) (*EncodedOrder, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}

	baseInt, err := encodeDecimal(
		"quantity",
		quantity,
		m.SupportedSizeDecimals,
		big.NewInt(math.MaxInt64),
	)
	if err != nil {
		return nil, err
	}

	priceInt, err := encodeDecimal(
		"price",
		price,
		m.SupportedPriceDecimals,
		new(big.Int).SetUint64(math.MaxUint32),
	)
	if err != nil {
		return nil, err
	}

	minBase, err := parsePositiveRat(
		"minimum base amount",
		m.MinBaseAmount,
	)
	if err != nil {
		return nil, err
	}

	minQuote, err := parsePositiveRat(
		"minimum quote amount",
		m.MinQuoteAmount,
	)
	if err != nil {
		return nil, err
	}

	qtyRat, ok := new(big.Rat).SetString(
		strconv.FormatFloat(
			quantity,
			'f',
			m.SupportedSizeDecimals,
			64,
		),
	)
	if !ok {
		return nil, fmt.Errorf("cannot encode quantity")
	}

	priceRat, ok := new(big.Rat).SetString(
		strconv.FormatFloat(
			price,
			'f',
			m.SupportedPriceDecimals,
			64,
		),
	)
	if !ok {
		return nil, fmt.Errorf("cannot encode price")
	}

	if qtyRat.Cmp(minBase) < 0 {
		return nil, fmt.Errorf(
			"quantity %.12f is below %s minimum base amount %s",
			quantity,
			m.Symbol,
			m.MinBaseAmount,
		)
	}

	notionalRat := new(big.Rat).Mul(
		qtyRat,
		priceRat,
	)

	if notionalRat.Cmp(minQuote) < 0 {
		f, _ := notionalRat.Float64()

		return nil, fmt.Errorf(
			"order notional %.8f is below %s minimum quote amount %s",
			f,
			m.Symbol,
			m.MinQuoteAmount,
		)
	}

	notional, _ := notionalRat.Float64()

	return &EncodedOrder{
		MarketIndex: m.MarketID,
		BaseAmount:  baseInt.Int64(),
		Price:       uint32(priceInt.Uint64()),
		Quantity:    quantity,
		PriceUSD:    price,
		Notional:    notional,
	}, nil
}

// MinimumQuantityAtPrices returns the smallest supported quantity that satisfies
// both the base and quote minimum at every supplied entry/protection price.
func (m Market) MinimumQuantityAtPrices(prices ...float64) (float64, error) {
	if err := m.Validate(); err != nil {
		return 0, err
	}
	if len(prices) == 0 {
		return 0, fmt.Errorf("at least one price is required")
	}
	minBase, err := strconv.ParseFloat(m.MinBaseAmount, 64)
	if err != nil || minBase <= 0 {
		return 0, fmt.Errorf("invalid minimum base amount %q", m.MinBaseAmount)
	}
	minQuote, err := strconv.ParseFloat(m.MinQuoteAmount, 64)
	if err != nil || minQuote <= 0 {
		return 0, fmt.Errorf("invalid minimum quote amount %q", m.MinQuoteAmount)
	}
	step := math.Pow10(-m.SupportedSizeDecimals)
	quantity := minBase
	for _, price := range prices {
		if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
			return 0, fmt.Errorf("price must be finite and greater than zero")
		}
		if needed := minQuote / price; needed > quantity {
			quantity = needed
		}
	}
	quantity = math.Ceil((quantity-1e-12)/step) * step
	quantity, err = strconv.ParseFloat(strconv.FormatFloat(quantity, 'f', m.SupportedSizeDecimals, 64), 64)
	if err != nil {
		return 0, err
	}
	for _, price := range prices {
		if _, err := m.EncodeOrder(quantity, price); err != nil {
			// Floating-point division can land exactly below a decimal boundary.
			quantity += step
			quantity, _ = strconv.ParseFloat(strconv.FormatFloat(quantity, 'f', m.SupportedSizeDecimals, 64), 64)
			break
		}
	}
	for _, price := range prices {
		if _, err := m.EncodeOrder(quantity, price); err != nil {
			return 0, fmt.Errorf("minimum protective quantity: %w", err)
		}
	}
	return quantity, nil
}
