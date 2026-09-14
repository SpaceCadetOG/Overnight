package oracleapi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"time"
)

const zoneModelVersion = "profile-cluster-v1"

type namedLevel struct {
	ID     string    `json:"id"`
	Type   string    `json:"type"`
	Price  string    `json:"price"`
	Period string    `json:"period"`
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
	Status string    `json:"status"`
}

type structuralZone struct {
	ID       string       `json:"id"`
	Low      string       `json:"low"`
	High     string       `json:"high"`
	Center   string       `json:"center"`
	Strength int          `json:"strength"`
	Sources  []namedLevel `json:"sources"`
}

func (s *Server) levels(w http.ResponseWriter, r *http.Request) { s.serveStructure(w, r, false) }
func (s *Server) zones(w http.ResponseWriter, r *http.Request)  { s.serveStructure(w, r, true) }

func (s *Server) serveStructure(w http.ResponseWriter, r *http.Request, includeZones bool) {
	in, err := parseAnalyticsInput(r)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	trades, source, status, err := s.analyticTrades(r, in)
	if err != nil {
		writeError(w, status, err)
		return
	}
	profile := buildProfile(trades, 0.70)
	levels := levelsFromProfile(in, profile)
	response := map[string]any{"schema_version": "oracle-market-structure-v1", "asset": in.Asset, "from": in.From, "to": in.To, "period": in.Period, "session_definition_version": sessionDefinitionVersion, "zone_model": zoneModelVersion, "levels": levels, "quality": source.Quality, "packages": source.Packages, "excluded_events": source.Excluded, "information_cutoff": in.To}
	if includeZones {
		tolerance := 10.0
		if raw := r.URL.Query().Get("tolerance_bps"); raw != "" {
			tolerance, err = strconv.ParseFloat(raw, 64)
			if err != nil || tolerance <= 0 || tolerance > 100 {
				writeError(w, 400, errors.New("tolerance_bps must be greater than 0 and no greater than 100"))
				return
			}
		}
		response["zones"] = clusterNamedLevels(in.Asset, levels, tolerance)
		response["tolerance_bps"] = tolerance
	}
	writeJSON(w, 200, response)
}

func levelsFromProfile(in analyticsInput, profile map[string]any) []namedLevel {
	out := []namedLevel{}
	add := func(kind, price string) {
		if price == "" {
			return
		}
		raw := in.Asset + ":" + in.Period + ":" + kind + ":" + price + ":" + in.To.Format(time.RFC3339Nano)
		sum := sha256.Sum256([]byte(raw))
		out = append(out, namedLevel{ID: "lvl_" + hex.EncodeToString(sum[:8]), Type: kind, Price: price, Period: in.Period, From: in.From, To: in.To, Status: "FROZEN"})
	}
	for _, kind := range []string{"open", "high", "low", "close", "vwap", "poc", "vah", "val"} {
		value, _ := profile[kind].(string)
		add(kindName(kind), value)
	}
	for _, spec := range []struct{ key, prefix string }{{"hvns", "HVN"}, {"lvns", "LVN"}} {
		if values, ok := profile[spec.key].([]string); ok {
			for i, value := range values {
				add(spec.prefix+strconv.Itoa(i+1), value)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := strconv.ParseFloat(out[i].Price, 64)
		b, _ := strconv.ParseFloat(out[j].Price, 64)
		return a < b
	})
	return out
}

func kindName(value string) string {
	names := map[string]string{"open": "OPEN", "high": "HIGH", "low": "LOW", "close": "CLOSE", "vwap": "VWAP", "poc": "POC", "vah": "VAH", "val": "VAL"}
	return names[value]
}

func clusterNamedLevels(asset string, levels []namedLevel, toleranceBPS float64) []structuralZone {
	groups := [][]namedLevel{}
	for _, level := range levels {
		price, _ := strconv.ParseFloat(level.Price, 64)
		if len(groups) == 0 {
			groups = append(groups, []namedLevel{level})
			continue
		}
		prior := groups[len(groups)-1]
		center := 0.0
		for _, v := range prior {
			p, _ := strconv.ParseFloat(v.Price, 64)
			center += p
		}
		center /= float64(len(prior))
		if center > 0 && (price-center)/center*10000 <= toleranceBPS {
			groups[len(groups)-1] = append(prior, level)
		} else {
			groups = append(groups, []namedLevel{level})
		}
	}
	out := make([]structuralZone, 0, len(groups))
	for _, group := range groups {
		low, _ := strconv.ParseFloat(group[0].Price, 64)
		high, _ := strconv.ParseFloat(group[len(group)-1].Price, 64)
		center := (low + high) / 2
		raw := asset + ":" + zoneModelVersion + ":" + decimal(low) + ":" + decimal(high)
		sum := sha256.Sum256([]byte(raw))
		out = append(out, structuralZone{ID: "zone_" + hex.EncodeToString(sum[:8]), Low: decimal(low), High: decimal(high), Center: decimal(center), Strength: len(group), Sources: group})
	}
	return out
}
