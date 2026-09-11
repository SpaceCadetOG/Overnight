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
