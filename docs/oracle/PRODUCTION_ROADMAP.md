# Market Data Oracle production roadmap

## Authoritative ownership

The Market Data Oracle owns all public market data, historical market memory,
Volume Profile, Footprint, Order Flow, DOM, persistent heatmaps, sessions,
anchored analytics, named levels, structural zones, quality/provenance, replay,
and the market-data dashboards.

Session Map owns setups, strategies, trade plans, funded-account risk, private
credentials, orders, positions, and execution recovery. The Oracle contains no
private exchange credentials and has no order authority.

## EOD live-core activation

The first production activation is intentionally narrower than final product
completion. It establishes the continuous source on which every derived tool
depends.

- [x] Historical normalized trade and event queries implemented.
- [x] Oracle API included in the ARM64 release and rollback process.
- [x] Normalized live ingestion connected to the existing single collector.
- [x] New deterministic L2 engine runs beside the legacy engine.
- [x] Full-depth current books exposed for all configured assets.
- [x] Full-depth checksummed checkpoints persisted.
- [x] Displayed-liquidity changes persisted in compact one-second batches.
- [x] Bounded reconnect backfill and `WS /v1/live` implemented.
- [x] Current instruments and market snapshots exposed.
- [x] Twelve-market readiness and parity gates implemented.
- [x] Full Go tests, vet, ARM64 build, and public-feed smoke test passed.
- [ ] Deploy the exact release to TradePi.
- [ ] Verify 12/12 books and parity on TradePi.
- [ ] Verify WebSocket snapshot, backfill, synchronization, and live increments.
- [ ] Keep the shadow pipeline running and capture a full-day parity report.

The service is `LIVE_CORE` after the deployment checks pass. It is not labeled
`ORACLE_PRODUCT_COMPLETE` until every section below passes.

## Persistent analytical datasets

- [ ] Correlate displayed-book reductions with executions without presenting
  unmatched reductions as cancellations.
- [ ] Add resting-duration and replenishment state.
- [ ] Build indexed heatmap tiles for 1m, 5m, 15m, 30m, 1h, 4h, and custom
  windows.
- [ ] Build trade-derived candles and complete volume-at-price distributions.
- [ ] Build footprints at 1m, 5m, 15m, 30m, 1h, 4h, and custom intervals.
- [ ] Build buy/sell volume, delta, CVD, OI change, displacement, absorption
  candidates, and follow-through measurements.
- [ ] Prove live and replay calculations are identical.

## Historical and anchored access

- [ ] Inventory every archive, asset, stream, schema, collector version,
  checksum, quality class, and available interval from initial recording.
- [ ] Import usable history into indexed warm storage without altering archives.
- [ ] Expose certified, legacy, partial, missing, invalid-book, and trade-only
  intervals explicitly.
- [ ] Add point-in-time books and deterministic checkpoint seeking.
- [ ] Add `WS /v1/replay` with pause, speed, seek, and stable ordering.
- [ ] Add arbitrary anchored VWAP, Volume Profile, delta profile, footprint,
  order-flow, heatmap, liquidation, OI, level, and zone queries.
- [ ] Prove no-future-data leakage at every information cutoff.

## Canonical sessions, profiles, levels, and zones

- [ ] Port Chicago, New York, London, and Tokyo boundary rules.
- [ ] Validate normal days and DST transitions against Session Map fixtures.
- [ ] Implement YTD, rolling 30D/14D/7D, canonical and rolling 24H/12H, PM,
  US, Previous US, Asia, Previous Asia, London, Previous London, and custom
  periods.
- [ ] Implement complete profiles with POC, VAH, VAL, VWAP, HVNs, LVNs, OHLC,
  buy/sell volume, delta, shape, and distribution.
- [ ] Port named-level and zone construction as versioned deterministic models.
- [ ] Preserve zone provenance, strength, age, status, and every source level.
- [ ] Shadow and compare every Session Map field before cutover.

Initial compatibility versions:

```text
eventSchemaVersion       = oracle-event-v1
sessionDefinitionVersion = global-sessions-v1
profileModel              = trade-profile-v1
zoneModel                 = profile-cluster-v1
```

## UI and Session Map cutover

- [ ] Host DOM, VP, FP, Order Flow, heatmap, tape, liquidations, sessions,
  profiles, levels, zones, coverage, quality, anchored analysis, and replay UI
  on the designated hypervisor.
- [ ] Make the browser a renderer only.
- [ ] Add the versioned Oracle client to Session Map.
- [ ] Shadow Oracle and direct-Lighter public data without affecting orders.
- [ ] Require fresh, complete, version-compatible Oracle state for new entries.
- [ ] Preserve cancellation and exposure-reducing actions during Oracle outages.
- [ ] Remove direct public Lighter ingestion and local market calculations only
  after the agreed stability period.
- [ ] Never silently fall back to candle approximations or lower-quality zones.

## Final production gate

`ORACLE_PRODUCT_COMPLETE` requires continuous collection, complete historical
discovery, persistent heatmaps, VP/FP/Order Flow, canonical and anchored
sessions, levels and zones, deterministic replay, hosted Oracle-only market
dashboards, Session Map cutover, visible quality/staleness, monitoring, backup,
restore, restart, and rollback tests.
