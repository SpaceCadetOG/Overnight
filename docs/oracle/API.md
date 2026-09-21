# Market Data Oracle API

The Oracle binds to `127.0.0.1:8083` on TradePi. Connect through the private
SSH or VPN path. It is read-only and receives no trading credentials.

All historical responses include an information cutoff, package provenance,
and quality state. Quarantined archives are rejected unless a research caller
explicitly opts into uncertified data. Session Map must not execute from
uncertified or stale responses.

Trade analytics also include `query_mode`. `INDEXED_HOURLY` means the request
used checksum-verified asset/hour shards. `RAW_SCAN` is the compatibility path
for a sealed package that has not been indexed yet.

## Endpoints

| Endpoint | Purpose |
|---|---|
| `GET /v1/trades` | Normalized trade and confirmed-liquidation tape |
| `GET /v1/events` | Normalized historical events |
| `GET /v1/candles` | Trade-derived OHLCV, delta, and CVD candles |
| `GET /v1/footprints` | Executed volume at price by time interval |
| `GET /v1/order-flow` | Buy/sell volume, delta, CVD, and rate |
| `GET /v1/profiles` | Anchored or named exact volume profiles |
| `GET /v1/session-definitions` | Versioned canonical session definitions |
| `GET /v1/sessions` | DST-safe session instances at a cutoff |
| `GET /v1/levels` | Named profile levels with stable IDs |
| `GET /v1/zones` | Versioned structural zones with provenance |
| `GET /v1/heatmap` | Persistent displayed-liquidity history |
| `GET /v1/liquidity` | Executed/cancel correlation, replenishment, absorption candidates |
| `GET /v1/open-interest` | Historical OI and mark observations |
| `GET /v1/books/{asset}` | Current authoritative full-depth book |
| `GET /v1/books/{asset}/at` | Reconstructed book at an exact timestamp |
| `WS /v1/live` | Normalized live stream and authoritative book states |

Developing-day data is available before archive sealing. Explicit `from` and
`to` ranges read the durable hot store when a sealed package does not yet
exist and report `query_mode: DEVELOPING_HOT_STORE` plus coverage. For the
lowest-latency rolling trade flow, use:

```text
GET /v1/order-flow?asset=BTC&window=5m
```

The rolling response includes buy volume, sell volume, delta/CVD,
`information_cutoff`, lag, quality, and whether the bounded live buffer fully
covers the requested window. Developing heatmap and liquidity responses expose
`source_coverage`; an empty developing range is never presented as proven zero
liquidity.
| `WS /v1/replay` | Deterministic historical replay with controls |
| `GET /dashboard` | Hosted read-only Oracle console |

Use `period=US`, `LONDON`, `ASIA`, `PREV_US`, `PREV_LONDON`, `PREV_ASIA`,
`PM`, `12H`, `24H`, `7D`, `14D`, `30D`, or `YTD` with an RFC3339 `at`
parameter for named profiles. Use `from` and `to` for an arbitrary anchored
profile. Long profiles are streamed and aggregated with bounded memory.

Candles, footprints, heatmaps, liquidity evidence, and OI calls are limited to
24-hour pages. This keeps Pi resource use bounded; clients join adjacent pages.

The labels `LIKELY_EXECUTED` and `LIKELY_CANCELLED` are correlation inferences.
They do not claim access to an exchange's private order lifecycle.

## Historical query index

The immutable daily packages remain the source of truth. The derived query
store lives at `/mnt/trading/oracle/index` and contains hourly Zstandard shards
for trades, liquidity observations, and market statistics. Every shard has its
own SHA-256 checksum and the index manifest pins the source checksums.

The daily archive lifecycle builds the index automatically before cloud upload.
To index an existing sealed package manually:

```text
/opt/overnight-strategy/current/bin/oracleindex \
  -root /mnt/trading/recorder/lighter \
  -index-root /mnt/trading/oracle/index \
  -package lighter-2026-09-10
```

Index publication is atomic and idempotent. An incomplete build is never made
visible to API readers. A missing index uses the raw compatibility path; an
invalid or corrupted published index fails closed.

## Acceptance

Run:

```text
scripts/test-oracle-live.sh http://127.0.0.1:8083 BTC
scripts/test-oracle-analytics.sh \
  http://127.0.0.1:8083 BTC \
  2026-09-10T14:00:00Z 2026-09-10T14:05:00Z
```

The first command validates all 12 live books and parity. The second prints
bounded response samples for every historical analytics family and fails on
any non-2xx response.
