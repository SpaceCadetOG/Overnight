# Overnight Strategy Troubleshooting

## Git Push Failed (HTTP 408)

Cause:

Repository contained very large historical files.

Fix:

Clean repository.

Commit again.

Push again.

Status:

Resolved.

---

## Wrong Git Root

Problem:

Git root was the home directory.

Fix:

Create dedicated repository.

Status:

Resolved.

---

## Universe Test Failure

Expected:

Observed assets = 4

Actual:

Observed assets = 5

Fix:

Update assets_test.go.

Status:

PASS.

---

## Environment Variables Missing

Problem:

LIGHTER_* variables not present.

Fix:

Run lighterexecutor -check.

Verify authenticated account.

---

## SSD Mounted Multiple Times

Problem:

Mounted as both

/mnt/hlssd

and

/mnt/trading

Fix:

Canonical mount:

/mnt/trading

Update fstab.

---

## Permission Denied

Problem:

Files owned by root.

Fix:

chown to runtime user.

---

## deploy.sh Missing

Cause:

Deployment directory incomplete.

Fix:

Clone repository correctly.

---

## HTTPS Clone Failed

Problem:

Authentication failed.

Fix:

Use SSH.

---

## SSH Clone Failed

Problem:

Wrong runtime user.

Fix:

Clone with configured GitHub SSH identity.

---

## Go Version

Repository:

Go 1.25.4

Pi:

Automatically downloaded correct toolchain.

Status:

PASS.

---

## Recorder Location

Old:

data/live/lighter

New:

/mnt/trading/recorder/lighter

Configured via systemd.

---

## Daily Report Permission

tee Permission denied.

Fix:

Correct ownership of

/mnt/trading/reports

---

## Notifications

Problem:

NTFY_TOPIC missing.

Fix:

Configure

/etc/overnight.env

Status:

PASS.

---

## Daily Levels Before 05:05

Expected output:

No report generated.

Reason:

Session incomplete.

Correct behavior.

---

## Collector Health

Check:

systemctl status lightercollector

Expected:

active (running)

---

## Recorder

Directory:

/mnt/trading/recorder/lighter

Should continuously grow.

---

## Daily Reports

Directory:

/mnt/trading/reports/daily-levels

One report per session.

---

## Deployment

Run:

deploy.sh

Expected:

Fetch

Reset

Test

Vet

Build

Restart services

---

## Full Health Check

Collector

systemctl status lightercollector

Timer

systemctl list-timers

Recorder

ls /mnt/trading/recorder

Reports

ls /mnt/trading/reports/daily-levels

Notifications

notify.sh

Deployment

deploy.sh

Git

git status

