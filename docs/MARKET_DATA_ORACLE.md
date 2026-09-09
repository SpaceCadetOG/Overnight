# Market Data Oracle

Market Data Oracle is the read-only interface to sealed, immutable recorder
packages. It runs independently of collection and execution and never exposes
exchange credentials or order submission.

## Secure access

The service listens only on TradePi loopback port `8083`. From another machine,
open an SSH tunnel through the existing `overnight-prod` host:

```sh
ssh -N -L 8083:127.0.0.1:8083 overnight-prod
```

The client repository can then use `http://127.0.0.1:8083`.

## Version 1 endpoints

```text
GET /healthz
GET /v1/version
GET /v1/datasets?from=YYYY-MM-DD&to=YYYY-MM-DD&asset=BTC
GET /v1/datasets/{package_id}/manifest
GET /v1/coverage?from=YYYY-MM-DD&to=YYYY-MM-DD&asset=BTC
GET /v1/quality/windows?package_id={package_id}
GET /v1/downloads/{package_id}/{manifest-listed-path}
```

Downloads support HTTP byte ranges and are restricted to files explicitly
listed in the package manifest, plus `MANIFEST.json` and `SHA256SUMS`.

Only closed days containing `MANIFEST.json` appear in the catalog. Current,
unsealed, incomplete, and arbitrary filesystem paths are not exposed.

## Consumer rules

Consumers must pin the package ID, collector commit, schema version, and file
checksum in every backtest result. They must inspect the certification class and
quality windows before using events. Uncertain intervals are excluded unless a
research run explicitly opts into them.
