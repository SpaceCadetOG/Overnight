package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	lightertx "github.com/elliottech/lighter-go/types/txtypes"
	"github.com/ogtrading/overnight-strategy/internal/execution"
	executionlighter "github.com/ogtrading/overnight-strategy/internal/execution/lighter"
	wsruntime "github.com/ogtrading/overnight-strategy/internal/execution/lighter/ws/runtime"
	"github.com/ogtrading/overnight-strategy/internal/forensics"
	"github.com/ogtrading/overnight-strategy/internal/journal"
	"github.com/ogtrading/overnight-strategy/internal/lighterexec"
	"github.com/ogtrading/overnight-strategy/internal/live"
	lighterdata "github.com/ogtrading/overnight-strategy/internal/marketdata/lighter"
	"github.com/ogtrading/overnight-strategy/internal/notify"
	"github.com/ogtrading/overnight-strategy/internal/store"
	"github.com/ogtrading/overnight-strategy/internal/universe"
)

const strategyVersion = "baseline-v1-20260810"

type liveRuntimeState struct {
	SchemaVersion    int                     `json:"schema_version"`
	SessionID        string                  `json:"session_id"`
	OpportunityID    string                  `json:"opportunity_id"`
	StrategyOrderID  string                  `json:"strategy_order_id"`
	TradeID          string                  `json:"trade_id"`
	Symbol           string                  `json:"symbol"`
	Order            execution.Order         `json:"order"`
	Managed          *execution.ManagedTrade `json:"managed_trade"`
	EntrySubmittedAt time.Time               `json:"entry_submitted_at"`
	UpdatedAt        time.Time               `json:"updated_at"`
	LastError        string                  `json:"last_error,omitempty"`
}

type app struct {
	root          string
	location      *time.Location
	events        *store.JSONL
	data          *lighterdata.Client
	markets       map[string]lighterdata.Market
	poll          time.Duration
	liveRequested bool
	liveExecutor  *executionlighter.Executor
	account       *lighterexec.Client
	liveMarkets   []lighterexec.Market
	lastPaperAt   time.Time
	lastHourlyAt  time.Time
	notifier      *notify.Client
}

func main() {
	root := flag.String("store", "data/research", "research event store")
	poll := flag.Duration("poll", 5*time.Second, "market/account reconciliation interval")
	liveRequested := flag.Bool("live", false, "enable the funded BTC/ETH route when the environment gate is also enabled")
	once := flag.Bool("once", false, "perform one reconciliation pass and exit")
	flag.Parse()
	if err := loadEnv(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
		fatal(err)
	}
	location, err := time.LoadLocation("America/Chicago")
	if err != nil {
		fatal(err)
	}
	events, err := store.NewJSONL(*root)
	if err != nil {
		fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	a := &app{root: *root, location: location, events: events, data: lighterdata.New(os.Getenv("LIGHTER_BASE_URL"), nil), poll: *poll, liveRequested: *liveRequested, notifier: notify.FromEnvironment()}
	if err := a.connect(ctx); err != nil {
		fatal(err)
	}
	fmt.Printf("trade runtime started paper=12/12 funded_requested=%t funded_enabled=%t\n", *liveRequested, a.liveExecutor != nil)
	a.notifier.BestEffort("Overnight Strategy Online", fmt.Sprintf("TradePi runtime started\nPaper markets: 12/12\nBTC/ETH funded route: %t", a.liveExecutor != nil), "high", "white_check_mark,chart_with_upwards_trend")
	for {
		if err := a.reconcile(ctx, time.Now().UTC()); err != nil {
			fmt.Fprintln(os.Stderr, "runtime reconciliation:", err)
			_ = events.Append("runtime_health", map[string]any{"timestamp": time.Now().UTC(), "status": "DEGRADED", "error": err.Error()})
			a.notifier.BestEffort("Overnight Runtime Degraded", err.Error(), "urgent", "warning")
		} else {
			_ = events.Append("runtime_health", map[string]any{"timestamp": time.Now().UTC(), "status": "PASS", "paper_assets": len(universe.All()), "funded_enabled": a.liveExecutor != nil})
		}
		a.hourlyNotice(time.Now().UTC())
		if *once {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(*poll):
		}
	}
}

func (a *app) connect(ctx context.Context) error {
	request, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	markets, err := a.data.MarketMap(request)
	if err != nil {
		return err
	}
	a.markets = markets
	for _, asset := range universe.All() {
		if _, ok := markets[asset.MarketSymbol()]; !ok {
			return fmt.Errorf("required market %s missing", asset.MarketSymbol())
		}
	}
	if !a.liveRequested {
		return nil
	}
	gate := execution.GateFromEnvironment(execution.Live)
	if err := gate.Authorize("BTC", time.Now()); err != nil {
		fmt.Fprintln(os.Stderr, "funded route remains disabled:", err)
		return nil
	}
	cfg, err := configFromEnv()
	if err != nil {
		return err
	}
	account, err := lighterexec.New(cfg)
	if err != nil {
		return err
	}
	if err := account.CheckCredentials(); err != nil {
		return err
	}
	public, err := lighterexec.CheckPublic(request, cfg.BaseURL)
	if err != nil {
		return err
	}
	for _, market := range public {
		if market.Symbol == "BTC" || market.Symbol == "ETH" {
			a.liveMarkets = append(a.liveMarkets, market)
		}
	}
	if len(a.liveMarkets) != 2 {
		return fmt.Errorf("BTC/ETH live market discovery incomplete")
	}
	executor, err := executionlighter.NewExecutor(cfg.BaseURL, cfg.PrivateKey, cfg.AccountIndex, cfg.APIKeyIndex, cfg.ChainID, wsruntime.NewOrderManager())
	if err != nil {
		return err
	}
	a.account, a.liveExecutor = account, executor
	return nil
}

func (a *app) reconcile(ctx context.Context, now time.Time) error {
	snapshots, err := store.ReadAll[live.MarketSnapshot](a.root, "market_snapshots")
	if err != nil {
		return err
	}
	latest := latestSnapshots(snapshots)
	intents, err := store.ReadAll[live.Intent](a.root, "paper_strategy_intents")
	if err != nil {
		return err
	}
	latestIntents := live.LatestIntents(intents)
	if a.lastPaperAt.IsZero() || now.Sub(a.lastPaperAt) >= time.Minute {
		for _, asset := range universe.All() {
			snapshot, ok := latest[asset.Symbol]
			if !ok || snapshot.Plan == nil || !snapshot.Plan.Valid {
				continue
			}
			intent, ok := intentFor(latestIntents, asset.Symbol, snapshot.OpportunityID)
			if !ok {
				continue
			}
			if err := a.reconcilePaper(ctx, now, asset, snapshot, intent); err != nil {
				return fmt.Errorf("%s paper: %w", asset.Symbol, err)
			}
		}
		a.lastPaperAt = now
	}
	if a.liveExecutor != nil {
		return a.reconcileLive(ctx, now, latest, latestIntents)
	}
	return nil
}

func (a *app) reconcilePaper(ctx context.Context, now time.Time, asset universe.Asset, snapshot live.MarketSnapshot, intent live.Intent) error {
	market := a.markets[asset.MarketSymbol()]
	spec, err := execution.SpecFromMarket(market)
	if err != nil {
		return err
	}
	side := "BUY"
	if snapshot.Plan.Direction == "SHORT" {
		side = "SELL"
	}
	expiry := time.Date(snapshot.SessionDate.Year(), snapshot.SessionDate.Month(), snapshot.SessionDate.Day(), 16, 0, 0, 0, a.location)
	order := spec.Normalize(execution.Order{Symbol: asset.MarketSymbol(), Side: side, Price: snapshot.Plan.Entry, Quantity: intent.Quantity, Stop: snapshot.Plan.Stop, TP1: snapshot.Plan.TP1, TP2: snapshot.Plan.TP2, ExpiresAt: expiry.Unix()})
	if err := spec.Validate(order); err != nil {
		return err
	}
	runID := "run_paper_" + strategyVersion + "_" + snapshot.SessionDate.Format("20060102")
	ids := forensics.IDs(snapshot.SessionDate, asset.Symbol, strategyVersion, forensics.PlanOpportunityKey(*snapshot.Plan), "PAPER", runID)
	trades, err := store.ReadAll[execution.PaperTrade](a.root, "paper_runtime_states")
	if err != nil {
		return err
	}
	trade, found := latestPaper(trades, ids.TradeID)
	if !found {
		trade = execution.PaperTrade{SchemaVersion: 1, SessionDate: snapshot.SessionDate, SessionID: ids.SessionID, OpportunityID: ids.OpportunityID, StrategyOrderID: ids.StrategyOrderID, TradeID: ids.TradeID, RunID: ids.RunID, Order: order, State: execution.Waiting, UpdatedAt: now}
	}
	start := time.Date(snapshot.SessionDate.Year(), snapshot.SessionDate.Month(), snapshot.SessionDate.Day(), 5, 0, 0, 0, a.location).UTC()
	from := start
	if !trade.LastCandleAt.IsZero() {
		from = trade.LastCandleAt
	}
	request, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	candles, err := a.data.Candles(request, market.MarketID, "5m", from, now.Truncate(5*time.Minute))
	if err != nil {
		return err
	}
	changed := !found
	for _, candle := range candles {
		var c bool
		trade, c, err = execution.AdvancePaper(trade, candle)
		if err != nil {
			return err
		}
		changed = changed || c
	}
	mark, _ := strconv.ParseFloat(fmt.Sprint(market.MarkPrice), 64)
	if expired, c := execution.ExpirePaper(trade, now, mark); c {
		trade, changed = expired, true
	}
	if changed {
		if err := a.events.Append("paper_runtime_states", trade); err != nil {
			return err
		}
		fmt.Printf("%-5s PAPER %-18s R=%+.2f\n", asset.Symbol, trade.State, trade.RMultiple)
		a.notifier.BestEffort("Paper Trade Update", fmt.Sprintf("%s %s\nResult: %+.2fR", asset.Symbol, trade.State, trade.RMultiple), "default", "memo")
	}
	if changed || !a.hasJournal(trade.TradeID) {
		if err := a.persistPaperForensics(snapshot, asset, trade, runID); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) hasJournal(tradeID string) bool {
	records, err := store.ReadAll[journal.TradeRecord](a.root, "trade_journal")
	if err != nil {
		return false
	}
	for _, record := range records {
		if record.ID == tradeID {
			return true
		}
	}
	return false
}

func (a *app) persistPaperForensics(snapshot live.MarketSnapshot, asset universe.Asset, trade execution.PaperTrade, runID string) error {
	lifecycle, err := forensics.PaperLifecycle(snapshot, trade, strategyVersion, runID)
	if err != nil {
		return err
	}
	existing, err := store.ReadAll[forensics.Envelope](a.root, "forensic_events")
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, event := range existing {
		seen[event.EventID] = true
	}
	for _, event := range lifecycle {
		if !seen[event.EventID] {
			if err := a.events.Append("forensic_events", event); err != nil {
				return err
			}
		}
	}
	record := journal.FromPaper(snapshot, asset.MarketSymbol(), strategyVersion, trade)
	record.ID, record.SessionID, record.OpportunityID, record.StrategyOrderID, record.RunID = trade.TradeID, trade.SessionID, trade.OpportunityID, trade.StrategyOrderID, trade.RunID
	if len(lifecycle) > 0 {
		record.ExchangeOrderID, record.CaseID = lifecycle[0].OrderID, lifecycle[0].CaseID
	}
	return a.events.Append("trade_journal", record)
}

func (a *app) reconcileLive(ctx context.Context, now time.Time, snapshots map[string]live.MarketSnapshot, intents map[string]live.Intent) error {
	request, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	account, err := a.account.ReadSnapshot(request, a.liveMarkets)
	if err != nil {
		return err
	}
	states, err := store.ReadAll[liveRuntimeState](a.root, "live_runtime_states")
	if err != nil {
		return err
	}
	latest := latestLive(states)
	for _, asset := range universe.Live() {
		snapshot, ok := snapshots[asset.Symbol]
		if !ok || snapshot.Plan == nil || !snapshot.Plan.Valid {
			continue
		}
		intent, ok := intentFor(intents, asset.Symbol, snapshot.OpportunityID)
		if !ok {
			continue
		}
		runID := "run_live_" + strategyVersion + "_" + snapshot.SessionDate.Format("20060102")
		ids := forensics.IDs(snapshot.SessionDate, asset.Symbol, strategyVersion, forensics.PlanOpportunityKey(*snapshot.Plan), "LIVE", runID)
		state, exists := latest[ids.TradeID]
		position, averageFill := positionFacts(account, asset.Symbol)
		mark := liveMark(a.liveMarkets, asset.Symbol)
		if !exists {
			if !execution.WithinOrderWindow(now, snapshot.SessionDate, a.location) {
				continue
			}
			market := a.markets[asset.MarketSymbol()]
			spec, e := execution.SpecFromMarket(market)
			if e != nil {
				return e
			}
			side := "BUY"
			if snapshot.Plan.Direction == "SHORT" {
				side = "SELL"
			}
			expiry := time.Date(snapshot.SessionDate.Year(), snapshot.SessionDate.Month(), snapshot.SessionDate.Day(), 16, 0, 0, 0, a.location)
			order := spec.Normalize(execution.Order{Symbol: asset.Symbol, Side: side, Price: snapshot.Plan.Entry, Quantity: intent.Quantity, Stop: snapshot.Plan.Stop, TP1: snapshot.Plan.TP1, TP2: snapshot.Plan.TP2, ExpiresAt: expiry.Unix()})
			if e = spec.Validate(order); e != nil {
				return e
			}
			managed, e := execution.NewManagedTrade(asset.Symbol, string(snapshot.Plan.Direction), order.Quantity, order.Price, order.Stop, order.TP1, order.TP2, expiry)
			if e != nil {
				return e
			}
			if e = managed.SetStrategyOrderID(ids.StrategyOrderID); e != nil {
				return e
			}
			response, e := a.liveExecutor.Submit(execution.OrderRequest{Symbol: asset.Symbol, Side: side, Price: order.Price, Size: order.Quantity, ExpiresAt: expiry, OrderType: lightertx.LimitOrder, ClientOrderIndex: managed.EntryOrderIndex, RiskUSD: intent.RiskUSD, RiskLimitUSD: live.DefaultRiskPolicy().RiskPerAssetUSD})
			if e != nil {
				return e
			}
			if e = managed.SetEntryOrderID(response.OrderID); e != nil {
				return e
			}
			state = liveRuntimeState{SchemaVersion: 1, SessionID: ids.SessionID, OpportunityID: ids.OpportunityID, StrategyOrderID: ids.StrategyOrderID, TradeID: ids.TradeID, Symbol: asset.Symbol, Order: order, Managed: managed, EntrySubmittedAt: now, UpdatedAt: now}
			if e = a.events.Append("live_runtime_states", state); e != nil {
				return e
			}
			if e = a.persistLiveJournal(snapshot, asset, state); e != nil {
				return e
			}
			fmt.Printf("%-5s LIVE ENTRY SUBMITTED qty=%.8f tx=%s\n", asset.Symbol, order.Quantity, response.OrderID)
			a.notifier.BestEffort("LIVE Order Submitted", fmt.Sprintf("%s %s\nEntry: %.8f\nQuantity: %.8f", asset.Symbol, side, order.Price, order.Quantity), "high", "rotating_light")
			continue
		}
		if state.Managed == nil || state.Managed.State == execution.ProtectionClosed {
			continue
		}
		changed := false
		if state.Managed.State == execution.ProtectionWaiting && position > 0 {
			if averageFill > 0 {
				state.Managed.Fill = averageFill
			}
			err = state.Managed.OnEntryFilled(a.liveExecutor)
			changed = err == nil
		} else if state.Managed.State == execution.ProtectionInitial && position > 0 && position <= state.Managed.RunnerQuantity+quantityTolerance(asset.Symbol) {
			err = state.Managed.OnTP1Filled(a.liveExecutor)
			changed = err == nil
		} else if state.Managed.State != execution.ProtectionWaiting && position == 0 {
			err = state.Managed.OnClosed(a.liveExecutor)
			changed = err == nil
		} else if !now.Before(state.Managed.Expiry) {
			err = state.Managed.OnExpiry(a.liveExecutor, position, mark)
			changed = err == nil
		}
		if err != nil {
			state.LastError = err.Error()
			changed = true
		}
		if changed {
			state.UpdatedAt = now
			if e := a.events.Append("live_runtime_states", state); e != nil {
				return e
			}
			if e := a.persistLiveJournal(snapshot, asset, state); e != nil {
				return e
			}
			fmt.Printf("%-5s LIVE %s position=%.8f\n", asset.Symbol, state.Managed.State, position)
			a.notifier.BestEffort("LIVE Trade Update", fmt.Sprintf("%s %s\nPosition: %.8f\nFill: %.8f", asset.Symbol, state.Managed.State, position, state.Managed.Fill), "high", "rotating_light")
		}
	}
	return nil
}

func (a *app) hourlyNotice(now time.Time) {
	if !a.notifier.Enabled() || (!a.lastHourlyAt.IsZero() && now.Sub(a.lastHourlyAt) < time.Hour) {
		return
	}
	states, err := store.ReadAll[execution.PaperTrade](a.root, "paper_runtime_states")
	if err != nil {
		return
	}
	latest := map[string]execution.PaperTrade{}
	for _, state := range states {
		if old, ok := latest[state.TradeID]; !ok || state.UpdatedAt.After(old.UpdatedAt) {
			latest[state.TradeID] = state
		}
	}
	filled, waiting, closed, noFill, totalR := 0, 0, 0, 0, 0.0
	for _, state := range latest {
		switch state.State {
		case execution.Waiting:
			waiting++
		case execution.PaperNoFill:
			noFill++
		case execution.PaperClosed:
			closed++
		default:
			filled++
		}
		totalR += state.RMultiple
	}
	a.notifier.BestEffort("Overnight Session Update", fmt.Sprintf("Tracked: %d/12\nFilled/open: %d\nWaiting: %d\nClosed: %d\nNo fill: %d\nRealized result: %+.2fR", len(latest), filled, waiting, closed, noFill, totalR), "default", "bar_chart")
	a.lastHourlyAt = now
}

func (a *app) persistLiveJournal(snapshot live.MarketSnapshot, asset universe.Asset, state liveRuntimeState) error {
	paperState, outcome := execution.Waiting, "WAITING_FOR_FILL"
	switch state.Managed.State {
	case execution.ProtectionInitial:
		paperState, outcome = execution.PaperFilled, "OPEN"
	case execution.ProtectionRunner:
		paperState, outcome = execution.PaperTP1, "TP1_OPEN"
	case execution.ProtectionClosed:
		paperState, outcome = execution.PaperClosed, "CLOSED"
	}
	record := journal.FromLive(snapshot, asset.MarketSymbol(), strategyVersion, state.Order, journal.LiveExecution{OrderID: state.Managed.EntryOrderID, State: string(paperState), Outcome: outcome, ActualFill: state.Managed.Fill, TP1Hit: state.Managed.State == execution.ProtectionRunner || state.Managed.State == execution.ProtectionClosed})
	record.ID, record.SessionID, record.OpportunityID, record.StrategyOrderID = state.TradeID, state.SessionID, state.OpportunityID, state.StrategyOrderID
	record.RunID = "run_live_" + strategyVersion + "_" + snapshot.SessionDate.Format("20060102")
	return a.events.Append("trade_journal", record)
}

func latestSnapshots(values []live.MarketSnapshot) map[string]live.MarketSnapshot {
	out := map[string]live.MarketSnapshot{}
	for _, v := range values {
		if old, ok := out[v.Symbol]; !ok || v.SessionDate.After(old.SessionDate) {
			out[v.Symbol] = v
		}
	}
	return out
}
func intentFor(values map[string]live.Intent, symbol, opportunity string) (live.Intent, bool) {
	for _, v := range values {
		if v.Symbol == symbol && v.OpportunityID == opportunity {
			return v, true
		}
	}
	return live.Intent{}, false
}
func latestPaper(values []execution.PaperTrade, id string) (execution.PaperTrade, bool) {
	var out execution.PaperTrade
	ok := false
	for _, v := range values {
		if v.TradeID == id && (!ok || v.UpdatedAt.After(out.UpdatedAt)) {
			out, ok = v, true
		}
	}
	return out, ok
}
func latestLive(values []liveRuntimeState) map[string]liveRuntimeState {
	out := map[string]liveRuntimeState{}
	for _, v := range values {
		if old, ok := out[v.TradeID]; !ok || v.UpdatedAt.After(old.UpdatedAt) {
			out[v.TradeID] = v
		}
	}
	return out
}
func positionFacts(s lighterexec.Snapshot, symbol string) (float64, float64) {
	for _, p := range s.Positions {
		if fmt.Sprint(p["symbol"]) == symbol {
			n, _ := strconv.ParseFloat(fmt.Sprint(p["position"]), 64)
			fill, _ := strconv.ParseFloat(fmt.Sprint(p["avg_entry_price"]), 64)
			return math.Abs(n), fill
		}
	}
	return 0, 0
}
func liveMark(markets []lighterexec.Market, symbol string) float64 {
	for _, m := range markets {
		if m.Symbol == symbol {
			n, _ := strconv.ParseFloat(fmt.Sprint(m.MarkPrice), 64)
			return n
		}
	}
	return 0
}
func quantityTolerance(symbol string) float64 {
	if symbol == "BTC" {
		return .000005
	}
	return .00005
}

func configFromEnv() (lighterexec.Config, error) {
	account, e := strconv.ParseInt(strings.TrimSpace(os.Getenv("LIGHTER_ACCOUNT_INDEX")), 10, 64)
	if e != nil {
		return lighterexec.Config{}, fmt.Errorf("invalid LIGHTER_ACCOUNT_INDEX")
	}
	key, e := strconv.ParseUint(strings.TrimSpace(os.Getenv("LIGHTER_API_KEY_INDEX")), 10, 8)
	if e != nil {
		return lighterexec.Config{}, fmt.Errorf("invalid LIGHTER_API_KEY_INDEX")
	}
	chain, e := strconv.ParseUint(strings.TrimSpace(os.Getenv("LIGHTER_CHAIN_ID")), 10, 32)
	if e != nil {
		return lighterexec.Config{}, fmt.Errorf("invalid LIGHTER_CHAIN_ID")
	}
	return lighterexec.Config{BaseURL: os.Getenv("LIGHTER_BASE_URL"), WSURL: os.Getenv("LIGHTER_WS_URL"), AccountIndex: account, APIKeyIndex: uint8(key), PrivateKey: os.Getenv("LIGHTER_API_PRIVATE_KEY"), ChainID: uint32(chain)}, nil
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
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), "'\"")
		if _, exists := os.LookupEnv(k); !exists {
			_ = os.Setenv(k, v)
		}
	}
	return s.Err()
}
func fatal(err error) { fmt.Fprintln(os.Stderr, "FAIL:", err); os.Exit(1) }
