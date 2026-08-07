# Lighter Controlled-Execution Operations

Live order submission is disabled. The system supports read-only reconciliation,
intent validation, and live-market paper execution.

## Required environment

Public collectors use `LIGHTER_BASE_URL` and `LIGHTER_WS_URL`. Authenticated
reconciliation additionally uses `LIGHTER_ACCOUNT_INDEX`,
`LIGHTER_API_KEY_INDEX`, `LIGHTER_API_PRIVATE_KEY`, and `LIGHTER_CHAIN_ID`.
Keep these values in the ignored, owner-readable `.env` file.

## Verify connectivity

```bash
go run ./cmd/lighterexecutor -check
```

This validates public data, account state, positions, orders, fills, and the
private WebSocket. It cannot submit or cancel orders.

## Collect historical research data

```bash
go run ./cmd/lighterhistorical -days 30 -out data/raw/lighter
```

This downloads five-minute candles and funding for BTC, ETH, ZEC, BNB, SOL,
LINK, HYPE, XAU, and XAG.

## Run the live recorder

```bash
go run ./cmd/lightercollector -store data/live/lighter -health-port 8082
```

The recorder subscribes to market stats, ticker, trades, and order-book channels
for all nine markets. It writes append-only JSONL streams and exposes
`/healthz`. Order-book continuity gaps are recorded separately.

## Generate the 05:00 maps and plans

```bash
go run ./cmd/dailyplans -dry-run=true -store data/research
```

The command writes nine market snapshots and exactly five eligible strategy
intents. Research-only markets cannot produce intents. Re-running the command is
idempotent for same-date intents.

## Run the production paper gate

```bash
KILL_SWITCH=false go run ./cmd/dailyplans -dry-run=false -paper-equity=100 -store data/research
```

`-dry-run=false` selects `PAPER_EXECUTION`; it does not enable exchange orders.
The command builds and validates five precision-normalized Lighter orders,
enforces equity-based per-trade and basket risk, simulates fill/no-fill/stop/TP1/
TP2 against live five-minute candles, expires unfilled entries at 16:00 CT, and
appends results to `paper_trades`.

Set `KILL_SWITCH=true` to prove that new paper or future live orders are blocked.

## Report stored coverage

```bash
go run ./cmd/researchreport -store data/research
```

## Safety invariants

- The production basket is BTC, ETH, ZEC, BNB, and SOL.
- LINK, HYPE, XAU, and XAG have no execution authority.
- Each live asset has an independent plan and risk allocation.
- Default per-trade risk is 0.5% of equity and maximum basket risk is 2.0%.
  With five simultaneous plans, the basket cap reduces each to 0.4%.
- Same-day strategy intent IDs provide restart-safe duplicate prevention.
- Live mode is restricted to the staged BTC/ETH rollout and requires a fresh
  manual approval file, but no command currently exposes live submission.
- No executable command in this milestone submits, changes, or cancels orders.
- Enabling real submission requires a separate reviewed implementation and an
  explicit runtime gate; changing `-dry-run` cannot enable it.
