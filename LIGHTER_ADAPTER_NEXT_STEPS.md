# Overnight Strategy: next steps

## Project responsibility

`overnight-strategy` owns strategy research, signal generation, trade plans, and
lifecycle policy. It does not own Lighter signing, nonce handling, encoding,
risk enforcement, exchange reconciliation, or private WebSocket recovery.

The funded runtime must continue using the standard adapter through
`internal/execution/lighter`. BTC and ETH are the only funded symbols. Every
other market remains paper-only unless the shared adapter policy is deliberately
changed and revalidated.

## What each strategy must produce

Before contacting the adapter, create a complete trade plan containing:

- deterministic strategy/order ID;
- symbol and long/short direction;
- entry price and quantity;
- stop price;
- TP1 and TP2 prices and quantities;
- expiration time;
- calculated risk amount and applicable risk limit.

The deterministic ID must remain identical after restart. Derive entry, stop,
TP1, TP2, breakeven, and expiry-close intent IDs from it. Never derive an intent
ID from the current time during normal strategy execution.

## Required execution flow

1. Generate and persist the strategy plan.
2. Validate entry/stop/target geometry.
3. Validate every protective quantity against dynamic Lighter minimums.
4. Persist the runtime state before submitting the entry.
5. Submit through the standard risk-managed execution wrapper.
6. Reconcile fills and positions from the adapter.
7. Let the managed lifecycle submit stops, targets, breakeven, and expiry close.
8. On restart, recover adapter state before generating or submitting new work.
9. At the terminal state, verify the position is flat and no related orders remain.

## Do not implement locally

Do not add another Lighter signer, nonce counter, market-ID table, decimal table,
REST order-history client, private account WebSocket, retry loop, or in-memory
idempotency map. Improvements to those areas belong in the shared
`lighter-adapter` baseline and must then be synchronized into this repository.

## Acceptance gate for a new strategy

Before funded enablement, the strategy must pass:

- unit tests for long/short plan geometry;
- restart test using the same deterministic intent IDs;
- paper/replay validation;
- minimum-size validation for entry, stop, TP1, and TP2;
- risk rejection tests;
- expiry-close test;
- final flat/no-active-orders test;
- one explicitly authorized watched funded validation.

Do not change signing or exchange code to make a strategy test pass. Fix the
trade plan or improve the shared adapter.
