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
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	lightertx "github.com/elliottech/lighter-go/types/txtypes"
	adapter "github.com/ogtrading/lighter-adapter/lighter"
	"github.com/ogtrading/overnight-strategy/internal/buildinfo"
	"github.com/ogtrading/overnight-strategy/internal/execution"
	executionlighter "github.com/ogtrading/overnight-strategy/internal/execution/lighter"
	"github.com/ogtrading/overnight-strategy/internal/forensics"
	"github.com/ogtrading/overnight-strategy/internal/journal"
	"github.com/ogtrading/overnight-strategy/internal/ledger"
	"github.com/ogtrading/overnight-strategy/internal/lighterexec"
	"github.com/ogtrading/overnight-strategy/internal/live"
	lighterdata "github.com/ogtrading/overnight-strategy/internal/marketdata/lighter"
	"github.com/ogtrading/overnight-strategy/internal/notify"
	"github.com/ogtrading/overnight-strategy/internal/store"
	"github.com/ogtrading/overnight-strategy/internal/universe"
)

const strategyVersion = "baseline-v1-20260810"

var (
	cycle1StartUTC = time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	cycle1EndUTC   = time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
)

type liveRuntimeState struct {
	SchemaVersion      int                     `json:"schema_version"`
	LifecycleVersion   string                  `json:"lifecycle_version"`
	SessionID          string                  `json:"session_id"`
	OpportunityID      string                  `json:"opportunity_id"`
	StrategyOrderID    string                  `json:"strategy_order_id"`
	TradeID            string                  `json:"trade_id"`
	Symbol             string                  `json:"symbol"`
	Order              execution.Order         `json:"order"`
	Managed            *execution.ManagedTrade `json:"managed_trade"`
	EntrySubmittedAt   time.Time               `json:"entry_submitted_at"`
	UpdatedAt          time.Time               `json:"updated_at"`
	LastError          string                  `json:"last_error,omitempty"`
	InitialRiskUSD     float64                 `json:"initial_risk_usd"`
	ClientOrderIndex   int64                   `json:"client_order_index,omitempty"`
	ExchangeOrderIndex int64                   `json:"exchange_order_index,omitempty"`
	MarketID           int16                   `json:"market_id,omitempty"`
	Nonce              int64                   `json:"nonce,omitempty"`
	EncodedBaseAmount  int64                   `json:"encoded_base_amount,omitempty"`
	EncodedPrice       uint32                  `json:"encoded_price,omitempty"`
	RequestedQuantity  float64                 `json:"requested_quantity,omitempty"`
	RequestedPrice     float64                 `json:"requested_price,omitempty"`
	ExecutionStatus    string                  `json:"execution_status,omitempty"`
}

type liveAssetOutcome struct {
	Timestamp         time.Time `json:"timestamp"`
	SessionID         string    `json:"session_id"`
	OpportunityID     string    `json:"opportunity_id"`
	StrategyOrderID   string    `json:"strategy_order_id"`
	TradeID           string    `json:"trade_id"`
	Symbol            string    `json:"symbol"`
	Status            string    `json:"status"`
	Reason            string    `json:"reason,omitempty"`
	RiskUSD           float64   `json:"risk_usd,omitempty"`
	RequestedNotional float64   `json:"requested_notional,omitempty"`
	MinimumNotional   float64   `json:"minimum_notional,omitempty"`
	StrategyVersion   string    `json:"strategy_version"`
	RuntimeVersion    string    `json:"runtime_version"`
}

type app struct {
	root             string
	location         *time.Location
	events           *store.JSONL
	data             *lighterdata.Client
	markets          map[string]lighterdata.Market
	poll             time.Duration
	liveRequested    bool
	liveExecutor     *executionlighter.Executor
	account          *lighterexec.Client
	liveMarkets      []lighterexec.Market
	lastPaperAt      time.Time
	lastHourlyAt     time.Time
	notifier         *notify.Client
	degraded         bool
	lastAlertAt      time.Time
	lastAlertKey     string
	healthySince     time.Time
	researchIssues   map[string]string
	researchIncident bool
	accountIndex     int64
	seenFillIDs      map[string]bool
	freshReconciles  int
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
	a := &app{root: *root, location: location, events: events, data: lighterdata.New(os.Getenv("LIGHTER_BASE_URL"), nil), poll: *poll, liveRequested: *liveRequested, notifier: notify.FromEnvironment().WithReceiptStore(events), researchIssues: map[string]string{}, seenFillIDs: map[string]bool{}}
	if err := a.connect(ctx); err != nil {
		fatal(err)
	}
	_ = events.Append("runtime_deployments", map[string]any{"timestamp": time.Now().UTC(), "strategy_version": strategyVersion, "runtime_version": execution.LifecycleVersion, "code_version": buildinfo.Version, "code_commit": buildinfo.Commit, "built_at": buildinfo.BuiltAt, "funded_enabled": a.liveExecutor != nil, "historical_records_mutated": false})
	fmt.Printf("trade runtime started paper=12/12 funded_requested=%t funded_enabled=%t\n", *liveRequested, a.liveExecutor != nil)
	a.notifier.BestEffort("Overnight Strategy Online", fmt.Sprintf("TradePi runtime started\nPaper markets: 12/12\nBTC/ETH funded route: %t", a.liveExecutor != nil), "high", "white_check_mark,chart_with_upwards_trend")
	go a.telegramCommands(ctx)
	for {
		now := time.Now().UTC()
		checked, err := a.reconcile(ctx, now)
		if err != nil {
			fmt.Fprintln(os.Stderr, "runtime reconciliation:", err)
			_ = events.Append("runtime_health", map[string]any{"timestamp": now, "status": "DEGRADED", "error": err.Error()})
			a.notifyDegraded(now, err)
		} else {
			_ = events.Append("runtime_health", map[string]any{"timestamp": now, "status": "PASS", "paper_assets": len(universe.All()), "funded_enabled": a.liveExecutor != nil})
			if checked {
				a.notifyRecovered(now)
			}
		}
		a.halfHourlyNotice(time.Now().UTC())
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
	riskConfig, err := riskConfigFromEnv()
	if err != nil {
		return err
	}
	executor, err := executionlighter.NewExecutor(request, executionlighter.Config{
		BaseURL: cfg.BaseURL, WSURL: cfg.WSURL, PrivateKey: cfg.PrivateKey, AccountIndex: cfg.AccountIndex,
		APIKeyIndex: cfg.APIKeyIndex, ChainID: cfg.ChainID, StateRoot: filepath.Join(a.root, "runtime"), Risk: riskConfig,
	})
	if err != nil {
		return err
	}
	executor.StartPrivateStream(ctx)
	go a.capturePrivateExecutionEvents(ctx, executor)
	a.account, a.liveExecutor = account, executor
	a.accountIndex = cfg.AccountIndex
	oldFills, _ := store.ReadAll[ledger.VenueFill](a.root, "venue_fills")
	for _, fill := range oldFills {
		a.seenFillIDs[fill.FillID] = true
	}
	return nil
}

func (a *app) capturePrivateExecutionEvents(ctx context.Context, executor *executionlighter.Executor) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-executor.Events():
			if event.Order != nil {
				if err := executor.ObserveOrder(event.Order); err != nil && !strings.Contains(err.Error(), "is not tracked") {
					fmt.Fprintf(os.Stderr, "private order reconciliation failed: %v\n", err)
				}
			}
			if err := a.events.Append("private_execution_events", event); err != nil {
				fmt.Fprintf(os.Stderr, "private execution event persistence failed: %v\n", err)
			}
		}
	}
}

func (a *app) reconcile(ctx context.Context, now time.Time) (bool, error) {
	snapshots, err := store.ReadAll[live.MarketSnapshot](a.root, "market_snapshots")
	if err != nil {
		return true, err
	}
	latest := latestSnapshots(snapshots)
	intents, err := store.ReadAll[live.Intent](a.root, "paper_strategy_intents")
	if err != nil {
		return true, err
	}
	latestIntents := live.LatestIntents(intents)
	var problems []error
	checked := false
	if a.lastPaperAt.IsZero() || now.Sub(a.lastPaperAt) >= time.Minute {
		checked = true
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
				if asset.ResearchOnly {
					a.recordResearchIssue(now, asset.Symbol, err)
					continue
				}
				problems = append(problems, fmt.Errorf("%s paper: %w", asset.Symbol, err))
				continue
			}
			if asset.ResearchOnly {
				a.clearResearchIssue(now, asset.Symbol)
			}
		}
		a.lastPaperAt = now
	}
	if a.liveExecutor != nil {
		checked = true
		if err := a.reconcileLive(ctx, now, latest, latestIntents); err != nil {
			problems = append(problems, fmt.Errorf("live reconciliation: %w", err))
		}
	}
	return checked, errors.Join(problems...)
}

func (a *app) recordResearchIssue(now time.Time, symbol string, err error) {
	key, message := runtimeErrorSummary(err)
	a.researchIssues[symbol] = key
	_ = a.events.Append("runtime_research_health", map[string]any{"timestamp": now, "symbol": symbol, "status": "DEGRADED", "error_key": key, "error": message, "strategy_version": strategyVersion, "runtime_version": execution.LifecycleVersion})
	fmt.Fprintf(os.Stderr, "%s research runtime: %s\n", symbol, message)
	if a.researchIncident {
		return
	}
	a.researchIncident = true
	a.notifier.BestEffort("Research Data Degraded", fmt.Sprintf("One or more paper markets are degraded.\nFirst detected: %s - %s\nFurther per-market alerts are suppressed. BTC/ETH live reconciliation continues.", symbol, message), "high", "warning")
}

func (a *app) clearResearchIssue(now time.Time, symbol string) {
	if _, exists := a.researchIssues[symbol]; !exists {
		return
	}
	delete(a.researchIssues, symbol)
	_ = a.events.Append("runtime_research_health", map[string]any{"timestamp": now, "symbol": symbol, "status": "RECOVERED", "strategy_version": strategyVersion, "runtime_version": execution.LifecycleVersion})
	if len(a.researchIssues) != 0 || !a.researchIncident {
		return
	}
	a.researchIncident = false
	a.notifier.BestEffort("Research Data Recovered", "All paper-market research checks are healthy again.", "default", "white_check_mark")
}

func (a *app) notifyDegraded(now time.Time, err error) {
	key, message := runtimeErrorSummary(err)
	shouldAlert := !a.degraded || now.Sub(a.lastAlertAt) >= time.Hour
	a.degraded = true
	a.healthySince = time.Time{}
	if !shouldAlert {
		return
	}
	a.lastAlertAt, a.lastAlertKey = now, key
	a.notifier.BestEffort("Overnight Runtime Degraded", message+"\nTrading state preserved; retrying automatically.", "urgent", "warning")
}

func (a *app) notifyRecovered(now time.Time) {
	if !a.degraded {
		return
	}
	if a.healthySince.IsZero() {
		a.healthySince = now
		return
	}
	if now.Sub(a.healthySince) < 5*time.Minute {
		return
	}
	a.degraded = false
	a.healthySince = time.Time{}
	a.notifier.BestEffort("Overnight Runtime Recovered", "Lighter connectivity and runtime reconciliation are healthy again.", "high", "white_check_mark")
}

func runtimeErrorSummary(err error) (string, string) {
	if err == nil {
		return "", ""
	}
	message := err.Error()
	lower := strings.ToLower(message)
	if strings.Contains(lower, "http 503") || strings.Contains(lower, "503 service temporarily unavailable") {
		return "lighter-http-503", "Lighter API temporarily unavailable (HTTP 503)."
	}
	if strings.Contains(lower, "http 502") {
		return "lighter-http-502", "Lighter API temporarily unavailable (HTTP 502)."
	}
	if strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded") {
		return "lighter-timeout", "Lighter API request timed out."
	}
	if strings.Contains(lower, "dial tcp") || strings.Contains(lower, "connection refused") {
		return "lighter-connection", "Lighter connection unavailable."
	}
	const limit = 180
	if len(message) > limit {
		message = message[:limit-3] + "..."
	}
	return message, message
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
		trade = execution.PaperTrade{SchemaVersion: 1, LifecycleVersion: execution.LifecycleVersion, SessionDate: snapshot.SessionDate, SessionID: ids.SessionID, OpportunityID: ids.OpportunityID, StrategyOrderID: ids.StrategyOrderID, TradeID: ids.TradeID, RunID: ids.RunID, Order: order, State: execution.Waiting, UpdatedAt: now}
	} else if trade.LifecycleVersion == "" {
		trade.LifecycleVersion = execution.LifecycleVersion
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
		a.freshReconciles = 0
		_ = a.events.Append("reconciliation_snapshots", map[string]any{"timestamp": now, "status": "FAILED", "endpoints": account.Endpoints, "error": err.Error(), "strategy_version": strategyVersion, "runtime_version": execution.LifecycleVersion})
		return err
	}
	a.freshReconciles++
	allowNewEntries := a.freshReconciles >= 2
	_ = a.events.Append("reconciliation_snapshots", map[string]any{"timestamp": now, "status": "FRESH", "endpoints": account.Endpoints, "positions": account.Positions, "active_orders": account.Orders, "historical_orders": account.HistoricalOrders, "strategy_version": strategyVersion, "runtime_version": execution.LifecycleVersion})
	if err := a.persistVenueFills(now, account.Fills); err != nil {
		return err
	}
	liveRiskUSD, basketRiskUSD, err := execution.DefaultRiskLimits().Budget(accountEquity(account), len(universe.Live()))
	if err != nil {
		return fmt.Errorf("live risk budget: %w", err)
	}
	states, err := store.ReadAll[liveRuntimeState](a.root, "live_runtime_states")
	if err != nil {
		return err
	}
	latest := latestLive(states)
	if err := a.persistLiveAccounting(now, account, latest); err != nil {
		return err
	}
	for _, asset := range universe.Live() {
		snapshot, ok := snapshots[asset.Symbol]
		if !ok || snapshot.Plan == nil || !snapshot.Plan.Valid {
			continue
		}
		_, ok = intentFor(intents, asset.Symbol, snapshot.OpportunityID)
		if !ok {
			continue
		}
		runID := "run_live_" + strategyVersion + "_" + snapshot.SessionDate.Format("20060102")
		ids := forensics.IDs(snapshot.SessionDate, asset.Symbol, strategyVersion, forensics.PlanOpportunityKey(*snapshot.Plan), "LIVE", runID)
		state, exists := latest[ids.TradeID]
		if exists && state.LifecycleVersion == "" {
			state.LifecycleVersion = execution.LifecycleVersion
		}
		if exists && strings.HasPrefix(state.ExecutionStatus, "SKIPPED_") {
			continue
		}
		if exists {
			if evidence, ok := a.liveExecutor.OrderEvidence(ids.StrategyOrderID + ":entry"); ok {
				evidenceChanged := state.ExchangeOrderIndex != evidence.ExchangeOrderIndex || state.Nonce != evidence.Nonce
				state.ClientOrderIndex, state.ExchangeOrderIndex, state.MarketID = evidence.ClientOrderIndex, evidence.ExchangeOrderIndex, evidence.MarketIndex
				state.Nonce, state.EncodedBaseAmount, state.EncodedPrice = evidence.Nonce, evidence.EncodedBaseAmount, evidence.EncodedPrice
				state.RequestedQuantity, state.RequestedPrice = evidence.RequestedQuantity, evidence.RequestedPrice
				if evidenceChanged {
					state.UpdatedAt = now
					if e := a.events.Append("live_runtime_states", state); e != nil {
						return e
					}
				}
			}
		}
		position, averageFill := positionFacts(account, asset.Symbol)
		mark := liveMark(a.liveMarkets, asset.Symbol)
		if !exists {
			if !allowNewEntries {
				continue
			}
			if !cycle1EntryAuthorized(now) {
				continue
			}
			if !execution.WithinOrderWindow(now, snapshot.SessionDate, a.location) {
				continue
			}
			market := a.markets[asset.MarketSymbol()]
			spec, e := execution.SpecFromMarket(market)
			if e != nil {
				return fmt.Errorf("%s market metadata: %w", asset.Symbol, e)
			}
			if !strings.EqualFold(spec.Status, "active") {
				a.persistLiveSkip(now, ids, asset.Symbol, execution.Order{}, liveRiskUSD, "SKIPPED_MARKET_INACTIVE", "market is not active")
				a.recordLiveAssetOutcome(now, ids, asset.Symbol, "SKIPPED_MARKET_INACTIVE", "market is not active", liveRiskUSD, 0, spec.MinimumNotional)
				continue
			}
			side := "BUY"
			if snapshot.Plan.Direction == "SHORT" {
				side = "SELL"
			}
			expiry := time.Date(snapshot.SessionDate.Year(), snapshot.SessionDate.Month(), snapshot.SessionDate.Day(), 16, 0, 0, 0, a.location)
			riskDistance := math.Abs(snapshot.Plan.Entry - snapshot.Plan.Stop)
			if riskDistance <= 0 {
				a.persistLiveSkip(now, ids, asset.Symbol, execution.Order{}, liveRiskUSD, "SKIPPED_RISK_LIMIT", "invalid live risk distance")
				a.recordLiveAssetOutcome(now, ids, asset.Symbol, "SKIPPED_RISK_LIMIT", "invalid live risk distance", liveRiskUSD, 0, spec.MinimumNotional)
				continue
			}
			liveQuantity := liveRiskUSD / riskDistance
			order := spec.Normalize(execution.Order{Symbol: asset.Symbol, Side: side, Price: snapshot.Plan.Entry, Quantity: liveQuantity, Stop: snapshot.Plan.Stop, TP1: snapshot.Plan.TP1, TP2: snapshot.Plan.TP2, ExpiresAt: expiry.Unix()})
			if e = spec.Validate(order); e != nil {
				status, skippable := classifyLiveAssetError(e)
				if !skippable {
					return fmt.Errorf("%s order validation: %w", asset.Symbol, e)
				}
				a.persistLiveSkip(now, ids, asset.Symbol, order, liveRiskUSD, status, e.Error())
				a.recordLiveAssetOutcome(now, ids, asset.Symbol, status, e.Error(), liveRiskUSD, order.Price*order.Quantity, spec.MinimumNotional)
				continue
			}
			managed, e := execution.NewManagedTrade(asset.Symbol, string(snapshot.Plan.Direction), order.Quantity, order.Price, order.Stop, order.TP1, order.TP2, expiry)
			if e != nil {
				return e
			}
			if e = managed.SetStrategyOrderID(ids.StrategyOrderID); e != nil {
				return e
			}
			if e = a.liveExecutor.ValidateProtection(ctx, asset.Symbol, managed.Quantity, managed.Stop, managed.TP1Quantity, managed.TP1, managed.RunnerQuantity, managed.TP2); e != nil {
				status, skippable := classifyLiveAssetError(e)
				if !skippable {
					return fmt.Errorf("%s protection preflight: %w", asset.Symbol, e)
				}
				a.persistLiveSkip(now, ids, asset.Symbol, order, liveRiskUSD, status, e.Error())
				a.recordLiveAssetOutcome(now, ids, asset.Symbol, status, e.Error(), liveRiskUSD, order.Price*order.Quantity, spec.MinimumNotional)
				continue
			}
			state = liveRuntimeState{SchemaVersion: 2, LifecycleVersion: execution.LifecycleVersion, SessionID: ids.SessionID, OpportunityID: ids.OpportunityID, StrategyOrderID: ids.StrategyOrderID, TradeID: ids.TradeID, Symbol: asset.Symbol, Order: order, Managed: managed, EntrySubmittedAt: now, UpdatedAt: now, InitialRiskUSD: liveRiskUSD}
			intentEvidence := map[string]any{"timestamp": now, "state": "PREPARED", "strategy_order_id": ids.StrategyOrderID, "trade_id": ids.TradeID, "symbol": asset.Symbol, "side": side, "client_order_index": managed.EntryOrderIndex, "price": order.Price, "quantity": order.Quantity, "expires_at": expiry, "strategy_version": strategyVersion, "runtime_version": execution.LifecycleVersion}
			if e = a.events.Append("order_intents", intentEvidence); e != nil {
				return e
			}
			// Persist the intent before contacting the venue. If submission is
			// ambiguous, this durable state prevents an automatic duplicate.
			if e = a.events.Append("live_runtime_states", state); e != nil {
				return e
			}
			response, e := a.liveExecutor.Submit(execution.OrderRequest{IntentKey: ids.StrategyOrderID + ":entry", Symbol: asset.Symbol, Side: side, Price: order.Price, Size: order.Quantity, StopPrice: order.Stop, ExpiresAt: expiry, OrderType: lightertx.LimitOrder, ClientOrderIndex: managed.EntryOrderIndex, RiskUSD: liveRiskUSD, RiskLimitUSD: liveRiskUSD})
			if e != nil {
				state.LastError = e.Error()
				state.UpdatedAt = time.Now().UTC()
				_ = a.events.Append("live_runtime_states", state)
				intentEvidence["timestamp"], intentEvidence["state"], intentEvidence["error"] = state.UpdatedAt, "UNRESOLVED", e.Error()
				_ = a.events.Append("order_intents", intentEvidence)
				status, skippable := classifyLiveAssetError(e)
				state.ExecutionStatus = status
				_ = a.events.Append("live_runtime_states", state)
				a.recordLiveAssetOutcome(now, ids, asset.Symbol, status, e.Error(), liveRiskUSD, order.Price*order.Quantity, spec.MinimumNotional)
				if skippable {
					continue
				}
				return e
			}
			if e = managed.SetEntryOrderID(response.OrderID); e != nil {
				return e
			}
			state.ClientOrderIndex, state.ExchangeOrderIndex, state.MarketID = response.ClientOrderIndex, response.ExchangeOrderIndex, response.MarketID
			state.Nonce, state.EncodedBaseAmount, state.EncodedPrice = response.Nonce, response.EncodedBaseAmount, response.EncodedPrice
			state.RequestedQuantity, state.RequestedPrice = response.RequestedQuantity, response.RequestedPrice
			state.ExecutionStatus = "LIVE_SUBMITTED"
			state.UpdatedAt = time.Now().UTC()
			if e = a.events.Append("live_runtime_states", state); e != nil {
				return e
			}
			intentEvidence["timestamp"], intentEvidence["state"], intentEvidence["venue_order_id"] = state.UpdatedAt, "ACKNOWLEDGED", response.OrderID
			intentEvidence["market_id"], intentEvidence["nonce"] = response.MarketID, response.Nonce
			intentEvidence["exchange_order_index"] = response.ExchangeOrderIndex
			intentEvidence["encoded_base_amount"], intentEvidence["encoded_price"] = response.EncodedBaseAmount, response.EncodedPrice
			if e = a.events.Append("order_intents", intentEvidence); e != nil {
				return e
			}
			if e = a.persistLiveJournal(snapshot, asset, state); e != nil {
				return e
			}
			a.recordLiveAssetOutcome(now, ids, asset.Symbol, "LIVE_SUBMITTED", "", liveRiskUSD, order.Price*order.Quantity, spec.MinimumNotional)
			fmt.Printf("%-5s LIVE ENTRY SUBMITTED qty=%.8f tx=%s\n", asset.Symbol, order.Quantity, response.OrderID)
			a.notifier.BestEffort("LIVE Order Submitted", fmt.Sprintf("%s %s\nEntry: %.8f\nQuantity: %.8f\nRisk: $%.4f\nBasket limit: $%.4f", asset.Symbol, side, order.Price, order.Quantity, liveRiskUSD, basketRiskUSD), "high", "rotating_light")
			continue
		}
		if state.Managed == nil || state.Managed.State == execution.ProtectionClosed {
			continue
		}
		if averageFill > 0 && state.Managed.State == execution.ProtectionWaiting {
			state.Managed.Fill = averageFill
		}
		plan := execution.PlanFromOrder(state.StrategyOrderID, state.Order)
		decision, decisionErr := execution.EvaluateOvernightLifecycle(plan, managedLifecycleState(state.Managed), execution.LifecycleInput{
			At: now, Mark: mark,
			EntryFilled:    position > 0 && state.Managed.State == execution.ProtectionWaiting,
			TP1Filled:      position > 0 && state.Managed.State == execution.ProtectionInitial && position <= state.Managed.RunnerQuantity+quantityTolerance(asset.Symbol),
			PositionClosed: position == 0 && state.Managed.State != execution.ProtectionWaiting,
			Expired:        !now.Before(state.Managed.Expiry),
		})
		changed := false
		if decisionErr != nil {
			err = decisionErr
		} else if containsLifecycleAction(decision.Actions, execution.ActionEntryFilled) {
			err = state.Managed.OnEntryFilled(a.liveExecutor)
			changed = err == nil
		} else if containsLifecycleAction(decision.Actions, execution.ActionTakeTP1) {
			err = state.Managed.OnTP1FilledAt(a.liveExecutor, now)
			changed = err == nil
		} else if containsLifecycleAction(decision.Actions, execution.ActionReconcileClosed) {
			err = state.Managed.OnClosed(a.liveExecutor)
			changed = err == nil
		} else if containsLifecycleAction(decision.Actions, execution.ActionCancelExpired) || containsLifecycleAction(decision.Actions, execution.ActionCloseExpired) {
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

func classifyLiveAssetError(err error) (string, bool) {
	if err == nil {
		return "LIVE_SUBMITTED", false
	}
	var executionErr *adapter.ExecutionError
	if errors.As(err, &executionErr) {
		if strings.Contains(strings.ToLower(executionErr.Operation), "reconcil") {
			return "FAILED_RECONCILIATION", false
		}
		if executionErr.Kind == adapter.ErrorAuth {
			return "FAILED_SIGNING", false
		}
		if executionErr.Kind != adapter.ErrorValidation {
			return "FAILED_SUBMISSION", false
		}
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "not active"):
		return "SKIPPED_MARKET_INACTIVE", true
	case strings.Contains(text, "notional") && strings.Contains(text, "minimum"),
		strings.Contains(text, "below minimum quote"):
		return "SKIPPED_MIN_NOTIONAL", true
	case strings.Contains(text, "below minimum"), strings.Contains(text, "not executable"):
		return "SKIPPED_EXCHANGE_MINIMUM", true
	case strings.Contains(text, "risk"), strings.Contains(text, "exposure"), strings.Contains(text, "collateral"):
		return "SKIPPED_RISK_LIMIT", true
	}
	if executionErr != nil && executionErr.Kind == adapter.ErrorValidation {
		return "SKIPPED_EXCHANGE_MINIMUM", true
	}
	return "FAILED_SUBMISSION", false
}

func (a *app) recordLiveAssetOutcome(now time.Time, ids forensics.Identity, symbol, status, reason string, riskUSD, requestedNotional, minimumNotional float64) {
	outcome := liveAssetOutcome{
		Timestamp: now, SessionID: ids.SessionID, OpportunityID: ids.OpportunityID,
		StrategyOrderID: ids.StrategyOrderID, TradeID: ids.TradeID, Symbol: symbol,
		Status: status, Reason: reason, RiskUSD: riskUSD, RequestedNotional: requestedNotional,
		MinimumNotional: minimumNotional, StrategyVersion: strategyVersion, RuntimeVersion: execution.LifecycleVersion,
	}
	if err := a.events.Append("live_execution_outcomes", outcome); err != nil {
		fmt.Fprintf(os.Stderr, "%s outcome persistence failed: %v\n", symbol, err)
	}
	if strings.HasPrefix(status, "SKIPPED_") {
		fmt.Printf("%-5s %s risk=$%.4f notional=$%.4f minimum=$%.4f reason=%s\n", symbol, status, riskUSD, requestedNotional, minimumNotional, reason)
	}
}

func (a *app) persistLiveSkip(now time.Time, ids forensics.Identity, symbol string, order execution.Order, riskUSD float64, status, reason string) {
	state := liveRuntimeState{
		SchemaVersion: 2, LifecycleVersion: execution.LifecycleVersion,
		SessionID: ids.SessionID, OpportunityID: ids.OpportunityID, StrategyOrderID: ids.StrategyOrderID,
		TradeID: ids.TradeID, Symbol: symbol, Order: order, EntrySubmittedAt: now, UpdatedAt: now,
		LastError: reason, InitialRiskUSD: riskUSD, ExecutionStatus: status,
	}
	if err := a.events.Append("live_runtime_states", state); err != nil {
		fmt.Fprintf(os.Stderr, "%s skip persistence failed: %v\n", symbol, err)
	}
}

func managedLifecycleState(trade *execution.ManagedTrade) execution.LifecycleState {
	state := execution.LifecycleState{FillPrice: trade.Fill, ActiveStop: trade.Stop}
	switch trade.State {
	case execution.ProtectionInitial:
		state.Phase = execution.LifecycleInitial
	case execution.ProtectionRunner:
		state.Phase, state.TP1Hit, state.ActiveStop = execution.LifecycleRunner, true, trade.Fill
	case execution.ProtectionClosing:
		state.Phase = execution.LifecycleInitial
	case execution.ProtectionClosed:
		state.Phase = execution.LifecycleClosed
	default:
		state.Phase = execution.LifecycleWaiting
	}
	return state
}

func containsLifecycleAction(actions []execution.LifecycleAction, wanted execution.LifecycleAction) bool {
	for _, action := range actions {
		if action == wanted {
			return true
		}
	}
	return false
}

func cycle1EntryAuthorized(now time.Time) bool {
	now = now.UTC()
	return !now.Before(cycle1StartUTC) && now.Before(cycle1EndUTC)
}

func accountEquity(snapshot lighterexec.Snapshot) float64 {
	for _, key := range []string{"total_asset_value", "collateral"} {
		value, err := strconv.ParseFloat(fmt.Sprint(snapshot.Account[key]), 64)
		if err == nil && value > 0 {
			return value
		}
	}
	return 0
}

func (a *app) halfHourlyNotice(now time.Time) {
	slot := now.Truncate(30 * time.Minute)
	if !a.notifier.Enabled() || now.Sub(slot) > 90*time.Second || a.lastHourlyAt.Equal(slot) {
		return
	}
	title, message := a.scannerMessage(context.Background(), now)
	if title == "" {
		return
	}
	a.notifier.BestEffort(title, message, "default", "bar_chart")
	a.lastHourlyAt = slot
}

func (a *app) telegramCommands(ctx context.Context) {
	a.notifier.PollTelegramCommands(ctx, func(request context.Context, command string) (string, string) {
		switch command {
		case "/scanner", "/status", "/board":
			return a.scannerMessage(request, time.Now().UTC())
		case "/help", "/start":
			return "Overnight Strategy Commands", "/scanner - current 12-market board\n/status - same live status view\n/help - command list\n\nAutomatic scanner: every :00 and :30 UTC"
		default:
			return "", ""
		}
	})
}

func (a *app) scannerMessage(ctx context.Context, now time.Time) (string, string) {
	states, err := store.ReadAll[execution.PaperTrade](a.root, "paper_runtime_states")
	if err != nil {
		return "", ""
	}
	latest := map[string]execution.PaperTrade{}
	day := now.In(a.location).Format("2006-01-02")
	for _, state := range states {
		if state.SessionDate.In(a.location).Format("2006-01-02") != day {
			continue
		}
		if old, ok := latest[state.Order.Symbol]; !ok || state.UpdatedAt.After(old.UpdatedAt) {
			latest[state.Order.Symbol] = state
		}
	}
	filled, waiting, closed, noFill, totalR := 0, 0, 0, 0, 0.0
	var rows []string
	for _, asset := range universe.All() {
		state, ok := latest[asset.MarketSymbol()]
		if !ok {
			state, ok = latest[asset.Symbol]
		}
		if !ok {
			rows = append(rows, fmt.Sprintf("%-5s WAIT PLAN", asset.Symbol))
			continue
		}
		side := "LONG"
		if state.Order.Side == "SELL" {
			side = "SHORT"
		}
		rows = append(rows, fmt.Sprintf("%-5s %-5s %-11s %+.2fR", asset.Symbol, side, telegramState(state), state.RMultiple))
	}
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
	liveLine := "LIVE FUNDED | account unavailable"
	if a.liveExecutor == nil {
		liveLine = "LIVE READ-ONLY"
	} else if a.account != nil {
		request, cancel := context.WithTimeout(ctx, 8*time.Second)
		snapshot, snapshotErr := a.account.ReadSnapshot(request, a.liveMarkets)
		cancel()
		if snapshotErr == nil {
			liveLine = fmt.Sprintf("LIVE FUNDED | Eq $%.2f | Pos %d | Orders %d", accountEquity(snapshot), openPositionCount(snapshot.Positions), len(snapshot.Orders))
		}
	}
	message := fmt.Sprintf("%s / %s CT\n%s\nDATA 12/12 markets\n\n%s\n\nTracked %d/12 | Open %d | Waiting %d\nClosed %d | No fill %d | Total %+.2fR\nNext plan: %s", now.Format("2006-01-02 15:04 UTC"), now.In(a.location).Format("15:04"), liveLine, strings.Join(rows, "\n"), len(latest), filled, waiting, closed, noFill, totalR, nextPlanUTC(now, a.location).Format("2006-01-02 15:04 UTC"))
	return "Overnight Scanner", message
}

func telegramState(state execution.PaperTrade) string {
	if state.Outcome != "" {
		return state.Outcome
	}
	return string(state.State)
}

func openPositionCount(positions []map[string]any) int {
	count := 0
	for _, position := range positions {
		for _, key := range []string{"position", "size", "position_size"} {
			value, err := strconv.ParseFloat(fmt.Sprint(position[key]), 64)
			if err == nil && value != 0 {
				count++
				break
			}
		}
	}
	return count
}

func nextPlanUTC(now time.Time, location *time.Location) time.Time {
	local := now.In(location)
	next := time.Date(local.Year(), local.Month(), local.Day(), 5, 0, 0, 0, location)
	if !local.Before(next) {
		next = next.AddDate(0, 0, 1)
	}
	return next.UTC()
}

func (a *app) persistLiveJournal(snapshot live.MarketSnapshot, asset universe.Asset, state liveRuntimeState) error {
	paperState, outcome := execution.Waiting, "WAITING_FOR_FILL"
	switch state.Managed.State {
	case execution.ProtectionInitial:
		paperState, outcome = execution.PaperFilled, "OPEN"
	case execution.ProtectionRunner:
		paperState, outcome = execution.PaperTP1, "TP1_OPEN"
	case execution.ProtectionClosing:
		paperState, outcome = execution.PaperFilled, "CLOSE_PENDING"
	case execution.ProtectionClosed:
		paperState, outcome = execution.PaperClosed, "CLOSED"
	}
	record := journal.FromLive(snapshot, asset.MarketSymbol(), strategyVersion, state.Order, journal.LiveExecution{OrderID: state.Managed.EntryOrderID, State: string(paperState), Outcome: outcome, ActualFill: state.Managed.Fill, TP1Hit: state.Managed.State == execution.ProtectionRunner || state.Managed.State == execution.ProtectionClosed, RuntimeVersion: state.LifecycleVersion})
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

func (a *app) persistVenueFills(now time.Time, rawFills []map[string]any) error {
	for _, raw := range rawFills {
		fill, err := ledger.ParseVenueFill(raw, a.accountIndex, now)
		if err != nil {
			return fmt.Errorf("normalize venue fill: %w", err)
		}
		if a.seenFillIDs[fill.FillID] {
			continue
		}
		if err := a.events.Append("venue_fills", fill); err != nil {
			return err
		}
		a.seenFillIDs[fill.FillID] = true
	}
	return nil
}

func orderIndex(raw map[string]any, keys ...string) int64 {
	for _, key := range keys {
		if value, ok := raw[key]; ok {
			parsed, _ := strconv.ParseInt(fmt.Sprint(value), 10, 64)
			if parsed != 0 {
				return parsed
			}
		}
	}
	return 0
}

func venueOrderPurposes(snapshot lighterexec.Snapshot, managed *execution.ManagedTrade) map[string]string {
	clientPurpose := map[int64]string{managed.EntryOrderIndex: "ENTRY", managed.StopOrderIndex: "EXIT", managed.BreakevenOrderIndex: "EXIT", managed.TP1OrderIndex: "EXIT", managed.TP2OrderIndex: "EXIT"}
	out := map[string]string{}
	for _, raw := range append(append([]map[string]any{}, snapshot.Orders...), snapshot.HistoricalOrders...) {
		client := orderIndex(raw, "client_order_index", "order_index")
		venue := orderIndex(raw, "order_id", "order_index")
		if purpose := clientPurpose[client]; purpose != "" && venue != 0 {
			out[strconv.FormatInt(venue, 10)] = purpose
		}
	}
	return out
}

func fundingTotal(rows []map[string]any, marketID int64, start time.Time) float64 {
	total := 0.0
	for _, row := range rows {
		if id := orderIndex(row, "market_id"); marketID > 0 && id > 0 && id != marketID {
			continue
		}
		stamp := orderIndex(row, "timestamp")
		if stamp > 0 {
			at := time.UnixMilli(stamp)
			if stamp < 1e12 {
				at = time.Unix(stamp, 0)
			}
			if at.Before(start) {
				continue
			}
		}
		for _, key := range []string{"amount", "funding", "funding_payment", "value"} {
			if raw, ok := row[key]; ok {
				value, _ := strconv.ParseFloat(fmt.Sprint(raw), 64)
				total += value
				break
			}
		}
	}
	return total
}

func (a *app) persistLiveAccounting(now time.Time, snapshot lighterexec.Snapshot, states map[string]liveRuntimeState) error {
	fills, err := store.ReadAll[ledger.VenueFill](a.root, "venue_fills")
	if err != nil {
		return err
	}
	for _, state := range states {
		if state.Managed == nil || state.InitialRiskUSD <= 0 {
			continue
		}
		purposes := venueOrderPurposes(snapshot, state.Managed)
		accounting, calcErr := ledger.ComputeAccounting(fills, purposes, state.InitialRiskUSD, fundingTotal(snapshot.FundingPayments, 0, state.EntrySubmittedAt))
		if calcErr != nil {
			continue
		}
		record := map[string]any{"schema_version": 1, "recorded_at": now, "trade_id": state.TradeID, "strategy_order_id": state.StrategyOrderID, "strategy_version": strategyVersion, "runtime_version": execution.LifecycleVersion, "gross_pnl": accounting.GrossPnL, "fees": accounting.Fees, "funding": accounting.Funding, "net_pnl": accounting.NetPnL, "r_multiple": accounting.RMultiple, "entry_quantity": accounting.EntryQuantity, "exit_quantity": accounting.ExitQuantity, "average_entry": accounting.AverageEntry, "average_exit": accounting.AverageExit, "complete": accounting.Complete, "source": "immutable_venue_fills"}
		if err := a.events.Append("live_trade_accounting", record); err != nil {
			return err
		}
	}
	return nil
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

func riskConfigFromEnv() (adapter.RiskConfig, error) {
	required := func(name string) (string, error) {
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			return "", fmt.Errorf("%s is required for funded execution", name)
		}
		return value, nil
	}
	maxOrder, err := required("LIGHTER_MAX_ORDER_NOTIONAL")
	if err != nil {
		return adapter.RiskConfig{}, err
	}
	maxPortfolio, err := required("LIGHTER_MAX_PORTFOLIO_EXPOSURE")
	if err != nil {
		return adapter.RiskConfig{}, err
	}
	btcExposure, err := required("LIGHTER_BTC_MAX_EXPOSURE")
	if err != nil {
		return adapter.RiskConfig{}, err
	}
	ethExposure, err := required("LIGHTER_ETH_MAX_EXPOSURE")
	if err != nil {
		return adapter.RiskConfig{}, err
	}
	minCollateral, err := required("LIGHTER_MIN_AVAILABLE_COLLATERAL")
	if err != nil {
		return adapter.RiskConfig{}, err
	}
	maxDailyLoss, err := required("LIGHTER_MAX_DAILY_LOSS")
	if err != nil {
		return adapter.RiskConfig{}, err
	}
	maxRiskFraction, err := required("LIGHTER_MAX_RISK_FRACTION")
	if err != nil {
		return adapter.RiskConfig{}, err
	}
	strategyFraction := execution.DefaultRiskLimits().PerTradePercent / 100
	configuredFraction, err := strconv.ParseFloat(maxRiskFraction, 64)
	if err != nil || configuredFraction <= 0 {
		return adapter.RiskConfig{}, fmt.Errorf("LIGHTER_MAX_RISK_FRACTION must be a positive decimal")
	}
	if configuredFraction+1e-12 < strategyFraction {
		return adapter.RiskConfig{}, fmt.Errorf(
			"LIGHTER_MAX_RISK_FRACTION %.6f conflicts with frozen strategy risk %.6f; configure it as an equal-or-higher global circuit breaker",
			configuredFraction, strategyFraction,
		)
	}
	return adapter.RiskConfig{
		AllowedSymbols: []string{"BTC", "ETH"}, MaxOrderNotional: maxOrder,
		MaxPortfolioExposure: maxPortfolio, MaxSymbolExposure: map[string]string{"BTC": btcExposure, "ETH": ethExposure},
		MinAvailableCollateral: minCollateral, MaxDailyLoss: maxDailyLoss, MaxRiskFraction: maxRiskFraction,
		MaxOpenPositions: execution.DefaultRiskLimits().MaxOpenPositions,
	}, nil
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
