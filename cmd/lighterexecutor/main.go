package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ogtrading/overnight-strategy/internal/lighterexec"
	"github.com/ogtrading/overnight-strategy/internal/universe"
)

func main() {
	check := flag.Bool("check", false, "run authenticated read-only checks")
	checkPublic := flag.Bool("check-public", false, "run a credential-free public check")
	flag.Parse()
	if err := loadEnv(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
		fail("configuration", err)
	}
	if !*check && !*checkPublic {
		fail("usage", fmt.Errorf("use -check or -check-public"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	markets, err := lighterexec.CheckPublic(ctx, os.Getenv("LIGHTER_BASE_URL"))
	if err != nil {
		fail("public connectivity", err)
	}
	if *checkPublic {
		printJSON(map[string]any{"public_connectivity": "PASS", "markets": len(markets), "authenticated": false, "mode": "read_reconcile", "automated_orders": false})
		return
	}
	cfg, err := configFromEnv()
	if err != nil {
		fail("configuration", err)
	}
	client, err := lighterexec.New(cfg)
	if err != nil {
		fail("authenticated client", err)
	}
	if err := client.CheckCredentials(); err != nil {
		fail("authenticated account snapshot", err)
	}
	snapshot, err := client.ReadSnapshot(ctx, markets)
	if err != nil {
		fail("authenticated account snapshot", err)
	}
	wsCtx, wsCancel := context.WithTimeout(ctx, 15*time.Second)
	defer wsCancel()
	if err := client.CheckPrivateWebSocket(wsCtx); err != nil {
		fail("private WebSocket", err)
	}
	precision := []map[string]any{}
	for _, asset := range universe.Live() {
		for _, market := range markets {
			if market.Symbol == asset.Symbol {
				precision = append(precision, map[string]any{"symbol": market.Symbol, "market_id": market.MarketID, "status": market.Status, "minimum_base": market.MinBaseAmount, "minimum_notional": market.MinQuoteAmount, "price_decimals": market.PriceDecimals, "quantity_decimals": market.SizeDecimals})
			}
		}
	}
	accountSummary := map[string]any{}
	for _, key := range []string{"account_index", "account_type", "account_trading_mode", "status", "collateral", "available_balance", "total_asset_value", "pending_order_count"} {
		if value, ok := snapshot.Account[key]; ok {
			accountSummary[key] = value
		}
	}
	accountSummary["trading_credentials"] = "PASS"
	printJSON(map[string]any{
		"public_connectivity": "PASS", "authenticated_account_snapshot": "PASS",
		"balances_equity": "PASS", "positions": "PASS", "active_orders": "PASS",
		"recent_fills_trades": "PASS", "private_websocket": "PASS",
		"position_count": len(snapshot.Positions), "active_order_count": len(snapshot.Orders),
		"recent_fill_count": len(snapshot.Fills), "mode": "read_reconcile", "automated_orders": false,
		"account": accountSummary, "market_precision": precision,
	})
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
	chain := uint64(304)
	if value := strings.TrimSpace(os.Getenv("LIGHTER_CHAIN_ID")); value != "" {
		chain, err = strconv.ParseUint(value, 10, 32)
		if err != nil {
			return lighterexec.Config{}, fmt.Errorf("invalid LIGHTER_CHAIN_ID")
		}
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
		if key != "" {
			if _, exists := os.LookupEnv(key); !exists {
				_ = os.Setenv(key, value)
			}
		}
	}
	return scanner.Err()
}

func fail(check string, err error) {
	printJSON(map[string]any{"check": check, "status": "FAIL", "error": err.Error(), "mode": "read_reconcile", "automated_orders": false})
	os.Exit(1)
}

func printJSON(value any) { _ = json.NewEncoder(os.Stdout).Encode(value) }
