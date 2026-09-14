# Market Data Oracle v1 event contract

This directory defines the first venue-independent Oracle wire contract. The
same envelope is used for live delivery, normalized persistence, and replay.

## Sequence semantics

- `oracle_sequence` is positive and monotonic within one asset and stream.
- `venue_sequence` preserves a Lighter order-book nonce or trade identifier
  when one exists. It is not a substitute for the Oracle sequence.
- Clients detect loss with the Oracle sequence and resynchronize book streams
  from a new full snapshot.

## Timestamp semantics

- `exchange_timestamp` is time stated by the upstream venue.
- `transaction_timestamp` identifies an executed transaction when available.
- `oracle_received_at` is the UTC time the Oracle received the source message.
- Missing source timestamps remain absent. They are never replaced with receipt
  time or the Go zero year.

## Precision

Prices, sizes, and monetary values are decimal strings. Consumers must not
assume IEEE-754 floats preserve venue precision.

## Quality

Quality belongs to each stream. A certified trade stream does not certify the
book, ticker, or liquidation stream for the same interval.

## Product boundary

These schemas contain no account identifiers, private keys, signatures,
positions, orders, or execution controls. The Oracle is read-only.

## Historical tape and market-data queries

The read-only API exposes normalized individual events without requiring a
client to download and decompress an entire archive:

```text
GET /v1/trades?asset=BTC&from=<RFC3339>&to=<RFC3339>&limit=1000
GET /v1/events?asset=BTC&stream=trade,book_snapshot,book_delta,ticker&from=<RFC3339>&to=<RFC3339>&limit=1000
```

Responses contain the versioned Oracle envelopes, package IDs, quality states,
excluded-event count, and an opaque `next_cursor`. Pass that cursor unchanged
on the next request. Time ranges are half-open `[from,to)`, limited to 24 hours,
and response pages contain at most 5,000 events.

Quarantined packages are always rejected. Legacy packages require the explicit
`allow_uncertified=true` research opt-in and return
`CHECKSUM_VERIFIED_UNCERTIFIED`. Certified-window filtering uses Oracle receipt
time, while the requested tape interval uses exchange time when the venue
provided it.

Every compressed source file is checked against its sealed manifest before its
events are returned. The service caches a successful checksum only while file
size and modification time remain unchanged. Only one historical scan runs at
a time on TradePi; additional requests receive HTTP 429 with `Retry-After: 5`.

## Live book and event feed

The production collector remains the only Lighter public WebSocket consumer.
It runs the legacy and Oracle L2 engines in parallel, publishes only normalized
events accepted by the recorder, and records full-depth Oracle checkpoints.

```text
GET /v1/instruments
GET /v1/market/{asset}
GET /v1/books/{asset}
WS  /v1/live?assets=BTC,ETH&streams=trade,book_delta&since=<RFC3339>
```

The WebSocket sends bounded in-memory backfill, current full-depth snapshots, a
`synchronized` marker, and then live increments. `backfill_start.truncated`
states whether the requested timestamp predates the in-memory buffer. Clients
must use `/v1/events` for older history and deduplicate with `event_id`.

On a collector reconnect, active live subscribers are closed, every cached book
is invalidated, and new subscribers receive no book until fresh venue snapshots
have rebuilt it. This prevents deltas from being applied to stale state.

`GET /v1/readiness` succeeds only when both book engines have all 12 markets,
the collector is connected, the latest event is fresh, and the most recent
parity comparison has no error.

Displayed-book changes are persisted in one-second batches as
`oracle-liquidity-observation-v1`. An increase, decrease, or removal is an
objective displayed-book observation; it is not labeled as execution,
cancellation, spoofing, or intent until correlated with the trade tape by a
versioned derived model.

For hot-storage efficiency each `changes` row is a versioned tuple:

```text
[time_ms, side, price, previous_size, new_size, size_delta,
 action, venue_sequence, oracle_sequence]
```
