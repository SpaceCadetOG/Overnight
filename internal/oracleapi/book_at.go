package oracleapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/oracle/book"
	"github.com/ogtrading/overnight-strategy/internal/oracle/model"
)

func (s *Server) bookAt(w http.ResponseWriter, r *http.Request) {
	asset := strings.ToUpper(strings.TrimSpace(r.PathValue("asset")))
	at, err := time.Parse(time.RFC3339Nano, r.URL.Query().Get("at"))
	if asset == "" || err != nil {
		writeError(w, 400, errors.New("asset and RFC3339 at are required"))
		return
	}
	location, _ := time.LoadLocation("America/Chicago")
	local := at.In(location)
	from := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location).UTC()
	query := eventQuery{Asset: asset, Streams: map[model.Stream]bool{model.StreamBookSnapshot: true, model.StreamBookDelta: true}, From: from, To: at.UTC().Add(time.Nanosecond), Limit: int(^uint(0) >> 1), AllowUncertified: r.URL.Query().Get("allow_uncertified") == "true"}
	result, status, err := s.runQuery(r, query)
	if err != nil {
		writeError(w, status, err)
		return
	}
	state := book.New(asset)
	sequence := uint64(0)
	connection := ""
	var appliedAt time.Time
	for _, event := range result.Events {
		var payload model.Book
		if json.Unmarshal(event.Payload, &payload) != nil {
			continue
		}
		sequence++
		if event.Stream == model.StreamBookSnapshot {
			err = state.ApplySnapshot(payload, sequence)
		} else {
			err = state.ApplyDelta(payload, sequence)
		}
		if err != nil {
			writeError(w, 422, err)
			return
		}
		connection, appliedAt = event.ConnectionID, eventTime(event)
	}
	if sequence == 0 {
		writeError(w, 404, errors.New("no book snapshot at or before requested timestamp"))
		return
	}
	snapshot, err := state.Snapshot(0, appliedAt, connection)
	if err != nil {
		writeError(w, 422, err)
		return
	}
	writeJSON(w, 200, map[string]any{"schema_version": "oracle-book-at-v1", "asset": asset, "requested_at": at.UTC(), "book_at": appliedAt, "book": snapshot, "quality": result.Quality, "packages": result.Packages, "excluded_events": result.Excluded, "information_cutoff": at.UTC()})
}
