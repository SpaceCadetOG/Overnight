package live

import (
	"fmt"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/indicators"
	"github.com/ogtrading/overnight-strategy/internal/liquidity"
	"github.com/ogtrading/overnight-strategy/internal/models"
	"github.com/ogtrading/overnight-strategy/internal/session"
	"github.com/ogtrading/overnight-strategy/internal/strategy"
	"github.com/ogtrading/overnight-strategy/internal/universe"
)

type PreviousDay struct {
	Open, High, Low, Close float64
	VWAP, POC, VAH, VAL    float64
}

type MarketSnapshot struct {
	SchemaVersion     int               `json:"schema_version"`
	StrategyVersion   string            `json:"strategy_version,omitempty"`
	SessionID         string            `json:"session_id,omitempty"`
	OpportunityID     string            `json:"opportunity_id,omitempty"`
	Timestamp         time.Time         `json:"timestamp"`
	Symbol            string            `json:"symbol"`
	Classification    string            `json:"classification"`
	SessionDate       time.Time         `json:"session_date"`
	OvernightHigh     float64           `json:"overnight_high"`
	OvernightLow      float64           `json:"overnight_low"`
	OvernightRange    float64           `json:"overnight_range"`
	OvernightMidpoint float64           `json:"overnight_midpoint"`
	SessionClose      float64           `json:"session_close"`
	Fib382            float64           `json:"fib_382"`
	Fib500            float64           `json:"fib_500"`
	Fib618            float64           `json:"fib_618"`
	VWAP              float64           `json:"vwap"`
	POC               float64           `json:"poc"`
	VAH               float64           `json:"vah"`
	VAL               float64           `json:"val"`
	PreviousDay       PreviousDay       `json:"previous_day"`
	Liquidity         []liquidity.Level `json:"liquidity"`
	Plan              *models.TradePlan `json:"plan,omitempty"`
	OrderAuthorized   bool              `json:"order_authorized"`
}

func BuildMarketSnapshot(symbol string, candles []models.Candle, location *time.Location) (MarketSnapshot, error) {
	asset, ok := universe.Find(symbol)
	if !ok {
		return MarketSnapshot{}, fmt.Errorf("asset %s is not registered", symbol)
	}
	sessions, err := session.BuildOvernightSessions(candles, location)
	if err != nil {
		return MarketSnapshot{}, err
	}
	if len(sessions) == 0 {
		return MarketSnapshot{}, fmt.Errorf("no complete overnight session for %s", symbol)
	}
	value, err := strategy.AnalyzeSession(sessions[len(sessions)-1])
	if err != nil {
		return MarketSnapshot{}, err
	}
	previous, err := calculatePreviousDay(candles, value.Date, location)
	if err != nil {
		return MarketSnapshot{}, err
	}
	snapshot := MarketSnapshot{
		SchemaVersion: 1, Timestamp: time.Now().UTC(), Symbol: symbol, Classification: string(asset.Classification), SessionDate: value.Date,
		OvernightHigh: value.High, OvernightLow: value.Low, OvernightRange: value.High - value.Low,
		OvernightMidpoint: (value.High + value.Low) / 2, SessionClose: value.Close,
		Fib382: value.Fib382, Fib500: value.Fib500, Fib618: value.Fib618,
		VWAP: value.VWAP, POC: value.POC, VAH: value.VAH, VAL: value.VAL,
		PreviousDay: previous, Liquidity: liquidity.DetectLevels(value.Candles), OrderAuthorized: asset.Tradable,
	}
	// Every observed market receives the frozen baseline plan for paper research.
	// OrderAuthorized remains the hard funded-execution boundary.
	plan := strategy.BuildTradePlan(value, strategy.DefaultStopBufferBPS)
	snapshot.Plan = &plan
	return snapshot, nil
}

func calculatePreviousDay(candles []models.Candle, sessionDate time.Time, location *time.Location) (PreviousDay, error) {
	day := sessionDate.In(location).AddDate(0, 0, -1)
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, location)
	end := start.AddDate(0, 0, 1)
	rows := []models.Candle{}
	for _, candle := range candles {
		local := candle.OpenTime.In(location)
		if !local.Before(start) && local.Before(end) {
			rows = append(rows, candle)
		}
	}
	if len(rows) == 0 {
		return PreviousDay{}, fmt.Errorf("previous-day candles unavailable")
	}
	value := PreviousDay{Open: rows[0].Open, High: rows[0].High, Low: rows[0].Low, Close: rows[len(rows)-1].Close}
	for _, candle := range rows[1:] {
		if candle.High > value.High {
			value.High = candle.High
		}
		if candle.Low < value.Low {
			value.Low = candle.Low
		}
	}
	vwap, err := indicators.VWAP(rows)
	if err != nil {
		return PreviousDay{}, err
	}
	profile, err := indicators.CalculateVolumeProfile(rows, indicators.DefaultProfileBins, indicators.DefaultValueArea)
	if err != nil {
		return PreviousDay{}, err
	}
	value.VWAP, value.POC, value.VAH, value.VAL = vwap, profile.POC, profile.VAH, profile.VAL
	return value, nil
}
