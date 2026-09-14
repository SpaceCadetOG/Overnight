package oracleapi

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/oracle/model"
)

const analyticsSchema = "oracle-analytics-v1"

type analyticTrade struct {
	At       time.Time
	Price    float64
	Size     float64
	Notional float64
	Buy      bool
}

type priceVolume struct {
	Price      string `json:"price"`
	BuyVolume  string `json:"buy_volume"`
	SellVolume string `json:"sell_volume"`
	Volume     string `json:"volume"`
	Delta      string `json:"delta"`
}

type candle struct {
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
	Open       string    `json:"open"`
	High       string    `json:"high"`
	Low        string    `json:"low"`
	Close      string    `json:"close"`
	Volume     string    `json:"volume"`
	BuyVolume  string    `json:"buy_volume"`
	SellVolume string    `json:"sell_volume"`
	Delta      string    `json:"delta"`
	CVD        string    `json:"cvd"`
	Trades     int       `json:"trades"`
}

type footprint struct {
	Candle candle        `json:"candle"`
	Levels []priceVolume `json:"levels"`
}

type analyticsInput struct {
	Asset string
	From  time.Time
	To    time.Time
	Allow bool
}

func parseAnalyticsInput(r *http.Request) (analyticsInput, error) {
	asset := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("asset")))
	if asset == "" || strings.ContainsAny(asset, `/\\`) {
		return analyticsInput{}, errors.New("asset is required")
	}
	from, err := time.Parse(time.RFC3339Nano, r.URL.Query().Get("from"))
	if err != nil {
		return analyticsInput{}, errors.New("from must be RFC3339")
	}
	to, err := time.Parse(time.RFC3339Nano, r.URL.Query().Get("to"))
	if err != nil {
		return analyticsInput{}, errors.New("to must be RFC3339")
	}
	if !to.After(from) || to.Sub(from) > maxQueryRange {
		return analyticsInput{}, errors.New("time range must be positive and no greater than 24h")
	}
	return analyticsInput{Asset: asset, From: from.UTC(), To: to.UTC(), Allow: r.URL.Query().Get("allow_uncertified") == "true"}, nil
}

func (s *Server) analyticTrades(r *http.Request, in analyticsInput) ([]analyticTrade, queryResponse, int, error) {
	query := eventQuery{Asset: in.Asset, Streams: map[model.Stream]bool{model.StreamTrade: true}, From: in.From, To: in.To, Limit: int(^uint(0) >> 1), AllowUncertified: in.Allow}
	result, status, err := s.runQuery(r, query)
	if err != nil {
		return nil, result, status, err
	}
	trades := make([]analyticTrade, 0, len(result.Events))
	for _, event := range result.Events {
		if event.Stream != model.StreamTrade {
			continue
		}
		var value model.Trade
		if json.Unmarshal(event.Payload, &value) != nil {
			continue
		}
		price, priceErr := strconv.ParseFloat(value.Price, 64)
		size, sizeErr := strconv.ParseFloat(value.Size, 64)
		if priceErr != nil || sizeErr != nil || price <= 0 || size <= 0 {
			continue
		}
		trades = append(trades, analyticTrade{At: eventTime(event), Price: price, Size: size, Notional: price * size, Buy: strings.EqualFold(value.AggressorSide, "BUY")})
	}
	return trades, result, 200, nil
}

func (s *Server) candles(w http.ResponseWriter, r *http.Request) {
	in, err := parseAnalyticsInput(r)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	interval, err := parseInterval(r.URL.Query().Get("interval"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	trades, source, status, err := s.analyticTrades(r, in)
	if err != nil {
		writeError(w, status, err)
		return
	}
	writeJSON(w, 200, map[string]any{"schema_version": analyticsSchema, "type": "CANDLES", "asset": in.Asset, "from": in.From, "to": in.To, "interval": interval.String(), "candles": buildCandles(trades, interval), "quality": source.Quality, "packages": source.Packages, "excluded_events": source.Excluded, "information_cutoff": in.To})
}

func (s *Server) profiles(w http.ResponseWriter, r *http.Request) {
	in, err := parseAnalyticsInput(r)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	valueArea := 0.70
	if raw := r.URL.Query().Get("value_area"); raw != "" {
		valueArea, err = strconv.ParseFloat(raw, 64)
		if err != nil || valueArea < 0.5 || valueArea >= 1 {
			writeError(w, 400, errors.New("value_area must be at least 0.5 and less than 1"))
			return
		}
	}
	trades, source, status, err := s.analyticTrades(r, in)
	if err != nil {
		writeError(w, status, err)
		return
	}
	profile := buildProfile(trades, valueArea)
	writeJSON(w, 200, map[string]any{"schema_version": analyticsSchema, "type": "VOLUME_PROFILE", "profile_model": "trade-volume-at-price-v1", "asset": in.Asset, "from": in.From, "to": in.To, "value_area_fraction": valueArea, "profile": profile, "quality": source.Quality, "packages": source.Packages, "excluded_events": source.Excluded, "information_cutoff": in.To})
}

func (s *Server) footprints(w http.ResponseWriter, r *http.Request) {
	in, err := parseAnalyticsInput(r)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	interval, err := parseInterval(r.URL.Query().Get("interval"))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	trades, source, status, err := s.analyticTrades(r, in)
	if err != nil {
		writeError(w, status, err)
		return
	}
	writeJSON(w, 200, map[string]any{"schema_version": analyticsSchema, "type": "FOOTPRINT", "asset": in.Asset, "from": in.From, "to": in.To, "interval": interval.String(), "footprints": buildFootprints(trades, interval), "quality": source.Quality, "packages": source.Packages, "excluded_events": source.Excluded, "information_cutoff": in.To})
}

func (s *Server) orderFlow(w http.ResponseWriter, r *http.Request) {
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
	var buy, sell, notional, cvd float64
	for _, trade := range trades {
		notional += trade.Notional
		if trade.Buy {
			buy += trade.Size
		} else {
			sell += trade.Size
		}
	}
	cvd = buy - sell
	duration := in.To.Sub(in.From).Seconds()
	writeJSON(w, 200, map[string]any{"schema_version": analyticsSchema, "type": "ORDER_FLOW", "asset": in.Asset, "from": in.From, "to": in.To, "trades": len(trades), "buy_volume": decimal(buy), "sell_volume": decimal(sell), "total_volume": decimal(buy + sell), "delta": decimal(cvd), "cvd": decimal(cvd), "delta_rate_per_second": decimal(cvd / duration), "notional": decimal(notional), "quality": source.Quality, "packages": source.Packages, "excluded_events": source.Excluded, "information_cutoff": in.To})
}

func parseInterval(raw string) (time.Duration, error) {
	if raw == "" {
		raw = "1m"
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value < time.Minute || value > 24*time.Hour || (24*time.Hour)%value != 0 {
		return 0, errors.New("interval must evenly divide 24h and be between 1m and 24h")
	}
	return value, nil
}

func buildCandles(trades []analyticTrade, interval time.Duration) []candle {
	out := []candle{}
	var current *candle
	var cvd float64
	for _, trade := range trades {
		start := trade.At.Truncate(interval)
		if current == nil || !current.Start.Equal(start) {
			out = append(out, candle{Start: start, End: start.Add(interval), Open: decimal(trade.Price), High: decimal(trade.Price), Low: decimal(trade.Price), Close: decimal(trade.Price)})
			current = &out[len(out)-1]
		}
		high, _ := strconv.ParseFloat(current.High, 64)
		low, _ := strconv.ParseFloat(current.Low, 64)
		if trade.Price > high {
			current.High = decimal(trade.Price)
		}
		if trade.Price < low {
			current.Low = decimal(trade.Price)
		}
		current.Close = decimal(trade.Price)
		volume, _ := strconv.ParseFloat(current.Volume, 64)
		buy, _ := strconv.ParseFloat(current.BuyVolume, 64)
		sell, _ := strconv.ParseFloat(current.SellVolume, 64)
		volume += trade.Size
		if trade.Buy {
			buy += trade.Size
			cvd += trade.Size
		} else {
			sell += trade.Size
			cvd -= trade.Size
		}
		current.Volume, current.BuyVolume, current.SellVolume = decimal(volume), decimal(buy), decimal(sell)
		current.Delta, current.CVD, current.Trades = decimal(buy-sell), decimal(cvd), current.Trades+1
	}
	return out
}

func buildFootprints(trades []analyticTrade, interval time.Duration) []footprint {
	candles := buildCandles(trades, interval)
	byStart := map[time.Time]map[float64]*[2]float64{}
	for _, trade := range trades {
		start := trade.At.Truncate(interval)
		if byStart[start] == nil {
			byStart[start] = map[float64]*[2]float64{}
		}
		if byStart[start][trade.Price] == nil {
			byStart[start][trade.Price] = &[2]float64{}
		}
		if trade.Buy {
			byStart[start][trade.Price][0] += trade.Size
		} else {
			byStart[start][trade.Price][1] += trade.Size
		}
	}
	out := make([]footprint, 0, len(candles))
	for _, c := range candles {
		prices := make([]float64, 0, len(byStart[c.Start]))
		for price := range byStart[c.Start] {
			prices = append(prices, price)
		}
		sort.Sort(sort.Reverse(sort.Float64Slice(prices)))
		levels := make([]priceVolume, 0, len(prices))
		for _, price := range prices {
			value := byStart[c.Start][price]
			levels = append(levels, priceVolume{Price: decimal(price), BuyVolume: decimal(value[0]), SellVolume: decimal(value[1]), Volume: decimal(value[0] + value[1]), Delta: decimal(value[0] - value[1])})
		}
		out = append(out, footprint{Candle: c, Levels: levels})
	}
	return out
}

func buildProfile(trades []analyticTrade, fraction float64) map[string]any {
	type bin struct{ price, buy, sell float64 }
	byPrice := map[float64]*bin{}
	var total, buy, sell, weighted float64
	for _, trade := range trades {
		if byPrice[trade.Price] == nil {
			byPrice[trade.Price] = &bin{price: trade.Price}
		}
		if trade.Buy {
			byPrice[trade.Price].buy += trade.Size
			buy += trade.Size
		} else {
			byPrice[trade.Price].sell += trade.Size
			sell += trade.Size
		}
		total += trade.Size
		weighted += trade.Price * trade.Size
	}
	prices := make([]float64, 0, len(byPrice))
	for price := range byPrice {
		prices = append(prices, price)
	}
	sort.Float64s(prices)
	distribution := make([]priceVolume, 0, len(prices))
	pocIndex, pocVolume := 0, -1.0
	for i, price := range prices {
		b := byPrice[price]
		volume := b.buy + b.sell
		if volume > pocVolume {
			pocIndex, pocVolume = i, volume
		}
		distribution = append(distribution, priceVolume{Price: decimal(price), BuyVolume: decimal(b.buy), SellVolume: decimal(b.sell), Volume: decimal(volume), Delta: decimal(b.buy - b.sell)})
	}
	low, high, accumulated := pocIndex, pocIndex, math.Max(pocVolume, 0)
	for accumulated < total*fraction && (low > 0 || high+1 < len(prices)) {
		below, above := -1.0, -1.0
		if low > 0 {
			below = byPrice[prices[low-1]].buy + byPrice[prices[low-1]].sell
		}
		if high+1 < len(prices) {
			above = byPrice[prices[high+1]].buy + byPrice[prices[high+1]].sell
		}
		if above >= below {
			high++
			accumulated += above
		} else {
			low--
			accumulated += below
		}
	}
	value := map[string]any{"trades": len(trades), "price_levels": len(prices), "total_volume": decimal(total), "buy_volume": decimal(buy), "sell_volume": decimal(sell), "delta": decimal(buy - sell), "distribution": distribution}
	if len(prices) > 0 {
		value["poc"], value["val"], value["vah"] = decimal(prices[pocIndex]), decimal(prices[low]), decimal(prices[high])
		value["low"], value["high"] = decimal(prices[0]), decimal(prices[len(prices)-1])
	}
	if total > 0 {
		value["vwap"] = decimal(weighted / total)
	}
	return value
}

func decimal(value float64) string {
	if math.Abs(value) < 1e-15 {
		value = 0
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}
