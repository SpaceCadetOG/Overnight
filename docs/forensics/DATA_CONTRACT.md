# Trade Forensics data contract

Contract version: `1`

## Time and asset conventions

- All event timestamps are RFC 3339 UTC with available sub-second precision.
- Storage partitions use the `America/Chicago` calendar date: `date=YYYY-MM-DD`.
- `occurred_at` is source/event time; `recorded_at` or `received_at` is TradePi arrival time.
- Research symbols are stable uppercase identities (`BTC`, `ETH`, `SOL`, `HYPE`, `LIT`, `XAU`, `XAG`, `LINK`, `AAVE`, `UNI`, `ZEC`, `BNB`).
- `exchange_symbol` contains any venue-native alias. ML datasets must join on the stable research symbol.
- Prices and quantities are JSON strings in raw Lighter messages and must be parsed as exact decimals. Do not use binary floating point for normalized financial facts.

## Immutable sources

Raw recorder messages and lifecycle envelopes are append-only. Corrections are additional events. A derived table, case, label, score, or feature must record its input package IDs and implementation version.

Confirmed liquidation records and inferred cascade records are intentionally separate:

- `confirmed_liquidations.jsonl.zst`: events explicitly identified by Lighter.
- `inferred_liquidation_cascades.jsonl.zst`: deterministic research inference, never represented as confirmed venue truth.

## L2 reconstruction

1. Accept `subscribed/order_book` as a complete snapshot and replace prior book state.
2. Apply subsequent updates only when `begin_nonce` equals the previous `nonce`.
3. Replace a price level with its supplied size.
4. Delete a price level when size is zero.
5. On a mismatch, invalidate the book, record a collector gap, disconnect, and require a new snapshot.
6. Never forward-fill across an invalid interval.

## Package acceptance

A package is eligible for research only when all conditions pass:

- Manifest schema is supported and package identity is present.
- Collector version and Git commit are present.
- Every listed compressed file exists and its SHA-256 matches.
- Decompressed JSONL row counts match the manifest.
- All expected assets have order-book coverage.
- Coverage begins within five minutes of 00:00 and ends within five minutes of 24:00 Chicago time.
- No nonce gaps or WebSocket errors are reported.
- Cloud validation independently returns `VALID`.

Reconnects are reported but are not automatically fatal if snapshots re-establish valid books and coverage/sequence tests pass. Packages failing any hard rule go to quarantine and never enter normalized or ML datasets.

## Known limitations

- Public L2 does not reveal exact queue position, so touch does not guarantee a simulated fill.
- Network arrival timestamps include Internet and host scheduling latency.
- Inferred cascades are hypotheses and must not be used as confirmed liquidation labels.
- The first partial collection day is not a full-day sample.
- Normalized feature rows must include `available_at` to prevent look-ahead leakage.
