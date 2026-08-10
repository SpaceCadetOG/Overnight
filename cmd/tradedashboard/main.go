package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/execution"
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

type recorderMark struct {
	Price float64
	At    time.Time
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
var cachedAccount lighterexec.Snapshot
var cachedAccountAt time.Time

func main() {
	root := flag.String("store", "data/test-run", "paper test-run store")
	marketDataRoot := flag.String("market-data-root", "data/live/lighter", "local Lighter recorder root")
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
			detailedScreen(*root, *marketDataRoot, *paperEquity, client, markets, err)
		} else {
			screen(*root, *marketDataRoot, *paperEquity, client, markets, err)
		}
		if *once {
			return
		}
		time.Sleep(*refresh)
	}
}

func detailedScreen(root, marketDataRoot string, paperEquity float64, client *lighterexec.Client, accountMarkets []lighterexec.Market, accountInitErr error) {
	now := time.Now().UTC()
	location, _ := time.LoadLocation("America/Chicago")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	account, accountErr := readAccountSnapshot(ctx, client, accountMarkets, accountInitErr)
	marks := readRecorderMarks(marketDataRoot, now, location)
	records, _ := store.ReadAll[journal.TradeRecord](root, "trade_journal")
	latest := currentRecords(records, now.In(location), location)
	overlayPaperRuntime(root, latest, now.In(location), location)
	var realized, openPnL, totalR, riskCommitted float64
	filled, waiting, noFill, wins, losses, open := 0, 0, 0, 0, 0, 0
	for _, r := range latest {
		risk := paperEquity * .005
		riskCommitted += risk
		resultR := r.RMultiple
		if activeTrade(r) {
			resultR, _, _, _ = paperMarkToMarket(r, markForRecord(marks, r).Price, risk)
		}
		totalR += resultR
		switch displayStatus(r) {
		case "NO_FILL":
			noFill++
		case "OPEN", "FILLED", "TP1/RUN":
			open++
			openPnL += resultR * risk
		default:
			filled++
			realized += resultR * risk
			if resultR > 0 {
				wins++
			} else if resultR < 0 {
				losses++
			}
		}
	}
	fmt.Print("\033[2J\033[H")
	fmt.Printf("%s %s %s\n", paint(ansiBold+ansiCyan, "OVERNIGHT STRATEGY — CONTROL ROOM"), paint(ansiBold+ansiBlue, "[PAPER]"), paint(ansiBold+ansiMagenta, "[LIVE]"))
	fmt.Printf("%s / %s\n", paint(ansiBold, now.Format("2006-01-02 15:04:05 UTC")), paint(ansiDim+ansiCyan, now.In(location).Format("15:04:05 CT")))
	detailedDivider("=")
	currentEquity := paperEquity + realized + openPnL
	weeklyR := priorWeeklyR(records, now, location) + totalR
	liveLabel, fundedLabel, liveColor, fundedColor := executionDisplay(execution.GateFromEnvironment(execution.Live))
	fmt.Printf("%s  Start $%.2f | Equity %s | Realized %s | Unrealized %s\n", paint(ansiBold+ansiBlue, "PAPER ACCOUNT"), paperEquity, paint(signColor(currentEquity-paperEquity), fmt.Sprintf("$%.2f", currentEquity)), signedMoney(realized), signedMoney(openPnL))
	fmt.Printf("%s DAILY R %s | WEEKLY R %s | Risk $%.2f | %s\n", strings.Repeat(" ", 15), signedR(totalR), signedR(weeklyR), riskCommitted, paint(fundedColor, fundedLabel))
	if accountErr == nil {
		fmt.Printf("%s Balance $%.2f | Available $%.2f | Margin $%.2f | Positions %d | Orders %d\n", paint(liveColor, liveLabel), value(account.Account, "collateral"), value(account.Account, "available_balance"), max0(value(account.Account, "collateral")-value(account.Account, "available_balance")), openPositions(account.Positions), len(account.Orders))
	} else {
		fmt.Printf("%s %s\n", paint(liveColor, liveLabel), paint(ansiBold+ansiRed, "ERROR: "+shortError(accountErr)))
	}
	printActiveTrades(latest, nil, account.Positions, marks, paperEquity, accountErr == nil)
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
		mark := marks[asset.MarketSymbol()].Price
		risk := paperEquity * .005
		printDetailedRow("PAPER", asset.Symbol, side, displayStatus(r), price(r.Order.Price), price(mark), price(r.Order.Stop), price(r.Order.TP1), price(r.Order.TP2))
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

// priorWeeklyR returns completed paper results from Monday 00:00 CT through
// yesterday. The caller adds today's mark-to-market R so active trades are
// represented exactly once.
func priorWeeklyR(records []journal.TradeRecord, now time.Time, location *time.Location) float64 {
	today := chicagoDayStart(now, location)
	daysSinceMonday := (int(today.Weekday()) + 6) % 7
	weekStart := today.AddDate(0, 0, -daysSinceMonday)
	latest := map[string]journal.TradeRecord{}
	for _, r := range records {
		if r.Mode == "LIVE_EXECUTION" {
			continue
		}
		session := chicagoDayStart(r.SessionDate, location)
		if session.Before(weekStart) || !session.Before(today) {
			continue
		}
		key := session.Format("2006-01-02") + "|" + r.Symbol
		if old, ok := latest[key]; !ok || r.RecordedAt.After(old.RecordedAt) {
			latest[key] = r
		}
	}
	var total float64
	for _, r := range latest {
		total += r.RMultiple
	}
	return total
}

func chicagoDayStart(t time.Time, location *time.Location) time.Time {
	local := t.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
}

func screen(root, marketDataRoot string, paperEquity float64, client *lighterexec.Client, markets []lighterexec.Market, accountInitErr error) {
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
	marks := readRecorderMarks(marketDataRoot, now, location)
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
	overlayPaperRuntime(root, latest, today, location)
	var totalR, realized, openPnL, riskCommitted float64
	for _, r := range latest {
		risk := paperEquity * .005
		riskCommitted += risk
		resultR := r.RMultiple
		if activeTrade(r) {
			resultR, _, _, _ = paperMarkToMarket(r, markForRecord(marks, r).Price, risk)
		}
		totalR += resultR
		if activeTrade(r) {
			openPnL += resultR * risk
		} else {
			realized += resultR * risk
		}
	}
	fmt.Print("\033[2J\033[H")
	fmt.Printf("%s %s %s                         %s\n", paint(ansiBold+ansiCyan, "OVERNIGHT TRADING BOARD"), paint(ansiBold+ansiBlue, "[PAPER]"), paint(ansiBold+ansiMagenta, "[LIVE]"), paint(ansiBold, now.Format("2006-01-02 15:04:05 UTC")))
	divider("=", 112)
	liveLabel, fundedLabel, liveColor, fundedColor := executionDisplay(execution.GateFromEnvironment(execution.Live))
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
		fmt.Printf("%s  Bal $%.2f  Eq %s  Avail $%.2f  Margin $%.2f  uPnL %s  Pos %d  Orders %d  %s\n", paint(liveColor, "LIVE"), balance, signedMoney(equity), available, used, signedMoney(upnl), openPositions(snapshot.Positions), len(snapshot.Orders), paint(fundedColor, "["+liveLabel+"]"))
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
	weeklyR := priorWeeklyR(records, now, location) + totalR
	fmt.Printf("%s Start $%.2f  Equity %s  Realized %s  Open %s  Daily R %s  Weekly R %s  Risk $%.2f\n", paint(ansiBold+ansiBlue, "PAPER"), paperEquity, paint(signColor(currentEquity-paperEquity), fmt.Sprintf("$%.2f", currentEquity)), signedMoney(realized), signedMoney(openPnL), signedR(totalR), signedR(weeklyR), riskCommitted)
	printActiveTrades(latest, liveLatest, snapshot.Positions, marks, paperEquity, accountErr == nil)
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
	fmt.Printf("%s | %s | %s %s\n", paint(ansiBold+ansiCyan, "12 PAPER SYSTEMS"), paint(fundedColor, fundedLabel), paint(ansiBold+ansiBlue, "NEXT PLAN"), paint(ansiBold, nextPlan(now, location).Format("2006-01-02 15:04 UTC")))
}

func executionDisplay(gate execution.Gate) (liveLabel, fundedLabel, liveColor, fundedColor string) {
	if gate.KillSwitch {
		return "LIVE KILLED", "FUNDED BLOCKED", ansiBold + ansiRed, ansiBold + ansiRed
	}
	if gate.FundedEnabled {
		return "LIVE FUNDED", "FUNDED ON", ansiBold + ansiGreen, ansiBold + ansiGreen
	}
	return "LIVE READ-ONLY", "FUNDED OFF", ansiBold + ansiCyan, ansiBold + ansiYellow
}

func markForRecord(marks map[string]recorderMark, record journal.TradeRecord) recorderMark {
	symbol := record.ExchangeSymbol
	if symbol == "" {
		symbol = record.Symbol
	}
	return marks[symbol]
}

func printTradeRow(mode, symbol, side string, r journal.TradeRecord, pnl float64) {
	status := displayStatus(r)
	fmt.Printf("%s %s %s %s %12s %12s %12s %12s %s %s\n", modeCell(mode), coloredCell(symbol, 5, false, ansiBold+ansiCyan), coloredCell(side, 5, false, sideColor(side)), coloredCell(status, 12, false, statusColor(status)), price(r.Order.Price), price(r.Order.Stop), price(r.Order.TP1), price(r.Order.TP2), paint(signColor(r.RMultiple), fmt.Sprintf("%+7.2fR", r.RMultiple)), paint(signColor(pnl), fmt.Sprintf("%+8.2f", pnl)))
}

func overlayPaperRuntime(root string, records map[string]journal.TradeRecord, today time.Time, location *time.Location) {
	states, err := store.ReadAll[execution.PaperTrade](root, "paper_runtime_states")
	if err != nil {
		return
	}
	for _, state := range states {
		if state.SessionDate.In(location).Format("2006-01-02") != today.Format("2006-01-02") {
			continue
		}
		symbol := state.Order.Symbol
		asset, ok := universe.Find(symbol)
		if ok {
			symbol = asset.Symbol
		}
		record, exists := records[symbol]
		if !exists {
			record = journal.TradeRecord{Symbol: symbol, Mode: "PAPER_EXECUTION", SessionDate: state.SessionDate, Order: state.Order}
		}
		record.State, record.Outcome, record.RMultiple = state.State, state.Outcome, state.RMultiple
		record.ActualFill, record.ExitPrice, record.MFE, record.MAE, record.TP1Hit, record.RecordedAt = state.FillPrice, state.ExitPrice, state.MFE, state.MAE, state.TP1Hit, state.UpdatedAt
		risk := math.Abs(state.Order.Price - state.Order.Stop)
		if risk > 0 {
			record.MFER, record.MAER = state.MFE/risk, state.MAE/risk
		}
		records[symbol] = record
	}
}

func displayStatus(r journal.TradeRecord) string {
	if strings.TrimSpace(r.Outcome) != "" {
		return compactStatus(r.Outcome)
	}
	switch r.State {
	case execution.Waiting:
		return "WAIT FILL"
	case execution.PaperFilled:
		return "FILLED"
	case execution.PaperTP1:
		return "TP1/RUN"
	case execution.PaperClosed:
		return "CLOSED"
	case execution.PaperNoFill:
		return "NO FILL"
	default:
		return "PLANNED"
	}
}

func activeTrade(r journal.TradeRecord) bool {
	return r.State == execution.PaperFilled || r.State == execution.PaperTP1 || r.Outcome == "OPEN" || r.Outcome == "TP1_OPEN"
}

func printActiveTrades(paper, liveRecords map[string]journal.TradeRecord, positions []map[string]any, marks map[string]recorderMark, paperEquity float64, accountFresh bool) {
	count := 0
	for _, record := range paper {
		if activeTrade(record) {
			count++
		}
	}
	for _, position := range positions {
		if value(position, "position", "size") != 0 {
			count++
		}
	}
	if count == 0 {
		fmt.Printf("%s %s\n", paint(ansiBold+ansiBlue, "ACTIVE TRADES"), paint(ansiDim, "NONE"))
		return
	}
	fmt.Printf("%s %d %s\n", paint(ansiBold+ansiBlue, "ACTIVE TRADES"), count, paint(ansiDim, "(real-time local marks)"))
	for _, asset := range universe.All() {
		record, ok := paper[asset.Symbol]
		if !ok || !activeTrade(record) {
			continue
		}
		side := "LONG"
		if record.Order.Side == "SELL" {
			side = "SHORT"
		}
		quote := marks[asset.MarketSymbol()]
		liveR, livePnL, remaining, next := paperMarkToMarket(record, quote.Price, paperEquity*.005)
		fmt.Printf("  %s %s %s %s  Qty %.6f  Rem %d%%\n", modeCell("PAPER"), coloredCell(asset.Symbol, 5, false, ansiBold+ansiCyan), coloredCell(side, 5, false, sideColor(side)), paint(statusColor(displayStatus(record)), displayStatus(record)), record.Order.Quantity*float64(remaining)/100, remaining)
		fmt.Printf("          Entry %s  Mark %s%s  Live %s / %s  Next %s\n", price(record.Order.Price), price(quote.Price), markFreshness(quote), signedR(liveR), signedMoney(livePnL), paint(ansiYellow, next))
	}
	for _, position := range positions {
		size := value(position, "position", "size")
		if size == 0 {
			continue
		}
		symbol := fmt.Sprint(position["symbol"])
		side := "LONG"
		if size < 0 {
			side = "SHORT"
		}
		entry := value(position, "avg_entry_price", "entry_price")
		upnl := value(position, "unrealized_pnl", "unrealized_pnl_usdc", "unrealized_profit")
		quote := marks[symbol]
		status := "LIVE POSITION"
		if record, ok := liveRecords[symbol]; ok {
			status = displayStatus(record)
		}
		fmt.Printf("  %s %s %s %s  Qty %.8f\n", modeCell("LIVE"), coloredCell(symbol, 5, false, ansiBold+ansiCyan), coloredCell(side, 5, false, sideColor(side)), paint(ansiBold+ansiMagenta, status), math.Abs(size))
		accountTag, accountColor := "[ACCOUNT LIVE]", ansiBold+ansiGreen
		if !accountFresh {
			accountTag, accountColor = "[ACCOUNT STALE]", ansiBold+ansiYellow
		}
		fmt.Printf("          Avg fill %s  Mark %s%s  uPnL %s  %s %s\n", price(entry), price(quote.Price), markFreshness(quote), signedMoney(upnl), paint(ansiBold+ansiRed, "EXCHANGE POSITION"), paint(accountColor, accountTag))
	}
}

func paperMarkToMarket(record journal.TradeRecord, mark, riskUSD float64) (float64, float64, int, string) {
	if mark <= 0 {
		return record.RMultiple, record.RMultiple * riskUSD, map[bool]int{true: 50, false: 100}[record.TP1Hit], "MARK UNAVAILABLE"
	}
	risk := math.Abs(record.Order.Price - record.Order.Stop)
	if risk == 0 {
		return 0, 0, 100, "INVALID RISK"
	}
	runnerR := (mark - record.Order.Price) / risk
	if record.Order.Side == "SELL" {
		runnerR = -runnerR
	}
	remaining, next, totalR := 100, "TP1 OR STOP", runnerR
	if record.State == execution.PaperTP1 || record.TP1Hit {
		remaining, next = 50, "TP2 OR BREAKEVEN"
		tp1R := math.Abs(record.Order.TP1-record.Order.Price) / risk
		totalR = .5*tp1R + .5*runnerR
	}
	return totalR, totalR * riskUSD, remaining, next
}

func markFreshness(mark recorderMark) string {
	if mark.Price <= 0 {
		return paint(ansiBold+ansiRed, " [NO MARK]")
	}
	if mark.At.IsZero() || time.Since(mark.At) > 30*time.Second {
		return paint(ansiBold+ansiYellow, " [STALE]")
	}
	return paint(ansiBold+ansiGreen, " [LIVE]")
}

func readRecorderMarks(root string, now time.Time, location *time.Location) map[string]recorderMark {
	marks := map[string]recorderMark{}
	day := "date=" + now.In(location).Format("2006-01-02")
	for _, asset := range universe.All() {
		line, err := lastJSONLine(filepath.Join(root, day, "asset="+asset.MarketSymbol(), "ticker_events.jsonl"))
		if err != nil {
			continue
		}
		var record struct {
			ReceivedAt time.Time `json:"received_at"`
			Event      struct {
				Ticker struct {
					Ask struct {
						Price string `json:"price"`
					} `json:"a"`
					Bid struct {
						Price string `json:"price"`
					} `json:"b"`
				} `json:"ticker"`
			} `json:"event"`
		}
		if json.Unmarshal(line, &record) != nil {
			continue
		}
		ask, _ := strconv.ParseFloat(record.Event.Ticker.Ask.Price, 64)
		bid, _ := strconv.ParseFloat(record.Event.Ticker.Bid.Price, 64)
		mark := 0.0
		if ask > 0 && bid > 0 {
			mark = (ask + bid) / 2
		} else if ask > 0 {
			mark = ask
		} else {
			mark = bid
		}
		marks[asset.MarketSymbol()] = recorderMark{Price: mark, At: record.ReceivedAt}
	}
	return marks
}

func lastJSONLine(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	const window int64 = 64 * 1024
	start := info.Size() - window
	if start < 0 {
		start = 0
	}
	if _, err = file.Seek(start, 0); err != nil {
		return nil, err
	}
	buffer, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	complete := len(buffer) > 0 && buffer[len(buffer)-1] == '\n'
	lines := bytes.Split(bytes.TrimSpace(buffer), []byte("\n"))
	if !complete && len(lines) > 1 {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty ticker stream")
	}
	return lines[len(lines)-1], nil
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
	lower := strings.ToLower(message)
	if strings.Contains(lower, "http 503") || strings.Contains(lower, "503 service temporarily unavailable") {
		return "Lighter API temporarily unavailable (HTTP 503)"
	}
	if strings.Contains(lower, "http 502") {
		return "Lighter API temporarily unavailable (HTTP 502)"
	}
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
	case "NO_FILL", "NO FILL", "WAITING_FOR_FILL", "WAIT FILL", "WAIT PLAN", "PLANNED":
		return ansiYellow
	case "CLOSED":
		return ansiDim
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
		if !cachedAccountAt.IsZero() {
			return cachedAccount, initErr
		}
		return lighterexec.Snapshot{}, initErr
	}
	if client == nil {
		return lighterexec.Snapshot{}, fmt.Errorf("live account unavailable")
	}
	snapshot, err := client.ReadSnapshot(ctx, markets)
	if err != nil {
		if !cachedAccountAt.IsZero() {
			return cachedAccount, err
		}
		return lighterexec.Snapshot{}, err
	}
	cachedAccount, cachedAccountAt = snapshot, time.Now().UTC()
	return snapshot, nil
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
