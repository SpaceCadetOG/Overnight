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

const (
	ansiReset   = "\033[0m"
	ansiBold    = "\033[1m"
	ansiDim     = "\033[2m"
	ansiRed     = "\033[31m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiBlue    = "\033[34m"
	ansiMagenta = "\033[35m"
	ansiCyan    = "\033[36m"
)

var colorEnabled bool

func main() {
	root := flag.String("store", "data/test-run", "paper test-run store")
	refresh := flag.Duration("refresh", 10*time.Second, "screen refresh interval")
	paperEquity := flag.Float64("paper-equity", 100, "paper account starting equity")
	view := flag.String("view", "compact", "compact or detailed")
	once := flag.Bool("once", false, "print one screen and exit")
	colorMode := flag.String("color", "auto", "terminal color: auto, always, or never")
	flag.Parse()
	colorEnabled = shouldColor(*colorMode)
	for _, path := range []string{"/etc/overnight-lighter.env", ".env"} {
		if err := loadEnv(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			fatal(err)
		}
	}
	client, markets, err := accountClient()
	for {
		if strings.EqualFold(*view, "detailed") {
			detailedScreen(*root, *paperEquity, client, markets, err)
		} else {
			screen(*root, *paperEquity, client, markets, err)
		}
		if *once {
			return
		}
		time.Sleep(*refresh)
	}
}

func detailedScreen(root string, paperEquity float64, client *lighterexec.Client, accountMarkets []lighterexec.Market, accountInitErr error) {
	now := time.Now().UTC()
	location, _ := time.LoadLocation("America/Chicago")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	account, accountErr := readAccountSnapshot(ctx, client, accountMarkets, accountInitErr)
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
	fmt.Printf("%s %s %s\n", paint(ansiBold+ansiCyan, "OVERNIGHT STRATEGY — CONTROL ROOM"), paint(ansiBold+ansiBlue, "[PAPER]"), paint(ansiBold+ansiMagenta, "[LIVE]"))
	fmt.Printf("%s / %s\n", paint(ansiBold, now.Format("2006-01-02 15:04:05 UTC")), paint(ansiDim+ansiCyan, now.In(location).Format("15:04:05 CT")))
	detailedDivider("=")
	currentEquity := paperEquity + realized + openPnL
	fmt.Printf("%s  Start $%.2f | Equity %s | Realized %s | Unrealized %s\n", paint(ansiBold+ansiBlue, "PAPER ACCOUNT"), paperEquity, paint(signColor(currentEquity-paperEquity), fmt.Sprintf("$%.2f", currentEquity)), signedMoney(realized), signedMoney(openPnL))
	fmt.Printf("%s Result %s | Risk $%.2f | %s\n", strings.Repeat(" ", 15), signedR(totalR), riskCommitted, paint(ansiBold+ansiYellow, "FUNDED OFF"))
	if accountErr == nil {
		fmt.Printf("%s Balance $%.2f | Available $%.2f | Margin $%.2f | Positions %d | Orders %d\n", paint(ansiBold+ansiCyan, "LIVE READ-ONLY"), value(account.Account, "collateral"), value(account.Account, "available_balance"), max0(value(account.Account, "collateral")-value(account.Account, "available_balance")), openPositions(account.Positions), len(account.Orders))
	} else {
		fmt.Printf("%s %s\n", paint(ansiBold+ansiCyan, "LIVE READ-ONLY"), paint(ansiBold+ansiRed, "ERROR: "+shortError(accountErr)))
	}
	detailedDivider("-")
	fmt.Println(paint(ansiBold+ansiCyan, fmt.Sprintf("| %-3s | %-5s | %-5s | %-12s | %9s | %9s | %9s | %9s | %9s |", "MODE", "ASSET", "SIDE", "STATUS", "ENTRY", "MARK", "STOP", "TP1", "TP2")))
	detailedDivider("-")
	for _, asset := range universe.All() {
		r, ok := latest[asset.Symbol]
		if !ok {
			waiting++
			printDetailedRow("PAPER", asset.Symbol, "—", "WAIT PLAN", "—", "—", "—", "—", "—")
			continue
		}
		side := "LONG"
		if r.Order.Side == "SELL" {
			side = "SHORT"
		}
		mark := marks[asset.MarketSymbol()]
		risk := paperEquity * .005
		printDetailedRow("PAPER", asset.Symbol, side, compactStatus(r.Outcome), price(r.Order.Price), price(mark), price(r.Order.Stop), price(r.Order.TP1), price(r.Order.TP2))
		detail := fmt.Sprintf("|     |       | %s | %s | Result %s / %s", paint(ansiDim, fmt.Sprintf("Qty %.6f", r.Order.Quantity)), paint(ansiYellow, fmt.Sprintf("Risk $%.2f", risk)), signedR(r.RMultiple), signedMoney(r.RMultiple*risk))
		excursions := fmt.Sprintf("|     |       | MFE %s | MAE %s", excursion(r.MFER, true), excursion(r.MAER, false))
		if r.Outcome == "NO_FILL" && mark > 0 {
			excursions += paint(ansiYellow, fmt.Sprintf(" | Entry distance %.1fbps", math.Abs(mark-r.Order.Price)/r.Order.Price*10000))
		}
		if r.ActualFill > 0 {
			excursions += paint(ansiCyan, fmt.Sprintf(" | Fill %s | Slip %.2fbps", price(r.ActualFill), r.EntrySlippageBPS))
		}
		fmt.Println(detail)
		fmt.Println(excursions)
	}
	detailedDivider("=")
	fmt.Printf("%s %d | %s %d | %s %d | %s %d | %s %d | %s %d | %s\n", paint(ansiBold+ansiCyan, "FILLED"), filled, paint(ansiYellow, "WAITING"), waiting, paint(ansiYellow, "NO FILL"), noFill, paint(ansiBold+ansiGreen, "WINS"), wins, paint(ansiBold+ansiRed, "LOSSES"), losses, paint(ansiBold+ansiCyan, "OPEN"), open, paint(ansiDim+ansiMagenta, "ML SNAPSHOT: SHADOW ONLY"))
}

func detailedDivider(char string) {
	// Matches the exact printable width of the detailed table below.
	fmt.Println(paint(ansiDim+ansiBlue, "+"+strings.Repeat(char, 5)+"+"+strings.Repeat(char, 7)+"+"+strings.Repeat(char, 7)+"+"+strings.Repeat(char, 14)+"+"+strings.Repeat(char, 11)+"+"+strings.Repeat(char, 11)+"+"+strings.Repeat(char, 11)+"+"+strings.Repeat(char, 11)+"+"+strings.Repeat(char, 11)+"+"))
}

func printDetailedRow(mode, symbol, side, status, entry, mark, stop, tp1, tp2 string) {
	modeColor := ansiBold + ansiBlue
	modeTag := "[P]"
	if mode == "LIVE" {
		modeColor, modeTag = ansiBold+ansiMagenta, "[L]"
	}
	sColor := sideColor(side)
	if side == "—" {
		sColor = ansiDim
	}
	fmt.Printf("| %s | %s | %s | %s | %9s | %9s | %9s | %9s | %9s |\n", coloredCell(modeTag, 3, false, modeColor), coloredCell(symbol, 5, false, ansiBold+ansiCyan), coloredCell(side, 5, false, sColor), coloredCell(status, 12, false, statusColor(status)), entry, mark, stop, tp1, tp2)
}

func excursion(value float64, favorable bool) string {
	color := ansiBold + ansiGreen
	if !favorable {
		color = ansiBold + ansiRed
	}
	if value == 0 {
		color = ansiDim
	}
	return paint(color, fmt.Sprintf("%.2fR", value))
}

func currentRecords(records []journal.TradeRecord, today time.Time, location *time.Location) map[string]journal.TradeRecord {
	latest := map[string]journal.TradeRecord{}
	for _, r := range records {
		if r.SessionDate.In(location).Format("2006-01-02") != today.Format("2006-01-02") {
			continue
		}
		if r.Mode == "LIVE_EXECUTION" {
			continue
		}
		if old, ok := latest[r.Symbol]; !ok || r.RecordedAt.After(old.RecordedAt) {
			latest[r.Symbol] = r
		}
	}
	return latest
}

func screen(root string, paperEquity float64, client *lighterexec.Client, markets []lighterexec.Market, accountInitErr error) {
	now := time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	snapshot, accountErr := readAccountSnapshot(ctx, client, markets, accountInitErr)
	health, healthErr := readHealth(ctx)
	records, recordsErr := store.ReadAll[journal.TradeRecord](root, "trade_journal")
	latest := map[string]journal.TradeRecord{}
	liveLatest := map[string]journal.TradeRecord{}
	location, _ := time.LoadLocation("America/Chicago")
	today := now.In(location)
	for _, r := range records {
		if r.SessionDate.In(location).Format("2006-01-02") != today.Format("2006-01-02") {
			continue
		}
		target := latest
		if r.Mode == "LIVE_EXECUTION" {
			target = liveLatest
		}
		if old, ok := target[r.Symbol]; !ok || r.RecordedAt.After(old.RecordedAt) {
			target[r.Symbol] = r
		}
	}
	if recordsErr != nil {
		latest = map[string]journal.TradeRecord{}
		liveLatest = map[string]journal.TradeRecord{}
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
	fmt.Printf("%s %s %s                         %s\n", paint(ansiBold+ansiCyan, "OVERNIGHT TRADING BOARD"), paint(ansiBold+ansiBlue, "[PAPER]"), paint(ansiBold+ansiMagenta, "[LIVE]"), paint(ansiBold, now.Format("2006-01-02 15:04:05 UTC")))
	divider("=", 112)
	if accountErr != nil {
		fmt.Printf("%s  %s\n", paint(ansiBold+ansiBlue, "LIGHTER ACCOUNT"), paint(ansiBold+ansiRed, "ERROR: "+shortError(accountErr)))
	} else {
		balance := value(snapshot.Account, "collateral", "total_asset_value")
		available := value(snapshot.Account, "available_balance")
		equity := value(snapshot.Account, "total_asset_value", "collateral")
		used := balance - available
		if used < 0 {
			used = 0
		}
		upnl := positionSum(snapshot.Positions, "unrealized_pnl", "unrealized_pnl_usdc", "unrealized_profit")
		fmt.Printf("%s  Bal $%.2f  Eq %s  Avail $%.2f  Margin $%.2f  uPnL %s  Pos %d  Orders %d  %s\n", paint(ansiBold+ansiBlue, "LIVE"), balance, signedMoney(equity), available, used, signedMoney(upnl), openPositions(snapshot.Positions), len(snapshot.Orders), paint(ansiDim+ansiCyan, "[READ ONLY]"))
	}
	if healthErr != nil {
		fmt.Printf("%s      %s\n", paint(ansiBold+ansiBlue, "MARKET DATA"), paint(ansiBold+ansiRed, "ERROR: "+shortError(healthErr)))
	} else {
		connected := paint(ansiBold+ansiRed, "false")
		if health.Connected {
			connected = paint(ansiBold+ansiGreen, "true ")
		}
		books := paint(ansiBold+ansiRed, fmt.Sprintf("%2d/12", health.BooksReady))
		if health.BooksReady == len(universe.All()) {
			books = paint(ansiBold+ansiGreen, fmt.Sprintf("%2d/12", health.BooksReady))
		}
		gaps := paint(ansiBold+ansiGreen, fmt.Sprintf("%-2d", health.NonceGaps))
		if health.NonceGaps > 0 {
			gaps = paint(ansiBold+ansiRed, fmt.Sprintf("%-2d", health.NonceGaps))
		}
		fmt.Printf("%s  Connected %s  Books %s  Events %-8d  Gaps %s  Reconnects %-2d  Liq %d/%d\n", paint(ansiBold+ansiBlue, "DATA"), connected, books, health.Events, gaps, health.Reconnects, health.ConfirmedLiquidations, health.InferredCascades)
	}
	currentEquity := paperEquity + realized + openPnL
	fmt.Printf("%s Start $%.2f  Equity %s  Realized %s  Open %s  Total %s  Risk $%.2f\n", paint(ansiBold+ansiBlue, "PAPER"), paperEquity, paint(signColor(currentEquity-paperEquity), fmt.Sprintf("$%.2f", currentEquity)), signedMoney(realized), signedMoney(openPnL), signedR(totalR), riskCommitted)
	divider("-", 112)
	fmt.Println(paint(ansiBold+ansiCyan, fmt.Sprintf("%-7s %-5s %-5s %-12s %12s %12s %12s %12s %8s %9s", "MODE", "ASSET", "SIDE", "STATUS", "ENTRY", "STOP", "TP1", "TP2", "R", "PNL")))
	divider("-", 112)
	for _, asset := range universe.All() {
		r, ok := latest[asset.Symbol]
		if !ok {
			fmt.Printf("%s %s %s %s %12s %12s %12s %12s %8s %9s\n", modeCell("PAPER"), coloredCell(asset.Symbol, 5, false, ansiBold+ansiCyan), coloredCell("—", 5, false, ansiDim), coloredCell("WAIT PLAN", 12, false, ansiYellow), "—", "—", "—", "—", "—", "—")
			continue
		}
		side := "LONG"
		if r.Order.Side == "SELL" {
			side = "SHORT"
		}
		pnl := r.RMultiple * (paperEquity * .005)
		printTradeRow("PAPER", asset.Symbol, side, r, pnl)
	}
	if len(liveLatest) > 0 {
		divider("-", 112)
		for _, asset := range universe.Live() {
			r, ok := liveLatest[asset.Symbol]
			if !ok {
				continue
			}
			side := "LONG"
			if r.Order.Side == "SELL" {
				side = "SHORT"
			}
			printTradeRow("LIVE", asset.Symbol, side, r, r.RMultiple*(paperEquity*.005))
		}
	}
	divider("=", 112)
	fmt.Printf("%s | %s | %s %s\n", paint(ansiBold+ansiCyan, "12 PAPER SYSTEMS"), paint(ansiBold+ansiYellow, "FUNDED OFF"), paint(ansiBold+ansiBlue, "NEXT PLAN"), paint(ansiBold, nextPlan(now, location).Format("2006-01-02 15:04 UTC")))
}

func printTradeRow(mode, symbol, side string, r journal.TradeRecord, pnl float64) {
	fmt.Printf("%s %s %s %s %12s %12s %12s %12s %s %s\n", modeCell(mode), coloredCell(symbol, 5, false, ansiBold+ansiCyan), coloredCell(side, 5, false, sideColor(side)), coloredCell(compactStatus(r.Outcome), 12, false, statusColor(r.Outcome)), price(r.Order.Price), price(r.Order.Stop), price(r.Order.TP1), price(r.Order.TP2), paint(signColor(r.RMultiple), fmt.Sprintf("%+7.2fR", r.RMultiple)), paint(signColor(pnl), fmt.Sprintf("%+8.2f", pnl)))
}

func modeCell(mode string) string {
	color := ansiBold + ansiBlue
	if mode == "LIVE" {
		color = ansiBold + ansiMagenta
	}
	return coloredCell("["+mode+"]", 7, false, color)
}

func shouldColor(mode string) bool {
	if os.Getenv("NO_COLOR") != "" || strings.EqualFold(mode, "never") {
		return false
	}
	if strings.EqualFold(mode, "always") {
		return true
	}
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0 && !strings.EqualFold(os.Getenv("TERM"), "dumb")
}

func shortError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if strings.Contains(message, "LIGHTER_ACCOUNT_INDEX") {
		return "LIGHTER_ACCOUNT_INDEX is not configured"
	}
	if strings.Contains(message, "LIGHTER_API_KEY_INDEX") {
		return "LIGHTER_API_KEY_INDEX is not configured"
	}
	if strings.Contains(message, "dial tcp") || strings.Contains(message, "connection refused") {
		return "connection unavailable - see service logs"
	}
	const limit = 68
	if len(message) > limit {
		return message[:limit-3] + "..."
	}
	return message
}

func paint(code, value string) string {
	if !colorEnabled || code == "" {
		return value
	}
	return code + value + ansiReset
}
func divider(char string, width int) {
	fmt.Println(paint(ansiDim+ansiBlue, strings.Repeat(char, width)))
}
func coloredCell(value string, width int, right bool, color string) string {
	if right {
		return paint(color, fmt.Sprintf("%*s", width, value))
	}
	return paint(color, fmt.Sprintf("%-*s", width, value))
}
func signColor(value float64) string {
	if value > 0 {
		return ansiBold + ansiGreen
	}
	if value < 0 {
		return ansiBold + ansiRed
	}
	return ansiDim
}
func signedMoney(value float64) string { return paint(signColor(value), fmt.Sprintf("%+.2f", value)) }
func signedR(value float64) string     { return paint(signColor(value), fmt.Sprintf("%+.2fR", value)) }
func sideColor(side string) string {
	if side == "LONG" {
		return ansiBlue
	}
	return ansiMagenta
}
func statusColor(status string) string {
	switch status {
	case "TP2", "TP2_COMPLETE":
		return ansiBold + ansiGreen
	case "STOPPED":
		return ansiBold + ansiRed
	case "TP1_THEN_BE", "TP1_THEN_STOP", "TP1_OPEN", "TP1→BE", "TP1/RUN":
		return ansiBold + ansiYellow
	case "OPEN", "FILLED":
		return ansiBold + ansiCyan
	case "NO_FILL", "WAITING_FOR_FILL", "WAIT PLAN":
		return ansiYellow
	default:
		return ""
	}
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
	account, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("LIGHTER_ACCOUNT_INDEX")), 10, 64)
	if err != nil {
		return nil, nil, fmt.Errorf("live account unavailable: LIGHTER_ACCOUNT_INDEX is not configured")
	}
	key, err := strconv.ParseUint(strings.TrimSpace(os.Getenv("LIGHTER_API_KEY_INDEX")), 10, 8)
	if err != nil {
		return nil, nil, fmt.Errorf("live account unavailable: LIGHTER_API_KEY_INDEX is not configured")
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
	markets, err := lighterexec.CheckPublic(ctx, os.Getenv("LIGHTER_BASE_URL"))
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

func readAccountSnapshot(ctx context.Context, client *lighterexec.Client, markets []lighterexec.Market, initErr error) (lighterexec.Snapshot, error) {
	if initErr != nil {
		return lighterexec.Snapshot{}, initErr
	}
	if client == nil {
		return lighterexec.Snapshot{}, fmt.Errorf("live account unavailable")
	}
	return client.ReadSnapshot(ctx, markets)
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
