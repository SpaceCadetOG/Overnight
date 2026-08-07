package strategy

import (
	"fmt"

	"github.com/ogtrading/overnight-strategy/internal/indicators"
	"github.com/ogtrading/overnight-strategy/internal/models"
)

// AnalyzeSession calculates the session indicators and planned entry.
func AnalyzeSession(session models.Session) (models.Session, error) {
	vwap, err := indicators.VWAP(session.Candles)
	if err != nil {
		return models.Session{}, fmt.Errorf(
			"calculate VWAP: %w",
			err,
		)
	}

	fib, err := indicators.CalculateFibonacci(session)
	if err != nil {
		return models.Session{}, fmt.Errorf(
			"calculate Fibonacci: %w",
			err,
		)
	}

	profile, err := indicators.CalculateVolumeProfile(
		session.Candles,
		indicators.DefaultProfileBins,
		indicators.DefaultValueArea,
	)
	if err != nil {
		return models.Session{}, fmt.Errorf(
			"calculate volume profile: %w",
			err,
		)
	}

	session.VWAP = vwap
	session.POC = profile.POC
	session.VAH = profile.VAH
	session.VAL = profile.VAL

	switch {
	case session.Close > vwap:
		session.Bias = models.BiasLong
		session.Fib382 = fib.Long382
		session.Fib500 = fib.Long500
		session.Fib618 = fib.Long618

	case session.Close < vwap:
		session.Bias = models.BiasShort
		session.Fib382 = fib.Short382
		session.Fib500 = fib.Short500
		session.Fib618 = fib.Short618

	default:
		session.Bias = models.BiasNone
	}

	if session.Bias != models.BiasNone {
		session.Entry = (session.Fib382 + session.Fib500) / 2
	}

	return session, nil
}
