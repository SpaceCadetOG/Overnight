package oracleapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

var replayUpgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
	return r.Host == "" || r.Host == "127.0.0.1" || r.Host == "localhost" || len(r.Host) > 10 && (r.Host[:10] == "127.0.0.1:" || r.Host[:10] == "localhost:")
}}

func (s *Server) replay(w http.ResponseWriter, r *http.Request) {
	query, err := parseEventQuery(r, nil)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	speed := 1.0
	if raw := r.URL.Query().Get("speed"); raw != "" {
		speed, err = strconv.ParseFloat(raw, 64)
		if err != nil || speed <= 0 || speed > 10000 {
			writeError(w, 400, errInvalidReplaySpeed)
			return
		}
	}
	query.Limit = maxQueryLimit
	result, status, err := s.runQuery(r, query)
	if err != nil {
		writeError(w, status, err)
		return
	}
	conn, err := replayUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	if conn.WriteJSON(map[string]any{"type": "replay_start", "schema_version": "oracle-replay-v1", "asset": query.Asset, "from": query.From, "to": query.To, "speed": speed, "events": len(result.Events), "quality": result.Quality, "packages": result.Packages, "information_cutoff": query.To}) != nil {
		return
	}
	var prior time.Time
	for index, event := range result.Events {
		at := eventTime(event)
		if !prior.IsZero() {
			delay := time.Duration(float64(at.Sub(prior)) / speed)
			if delay > 0 {
				timer := time.NewTimer(delay)
				select {
				case <-r.Context().Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		}
		if conn.WriteJSON(map[string]any{"type": "replay_event", "index": index, "event": event}) != nil {
			return
		}
		prior = at
	}
	_ = conn.WriteJSON(map[string]any{"type": "replay_end", "events": len(result.Events), "at": query.To})
}

var errInvalidReplaySpeed = &replayParameterError{"speed must be greater than 0 and no greater than 10000"}

type replayParameterError struct{ message string }

func (e *replayParameterError) Error() string { return e.message }
