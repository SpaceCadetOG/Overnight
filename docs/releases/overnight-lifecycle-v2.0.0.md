# Overnight Lifecycle v2.0.0

Release date: 2026-08-14

## Identity

- Strategy version: `baseline-v1-20260810`
- Runtime version: `overnight-lifecycle-v2.0.0`
- Rollback strategy identity: `baseline-v1-20260810`
- Rollback release: capture and preserve the verified TradePi `current` target
  immediately before deployment

The strategy contract remains frozen. This release changes runtime lifecycle
correctness and parity only; it does not change session boundaries, direction,
entry, initial stop, targets, expiry, risk, asset authority, or ML policy.

## Changes

- Paper and live consume one immutable lifecycle plan.
- Paper candles and authenticated live events produce decisions through one
  deterministic lifecycle evaluator.
- TP1, breakeven, TP2, stop, close, and expiry transitions are shared.
- Paper and live retain separate execution adapters.
- Runtime versions are recorded alongside the frozen strategy version in trade
  journals, forensic events, daily reports, and startup deployment records.
- Research-only failures are isolated from BTC/ETH live reconciliation and
  recorded in `runtime_research_health`.
- Existing records remain append-only and are not rewritten.

## XAG correction

The batch paper simulator and incremental paper runtime previously disagreed
after TP1. Batch simulation could continue using the original stop while the
incremental runtime promoted the remaining half to breakeven.

Both paths now use:

```text
TP1 -> close 50% -> move remaining stop to breakeven -> TP2 or breakeven
```

Regression coverage includes XAG's tight three-decimal long and short geometry,
batch/incremental outcome parity, and paper/live lifecycle-action parity.

## Deployment gate

- Resolve and preserve the verified TradePi `current` release as the rollback
  target before activation.
- Build and checksum the complete Linux ARM64 release.
- Confirm the account is reachable and reconcile positions/orders before
  activation.
- Confirm collector connectivity, 12/12 books, and zero nonce gaps.
- Deploy atomically and verify the reported commit and runtime version.
- Do not mutate or backfill historical trade records.

## Post-deployment observation

For XAG:

- Confirm normalized entry/stop/TP geometry is valid.
- Confirm a research error creates a versioned research-health event without
  degrading BTC/ETH reconciliation.
- Compare batch replay and incremental lifecycle outcomes.
- Confirm TP1 promotes the runner to breakeven exactly once.

For BTC and ETH:

- Confirm the account, open orders, positions, and private WebSocket reconcile.
- Confirm no duplicate entry or protection order is submitted after restart.
- Confirm entry fill activates the initial stop and TP1 once.
- Confirm TP1 replaces initial protection with breakeven and TP2 once.
- Confirm expiry cancellation/flattening and Telegram reporting remain healthy.

## Rollback

Rollback to the pre-deployment TradePi release if lifecycle decisions, exchange
reconciliation, or protection orders diverge. Its strategy identity must remain
`baseline-v1-20260810`. Preserve all v2 observations and record the rollback
timestamp and reason; do not delete or rewrite them.
