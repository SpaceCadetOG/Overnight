# Cycle 1 production/ML release contract

Release tag: `cycle1-baseline-v1-20260810`

Strategy version: `baseline-v1-20260810`

Cycle boundary: `2026-08-10T00:00:00Z`

## Session schedule

- Continuous recorder: 00:00–23:59:59 UTC.
- Overnight auction: 19:00–05:00 America/Chicago.
- Plan freeze: 05:00 CT (10:00 UTC during Cycle 1).
- Funded entry window: 05:00–05:05 CT.
- Entry/position expiry: 16:00 CT.
- EOD forensic and shadow-ML export: 16:05 CT.

## Authority

- BTC and ETH: live/control eligible and simultaneous paper equivalents.
- SOL, HYPE, LIT, XAU, XAG, LINK, AAVE, UNI, ZEC, BNB: paper/research only.
- ML outputs are isolated in `ml_shadow_predictions`, always carry
  `shadow_only=true`, and have no execution/order-routing interface.
- JumpPi and GCP validate, transform, and upload data but are never execution
  dependencies.

## Versioned contracts

- Market map schema: 1.
- Trade journal schema: 1.
- Forensic event envelope: 1.
- Checkpoint manifest: 1.
- ML prediction: 1.
- Replay assumptions: `lighter-replay-v1-20260810`.

Paper and live records pair only through `opportunity_id`. They retain distinct
`trade_id` and exchange-order identities; approximate timestamp pairing is
forbidden.

## Reference package

The local first-reference build is:

`data/packages/pending/package=dataset_20260808_baseline-v1-20260810_v1`

It contains 12 paper opportunities, versioned maps/journals, 74 lifecycle
events, checkpoint manifests, L2 snapshot/deltas, tape, ticker, confirmed and
inferred liquidation samples, SHA-256 hashes, and a passing data-quality file.
Live pairing is contract-tested; the reference date has no baseline live trade,
so it does not fabricate one. The first genuine BTC/ETH live record will share
the control opportunity ID and remain a distinct trade.

## Activation rule

The Git release does not itself enable funded execution. Activation still
requires the full overnight soak, final authenticated reconciliation, a flat
account, no unmanaged orders, healthy 12/12 book coverage, and explicit funded
scheduler enablement.
