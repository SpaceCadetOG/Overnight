# Current Local Operations Guide

Updated: 2026-08-08 CT

This is the current operator guide for the local-first Overnight Strategy
system. TradePi records and executes the 12 paper strategies. JumpPi validates
and retains daily packages. AWS is not required for recording or trading.

## Current safety state

```text
ENABLE_FUNDED_EXECUTION=false
KILL_SWITCH=false
```

All 12 markets actively paper trade from live Lighter data. No funded order can
be submitted while `ENABLE_FUNDED_EXECUTION=false`.

The 12 markets are BTC, ETH, SOL, HYPE, LIT, XAU, XAG, LINK, AAVE, UNI, ZEC,
and BNB. BTC and ETH can receive funded authority only after the complete paper
rehearsal passes and the operator explicitly approves activation.

## Machines and responsibilities

| Machine | Address | User | Responsibility |
|---|---|---|---|
| JumpPi (`jumppi`) | `192.168.3.42` | `trader2` | SSH jump, archive validation, lightweight research, later AWS transfer |
| TradePi (`tradepi`) | `192.168.3.28` | `traderbot` | Continuous recording, paper runtime, plans, EOD export, local archive |

TradePi does not depend on the Mac, JumpPi, AWS, or ntfy to continue recording
and managing paper lifecycles.

## Connect from the Mac

The Mac SSH shortcut uses JumpPi automatically:

```bash
ssh overnight-prod
```

You will be prompted first for the JumpPi password and then for the TradePi
password.

## Live dashboards

Compact view:

```bash
ssh -t overnight-prod '/opt/overnight-strategy/build/tradedashboard -store /mnt/trading/research -paper-equity=100 -view=compact -color=always -refresh=5s'
```

Full detailed view:

```bash
ssh -t overnight-prod '/opt/overnight-strategy/build/tradedashboard -store /mnt/trading/research -paper-equity=100 -view=detailed -color=always -refresh=5s'
```

Press `Ctrl-C` to close a dashboard. Closing the terminal or Mac does not stop
TradePi.

## Other live views

Runtime lifecycle messages:

```bash
ssh -t overnight-prod 'journalctl -fu traderuntime.service'
```

Recorder health:

```bash
ssh -t overnight-prod 'watch -n 2 "curl -s http://127.0.0.1:8082/healthz | jq"'
```

Raw durable paper states:

```bash
ssh -t overnight-prod 'tail -f /mnt/trading/research/paper_runtime_states.jsonl | jq -c'
```

Healthy recorder output has:

```text
connected=true
books_ready=12
nonce_gaps=0
```

Reconnects are acceptable only when the collector restores all 12 books and
nonce gaps remain zero.

## Daily schedule

All schedules use America/Chicago time.

| Time | Job |
|---|---|
| Continuous | Record 12 L2 books, trades, tickers and liquidations |
| 00:15 | Compress and manifest the closed TradePi recorder day |
| 00:45 | JumpPi pulls compressed files and validates the package |
| 05:00 | Generate 12 independent plans and run paper lifecycles |
| 16:00 | Expire remaining session opportunities |
| 16:05 | Produce the EOD forensic and shadow-ML export |

TradePi retains source JSONL. Local compression or JumpPi validation alone does
not authorize source deletion. Deletion eligibility requires a future verified
cloud acknowledgement.

## Services and timers

Check the production processes:

```bash
ssh overnight-prod 'systemctl is-active lightercollector.service traderuntime.service dailyplans.timer eodexport.timer lighterarchive.timer'
```

Every result must be `active`.

Check scheduled jobs:

```bash
ssh overnight-prod 'systemctl list-timers dailyplans.timer eodexport.timer lighterarchive.timer --all'
```

Check the current release and rollback release:

```bash
ssh overnight-prod 'readlink -f /opt/overnight-strategy/current; readlink -f /opt/overnight-strategy/previous'
```

Verify the current release checksums:

```bash
ssh overnight-prod 'cd /opt/overnight-strategy/current && sha256sum --check SHA256SUMS'
```

## Protected environments

TradePi uses three protected environment files:

```text
/etc/overnight.env             ntfy channel
/etc/overnight-strategy.env    funded gate and kill switch
/etc/overnight-lighter.env     Lighter account and API configuration
```

Edit Lighter configuration directly on TradePi so secrets do not enter chat or
shell history:

```bash
ssh overnight-prod
sudo nano /etc/overnight-lighter.env
```

The file needs these names:

```text
LIGHTER_BASE_URL=
LIGHTER_WS_URL=
LIGHTER_ACCOUNT_INDEX=
LIGHTER_API_KEY_INDEX=
LIGHTER_API_PRIVATE_KEY=
LIGHTER_CHAIN_ID=304
```

Do not add `LIGHTER_EXECUTOR_TOKEN` merely to display the read-only account.
Never paste the private key into chat, screenshots, logs, or command arguments.

After editing:

```bash
sudo chown root:traderbot /etc/overnight-lighter.env
sudo chmod 640 /etc/overnight-lighter.env
sudo systemctl restart traderuntime.service
exit
```

The dashboard should then replace `live account unavailable` with the read-only
account balance, available balance, positions, and active orders.

## JumpPi local package workflow

JumpPi pulls the previous closed day at 00:45 CT through its dedicated SSH key.
Packages move through:

```text
/srv/trade-forensics/packages/pending
/srv/trade-forensics/packages/validated
/srv/trade-forensics/packages/quarantine
```

Check the timer:

```bash
ssh overnight-jump 'systemctl is-active jumppi-package-pull.timer; systemctl list-timers jumppi-package-pull.timer --all'
```

Check classified packages:

```bash
ssh overnight-jump 'find /srv/trade-forensics/packages -maxdepth 2 -type f -name MANIFEST.json -print'
```

JumpPi is appropriate for package validation, reconstruction tests, feature
extraction, small backtests and lightweight CPU models. Large training and
hyperparameter searches belong in AWS.

## ntfy

ntfy is configured through `/etc/overnight.env`. Notification delivery is not
a trading dependency. If ntfy is unavailable, recording and paper management
continue.

Follow runtime notification errors with:

```bash
ssh overnight-prod 'journalctl -fu traderuntime.service'
```

## Restart recovery

Restart the recorder and runtime only when testing recovery or correcting a
fault:

```bash
ssh overnight-prod 'sudo systemctl restart lightercollector.service traderuntime.service'
```

Wait for snapshot reconstruction, then require 12 books and zero nonce gaps:

```bash
ssh overnight-prod 'sleep 20; curl -s http://127.0.0.1:8082/healthz | jq'
```

Paper states and deterministic opportunity identities survive runtime restarts.

## Full paper-rehearsal acceptance

Milestone 10 paper rehearsal passes only when the complete session reports:

```text
Recorder:          PASS
Books:             12/12
Nonce gaps:        0
Paper lifecycle:   PASS
Notifications:     PASS
EOD export:        PASS
ML export:         PASS
Funded orders:     0
```

Do not enable funded execution after a partial session. Review the full EOD
package, authenticated account state, unexpected orders and positions, restart
recovery, and kill-switch test first.

## Emergency controls

To block all new entries, edit `/etc/overnight-strategy.env`:

```text
KILL_SWITCH=true
ENABLE_FUNDED_EXECUTION=false
```

Then reload the runtime:

```bash
ssh overnight-prod 'sudo systemctl restart traderuntime.service'
```

Never delete recorder data to clear an error. Never classify an inferred
liquidation cascade as a confirmed liquidation. Never give the ten research
assets funded authority.
