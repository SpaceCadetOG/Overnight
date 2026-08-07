package strategy

import (
	"fmt"
	"math"

	"github.com/ogtrading/overnight-strategy/internal/models"
)

type EntryMethod string
type StopMethod string
type TP1Method string

const (
	EntryMidpoint EntryMethod = "MIDPOINT_382_500"
	EntryFib382   EntryMethod = "FIB_382"
	EntryFib500   EntryMethod = "FIB_500"

	StopProfileFib StopMethod = "PROFILE_FIB"
	StopFib382     StopMethod = "FIB_382"
	StopValueArea  StopMethod = "VALUE_AREA"
	StopFVGClose   StopMethod = "FVG_CLOSE"

	TP1Fib618       TP1Method = "FIB_618"
	TP1POC          TP1Method = "POC"
	TP1NearestValid TP1Method = "NEAREST_VALID"
)

const DefaultStopBufferBPS = 2.0

type PlanConfig struct {
	EntryMethod   EntryMethod
	StopMethod    StopMethod
	TP1Method     TP1Method
	StopBufferBPS float64

	// POC cannot become TP1 unless its reward is at least this many R.
	MinimumPOCRR float64
}

func DefaultPlanConfig() PlanConfig {
	return PlanConfig{
		EntryMethod:   EntryMidpoint,
		StopMethod:    StopProfileFib,
		TP1Method:     TP1Fib618,
		StopBufferBPS: DefaultStopBufferBPS,
		MinimumPOCRR:  0.75,
	}
}

// BuildTradePlan preserves the original API.
func BuildTradePlan(
	session models.Session,
	stopBufferBPS float64,
) models.TradePlan {
	config := DefaultPlanConfig()
	config.StopBufferBPS = stopBufferBPS

	return BuildTradePlanWithConfig(session, config)
}

func BuildTradePlanWithConfig(
	session models.Session,
	config PlanConfig,
) models.TradePlan {
	plan := models.TradePlan{
		Date:      session.Date,
		Direction: session.Bias,
	}

	if config.StopBufferBPS < 0 {
		return invalidPlan(plan, "stop buffer cannot be negative")
	}

	if config.MinimumPOCRR < 0 {
		return invalidPlan(plan, "minimum POC RR cannot be negative")
	}

	entry, entrySource := calculateEntry(session, config.EntryMethod)
	plan.Entry = entry
	plan.EntrySource = entrySource

	switch session.Bias {
	case models.BiasLong:
		buildLongPlan(&plan, session, config)

	case models.BiasShort:
		buildShortPlan(&plan, session, config)

	default:
		return invalidPlan(plan, "session has no directional bias")
	}

	if err := plan.Validate(); err != nil {
		return invalidPlan(plan, err.Error())
	}

	calculateTradeMetrics(&plan)
	plan.Valid = true

	return plan
}

func calculateEntry(
	session models.Session,
	method EntryMethod,
) (float64, string) {
	switch method {
	case EntryFib382:
		return session.Fib382, string(EntryFib382)

	case EntryFib500:
		return session.Fib500, string(EntryFib500)

	case EntryMidpoint:
		fallthrough

	default:
		return (session.Fib382 + session.Fib500) / 2,
			string(EntryMidpoint)
	}
}

func buildLongPlan(
	plan *models.TradePlan,
	session models.Session,
	config PlanConfig,
) {
	stopReference, stopSource := longStopReference(
		session,
		config.StopMethod,
	)

	buffer := stopReference * config.StopBufferBPS / 10000

	if config.StopMethod == StopFVGClose && stopReference == 0 {
		plan.Valid = false
		plan.InvalidReason = "no active bullish FVG"
		return
	}

	plan.Stop = stopReference - buffer
	plan.StopSource = stopSource
	plan.TP2 = session.High

	setTP1(plan, session, config)
}

func buildShortPlan(
	plan *models.TradePlan,
	session models.Session,
	config PlanConfig,
) {
	stopReference, stopSource := shortStopReference(
		session,
		config.StopMethod,
	)

	buffer := stopReference * config.StopBufferBPS / 10000

	if config.StopMethod == StopFVGClose && stopReference == 0 {
		plan.Valid = false
		plan.InvalidReason = "no active bearish FVG"
		return
	}

	plan.Stop = stopReference + buffer
	plan.StopSource = stopSource
	plan.TP2 = session.Low

	setTP1(plan, session, config)
}

func longStopReference(
	session models.Session,
	method StopMethod,
) (float64, string) {
	switch method {
	case StopFib382:
		return session.Fib382, string(StopFib382)

	case StopValueArea:
		return session.VAL, string(StopValueArea)

	case StopFVGClose:
		stop, ok := longFVGStop(session)
		if !ok {
			return 0, string(StopFVGClose)
		}
		return stop, string(StopFVGClose)

	case StopProfileFib:
		fallthrough

	default:
		return math.Min(session.VAL, session.Fib382),
			string(StopProfileFib)
	}
}

func longFVGStop(
	session models.Session,
) (float64, bool) {
	beforeIndex := len(session.Candles) - 1

	fvg, ok := FindActiveBullishFVG(
		session.Candles,
		beforeIndex,
	)
	if !ok || !fvg.HasCloseObservation {
		return 0, false
	}

	return fvg.LowestClose, true
}

func shortFVGStop(
	session models.Session,
) (float64, bool) {
	beforeIndex := len(session.Candles) - 1

	fvg, ok := FindActiveBearishFVG(
		session.Candles,
		beforeIndex,
	)
	if !ok || !fvg.HasCloseObservation {
		return 0, false
	}

	return fvg.HighestClose, true
}

func shortStopReference(
	session models.Session,
	method StopMethod,
) (float64, string) {
	switch method {
	case StopFib382:
		return session.Fib382, string(StopFib382)

	case StopValueArea:
		return session.VAH, string(StopValueArea)

	case StopFVGClose:
		stop, ok := shortFVGStop(session)
		if !ok {
			return 0, string(StopFVGClose)
		}
		return stop, string(StopFVGClose)

	case StopProfileFib:
		fallthrough

	default:
		return math.Max(session.VAH, session.Fib382),
			string(StopProfileFib)
	}
}

func setTP1(
	plan *models.TradePlan,
	session models.Session,
	config PlanConfig,
) {
	switch config.TP1Method {
	case TP1POC:
		plan.TP1 = session.POC
		plan.TP1Source = string(TP1POC)

	case TP1NearestValid:
		setNearestValidTP1(plan, session, config.MinimumPOCRR)

	case TP1Fib618:
		fallthrough

	default:
		plan.TP1 = session.Fib618
		plan.TP1Source = string(TP1Fib618)
	}
}

func setNearestValidTP1(
	plan *models.TradePlan,
	session models.Session,
	minimumPOCRR float64,
) {
	plan.TP1 = session.Fib618
	plan.TP1Source = string(TP1Fib618)

	risk := math.Abs(plan.Entry - plan.Stop)
	if risk <= 0 {
		return
	}

	switch plan.Direction {
	case models.BiasLong:
		if session.POC <= plan.Entry {
			return
		}

		pocRR := (session.POC - plan.Entry) / risk
		fibDistance := session.Fib618 - plan.Entry
		pocDistance := session.POC - plan.Entry

		if pocRR >= minimumPOCRR && pocDistance < fibDistance {
			plan.TP1 = session.POC
			plan.TP1Source = string(TP1POC)
		}

	case models.BiasShort:
		if session.POC >= plan.Entry {
			return
		}

		pocRR := (plan.Entry - session.POC) / risk
		fibDistance := plan.Entry - session.Fib618
		pocDistance := plan.Entry - session.POC

		if pocRR >= minimumPOCRR && pocDistance < fibDistance {
			plan.TP1 = session.POC
			plan.TP1Source = string(TP1POC)
		}
	}
}

func calculateTradeMetrics(plan *models.TradePlan) {
	plan.RiskDistance = math.Abs(plan.Entry - plan.Stop)
	plan.Reward1Distance = math.Abs(plan.TP1 - plan.Entry)
	plan.Reward2Distance = math.Abs(plan.TP2 - plan.Entry)

	if plan.RiskDistance > 0 {
		plan.RR1 = plan.Reward1Distance / plan.RiskDistance
		plan.RR2 = plan.Reward2Distance / plan.RiskDistance
	}
}

func invalidPlan(
	plan models.TradePlan,
	reason string,
) models.TradePlan {
	plan.Valid = false
	plan.InvalidReason = reason
	return plan
}

func FormatTradePlan(plan models.TradePlan) string {
	if !plan.Valid {
		return fmt.Sprintf("INVALID — %s", plan.InvalidReason)
	}

	return fmt.Sprintf(
		"%s | Entry %.2f (%s) | Stop %.2f (%s) | "+
			"TP1 %.2f (%s) | TP2 %.2f | RR1 %.2f | RR2 %.2f",
		plan.Direction,
		plan.Entry,
		plan.EntrySource,
		plan.Stop,
		plan.StopSource,
		plan.TP1,
		plan.TP1Source,
		plan.TP2,
		plan.RR1,
		plan.RR2,
	)
}
