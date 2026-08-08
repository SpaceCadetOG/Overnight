package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/execution"
	"github.com/ogtrading/overnight-strategy/internal/forensics"
	"github.com/ogtrading/overnight-strategy/internal/journal"
	"github.com/ogtrading/overnight-strategy/internal/store"
	"github.com/ogtrading/overnight-strategy/internal/universe"
)

const exportSchema = 1

type exportRow struct {
	SchemaVersion  int                          `json:"schema_version"`
	DatasetVersion string                       `json:"dataset_version"`
	ShadowOnly     bool                         `json:"shadow_only"`
	Journal        journal.TradeRecord          `json:"journal"`
	Checkpoints    forensics.CheckpointManifest `json:"checkpoints"`
}
type quality struct {
	SchemaVersion int      `json:"schema_version"`
	Date          string   `json:"date"`
	Passed        bool     `json:"passed"`
	Expected      int      `json:"expected"`
	Journals      int      `json:"journals"`
	Opportunities int      `json:"opportunities"`
	Issues        []string `json:"issues,omitempty"`
}

func main() {
	root := flag.String("store", "data/test-run", "research store")
	dateFlag := flag.String("date", "", "Chicago session date")
	out := flag.String("out", "data/packages/pending", "package output root")
	marketRoot := flag.String("market-data-root", "data/live/lighter", "daily collector root")
	flag.Parse()
	loc, _ := time.LoadLocation("America/Chicago")
	date := time.Now().In(loc)
	var err error
	if *dateFlag != "" {
		date, err = time.ParseInLocation("2006-01-02", *dateFlag, loc)
		if err != nil {
			fatal(err)
		}
	}
	journals, err := store.ReadAll[journal.TradeRecord](*root, "trade_journal")
	if err != nil {
		fatal(err)
	}
	trades, err := store.ReadAll[execution.PaperTrade](*root, "paper_trades")
	if err != nil {
		fatal(err)
	}
	events, err := store.ReadAll[forensics.Envelope](*root, "forensic_events")
	if err != nil {
		fatal(err)
	}
	latest := map[string]journal.TradeRecord{}
	for _, r := range journals {
		if r.SessionDate.In(loc).Format("2006-01-02") != date.Format("2006-01-02") {
			continue
		}
		if old, ok := latest[r.Symbol]; !ok || r.RecordedAt.After(old.RecordedAt) {
			latest[r.Symbol] = r
		}
	}
	tradeByOpportunity := map[string]execution.PaperTrade{}
	for _, t := range trades {
		if t.SessionDate.In(loc).Format("2006-01-02") == date.Format("2006-01-02") {
			tradeByOpportunity[t.OpportunityID] = t
		}
	}
	byOpp := map[string][]forensics.Envelope{}
	for _, e := range events {
		if strings.HasSuffix(e.RunID, date.Format("20060102")) {
			byOpp[e.OpportunityID] = append(byOpp[e.OpportunityID], e)
		}
	}
	q := quality{SchemaVersion: 1, Date: date.Format("2006-01-02"), Expected: len(universe.All()), Journals: len(latest), Opportunities: len(byOpp)}
	if len(latest) != len(universe.All()) {
		q.Issues = append(q.Issues, fmt.Sprintf("journal coverage %d/%d", len(latest), len(universe.All())))
	}
	rows := []exportRow{}
	datasetVersion := "dataset_" + date.Format("20060102") + "_baseline-v1-20260810_v1"
	for _, asset := range universe.All() {
		r, ok := latest[asset.Symbol]
		if !ok {
			continue
		}
		if r.SchemaVersion != 1 || r.SessionID == "" || r.OpportunityID == "" || r.StrategyOrderID == "" || r.RunID == "" {
			q.Issues = append(q.Issues, asset.Symbol+": incomplete identity/schema")
		}
		caseEvents := byOpp[r.OpportunityID]
		c, caseErr := forensics.BuildCase(caseEvents)
		if caseErr != nil {
			q.Issues = append(q.Issues, asset.Symbol+": "+caseErr.Error())
		} else if len(c.DataQuality) > 0 {
			q.Issues = append(q.Issues, asset.Symbol+": "+fmt.Sprint(c.DataQuality))
		}
		trade, hasTrade := tradeByOpportunity[r.OpportunityID]
		if !hasTrade {
			q.Issues = append(q.Issues, asset.Symbol+": missing paired paper trade")
		}
		rows = append(rows, exportRow{SchemaVersion: exportSchema, DatasetVersion: datasetVersion, ShadowOnly: true, Journal: r, Checkpoints: forensics.Checkpoints(r, trade)})
	}
	q.Passed = len(q.Issues) == 0
	sort.Slice(rows, func(i, j int) bool { return rows[i].Journal.Symbol < rows[j].Journal.Symbol })
	dir := filepath.Join(*out, "package="+datasetVersion)
	if err := os.MkdirAll(dir, 0750); err != nil {
		fatal(err)
	}
	write(filepath.Join(dir, "ml_rows.jsonl"), rows)
	write(filepath.Join(dir, "lifecycle_events.jsonl"), filteredEvents(events, date))
	write(filepath.Join(dir, "paper_trades.jsonl"), filteredTrades(trades, date, loc))
	write(filepath.Join(dir, "data_quality.json"), q)
	if sampleIssues := copyMarketSamples(*marketRoot, date.Format("2006-01-02"), dir); len(sampleIssues) > 0 {
		q.Issues = append(q.Issues, sampleIssues...)
		q.Passed = false
		write(filepath.Join(dir, "data_quality.json"), q)
	}
	manifest := map[string]any{"schema_version": 1, "dataset_version": datasetVersion, "strategy_version": "baseline-v1-20260810", "market_map_schema_version": 1, "trade_journal_schema_version": 1, "event_schema_version": forensics.SchemaVersion, "checkpoint_schema_version": forensics.CheckpointSchemaVersion, "code_version": "cycle1-baseline-v1-20260810", "created_at": time.Now().UTC(), "shadow_only": true, "files": fileHashes(dir), "data_quality_pass": q.Passed}
	write(filepath.Join(dir, "manifest.json"), manifest)
	b, _ := json.Marshal(map[string]any{"status": map[bool]string{true: "PASS", false: "FAIL"}[q.Passed], "package": dir, "rows": len(rows), "issues": q.Issues})
	fmt.Println(string(b))
	if !q.Passed {
		os.Exit(1)
	}
}
func write(path string, value any) {
	f, e := os.Create(path)
	if e != nil {
		fatal(e)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	if rows, ok := value.([]exportRow); ok {
		for _, row := range rows {
			if e = enc.Encode(row); e != nil {
				fatal(e)
			}
		}
	} else if rows, ok := value.([]forensics.Envelope); ok {
		for _, row := range rows {
			if e = enc.Encode(row); e != nil {
				fatal(e)
			}
		}
	} else if rows, ok := value.([]execution.PaperTrade); ok {
		for _, row := range rows {
			if e = enc.Encode(row); e != nil {
				fatal(e)
			}
		}
	} else if e = enc.Encode(value); e != nil {
		fatal(e)
	}
}
func fileHashes(dir string) map[string]string {
	out := map[string]string{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() || e.Name() == "manifest.json" {
			continue
		}
		b, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		h := sha256.Sum256(b)
		out[e.Name()] = hex.EncodeToString(h[:])
	}
	return out
}

func filteredEvents(events []forensics.Envelope, date time.Time) []forensics.Envelope {
	out, suffix := []forensics.Envelope{}, date.Format("20060102")
	for _, event := range events {
		if strings.HasSuffix(event.RunID, suffix) {
			out = append(out, event)
		}
	}
	return out
}

func filteredTrades(trades []execution.PaperTrade, date time.Time, location *time.Location) []execution.PaperTrade {
	out := []execution.PaperTrade{}
	for _, trade := range trades {
		if trade.SessionDate.In(location).Format("2006-01-02") == date.Format("2006-01-02") {
			out = append(out, trade)
		}
	}
	return out
}

func copyMarketSamples(root, date, dir string) []string {
	base := filepath.Join(root, "date="+date)
	specs := map[string]string{"l2_sample.jsonl": "asset=BTC/orderbook_events.jsonl", "tape_sample.jsonl": "asset=BTC/trade_flow.jsonl", "ticker_sample.jsonl": "asset=BTC/ticker_events.jsonl", "confirmed_liquidations_sample.jsonl": "asset=BTC/confirmed_liquidations.jsonl"}
	issues := []string{}
	for destination, relative := range specs {
		if err := copyLines(filepath.Join(base, relative), filepath.Join(dir, destination), 3); err != nil {
			issues = append(issues, relative+": "+err.Error())
		}
	}
	matches, _ := filepath.Glob(filepath.Join(base, "asset=*/inferred_liquidation_cascades.jsonl"))
	if len(matches) == 0 {
		issues = append(issues, "no inferred liquidation cascade sample available")
	} else if err := copyLines(matches[0], filepath.Join(dir, "inferred_liquidation_cascades_sample.jsonl"), 3); err != nil {
		issues = append(issues, err.Error())
	}
	return issues
}

func copyLines(source, destination string, limit int) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer out.Close()
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	writer := bufio.NewWriter(out)
	defer writer.Flush()
	count := 0
	for scanner.Scan() && count < limit {
		if _, err = writer.Write(append(scanner.Bytes(), '\n')); err != nil {
			return err
		}
		count++
	}
	if err = scanner.Err(); err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("empty source")
	}
	return nil
}
func fatal(e error) { fmt.Fprintln(os.Stderr, e); os.Exit(1) }
