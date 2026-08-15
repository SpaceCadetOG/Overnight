# Overnight Lifecycle v2.0.1

Release status: pending commit and deployment

## Identity

- Strategy version: `baseline-v1-20260810` (unchanged)
- Runtime version: `overnight-lifecycle-v2.0.1`

## Runtime correctness changes

- Reject an expired open position when no positive mark is available. The
  runtime must leave it unresolved and surface an error rather than fabricate
  a zero or entry-price exit.
- Record `TP1ReconciledAt` and `BreakevenPromotedAt` on the durable managed
  trade state. Replayed TP1 observations remain idempotent.
- Classify authenticated remote-flat reconciliation as `RECONCILED_FLAT`
  rather than an ambiguous generic close.
- Validate trade-journal rows before reporting or ML export. Invalid terminal
  prices, missing identities/versions/timestamps, and incomplete filled states
  are preserved in quarantine and excluded from performance data.
- EOD output now records the exact runtime/code identity and journal schema.

## Non-goals

This release does not modify direction, entry, initial stop, targets, expiry
time, risk, funded assets, or ML authority.

## Remaining production evidence

- Persist immutable venue fills including authoritative fees and timestamps.
- Reconstruct live gross PnL, fees, funding, net PnL, and R from those fills.
- Require complete fresh position, open-order, historical-order, and fill
  snapshots before funded reconciliation is declared healthy.
