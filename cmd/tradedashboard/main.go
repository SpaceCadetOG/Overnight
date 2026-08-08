package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/journal"
	"github.com/ogtrading/overnight-strategy/internal/lighterexec"
	"github.com/ogtrading/overnight-strategy/internal/store"
	"github.com/ogtrading/overnight-strategy/internal/universe"
)

type collectorHealth struct {
	Connected             bool      `json:"connected"`
	LastEvent             time.Time `json:"last_event"`
	Events                uint64    `json:"events"`
	NonceGaps             uint64    `json:"nonce_gaps"`
	Reconnects            uint64    `json:"reconnects"`
	ConfirmedLiquidations uint64    `json:"confirmed_liquidations"`
	InferredCascades      uint64    `json:"inferred_liquidation_cascades"`
	BooksReady            int       `json:"books_ready"`
}

func main() {
	root := flag.String("store", "data/test-run", "paper test-run store")
	refresh := flag.Duration("refresh", 10*time.Second, "screen refresh interval")
	paperEquity := flag.Float64("paper-equity", 100, "paper account starting equity")
	view := flag.String("view", "compact", "compact or detailed")
	once := flag.Bool("once", false, "print one screen and exit")
	flag.Parse()
	if err := loadEnv(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
		fatal(err)
	}
	client, markets, err := accountClient()
	if err != nil {
		fatal(err)
	}
	for {
		if strings.EqualFold(*view, "detailed") {
			detailedScreen(*root, *paperEquity, client, markets)
		} else {
			screen(*root, *paperEquity, client, markets)
		}
		if *once {
			return
		}
		time.Sleep(*refresh)
	}
}

func detailedScreen(root string, paperEquity float64, client *lighterexec.Client, accountMarkets []lighterexec.Market) {
	now := time.Now().UTC()
	location, _ := time.LoadLocation("America/Chicago")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	account, accountErr := client.ReadSnapshot(ctx, accountMarkets)
	publicMarkets, marketErr := lighterexec.CheckPublic(ctx, os.Getenv("LIGHTER_BASE_URL"))
	marks := map[string]float64{}
	if marketErr == nil {
		for _, market := range publicMarkets {
			marks[market.Symbol] = number(market.MarkPrice)
		}
	}
	records, _ := store.ReadAll[journal.TradeRecord](root, "trade_journal")
	latest := currentRecords(records, now.In(location), location)
	var realized, openPnL, totalR, riskCommitted float64
	filled, waiting, noFill, wins, losses, open := 0, 0, 0, 0, 0, 0
	for _, r := range latest {
		risk := paperEquity * .005
		riskCommitted += risk
		totalR += r.RMultiple
		switch r.Outcome {
		case "NO_FILL":
			noFill++
		case "OPEN", "TP1_OPEN":
			open++
			openPnL += r.RMultiple * risk
		default:
			filled++
			realized += r.RMultiple * risk
			if r.RMultiple > 0 {
				wins++
			} else if r.RMultiple < 0 {
				losses++
			}
		}
	}
	fmt.Print("\033[2J\033[H")
	fmt.Println("OVERNIGHT STRATEGY — PAPER CONTROL ROOM")
	fmt.Printf("%s / %s\n", now.Format("2006-01-02 15:04:05 UTC"), now.In(location).Format("15:04:05 CT"))
	fmt.Println(strings.Repeat("=", 96))
	fmt.Printf("PAPER ACCOUNT  Start $%.2f | Equity $%.2f | Realized %+.2f | Unrealized %+.2f | Result %+.2fR | Risk $%.2f | FUNDED OFF\n", paperEquity, paperEquity+realized+openPnL, realized, openPnL, totalR, riskCommitted)
	if accountErr == nil {
		fmt.Printf("LIVE READ-ONLY Balance $%.2f | Available $%.2f | Margin $%.2f | Positions %d | Orders %d\n", value(account.Account, "collateral"), value(account.Account, "available_balance"), max0(value(account.Account, "collateral")-value(account.Account, "available_balance")), openPositions(account.Positions), len(account.Orders))
	}
	fmt.Println(strings.Repeat("-", 96))
	fmt.Printf("%-5s %-5s %-13s %12s %12s %12s %12s %12s\n", "ASSET", "SIDE", "STATUS", "ENTRY", "MARK", "STOP", "TP1", "TP2")
	fmt.Println(strings.Repeat("-", 96))
	for _, asset := range universe.All() {
		r, ok := latest[asset.Symbol]
		if !ok {
			waiting++
			fmt.Printf("%-5s %-5s %-13s\n", asset.Symbol, "—", "WAIT PLAN")
			continue
		}
		side := "LONG"
		if r.Order.Side == "SELL" {
			side = "SHORT"
		}
		mark := marks[asset.MarketSymbol()]
		risk := paperEquity * .005
		fmt.Printf("%-5s %-5s %-13s %12s %12s %12s %12s %12s\n", asset.Symbol, side, compactStatus(r.Outcome), price(r.Order.Price), price(mark), price(r.Order.Stop), price(r.Order.TP1), price(r.Order.TP2))
		detail := fmt.Sprintf("      Qty %.6f | Risk $%.2f | Result %+.2fR / %+.2f | MFE %.2fR | MAE %.2fR", r.Order.Quantity, risk, r.RMultiple, r.RMultiple*risk, r.MFER, r.MAER)
		if r.Outcome == "NO_FILL" && mark > 0 {
			detail += fmt.Sprintf(" | Entry distance %.1fbps", math.Abs(mark-r.Order.Price)/r.Order.Price*10000)
		}
		if r.ActualFill > 0 {
			detail += fmt.Sprintf(" | Fill %s | Slip %.2fbps", price(r.ActualFill), r.EntrySlippageBPS)
		}
		fmt.Println(detail)
	}
	fmt.Println(strings.Repeat("=", 96))
	fmt.Printf("FILLED %d | WAITING %d | NO FILL %d | WINS %d | LOSSES %d | OPEN %d | ML SNAPSHOT: SHADOW ONLY\n", filled, waiting, noFill, wins, losses, open)
}

func currentRecords(records []journal.TradeRecord, today time.Time, location *time.Location) map[string]journal.TradeRecord {
	latest := map[string]journal.TradeRecord{}
	for _, r := range records {
		if r.SessionDate.In(location).Format("2006-01-02") != today.Format("2006-01-02") {
			continue
		}
		if old, ok := latest[r.Symbol]; !ok || r.RecordedAt.After(old.RecordedAt) {
			latest[r.Symbol] = r
		}
	}
	return latest
}

func screen(root string, paperEquity float64, client *lighterexec.Client, markets []lighterexec.Market) {
	now := time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	snapshot, accountErr := client.ReadSnapshot(ctx, markets)
	health, healthErr := readHealth(ctx)
	records, recordsErr := store.ReadAll[journal.TradeRecord](root, "trade_journal")
	latest := map[string]journal.TradeRecord{}
	location, _ := time.LoadLocation("America/Chicago")
	today := now.In(location)
	for _, r := range records {
		if r.SessionDate.In(location).Format("2006-01-02") != today.Format("2006-01-02") {
			continue
		}
		if old, ok := latest[r.Symbol]; !ok || r.RecordedAt.After(old.RecordedAt) {
			latest[r.Symbol] = r
		}
	}
	if recordsErr != nil {
		latest = map[string]journal.TradeRecord{}
	}
	var totalR, realized, openPnL, riskCommitted float64
	for _, r := range latest {
		risk := paperEquity * .005
		riskCommitted += risk
		totalR += r.RMultiple
		if r.Outcome == "OPEN" || r.Outcome == "TP1_OPEN" {
			openPnL += r.RMultiple * risk
		} else {
			realized += r.RMultiple * risk
		}
	}
	fmt.Print("\033[2J\033[H")
	fmt.Printf("OVERNIGHT PAPER BOARD                                      %s\n", now.Format("2006-01-02 15:04:05 UTC"))
	fmt.Println(strings.Repeat("=", 104))
	if accountErr != nil {
		fmt.Printf("LIGHTER ACCOUNT  ERROR: %v\n", accountErr)
	} else {
		balance := value(snapshot.Account, "collateral", "total_asset_value")
		available := value(snapshot.Account, "available_balance")
		equity := value(snapshot.Account, "total_asset_value", "collateral")
		used := balance - available
		if used < 0 {
			used = 0
		}
		upnl := positionSum(snapshot.Positions, "unrealized_pnl", "unrealized_pnl_usdc", "unrealized_profit")
		fmt.Printf("LIVE  Bal $%.2f  Eq $%.2f  Avail $%.2f  Margin $%.2f  uPnL %+.2f  Pos %d  Orders %d  [READ ONLY]\n", balance, equity, available, used, upnl, openPositions(snapshot.Positions), len(snapshot.Orders))
	}
	if healthErr != nil {
		fmt.Printf("MARKET DATA      ERROR: %v\n", healthErr)
	} else {
		fmt.Printf("DATA  Connected %-5t  Books %2d/12  Events %-8d  Gaps %-2d  Reconnects %-2d  Liq %d/%d\n", health.Connected, health.BooksReady, health.Events, health.NonceGaps, health.Reconnects, health.ConfirmedLiquidations, health.InferredCascades)
	}
	fmt.Printf("PAPER Start $%.2f  Equity $%.2f  Realized %+.2f  Open %+.2f  Total %+.2fR  Risk $%.2f\n", paperEquity, paperEquity+realized+openPnL, realized, openPnL, totalR, riskCommitted)
	fmt.Println(strings.Repeat("-", 104))
	fmt.Printf("%-5s %-5s %-12s %12s %12s %12s %12s %8s %9s\n", "ASSET", "SIDE", "STATUS", "ENTRY", "STOP", "TP1", "TP2", "R", "PNL")
	fmt.Println(strings.Repeat("-", 104))
	for _, asset := range universe.All() {
		r, ok := latest[asset.Symbol]
		if !ok {
			fmt.Printf("%-5s %-5s %-12s %12s %12s %12s %12s %8s %9s\n", asset.Symbol, "—", "WAIT PLAN", "—", "—", "—", "—", "—", "—")
			continue
		}
		side := "LONG"
		if r.Order.Side == "SELL" {
			side = "SHORT"
		}
		pnl := r.RMultiple * (paperEquity * .005)
		fmt.Printf("%-5s %-5s %-12s %12s %12s %12s %12s %+7.2fR %+8.2f\n", asset.Symbol, side, compactStatus(r.Outcome), price(r.Order.Price), price(r.Order.Stop), price(r.Order.TP1), price(r.Order.TP2), r.RMultiple, pnl)
	}
	fmt.Println(strings.Repeat("=", 104))
	fmt.Printf("12 PAPER SYSTEMS | FUNDED OFF | NEXT PLAN %s\n", nextPlan(now, location).Format("2006-01-02 15:04 UTC"))
}

func compactStatus(value string) string {
	switch value {
	case "TP1_THEN_STOP":
		return "TP1→BE"
	case "TP1_OPEN":
		return "TP1/RUN"
	case "WAITING_FOR_FILL":
		return "WAIT FILL"
	default:
		if len(value) > 12 {
			return value[:12]
		}
		return value
	}
}

func price(value float64) string {
	switch {
	case value >= 1000:
		return fmt.Sprintf("%.1f", value)
	case value >= 100:
		return fmt.Sprintf("%.2f", value)
	case value >= 10:
		return fmt.Sprintf("%.3f", value)
	default:
		return fmt.Sprintf("%.5f", value)
	}
}

func accountClient() (*lighterexec.Client, []lighterexec.Market, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	markets, err := lighterexec.CheckPublic(ctx, os.Getenv("LIGHTER_BASE_URL"))
	if err != nil {
		return nil, nil, err
	}
	account, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("LIGHTER_ACCOUNT_INDEX")), 10, 64)
	if err != nil {
		return nil, nil, err
	}
	key, err := strconv.ParseUint(strings.TrimSpace(os.Getenv("LIGHTER_API_KEY_INDEX")), 10, 8)
	if err != nil {
		return nil, nil, err
	}
	chain := uint64(304)
	if raw := strings.TrimSpace(os.Getenv("LIGHTER_CHAIN_ID")); raw != "" {
		chain, err = strconv.ParseUint(raw, 10, 32)
		if err != nil {
			return nil, nil, err
		}
	}
	client, err := lighterexec.New(lighterexec.Config{BaseURL: os.Getenv("LIGHTER_BASE_URL"), WSURL: os.Getenv("LIGHTER_WS_URL"), PrivateKey: os.Getenv("LIGHTER_API_PRIVATE_KEY"), AccountIndex: account, APIKeyIndex: uint8(key), ChainID: uint32(chain)})
	if err != nil {
		return nil, nil, err
	}
	live := []lighterexec.Market{}
	for _, m := range markets {
		for _, a := range universe.Live() {
			if m.Symbol == a.MarketSymbol() {
				live = append(live, m)
			}
		}
	}
	return client, live, nil
}

func readHealth(ctx context.Context) (collectorHealth, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:8082/healthz", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return collectorHealth{}, err
	}
	defer resp.Body.Close()
	var h collectorHealth
	err = json.NewDecoder(resp.Body).Decode(&h)
	return h, err
}
func value(m map[string]any, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			f, e := strconv.ParseFloat(fmt.Sprint(v), 64)
			if e == nil {
				return f
			}
		}
	}
	return 0
}
func number(v any) float64 { f, _ := strconv.ParseFloat(fmt.Sprint(v), 64); return f }
func max0(v float64) float64 {
	if v < 0 {
		return 0
	}
	return v
}
func positionSum(v []map[string]any, keys ...string) float64 {
	var out float64
	for _, m := range v {
		out += value(m, keys...)
	}
	return out
}
func openPositions(v []map[string]any) int {
	n := 0
	for _, m := range v {
		if value(m, "position", "size") != 0 {
			n++
		}
	}
	return n
}
func nextPlan(now time.Time, loc *time.Location) time.Time {
	local := now.In(loc)
	p := time.Date(local.Year(), local.Month(), local.Day(), 5, 0, 0, 0, loc)
	if !local.Before(p) {
		p = p.AddDate(0, 0, 1)
	}
	return p.UTC()
}
func loadEnv(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok {
			k = strings.TrimSpace(k)
			v = strings.Trim(strings.TrimSpace(v), "'\"")
			if _, exists := os.LookupEnv(k); !exists {
				_ = os.Setenv(k, v)
			}
		}
	}
	return s.Err()
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
