package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	lightertx "github.com/elliottech/lighter-go/types/txtypes"
	adapter "github.com/ogtrading/lighter-adapter/lighter"
	"github.com/ogtrading/overnight-strategy/internal/execution"
	executionlighter "github.com/ogtrading/overnight-strategy/internal/execution/lighter"
	"github.com/ogtrading/overnight-strategy/internal/lighterexec"
)

func main() {
	execute := flag.Bool("execute", false, "perform the controlled BTC exchange transaction test")
	flag.Parse()
	if err := loadEnv(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
		fatal(err)
	}
	if !*execute {
		fatal(fmt.Errorf("-execute is required"))
	}
	if strings.TrimSpace(os.Getenv("LIGHTER_EXECUTOR_TOKEN")) == "" {
		fatal(fmt.Errorf("LIGHTER_EXECUTOR_TOKEN is required"))
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("KILL_SWITCH")), "true") {
		fatal(fmt.Errorf("KILL_SWITCH=true"))
	}
	cfg, err := configFromEnv()
	if err != nil {
		fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	markets, err := lighterexec.CheckPublic(ctx, cfg.BaseURL)
	if err != nil {
		fatal(err)
	}
	var btc lighterexec.Market
	for _, market := range markets {
		if market.Symbol == "BTC" {
			btc = market
			break
		}
	}
	if btc.Symbol == "" {
		fatal(fmt.Errorf("BTC market unavailable"))
	}
	mark, err := strconv.ParseFloat(fmt.Sprint(btc.MarkPrice), 64)
	if err != nil || mark <= 0 {
		fatal(fmt.Errorf("invalid BTC mark price"))
	}
	client, err := lighterexec.New(cfg)
	if err != nil {
		fatal(err)
	}
	if err := client.CheckCredentials(); err != nil {
		fatal(err)
	}
	snapshot, err := client.ReadSnapshot(ctx, []lighterexec.Market{btc})
	if err != nil {
		fatal(err)
	}
	if size := btcPositionSize(snapshot); size != 0 {
		fatal(fmt.Errorf("BTC must start flat; current size %.8f", size))
	}
	if len(snapshot.Orders) != 0 {
		fatal(fmt.Errorf("BTC must start with no active orders; found %d", len(snapshot.Orders)))
	}

	executor, err := executionlighter.NewExecutor(ctx, executionlighter.Config{
		BaseURL: cfg.BaseURL, WSURL: cfg.WSURL, PrivateKey: cfg.PrivateKey, AccountIndex: cfg.AccountIndex,
		APIKeyIndex: cfg.APIKeyIndex, ChainID: cfg.ChainID, StateRoot: filepath.Join(".state", "transaction-test"),
		Risk: adapter.RiskConfig{
			AllowedSymbols: []string{"BTC", "ETH"}, MaxOrderNotional: os.Getenv("LIGHTER_MAX_ORDER_NOTIONAL"),
			MaxPortfolioExposure: os.Getenv("LIGHTER_MAX_PORTFOLIO_EXPOSURE"), MaxSymbolExposure: map[string]string{"BTC": os.Getenv("LIGHTER_BTC_MAX_EXPOSURE"), "ETH": os.Getenv("LIGHTER_ETH_MAX_EXPOSURE")},
			MinAvailableCollateral: os.Getenv("LIGHTER_MIN_AVAILABLE_COLLATERAL"), MaxDailyLoss: os.Getenv("LIGHTER_MAX_DAILY_LOSS"), MaxRiskFraction: os.Getenv("LIGHTER_MAX_RISK_FRACTION"),
		},
	})
	if err != nil {
		fatal(err)
	}
	limitPrice := math.Floor(mark*.80*10) / 10
	quantity := math.Ceil((10.25/limitPrice)*1e5) / 1e5
	index, _ := execution.ClientOrderIndex("controlled-test-limit-" + time.Now().UTC().Format("200601021504"))
	limit, err := executor.SubmitControlledTest(execution.OrderRequest{IntentKey: fmt.Sprintf("controlled:%d:limit", index), Symbol: "BTC", Side: "BUY", Price: limitPrice, StopPrice: limitPrice * .99, Size: quantity, ExpiresAt: time.Now().Add(6 * time.Minute), OrderType: lightertx.LimitOrder, ClientOrderIndex: index, RiskUSD: .01, RiskLimitUSD: .50})
	if err != nil {
		fatal(fmt.Errorf("limit submit: %w", err))
	}
	fmt.Printf("limit submit: PASS tx=%s price=%.1f size=%.5f\n", limit.OrderID, limitPrice, quantity)
	if err := executor.Cancel(limit.OrderID); err != nil {
		fatal(fmt.Errorf("limit cancel: %w", err))
	}
	fmt.Println("limit cancel: PASS")

	quantity = math.Ceil((10.25/mark)*1e5) / 1e5
	index, _ = execution.ClientOrderIndex("controlled-test-open-" + time.Now().UTC().Format("200601021504"))
	opened, err := executor.SubmitControlledTest(execution.OrderRequest{IntentKey: fmt.Sprintf("controlled:%d:open", index), Symbol: "BTC", Side: "BUY", Price: mark * 1.01, StopPrice: mark * .99, Size: quantity, OrderType: lightertx.MarketOrder, ClientOrderIndex: index, RiskUSD: .01, RiskLimitUSD: .50})
	if err != nil {
		fatal(fmt.Errorf("market open: %w", err))
	}
	fmt.Printf("market open submitted: PASS tx=%s size=%.5f\n", opened.OrderID, quantity)
	positionSize, err := waitPosition(ctx, client, btc, true)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("market open reconciled: PASS size=%.8f\n", positionSize)
	if _, err := executor.Close("BTC", "LONG", positionSize, mark*.99); err != nil {
		fatal(fmt.Errorf("reduce-only close: %w", err))
	}
	if _, err := waitPosition(ctx, client, btc, false); err != nil {
		fatal(err)
	}
	fmt.Println("reduce-only close reconciled: PASS")
	fmt.Println("controlled exchange transaction test: PASS")
}

func waitPosition(ctx context.Context, client *lighterexec.Client, market lighterexec.Market, wantOpen bool) (float64, error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		snapshot, err := client.ReadSnapshot(ctx, []lighterexec.Market{market})
		if err == nil {
			size := btcPositionSize(snapshot)
			if wantOpen && size != 0 {
				return math.Abs(size), nil
			}
			if !wantOpen && size == 0 {
				return 0, nil
			}
		}
		select {
		case <-ctx.Done():
			return 0, fmt.Errorf("position reconciliation timeout: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func btcPositionSize(snapshot lighterexec.Snapshot) float64 {
	for _, position := range snapshot.Positions {
		if fmt.Sprint(position["symbol"]) != "BTC" {
			continue
		}
		value, _ := strconv.ParseFloat(fmt.Sprint(position["position"]), 64)
		return value
	}
	return 0
}

func configFromEnv() (lighterexec.Config, error) {
	account, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("LIGHTER_ACCOUNT_INDEX")), 10, 64)
	if err != nil {
		return lighterexec.Config{}, fmt.Errorf("invalid LIGHTER_ACCOUNT_INDEX")
	}
	key, err := strconv.ParseUint(strings.TrimSpace(os.Getenv("LIGHTER_API_KEY_INDEX")), 10, 8)
	if err != nil {
		return lighterexec.Config{}, fmt.Errorf("invalid LIGHTER_API_KEY_INDEX")
	}
	chain, err := strconv.ParseUint(strings.TrimSpace(os.Getenv("LIGHTER_CHAIN_ID")), 10, 32)
	if err != nil {
		return lighterexec.Config{}, fmt.Errorf("invalid LIGHTER_CHAIN_ID")
	}
	return lighterexec.Config{BaseURL: os.Getenv("LIGHTER_BASE_URL"), WSURL: os.Getenv("LIGHTER_WS_URL"), AccountIndex: account, APIKeyIndex: uint8(key), PrivateKey: os.Getenv("LIGHTER_API_PRIVATE_KEY"), ChainID: uint32(chain)}, nil
}

func loadEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.Trim(strings.TrimSpace(value), "'\"")
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
	return scanner.Err()
}

func fatal(err error) { fmt.Fprintln(os.Stderr, "FAIL:", err); os.Exit(1) }
