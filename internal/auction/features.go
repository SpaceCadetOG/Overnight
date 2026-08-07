package auction

import "github.com/ogtrading/overnight-strategy/internal/models"

func populateFeatures(structure *AuctionStructure) {
	structure.EntryInsideValue = Between(
		structure.Entry,
		structure.VAL,
		structure.VAH,
		DefaultTolerance,
	)

	structure.EntryAboveVAH = RelativePosition(
		structure.Entry,
		structure.VAH,
		DefaultTolerance,
	) == PositionAbove

	structure.EntryBelowVAL = RelativePosition(
		structure.Entry,
		structure.VAL,
		DefaultTolerance,
	) == PositionBelow

	structure.POCBetweenEntryAndTP1 = Between(
		structure.POC,
		structure.Entry,
		structure.TP1,
		DefaultTolerance,
	)

	structure.Fib618AbovePOC = RelativePosition(
		structure.Fib618,
		structure.POC,
		DefaultTolerance,
	) == PositionAbove

	structure.Fib618BelowPOC = RelativePosition(
		structure.Fib618,
		structure.POC,
		DefaultTolerance,
	) == PositionBelow

	switch structure.Bias {
	case models.BiasLong:
		structure.POCBehindEntry =
			RelativePosition(
				structure.POC,
				structure.Entry,
				DefaultTolerance,
			) == PositionBelow

		structure.POCBeyondTP1 =
			RelativePosition(
				structure.POC,
				structure.TP1,
				DefaultTolerance,
			) == PositionAbove

		vwapPosition := RelativePosition(
			structure.VWAP,
			structure.Entry,
			DefaultTolerance,
		)

		structure.VWAPSupportsDirection =
			vwapPosition == PositionBelow ||
				vwapPosition == PositionEqual

	case models.BiasShort:
		structure.POCBehindEntry =
			RelativePosition(
				structure.POC,
				structure.Entry,
				DefaultTolerance,
			) == PositionAbove

		structure.POCBeyondTP1 =
			RelativePosition(
				structure.POC,
				structure.TP1,
				DefaultTolerance,
			) == PositionBelow

		vwapPosition := RelativePosition(
			structure.VWAP,
			structure.Entry,
			DefaultTolerance,
		)

		structure.VWAPSupportsDirection =
			vwapPosition == PositionAbove ||
				vwapPosition == PositionEqual
	}
}
