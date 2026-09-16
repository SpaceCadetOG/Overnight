# Oracle Acceptance Tests

Run the REST acceptance suite against a loopback Oracle or an SSH tunnel:

```text
scripts/test-oracle-live.sh http://127.0.0.1:8083 BTC
```

Run the live WebSocket synchronization and authoritative DOM-state probe:

```text
ORACLE_LIVE_SMOKE_URL='ws://127.0.0.1:8083/v1/live?assets=BTC&streams=book_delta&depth=25' \
ORACLE_LIVE_REQUIRE_BOOK_STATE=1 \
go test ./internal/oracleapi -run TestExternalLiveOracle -count=1 -v
```

Run the historical API, normalization, book-continuity, checkpoint, and
WebSocket contract suites:

```text
go test ./internal/oracleapi ./internal/oracle/live \
  ./internal/oracle/book ./internal/oracle/normalize/lighter -count=1 -v
```

Index acceptance additionally verifies atomic publication, source checksum
binding, hourly selection, trade/profile parity, heatmap/OI reads without the
raw source file, and fail-closed behavior for corrupted partitions.

Production acceptance additionally requires the deployed commit to match the
intended release and the collector health response to report zero nonce gaps,
crossed books, invalid levels, parity failures, and subscriber drops.
