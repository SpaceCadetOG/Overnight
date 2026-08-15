# TradePi Runtime Operations

TradePi is the authoritative edge recorder and execution host. JumpPi is an
SSH bastion, archive verifier, and transfer relay. Neither JumpPi, the Mac,
AWS, nor notification delivery is required for local recording, risk
management, or position management.

## Runtime layout

```text
/opt/overnight-strategy/
  releases/<commit>-<build-id>/
    bin/
    systemd/
    scripts/
    journald/
    BUILD.json
    SHA256SUMS
  current -> releases/<active>
  previous -> releases/<prior>
  deploy-lock

/var/lib/overnight/
  health/
  reports/

/mnt/trading/
  recorder/lighter/
  research/
  packages/pending/
  packages/verified/
  packages/quarantine/
  logs/
```

Releases are immutable. Services execute binaries only through
`/opt/overnight-strategy/current/bin`. Mutable state never resides inside a
release directory.

## Supervision

- `overnight-recorder.target` owns the collector and local health guards.
- `overnight-trading.target` owns the continuously supervised runtime.
- `overnight-operations.target` owns planning, reporting, export, and archive
  timers.
- The runtime wants the collector but does not require it. A collector restart
  must not terminate position management.
- Notification configuration is optional and cannot make plan generation,
  recording, or trading fail.

## Deployment gate

CI creates a clean `linux/arm64` artifact with an immutable `BUILD.json` and
`SHA256SUMS`. JumpPi stages a new release, verifies its commit, target, and
checksums, then invokes the root-owned activation command. Activation uses a
deployment lock, atomically updates `current` and `previous`, installs unit
files, and restarts supervised services.

The deployment is accepted only when:

```text
lightercollector.service active
traderuntime.service active
connected=true
books_ready=12
nonce_gaps=0
crossed_books=0
invalid_levels=0
```

Failure switches `current` back to `previous` and restarts the prior release.

### One-time activator v2 bootstrap

The immutable `bin/` layout requires activator version 2. Before the first v2
deployment, an operator must install the reviewed script on TradePi:

```text
sudo install -o root -g root -m 0755 \
  /path/to/reviewed/activate-release.sh \
  /opt/overnight-strategy/scripts/activate-release.sh
```

The CI deployer verifies that the fixed root-owned command reports version 2
and refuses deployment otherwise. The deploy identity should receive sudo
permission only for this fixed command, not arbitrary root access.

## Health and storage

`overnight-health.timer` samples the loopback recorder endpoint every minute
and appends evidence under `/var/lib/overnight/health`. It reports failure when
the stream is stale, any of the twelve books is unready, or structural book
validation fails. It does not automatically restart or delete state.

`overnight-disk-guard.timer` checks `/mnt/trading` every five minutes:

- warning at 70 percent;
- critical at 80 percent;
- emergency at 90 percent.

No unverified recorder day is eligible for deletion. Cloud acknowledgement is
still required before future retention automation may remove local raw data.

## Reporting and retention

The ARM64 release includes `dailyreport`. At 16:10 Chicago time, the report
wrapper atomically writes JSON and text reports under
`/var/lib/overnight/reports/day=YYYY-MM-DD`.

Journald is capped at 2 GB, retains up to 14 days, preserves 20 GB of free disk,
and compresses old entries. Execution and forensic ledgers remain separate
from diagnostic log retention.

## Evidence and certification

Venue fills are appended to the immutable `venue_fills` ledger. Live PnL and R
are derived from those fills, fees, and funding rather than dashboard marks.
Positions, active orders, historical orders, fills, and funding are recorded
with independent freshness states; a failed or stale source blocks new entry
authority and is never treated as an empty account.

Each closed recorder day must contain `RECORDER_CERTIFICATE.json`. The archive
and package validators require twelve independently replayed books, complete
daily coverage, zero nonce gaps, zero crossed books, valid quantities, matching
live reconstruction checkpoints, valid checksums, parseable JSON, and an exact
collector commit. A package that fails any gate remains incomplete.
