package oracleapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const tapeSchema = "oracle-tape-v1"
const maxTapeRange = 15 * time.Minute
const maxTapeTrades = 250_000

type tapeTrade struct {
	At            time.Time `json:"at"`
	Price         string    `json:"price"`
	Size          string    `json:"size"`
	AggressorSide string    `json:"aggressor_side"`
}

// tape exposes individual, ordered trades from the checksum-bound hourly
// index. It is intentionally read-only and bounded for live-style consumers.
// Clients advance the information cutoff in adjacent windows and must only
// apply events whose recorded timestamp has been reached.
func (s *Server) tape(w http.ResponseWriter, r *http.Request) {
	in, err := parseAnalyticsInput(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if in.To.Sub(in.From) > maxTapeRange {
		writeError(w, http.StatusBadRequest, errors.New("tape queries are limited to 15m; request adjacent windows"))
		return
	}
	select {
	case s.querySlot <- struct{}{}:
		defer func() { <-s.querySlot }()
	default:
		w.Header().Set("Retry-After", "5")
		writeError(w, http.StatusTooManyRequests, errors.New("historical query already running"))
		return
	}
	trades := make([]tapeTrade, 0, 4096)
	overflow := false
	source, status, err := s.walkAnalyticTrades(r, in, func(trade analyticTrade) {
		if overflow {
			return
		}
		if len(trades) >= maxTapeTrades {
			overflow = true
			return
		}
		side := "SELL"
		if trade.Buy {
			side = "BUY"
		}
		trades = append(trades, tapeTrade{At: trade.At, Price: strconv.FormatFloat(trade.Price, 'f', -1, 64), Size: strconv.FormatFloat(trade.Size, 'f', -1, 64), AggressorSide: side})
	})
	if err != nil {
		writeError(w, status, err)
		return
	}
	if overflow {
		writeError(w, http.StatusRequestEntityTooLarge, errors.New("tape window contains too many trades; request a smaller window"))
		return
	}
	for _, quality := range source.Quality {
		if quality != "CERTIFIED" && quality != "CERTIFIED_WITH_EXCLUDED_INTERVALS" {
			writeError(w, http.StatusUnprocessableEntity, errors.New("tape is not certified for replay"))
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version":     tapeSchema,
		"type":               "TRADE_TAPE",
		"asset":              strings.ToUpper(in.Asset),
		"from":               in.From,
		"to":                 in.To,
		"trades":             trades,
		"count":              len(trades),
		"quality":            source.Quality,
		"packages":           source.Packages,
		"excluded_events":    source.Excluded,
		"query_mode":         source.QueryMode,
		"coverage":           source.Coverage,
		"information_cutoff": in.To,
	})
}
