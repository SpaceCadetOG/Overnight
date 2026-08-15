package recordercert

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

type AssetResult struct {
	Symbol      string          `json:"symbol"`
	Snapshot    bool            `json:"snapshot"`
	Events      uint64          `json:"events"`
	FirstEvent  time.Time       `json:"first_event"`
	LastEvent   time.Time       `json:"last_event"`
	FinalNonce  int64           `json:"final_nonce"`
	BestBid     float64         `json:"best_bid"`
	BestAsk     float64         `json:"best_ask"`
	Checkpoints int             `json:"checkpoints"`
	Compared    int             `json:"compared"`
	Gaps        uint64          `json:"nonce_gaps"`
	Crossed     uint64          `json:"crossed_books"`
	Invalid     uint64          `json:"invalid_levels"`
	Streams     map[string]bool `json:"streams"`
	Pass        bool            `json:"pass"`
	Issues      []string        `json:"issues,omitempty"`
}
type Certificate struct {
	SchemaVersion                          int           `json:"schema_version"`
	GeneratedAt                            time.Time     `json:"generated_at"`
	Assets                                 []AssetResult `json:"assets"`
	Ready                                  int           `json:"ready"`
	Expected                               int           `json:"expected"`
	NonceGaps, CrossedBooks, InvalidLevels uint64
	SnapshotComparisons                    bool `json:"snapshot_comparisons"`
	DailyCoverage                          bool `json:"daily_coverage"`
	Pass                                   bool `json:"pass"`
}
type book struct {
	bids, asks map[string]float64
	nonce      int64
	snapshot   bool
}
type checkpoint struct {
	Nonce   int64   `json:"nonce"`
	BestBid float64 `json:"best_bid"`
	BestAsk float64 `json:"best_ask"`
}

func Certify(dir string, expected []string) (Certificate, error) {
	cert := Certificate{SchemaVersion: 1, GeneratedAt: time.Now().UTC(), Expected: len(expected), SnapshotComparisons: true, DailyCoverage: true}
	for _, symbol := range expected {
		result, err := certifyAsset(filepath.Join(dir, "asset="+symbol), symbol)
		if err != nil {
			result.Symbol = symbol
			result.Issues = append(result.Issues, err.Error())
		}
		if result.Pass {
			cert.Ready++
		}
		cert.NonceGaps += result.Gaps
		cert.CrossedBooks += result.Crossed
		cert.InvalidLevels += result.Invalid
		if result.Compared != result.Checkpoints || result.Checkpoints == 0 {
			cert.SnapshotComparisons = false
		}
		if result.FirstEvent.IsZero() || result.LastEvent.Sub(result.FirstEvent) < 23*time.Hour+50*time.Minute {
			cert.DailyCoverage = false
		}
		cert.Assets = append(cert.Assets, result)
	}
	cert.Pass = cert.Ready == cert.Expected && cert.NonceGaps == 0 && cert.CrossedBooks == 0 && cert.InvalidLevels == 0 && cert.SnapshotComparisons && cert.DailyCoverage
	return cert, nil
}

func certifyAsset(dir, symbol string) (AssetResult, error) {
	r := AssetResult{Symbol: symbol, Streams: map[string]bool{}}
	for _, name := range []string{"orderbook_events", "ticker_events", "trade_flow"} {
		path := findFile(dir, name)
		rows := 0
		if path != "" && scan(path, func([]byte) error { rows++; return nil }) == nil && rows > 0 {
			r.Streams[name] = true
		} else {
			r.Issues = append(r.Issues, "missing "+name)
		}
	}
	cpFile := findFile(dir, "reconstructed_book_checkpoints")
	checkpoints := map[int64]checkpoint{}
	if cpFile != "" {
		_ = scan(cpFile, func(line []byte) error {
			var c checkpoint
			if err := json.Unmarshal(line, &c); err != nil {
				return err
			}
			if _, exists := checkpoints[c.Nonce]; !exists {
				r.Checkpoints++
			}
			checkpoints[c.Nonce] = c
			return nil
		})
	} else {
		r.Issues = append(r.Issues, "missing reconstructed checkpoints")
	}
	b := book{bids: map[string]float64{}, asks: map[string]float64{}}
	observed := map[int64][2]float64{}
	err := scan(findFile(dir, "orderbook_events"), func(line []byte) error {
		var row struct {
			ReceivedAt time.Time      `json:"received_at"`
			Event      map[string]any `json:"event"`
		}
		if err := json.Unmarshal(line, &row); err != nil {
			return err
		}
		if r.FirstEvent.IsZero() {
			r.FirstEvent = row.ReceivedAt
		}
		r.LastEvent = row.ReceivedAt
		ob, _ := row.Event["order_book"].(map[string]any)
		typ := fmt.Sprint(row.Event["type"])
		begin, end := i64(ob["begin_nonce"]), i64(ob["nonce"])
		snap := typ == "subscribed/order_book"
		if snap {
			b = book{bids: map[string]float64{}, asks: map[string]float64{}, snapshot: true}
		} else if !b.snapshot {
			return fmt.Errorf("delta before snapshot")
		}
		if !snap && b.nonce > 0 && begin != b.nonce {
			r.Gaps++
		}
		if err := levels(b.asks, ob["asks"]); err != nil {
			r.Invalid++
		}
		if err := levels(b.bids, ob["bids"]); err != nil {
			r.Invalid++
		}
		b.nonce = end
		r.Events++
		bid, ask := best(b)
		if bid > 0 && ask > 0 && bid >= ask {
			r.Crossed++
		}
		if _, ok := checkpoints[end]; ok {
			observed[end] = [2]float64{bid, ask}
		}
		return nil
	})
	if err != nil {
		return r, err
	}
	r.Snapshot = b.snapshot
	r.FinalNonce = b.nonce
	r.BestBid, r.BestAsk = best(b)
	for nonce, want := range checkpoints {
		if got, ok := observed[nonce]; ok && close(got[0], want.BestBid) && close(got[1], want.BestAsk) {
			r.Compared++
		}
	}
	if r.FirstEvent.IsZero() || r.LastEvent.Sub(r.FirstEvent) < 23*time.Hour+50*time.Minute {
		r.Issues = append(r.Issues, "daily coverage shorter than 23h50m")
	}
	r.Pass = r.Snapshot && r.Gaps == 0 && r.Crossed == 0 && r.Invalid == 0 && r.Checkpoints > 0 && r.Compared == r.Checkpoints && len(r.Issues) == 0
	return r, nil
}

func scan(path string, fn func([]byte) error) error {
	if path == "" {
		return fmt.Errorf("file missing")
	}
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	var reader io.Reader = f
	if strings.HasSuffix(path, ".zst") {
		z, e := zstd.NewReader(f)
		if e != nil {
			return e
		}
		defer z.Close()
		reader = z
	}
	s := bufio.NewScanner(reader)
	s.Buffer(make([]byte, 64<<10), 16<<20)
	for s.Scan() {
		if e := fn(s.Bytes()); e != nil {
			return e
		}
	}
	return s.Err()
}
func findFile(dir, name string) string {
	for _, suffix := range []string{".jsonl.zst", ".jsonl"} {
		p := filepath.Join(dir, name+suffix)
		if _, e := os.Stat(p); e == nil {
			return p
		}
	}
	return ""
}
func levels(side map[string]float64, raw any) error {
	items, _ := raw.([]any)
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("malformed")
		}
		p := fmt.Sprint(m["price"])
		price, e := strconv.ParseFloat(p, 64)
		qty, e2 := strconv.ParseFloat(fmt.Sprint(m["size"]), 64)
		if e != nil || e2 != nil || price <= 0 || qty < 0 {
			return fmt.Errorf("invalid")
		}
		if qty == 0 {
			delete(side, p)
		} else {
			side[p] = qty
		}
	}
	return nil
}
func best(b book) (float64, float64) {
	bid, ask := 0.0, 0.0
	for p := range b.bids {
		x, _ := strconv.ParseFloat(p, 64)
		if x > bid {
			bid = x
		}
	}
	for p := range b.asks {
		x, _ := strconv.ParseFloat(p, 64)
		if ask == 0 || x < ask {
			ask = x
		}
	}
	return bid, ask
}
func i64(v any) int64 {
	x, _ := strconv.ParseInt(fmt.Sprint(v), 10, 64)
	if x == 0 {
		if f, ok := v.(float64); ok {
			return int64(f)
		}
	}
	return x
}
func close(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}
func SymbolsSorted(values []string) []string {
	out := append([]string{}, values...)
	sort.Strings(out)
	return out
}
