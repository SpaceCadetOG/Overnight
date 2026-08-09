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

This downloads five-minute candles and funding for the registered markets.

## Run the live recorder

```bash
go run ./cmd/lightercollector -store data/live/lighter -health-port 8082
```

The recorder subscribes to market stats, ticker, trades, and order-book channels
for all 12 markets. It writes append-only JSONL streams and exposes
`/healthz`. Order-book continuity gaps are recorded separately.

Recording is continuous from 00:00 through 23:59:59 UTC every day. The
overnight-session and 05:00 CT plan boundaries do not start, stop, or filter the
recorder. Every L2 snapshot/delta, public trade-tape event, ticker update, and
market-statistics event is retained for the complete day.

The trade channel's explicit `liquidation_trades` records are written to each
asset's `confirmed_liquidations` stream. They are enriched with the current
reconstructed L2 book, multi-depth imbalance, microprice, and preceding tape
windows. A separate `inferred_liquidation_cascades` stream contains heuristic
order-flow detections with `confirmed=false`; the two labels never mix.

## Generate the 05:00 maps and plans

```bash
go run ./cmd/dailyplans -dry-run=true -store data/research
```

The command writes market snapshots and exactly two eligible funded strategy
intents. Research-only markets cannot produce funded intents. Re-running the command is
idempotent for same-date intents.

## Run the production paper gate

```bash
KILL_SWITCH=false go run ./cmd/dailyplans -dry-run=false -paper-equity=100 -store data/research
```

`-dry-run=false` selects `PAPER_EXECUTION`; it does not enable exchange orders.
The command builds and validates two precision-normalized Lighter orders,
enforces equity-based per-trade and basket risk, simulates fill/no-fill/stop/TP1/
TP2 against live five-minute candles, expires unfilled entries at 16:00 CT, and
appends results to `paper_trades`.

It also appends a consolidated factual record to `trade_journal.jsonl`. Each
record contains the complete 05:00 market map, planned order, exchange symbol,
fill/exit result, slippage, MFE/MAE, R multiple, execution mode, and frozen
strategy version. Subjective emotion, discipline, patience, and screenshot
fields are intentionally excluded.

Generate the daily 12-market control report after the 16:00 CT close:

```bash
go run ./cmd/dailyreport -date=2026-08-10
```

Use `-json` for the machine-readable version. The report deduplicates reruns by
trade ID, verifies coverage against all 12 registered markets, ranks individual
results, and reports fills, no-fills, open trades, win rate, total/average R,
MFE, MAE, and entry slippage.

Run the local control-room dashboard during the paper test:

```bash
go run ./cmd/tradedashboard -store data/test-run -paper-equity 100
```

The dashboard refreshes the authenticated Lighter account read-only and shows
balance, equity, available balance, margin usage, unrealized P&L, open positions,
orders, recorder health, and one paper-strategy row for each of the 12 markets.
It has no exchange-order submission path.

Cycle 1 begins at `2026-08-10T00:00:00Z`. Continuous collection begins at that
boundary. The first plan boundary is `2026-08-10T10:00:00Z` (05:00 CT). The
frozen identifier recorded on every trade is `baseline-v1-20260810`.

The launch candidate is frozen in `config/launch-cycle-1.json`. Its scheduler
remains disabled until the overnight 12-market local soak and morning coverage,
reconciliation, and flat-account gates pass.

Set `KILL_SWITCH=true` to prove that new paper or future live orders are blocked.

## Report stored coverage

```bash
go run ./cmd/researchreport -store data/research
```

## Safety invariants

- The initial production basket is BTC and ETH.
- The other 10 registered markets are paper/research assets and have no funded
  execution authority.
- Each live asset has an independent plan and risk allocation.
- Default per-trade risk is 0.5% of equity and maximum basket risk is 2.0%.
  With the initial two-asset basket, each remains capped at 0.5%.
- Same-day strategy intent IDs provide restart-safe duplicate prevention.
- Live mode is restricted to BTC/ETH. Normal operation is unattended; the kill
  switch and risk controls remain mandatory safety boundaries.
- Changing `dailyplans -dry-run` cannot enable funded execution. The Lighter
  transaction executor is a separate path guarded by live symbols, the kill
  switch, the 05:00-05:05 CT window, risk checks, and reconciliation.

## Frozen live order lifecycle

The funded executor supports BTC (market 1, price 1/size 5 decimals) and ETH
(market 0, price 2/size 4 decimals). No other market can enter the funded path.

1. Submit the limit entry only during 05:00-05:05 CT.
2. After the authenticated fill, submit a reduce-only stop for 100% and a
   reduce-only TP1 for 50%.
3. After TP1 is confirmed filled, cancel the original stop, submit a
   reduce-only breakeven stop at the actual fill for the remaining 50%, and
   submit reduce-only TP2 for the remaining 50%.
4. When the stop or TP2 closes the position, cancel every surviving sibling.
5. At 16:00 CT, cancel an unfilled entry or close any remaining position and
   reconcile the account as flat.

Lifecycle callbacks are idempotent. Protective reduce-only orders remain
available while the kill switch is active so an existing position can still be
made safe or closed.

`traderuntime` is the continuous lifecycle service. It always advances the
paper route for all 12 markets. Its funded route is fail-closed and requires
the `-live=true` process flag, `ENABLE_FUNDED_EXECUTION=true`,
`KILL_SWITCH=false`, valid Lighter credentials, the 05:00-05:05 CT entry
window, and BTC/ETH symbol authority. Leave `ENABLE_FUNDED_EXECUTION=false`
during the paper dress rehearsal. Changing that protected TradePi environment
value and restarting `traderuntime.service` is the explicit funded activation
action.

## Production connectivity gate

The executor must not start funded execution unless `lighterexecutor -check`
passes the authenticated private WebSocket check. If Lighter returns a
jurisdiction or authentication rejection, funded execution remains blocked and
must not be bypassed. Repeat the complete authenticated check immediately before
activation.

## Controlled exchange transaction test

Milestone 9 passed on 2026-08-08. The executor submitted and canceled a
non-marketable BTC limit, opened the smallest valid BTC position (0.00016),
confirmed it through reconciliation, and closed it reduce-only. The final
snapshot showed zero active orders and BTC flat. This command is destructive
and must not be included in a scheduler:

```bash
go run ./cmd/lightertransactiontest -execute
```
