package live

import (
	"fmt"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/liquidity"
	"github.com/ogtrading/overnight-strategy/internal/models"
	"github.com/ogtrading/overnight-strategy/internal/session"
	"github.com/ogtrading/overnight-strategy/internal/strategy"
	"github.com/ogtrading/overnight-strategy/internal/universe"
)

// BuildMarketSnapshotForDate builds the market map for one completed overnight
// session date. The session date is the morning on which the 19:00-05:00 CT
// overnight session ends.
func BuildMarketSnapshotForDate(
	symbol string,
	candles []models.Candle,
	location *time.Location,
	sessionDate time.Time,
) (MarketSnapshot, error) {
	if location == nil {
		return MarketSnapshot{}, fmt.Errorf("location is required")
	}

	asset, ok := universe.Find(symbol)
	if !ok {
		return MarketSnapshot{}, fmt.Errorf("asset %s is not registered", symbol)
	}

	localDate := sessionDate.In(location)
	target := time.Date(
		localDate.Year(),
		localDate.Month(),
		localDate.Day(),
		0,
		0,
		0,
		0,
		location,
	)

	sessions, err := session.BuildOvernightSessions(candles, location)
	if err != nil {
		return MarketSnapshot{}, err
	}

	var selected *models.Session

	for index := range sessions {
		candidate := sessions[index]
		candidateDate := candidate.Date.In(location)

		if candidateDate.Year() == target.Year() &&
			candidateDate.Month() == target.Month() &&
			candidateDate.Day() == target.Day() {
			selected = &candidate
			break
		}
	}

	if selected == nil {
		return MarketSnapshot{}, fmt.Errorf(
			"no complete overnight session for %s on %s",
			symbol,
			target.Format("2006-01-02"),
		)
	}

	value, err := strategy.AnalyzeSession(*selected)
	if err != nil {
		return MarketSnapshot{}, err
	}

	previous, err := calculatePreviousDay(candles, value.Date, location)
	if err != nil {
		return MarketSnapshot{}, err
	}

	snapshot := MarketSnapshot{
		Timestamp:         time.Now().UTC(),
		Symbol:            symbol,
		Classification:    string(asset.Classification),
		SessionDate:       value.Date,
		OvernightHigh:     value.High,
		OvernightLow:      value.Low,
		OvernightRange:    value.High - value.Low,
		OvernightMidpoint: (value.High + value.Low) / 2,
		SessionClose:      value.Close,
		Fib382:            value.Fib382,
		Fib500:            value.Fib500,
		Fib618:            value.Fib618,
		VWAP:              value.VWAP,
		POC:               value.POC,
		VAH:               value.VAH,
		VAL:               value.VAL,
		PreviousDay:       previous,
		Liquidity:         liquidity.DetectLevels(value.Candles),
		OrderAuthorized:   asset.Tradable,
	}

	// Build the frozen baseline plan for every monitored asset.
	//
	// IMPORTANT:
	// Plan generation does NOT grant execution authority.
	// Research and observe-only assets receive hypothetical plans
	// so their baseline performance can be measured identically to
	// live assets, while OrderAuthorized remains false.
	plan := strategy.BuildTradePlan(
		value,
		strategy.DefaultStopBufferBPS,
	)
	snapshot.Plan = &plan

	return snapshot, nil
}
