package oracleapi

import (
	"errors"
	"net/http"
	"strings"
	"time"
)

const sessionDefinitionVersion = "global-sessions-v1"

type sessionDefinition struct {
	Type      string `json:"type"`
	Timezone  string `json:"timezone"`
	Start     string `json:"local_start,omitempty"`
	End       string `json:"local_end,omitempty"`
	Rolling   string `json:"rolling_duration,omitempty"`
	Previous  bool   `json:"previous,omitempty"`
	Semantics string `json:"semantics"`
}

type sessionInstance struct {
	ID                       string    `json:"id"`
	Type                     string    `json:"type"`
	DefinitionVersion        string    `json:"session_definition_version"`
	Timezone                 string    `json:"timezone"`
	TradingDate              string    `json:"trading_date"`
	LocalStart               string    `json:"local_start"`
	LocalEnd                 string    `json:"local_end"`
	UTCStart                 time.Time `json:"utc_start"`
	UTCEnd                   time.Time `json:"utc_end"`
	UTCOffsetSeconds         int       `json:"utc_offset_seconds"`
	Status                   string    `json:"status"`
	InformationCutoff        time.Time `json:"information_cutoff"`
	CoverageExpectedComplete bool      `json:"coverage_expected_complete"`
}

var canonicalDefinitions = []sessionDefinition{
	{"YTD", "UTC", "01-01 00:00", "request time", "", false, "calendar year to information cutoff"},
	{"30D", "UTC", "", "", "720h", false, "rolling 30 days"},
	{"14D", "UTC", "", "", "336h", false, "rolling 14 days"},
	{"7D", "UTC", "", "", "168h", false, "rolling 7 days"},
	{"24H", "America/Chicago", "19:00", "19:00", "", false, "most recently started Chicago trading day"},
	{"12H", "America/Chicago", "19:00", "07:00", "", false, "overnight half-day"},
	{"PM", "America/Chicago", "07:00", "08:30", "", false, "pre-US window"},
	{"US", "America/New_York", "09:30", "16:00", "", false, "US cash session"},
	{"PREV_US", "America/New_York", "09:30", "16:00", "", true, "previous US cash session"},
	{"ASIA", "Asia/Tokyo", "09:00", "15:30", "", false, "Tokyo cash session"},
	{"PREV_ASIA", "Asia/Tokyo", "09:00", "15:30", "", true, "previous Tokyo cash session"},
	{"LONDON", "Europe/London", "08:00", "16:30", "", false, "London session"},
	{"PREV_LONDON", "Europe/London", "08:00", "16:30", "", true, "previous London session"},
}

func findSessionDefinition(value string) (sessionDefinition, bool) {
	for _, definition := range canonicalDefinitions {
		if definition.Type == value {
			return definition, true
		}
	}
	return sessionDefinition{}, false
}

func (s *Server) sessionDefinitions(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"schema_version": "oracle-session-definitions-v1", "session_definition_version": sessionDefinitionVersion, "definitions": canonicalDefinitions})
}

func (s *Server) sessions(w http.ResponseWriter, r *http.Request) {
	at := time.Now().UTC()
	var err error
	if raw := r.URL.Query().Get("at"); raw != "" {
		at, err = time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			writeError(w, 400, errors.New("at must be RFC3339"))
			return
		}
	}
	wanted := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("type")))
	instances := []sessionInstance{}
	for _, definition := range canonicalDefinitions {
		if wanted != "" && definition.Type != wanted {
			continue
		}
		instance, err := resolveSession(definition, at.UTC())
		if err != nil {
			writeError(w, 500, err)
			return
		}
		instances = append(instances, instance)
	}
	if wanted != "" && len(instances) == 0 {
		writeError(w, 404, errors.New("unknown session type"))
		return
	}
	writeJSON(w, 200, map[string]any{"schema_version": "oracle-session-instances-v1", "session_definition_version": sessionDefinitionVersion, "as_of": at.UTC(), "sessions": instances})
}

func resolveSession(definition sessionDefinition, at time.Time) (sessionInstance, error) {
	if definition.Rolling != "" {
		duration, _ := time.ParseDuration(definition.Rolling)
		return makeSessionInstance(definition, at.Add(-duration), at, at), nil
	}
	if definition.Type == "YTD" {
		start := time.Date(at.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		return makeSessionInstance(definition, start, at, at), nil
	}
	location, err := time.LoadLocation(definition.Timezone)
	if err != nil {
		return sessionInstance{}, err
	}
	local := at.In(location)
	startHour, startMinute := parseClock(definition.Start)
	endHour, endMinute := parseClock(definition.End)
	start := time.Date(local.Year(), local.Month(), local.Day(), startHour, startMinute, 0, 0, location)
	if local.Before(start) {
		start = start.AddDate(0, 0, -1)
	}
	end := time.Date(start.Year(), start.Month(), start.Day(), endHour, endMinute, 0, 0, location)
	if !end.After(start) {
		end = end.AddDate(0, 0, 1)
	}
	if definition.Previous {
		start, end = start.AddDate(0, 0, -1), end.AddDate(0, 0, -1)
	}
	return makeSessionInstance(definition, start, end, at), nil
}

func makeSessionInstance(definition sessionDefinition, start, end, cutoff time.Time) sessionInstance {
	status := "DEVELOPING"
	if !end.After(cutoff) {
		status = "COMPLETED"
	}
	_, offset := start.Zone()
	tradingDate := end.In(start.Location()).Format("2006-01-02")
	return sessionInstance{ID: definition.Type + ":" + tradingDate, Type: definition.Type, DefinitionVersion: sessionDefinitionVersion, Timezone: definition.Timezone, TradingDate: tradingDate, LocalStart: start.Format(time.RFC3339), LocalEnd: end.Format(time.RFC3339), UTCStart: start.UTC(), UTCEnd: end.UTC(), UTCOffsetSeconds: offset, Status: status, InformationCutoff: cutoff.UTC(), CoverageExpectedComplete: status == "COMPLETED"}
}

func parseClock(value string) (int, int) {
	parsed, _ := time.Parse("15:04", value)
	return parsed.Hour(), parsed.Minute()
}
