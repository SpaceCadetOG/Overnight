package auction

import (
	"fmt"
	"strings"
)

// FormatStructure returns a deterministic, human-readable description of
// one pre-entry auction structure.
//
// This function only formats existing research data. It does not change
// trade selection, execution, entries, stops, targets, or outcomes.
func FormatStructure(structure AuctionStructure) string {
	var builder strings.Builder

	fmt.Fprintln(&builder, "RAW LEVELS")
	fmt.Fprintln(&builder, "--------------------------------------------------------")
	fmt.Fprintf(&builder, "Overnight High:        %.2f\n", structure.OvernightHigh)
	fmt.Fprintf(&builder, "Overnight Low:         %.2f\n", structure.OvernightLow)
	fmt.Fprintf(&builder, "Overnight Range:       %.2f\n", structure.OvernightRange)
	fmt.Fprintf(&builder, "VWAP:                  %.2f\n", structure.VWAP)
	fmt.Fprintf(&builder, "POC:                   %.2f\n", structure.POC)
	fmt.Fprintf(&builder, "VAH:                   %.2f\n", structure.VAH)
	fmt.Fprintf(&builder, "VAL:                   %.2f\n", structure.VAL)
	fmt.Fprintf(&builder, "Fib 38.2%%:             %.2f\n", structure.Fib382)
	fmt.Fprintf(&builder, "Fib 50.0%%:             %.2f\n", structure.Fib500)
	fmt.Fprintf(&builder, "Fib 61.8%%:             %.2f\n", structure.Fib618)
	fmt.Fprintf(&builder, "Entry:                 %.2f\n", structure.Entry)
	fmt.Fprintf(&builder, "Stop:                  %.2f\n", structure.Stop)
	fmt.Fprintf(&builder, "TP1:                   %.2f\n", structure.TP1)

	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, "RELATIVE POSITIONS")
	fmt.Fprintln(&builder, "--------------------------------------------------------")
	fmt.Fprintf(&builder, "POC vs Entry:          %s\n", structure.POCVsEntry)
	fmt.Fprintf(&builder, "POC vs TP1:            %s\n", structure.POCVsTP1)
	fmt.Fprintf(&builder, "VWAP vs Entry:         %s\n", structure.VWAPVsEntry)
	fmt.Fprintf(&builder, "Fib618 vs POC:         %s\n", structure.Fib618VsPOC)
	fmt.Fprintf(&builder, "VAH vs Entry:          %s\n", structure.VAHVsEntry)
	fmt.Fprintf(&builder, "VAL vs Entry:          %s\n", structure.VALVsEntry)

	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, "NORMALIZED DISTANCES")
	fmt.Fprintln(&builder, "--------------------------------------------------------")
	fmt.Fprintf(&builder, "Entry to POC:          %.3fR\n", structure.EntryToPOCR)
	fmt.Fprintf(&builder, "Entry to VWAP:         %.3fR\n", structure.EntryToVWAPR)
	fmt.Fprintf(&builder, "POC to TP1:            %.3fR\n", structure.POCToTP1R)
	fmt.Fprintf(&builder, "VAH to TP1:            %.3fR\n", structure.VAHToTP1R)
	fmt.Fprintf(&builder, "VAL to Entry:          %.3fR\n", structure.VALToEntryR)
	fmt.Fprintf(&builder, "Fib618 to POC:         %.3fR\n", structure.Fib618ToPOCR)

	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, "STRUCTURAL FEATURES")
	fmt.Fprintln(&builder, "--------------------------------------------------------")
	fmt.Fprintf(
		&builder,
		"Entry inside value:           %s\n",
		formatBoolean(structure.EntryInsideValue),
	)
	fmt.Fprintf(
		&builder,
		"Entry above VAH:              %s\n",
		formatBoolean(structure.EntryAboveVAH),
	)
	fmt.Fprintf(
		&builder,
		"Entry below VAL:              %s\n",
		formatBoolean(structure.EntryBelowVAL),
	)
	fmt.Fprintf(
		&builder,
		"POC between Entry and TP1:    %s\n",
		formatBoolean(structure.POCBetweenEntryAndTP1),
	)
	fmt.Fprintf(
		&builder,
		"POC behind Entry:             %s\n",
		formatBoolean(structure.POCBehindEntry),
	)
	fmt.Fprintf(
		&builder,
		"POC beyond TP1:               %s\n",
		formatBoolean(structure.POCBeyondTP1),
	)
	fmt.Fprintf(
		&builder,
		"Fib618 above POC:             %s\n",
		formatBoolean(structure.Fib618AbovePOC),
	)
	fmt.Fprintf(
		&builder,
		"Fib618 below POC:             %s\n",
		formatBoolean(structure.Fib618BelowPOC),
	)
	fmt.Fprintf(
		&builder,
		"VWAP supports direction:      %s\n",
		formatBoolean(structure.VWAPSupportsDirection),
	)

	return builder.String()
}

func formatBoolean(value bool) string {
	if value {
		return "YES"
	}

	return "NO"
}
