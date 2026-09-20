package oracleapi

import (
	"errors"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const analyticsSchema = "oracle-analytics-v1"

const maxProfileRange = 366 * 24 * time.Hour

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

type volumeBin struct{ price, buy, sell float64 }
type volumeNode struct{ price, volume float64 }

type analyticsInput struct {
	Asset   string
	From    time.Time
	To      time.Time
	Allow   bool
	Period  string
	Session *sessionInstance
}

func parseAnalyticsInput(r *http.Request) (analyticsInput, error) {
	asset := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("asset")))
	if asset == "" || strings.ContainsAny(asset, `/\\`) {
		return analyticsInput{}, errors.New("asset is required")
	}
	period := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("period")))
	if period != "" && r.URL.Query().Get("from") == "" && r.URL.Query().Get("to") == "" {
		at := time.Now().UTC()
		var err error
		if raw := r.URL.Query().Get("at"); raw != "" {
			at, err = time.Parse(time.RFC3339Nano, raw)
			if err != nil {
				return analyticsInput{}, errors.New("at must be RFC3339")
			}
		}
		definition, ok := findSessionDefinition(period)
		if !ok {
			return analyticsInput{}, errors.New("unknown profile period")
		}
		instance, err := resolveSession(definition, at.UTC())
		if err != nil {
			return analyticsInput{}, err
		}
		if instance.UTCEnd.Sub(instance.UTCStart) > maxProfileRange {
			return analyticsInput{}, errors.New("profile range cannot exceed 366 days")
		}
		return analyticsInput{Asset: asset, From: instance.UTCStart, To: minTime(instance.UTCEnd, at.UTC()), Allow: r.URL.Query().Get("allow_uncertified") == "true", Period: period, Session: &instance}, nil
	}
	from, err := time.Parse(time.RFC3339Nano, r.URL.Query().Get("from"))
	if err != nil {
		return analyticsInput{}, errors.New("from must be RFC3339")
	}
	to, err := time.Parse(time.RFC3339Nano, r.URL.Query().Get("to"))
	if err != nil {
		return analyticsInput{}, errors.New("to must be RFC3339")
	}
	if !to.After(from) || to.Sub(from) > maxProfileRange {
		return analyticsInput{}, errors.New("time range must be positive and no greater than 366 days")
	}
	return analyticsInput{Asset: asset, From: from.UTC(), To: to.UTC(), Allow: r.URL.Query().Get("allow_uncertified") == "true", Period: "ANCHORED"}, nil
}

func (s *Server) analyticTrades(r *http.Request, in analyticsInput) ([]analyticTrade, queryResponse, int, error) {
	trades := []analyticTrade{}
	result, status, err := s.walkAnalyticTrades(r, in, func(trade analyticTrade) { trades = append(trades, trade) })
	return trades, result, status, err
}

func (s *Server) candles(w http.ResponseWriter, r *http.Request) {
	in, err := parseAnalyticsInput(r)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	if in.To.Sub(in.From) > maxQueryRange {
		writeError(w, 400, errors.New("candle queries are limited to 24h; request adjacent pages"))
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
	writeJSON(w, 200, map[string]any{"schema_version": analyticsSchema, "type": "CANDLES", "asset": in.Asset, "from": in.From, "to": in.To, "interval": interval.String(), "candles": buildCandles(trades, interval), "quality": source.Quality, "packages": source.Packages, "excluded_events": source.Excluded, "query_mode": source.QueryMode, "coverage": source.Coverage, "information_cutoff": in.To})
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
	profile, source, status, err := s.streamingProfile(r, in, valueArea)
	if err != nil {
		writeError(w, status, err)
		return
	}
	writeJSON(w, 200, map[string]any{"schema_version": analyticsSchema, "type": "VOLUME_PROFILE", "profile_model": "trade-volume-at-price-v1", "session_definition_version": sessionDefinitionVersion, "period": in.Period, "session": in.Session, "asset": in.Asset, "from": in.From, "to": in.To, "value_area_fraction": valueArea, "profile": profile, "quality": source.Quality, "packages": source.Packages, "excluded_events": source.Excluded, "query_mode": source.QueryMode, "coverage": source.Coverage, "information_cutoff": in.To})
}

func (s *Server) footprints(w http.ResponseWriter, r *http.Request) {
	in, err := parseAnalyticsInput(r)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	if in.To.Sub(in.From) > maxQueryRange {
		writeError(w, 400, errors.New("footprint queries are limited to 24h; request adjacent pages"))
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
	writeJSON(w, 200, map[string]any{"schema_version": analyticsSchema, "type": "FOOTPRINT", "asset": in.Asset, "from": in.From, "to": in.To, "interval": interval.String(), "footprints": buildFootprints(trades, interval), "quality": source.Quality, "packages": source.Packages, "excluded_events": source.Excluded, "query_mode": source.QueryMode, "coverage": source.Coverage, "information_cutoff": in.To})
}

func (s *Server) orderFlow(w http.ResponseWriter, r *http.Request) {
	// A bounded window requests the collector's low-latency developing view.
	// Explicit from/to queries continue to use durable historical storage.
	if r.URL.Query().Get("window") != "" && r.URL.Query().Get("from") == "" && r.URL.Query().Get("to") == "" {
		s.live(w, r)
		return
	}
	in, err := parseAnalyticsInput(r)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	accumulator := newProfileAccumulator()
	source, status, err := s.walkAnalyticTrades(r, in, accumulator.Add)
	if err != nil {
		writeError(w, status, err)
		return
	}
	buy, sell, notional, count := accumulator.buy, accumulator.sell, accumulator.weighted, accumulator.trades
	cvd := buy - sell
	duration := in.To.Sub(in.From).Seconds()
	lag := time.Since(in.To).Milliseconds()
	if lag < 0 {
		lag = 0
	}
	writeJSON(w, 200, map[string]any{"schema_version": analyticsSchema, "type": "ORDER_FLOW", "asset": in.Asset, "from": in.From, "to": in.To, "trades": count, "buy_volume": decimal(buy), "sell_volume": decimal(sell), "total_volume": decimal(buy + sell), "delta": decimal(cvd), "cvd": decimal(cvd), "delta_rate_per_second": decimal(cvd / duration), "notional": decimal(notional), "quality": source.Quality, "packages": source.Packages, "excluded_events": source.Excluded, "query_mode": source.QueryMode, "coverage": source.Coverage, "information_cutoff": in.To, "lag_ms": lag})
}

type profileAccumulator struct {
	byPrice             map[float64]*volumeBin
	buy, sell, weighted float64
	trades              int
	open, close         float64
}

func newProfileAccumulator() *profileAccumulator {
	return &profileAccumulator{byPrice: map[float64]*volumeBin{}}
}
func (a *profileAccumulator) Add(trade analyticTrade) {
	if a.trades == 0 {
		a.open = trade.Price
	}
	a.close = trade.Price
	a.trades++
	a.weighted += trade.Notional
	if a.byPrice[trade.Price] == nil {
		a.byPrice[trade.Price] = &volumeBin{price: trade.Price}
	}
	if trade.Buy {
		a.byPrice[trade.Price].buy += trade.Size
		a.buy += trade.Size
	} else {
		a.byPrice[trade.Price].sell += trade.Size
		a.sell += trade.Size
	}
}

func (a *profileAccumulator) Profile(fraction float64) map[string]any {
	prices := make([]float64, 0, len(a.byPrice))
	total := a.buy + a.sell
	for price := range a.byPrice {
		prices = append(prices, price)
	}
	sort.Float64s(prices)
	distribution := make([]priceVolume, 0, len(prices))
	pocIndex, pocVolume := 0, -1.0
	for i, price := range prices {
		b := a.byPrice[price]
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
			b := a.byPrice[prices[low-1]]
			below = b.buy + b.sell
		}
		if high+1 < len(prices) {
			b := a.byPrice[prices[high+1]]
			above = b.buy + b.sell
		}
		if above >= below {
			high++
			accumulated += above
		} else {
			low--
			accumulated += below
		}
	}
	value := map[string]any{"trades": a.trades, "price_levels": len(prices), "total_volume": decimal(total), "buy_volume": decimal(a.buy), "sell_volume": decimal(a.sell), "delta": decimal(a.buy - a.sell), "distribution": distribution}
	if len(prices) > 0 {
		value["poc"], value["val"], value["vah"] = decimal(prices[pocIndex]), decimal(prices[low]), decimal(prices[high])
		value["low"], value["high"] = decimal(prices[0]), decimal(prices[len(prices)-1])
		value["open"], value["close"] = decimal(a.open), decimal(a.close)
		value["hvns"], value["lvns"] = profileNodes(prices, a.byPrice, pocIndex)
	}
	if total > 0 {
		value["vwap"] = decimal(a.weighted / total)
	}
	return value
}

func (s *Server) streamingProfile(r *http.Request, in analyticsInput, fraction float64) (map[string]any, queryResponse, int, error) {
	acc := newProfileAccumulator()
	source, status, err := s.walkAnalyticTrades(r, in, acc.Add)
	return acc.Profile(fraction), source, status, err
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
	acc := newProfileAccumulator()
	for _, trade := range trades {
		acc.Add(trade)
	}
	return acc.Profile(fraction)
}

func profileNodes(prices []float64, values map[float64]*volumeBin, poc int) ([]string, []string) {
	highs, lows := []volumeNode{}, []volumeNode{}
	for i := 1; i+1 < len(prices); i++ {
		volume := values[prices[i]].buy + values[prices[i]].sell
		below := values[prices[i-1]].buy + values[prices[i-1]].sell
		above := values[prices[i+1]].buy + values[prices[i+1]].sell
		if i != poc && volume >= below && volume >= above {
			highs = append(highs, volumeNode{prices[i], volume})
		}
		if volume <= below && volume <= above {
			lows = append(lows, volumeNode{prices[i], volume})
		}
	}
	sort.Slice(highs, func(i, j int) bool {
		if highs[i].volume == highs[j].volume {
			return highs[i].price < highs[j].price
		}
		return highs[i].volume > highs[j].volume
	})
	sort.Slice(lows, func(i, j int) bool {
		if lows[i].volume == lows[j].volume {
			return lows[i].price < lows[j].price
		}
		return lows[i].volume < lows[j].volume
	})
	return nodePrices(highs, 2), nodePrices(lows, 2)
}

func nodePrices(nodes []volumeNode, limit int) []string {
	if len(nodes) < limit {
		limit = len(nodes)
	}
	out := make([]string, 0, limit)
	for _, node := range nodes[:limit] {
		out = append(out, decimal(node.price))
	}
	return out
}

func decimal(value float64) string {
	if math.Abs(value) < 1e-15 {
		value = 0
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func minTime(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}
