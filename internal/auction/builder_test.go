package auction

import (
	"math"
	"strings"
	"testing"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

func TestBuildAuctionStructureBullish(t *testing.T) {
	session := models.Session{
		High:   110,
		Low:    90,
		VWAP:   99,
		POC:    102,
		VAH:    106,
		VAL:    96,
		Fib382: 97.64,
		Fib500: 100,
		Fib618: 102.36,
	}

	plan := models.TradePlan{
		Direction: models.BiasLong,
		Entry:     100,
		Stop:      95,
		TP1:       110,
	}

	got, err := BuildAuctionStructure(session, plan)
	if err != nil {
		t.Fatalf("BuildAuctionStructure returned error: %v", err)
	}

	if got.Bias != models.BiasLong {
		t.Fatalf("Bias = %v, want %v", got.Bias, models.BiasLong)
	}

	assertFloatEqual(t, "OvernightHigh", got.OvernightHigh, 110)
	assertFloatEqual(t, "OvernightLow", got.OvernightLow, 90)
	assertFloatEqual(t, "OvernightRange", got.OvernightRange, 20)

	assertFloatEqual(t, "VWAP", got.VWAP, 99)
	assertFloatEqual(t, "POC", got.POC, 102)
	assertFloatEqual(t, "VAH", got.VAH, 106)
	assertFloatEqual(t, "VAL", got.VAL, 96)

	assertFloatEqual(t, "Fib382", got.Fib382, 97.64)
	assertFloatEqual(t, "Fib500", got.Fib500, 100)
	assertFloatEqual(t, "Fib618", got.Fib618, 102.36)

	assertFloatEqual(t, "Entry", got.Entry, 100)
	assertFloatEqual(t, "Stop", got.Stop, 95)
	assertFloatEqual(t, "TP1", got.TP1, 110)

	assertPosition(t, "POCVsEntry", got.POCVsEntry, PositionAbove)
	assertPosition(t, "POCVsTP1", got.POCVsTP1, PositionBelow)
	assertPosition(t, "VWAPVsEntry", got.VWAPVsEntry, PositionBelow)
	assertPosition(t, "Fib618VsPOC", got.Fib618VsPOC, PositionAbove)
	assertPosition(t, "VAHVsEntry", got.VAHVsEntry, PositionAbove)
	assertPosition(t, "VALVsEntry", got.VALVsEntry, PositionBelow)

	assertFloatEqual(t, "EntryToPOCR", got.EntryToPOCR, 0.4)
	assertFloatEqual(t, "EntryToVWAPR", got.EntryToVWAPR, 0.2)
	assertFloatEqual(t, "POCToTP1R", got.POCToTP1R, 1.6)
	assertFloatEqual(t, "VAHToTP1R", got.VAHToTP1R, 0.8)
	assertFloatEqual(t, "VALToEntryR", got.VALToEntryR, 0.8)
	assertFloatEqual(t, "Fib618ToPOCR", got.Fib618ToPOCR, 0.072)

	assertBool(t, "EntryInsideValue", got.EntryInsideValue, true)
	assertBool(t, "EntryAboveVAH", got.EntryAboveVAH, false)
	assertBool(t, "EntryBelowVAL", got.EntryBelowVAL, false)
	assertBool(t, "POCBetweenEntryAndTP1", got.POCBetweenEntryAndTP1, true)
	assertBool(t, "POCBehindEntry", got.POCBehindEntry, false)
	assertBool(t, "POCBeyondTP1", got.POCBeyondTP1, false)
	assertBool(t, "Fib618AbovePOC", got.Fib618AbovePOC, true)
	assertBool(t, "Fib618BelowPOC", got.Fib618BelowPOC, false)
	assertBool(t, "VWAPSupportsDirection", got.VWAPSupportsDirection, true)
}

func TestBuildAuctionStructureBearish(t *testing.T) {
	session := models.Session{
		High:   210,
		Low:    190,
		VWAP:   202,
		POC:    198,
		VAH:    205,
		VAL:    195,
		Fib382: 202.36,
		Fib500: 200,
		Fib618: 197.64,
	}

	plan := models.TradePlan{
		Direction: models.BiasShort,
		Entry:     200,
		Stop:      205,
		TP1:       190,
	}

	got, err := BuildAuctionStructure(session, plan)
	if err != nil {
		t.Fatalf("BuildAuctionStructure returned error: %v", err)
	}

	if got.Bias != models.BiasShort {
		t.Fatalf("Bias = %v, want %v", got.Bias, models.BiasShort)
	}

	assertPosition(t, "POCVsEntry", got.POCVsEntry, PositionBelow)
	assertPosition(t, "POCVsTP1", got.POCVsTP1, PositionAbove)
	assertPosition(t, "VWAPVsEntry", got.VWAPVsEntry, PositionAbove)
	assertPosition(t, "Fib618VsPOC", got.Fib618VsPOC, PositionBelow)
	assertPosition(t, "VAHVsEntry", got.VAHVsEntry, PositionAbove)
	assertPosition(t, "VALVsEntry", got.VALVsEntry, PositionBelow)

	assertFloatEqual(t, "EntryToPOCR", got.EntryToPOCR, 0.4)
	assertFloatEqual(t, "EntryToVWAPR", got.EntryToVWAPR, 0.4)
	assertFloatEqual(t, "POCToTP1R", got.POCToTP1R, 1.6)
	assertFloatEqual(t, "VAHToTP1R", got.VAHToTP1R, 3)
	assertFloatEqual(t, "VALToEntryR", got.VALToEntryR, 1)
	assertFloatEqual(t, "Fib618ToPOCR", got.Fib618ToPOCR, 0.072)

	assertBool(t, "EntryInsideValue", got.EntryInsideValue, true)
	assertBool(t, "EntryAboveVAH", got.EntryAboveVAH, false)
	assertBool(t, "EntryBelowVAL", got.EntryBelowVAL, false)
	assertBool(t, "POCBetweenEntryAndTP1", got.POCBetweenEntryAndTP1, true)
	assertBool(t, "POCBehindEntry", got.POCBehindEntry, false)
	assertBool(t, "POCBeyondTP1", got.POCBeyondTP1, false)
	assertBool(t, "Fib618AbovePOC", got.Fib618AbovePOC, false)
	assertBool(t, "Fib618BelowPOC", got.Fib618BelowPOC, true)
	assertBool(t, "VWAPSupportsDirection", got.VWAPSupportsDirection, true)
}

func TestRelationshipHelpersEqualityTolerance(t *testing.T) {
	const tolerance = 0.001

	if !EqualWithinTolerance(100, 100.0005, tolerance) {
		t.Fatal("EqualWithinTolerance should treat values inside tolerance as equal")
	}

	if EqualWithinTolerance(100, 100.01, tolerance) {
		t.Fatal("EqualWithinTolerance should reject values outside tolerance")
	}

	got := RelativePosition(100.0005, 100, tolerance)
	if got != PositionEqual {
		t.Fatalf("RelativePosition = %v, want %v", got, PositionEqual)
	}

	if !Between(100.0005, 90, 100, tolerance) {
		t.Fatal("Between should include a value within endpoint tolerance")
	}

	if !Between(95, 100, 90, tolerance) {
		t.Fatal("Between should work when endpoints are reversed")
	}

	if Between(101, 90, 100, tolerance) {
		t.Fatal("Between should reject values outside the range")
	}
}

func TestBuildAuctionStructureZeroRisk(t *testing.T) {
	session := validSession()
	plan := models.TradePlan{
		Direction: models.BiasLong,
		Entry:     100,
		Stop:      100,
		TP1:       110,
	}

	got, err := BuildAuctionStructure(session, plan)
	if err != nil {
		t.Fatalf("BuildAuctionStructure returned error: %v", err)
	}

	distances := map[string]float64{
		"EntryToPOCR":  got.EntryToPOCR,
		"EntryToVWAPR": got.EntryToVWAPR,
		"POCToTP1R":    got.POCToTP1R,
		"VAHToTP1R":    got.VAHToTP1R,
		"VALToEntryR":  got.VALToEntryR,
		"Fib618ToPOCR": got.Fib618ToPOCR,
	}

	for name, value := range distances {
		if value != 0 {
			t.Errorf("%s = %v, want 0 for zero-risk plan", name, value)
		}

		if math.IsNaN(value) {
			t.Errorf("%s unexpectedly produced NaN", name)
		}

		if math.IsInf(value, 0) {
			t.Errorf("%s unexpectedly produced infinity", name)
		}
	}
}

func TestBuildAuctionStructureRejectsInvalidLevels(t *testing.T) {
	tests := []struct {
		name        string
		session     models.Session
		plan        models.TradePlan
		wantMessage string
	}{
		{
			name: "zero overnight high",
			session: func() models.Session {
				s := validSession()
				s.High = 0
				return s
			}(),
			plan:        validLongPlan(),
			wantMessage: "overnight high must be positive",
		},
		{
			name: "negative POC",
			session: func() models.Session {
				s := validSession()
				s.POC = -1
				return s
			}(),
			plan:        validLongPlan(),
			wantMessage: "POC must be positive",
		},
		{
			name:    "zero entry",
			session: validSession(),
			plan: func() models.TradePlan {
				p := validLongPlan()
				p.Entry = 0
				return p
			}(),
			wantMessage: "entry must be positive",
		},
		{
			name: "NaN VWAP",
			session: func() models.Session {
				s := validSession()
				s.VWAP = math.NaN()
				return s
			}(),
			plan:        validLongPlan(),
			wantMessage: "VWAP must be finite",
		},
		{
			name: "infinite Fib618",
			session: func() models.Session {
				s := validSession()
				s.Fib618 = math.Inf(1)
				return s
			}(),
			plan:        validLongPlan(),
			wantMessage: "Fib618 must be finite",
		},
		{
			name: "inverted overnight range",
			session: func() models.Session {
				s := validSession()
				s.High = 90
				s.Low = 110
				return s
			}(),
			plan:        validLongPlan(),
			wantMessage: "overnight high",
		},
		{
			name: "inverted value area",
			session: func() models.Session {
				s := validSession()
				s.VAH = 95
				s.VAL = 105
				return s
			}(),
			plan:        validLongPlan(),
			wantMessage: "VAH",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := BuildAuctionStructure(test.session, test.plan)
			if err == nil {
				t.Fatal("BuildAuctionStructure returned nil error")
			}

			if !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf(
					"error = %q, want message containing %q",
					err.Error(),
					test.wantMessage,
				)
			}
		})
	}
}

func TestPositionString(t *testing.T) {
	tests := []struct {
		position Position
		want     string
	}{
		{PositionUnknown, "UNKNOWN"},
		{PositionBelow, "BELOW"},
		{PositionEqual, "EQUAL"},
		{PositionAbove, "ABOVE"},
		{Position(99), "UNKNOWN"},
	}

	for _, test := range tests {
		if got := test.position.String(); got != test.want {
			t.Errorf(
				"Position(%d).String() = %q, want %q",
				test.position,
				got,
				test.want,
			)
		}
	}
}

func TestRelationshipHelpersRejectNonFiniteValues(t *testing.T) {
	if EqualWithinTolerance(math.NaN(), 100, DefaultTolerance) {
		t.Fatal("EqualWithinTolerance should reject NaN")
	}

	if EqualWithinTolerance(math.Inf(1), 100, DefaultTolerance) {
		t.Fatal("EqualWithinTolerance should reject infinity")
	}

	if got := RelativePosition(math.NaN(), 100, DefaultTolerance); got != PositionUnknown {
		t.Fatalf("RelativePosition with NaN = %v, want PositionUnknown", got)
	}

	if Between(math.NaN(), 90, 100, DefaultTolerance) {
		t.Fatal("Between should reject NaN")
	}
}

func validSession() models.Session {
	return models.Session{
		High:   110,
		Low:    90,
		VWAP:   99,
		POC:    102,
		VAH:    106,
		VAL:    96,
		Fib382: 97.64,
		Fib500: 100,
		Fib618: 102.36,
	}
}

func validLongPlan() models.TradePlan {
	return models.TradePlan{
		Direction: models.BiasLong,
		Entry:     100,
		Stop:      95,
		TP1:       110,
	}
}

func assertFloatEqual(t *testing.T, name string, got, want float64) {
	t.Helper()

	const tolerance = 0.000000001

	if math.Abs(got-want) > tolerance {
		t.Errorf("%s = %.12f, want %.12f", name, got, want)
	}
}

func assertPosition(t *testing.T, name string, got, want Position) {
	t.Helper()

	if got != want {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func assertBool(t *testing.T, name string, got, want bool) {
	t.Helper()

	if got != want {
		t.Errorf("%s = %t, want %t", name, got, want)
	}
}
