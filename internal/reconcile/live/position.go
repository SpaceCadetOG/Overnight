package live

import (
	"math"

	"github.com/ogtrading/overnight-strategy/internal/execution"
	"github.com/ogtrading/overnight-strategy/internal/execution/lighter/ws"
)

type Status string

const (
	Synced          Status = "SYNCED"
	MissingInternal Status = "MISSING_INTERNAL"
	MissingExchange Status = "MISSING_EXCHANGE"
	SizeMismatch    Status = "SIZE_MISMATCH"
	EntryMismatch   Status = "ENTRY_MISMATCH"
)

type PositionDiff struct {
	Symbol string

	Status Status

	InternalSize float64
	ExchangeSize float64

	InternalEntry float64
	ExchangeEntry float64
}

func Compare(
	internal *execution.Position,
	exchange *ws.PositionSnapshot,
) PositionDiff {

	if internal == nil && exchange == nil {
		return PositionDiff{
			Status: Synced,
		}
	}

	if internal != nil && exchange == nil {

		return PositionDiff{
			Symbol:        internal.Symbol,
			Status:        MissingExchange,
			InternalSize:  internal.Size,
			InternalEntry: internal.Entry,
		}
	}

	if internal == nil && exchange != nil {

		return PositionDiff{
			Symbol:        exchange.Symbol,
			Status:        MissingInternal,
			ExchangeSize:  exchange.Size,
			ExchangeEntry: exchange.EntryPrice,
		}
	}

	diff := PositionDiff{
		Symbol: exchange.Symbol,

		InternalSize: internal.Size,
		ExchangeSize: exchange.Size,

		InternalEntry: internal.Entry,
		ExchangeEntry: exchange.EntryPrice,
	}

	if math.Abs(
		internal.Size-exchange.Size,
	) > 0.000001 {

		diff.Status = SizeMismatch
		return diff
	}

	if math.Abs(
		internal.Entry-exchange.EntryPrice,
	) > 0.01 {

		diff.Status = EntryMismatch
		return diff
	}

	diff.Status = Synced

	return diff
}
