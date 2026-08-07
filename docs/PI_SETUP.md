# Raspberry Pi Production Setup

## Hardware

- Raspberry Pi 4 (8GB)
- Debian 13 (Trixie)
- Go 1.25.4
- Docker 29
- systemd
- Samsung 1TB SSD (HLSSD)

Timezone:

America/Chicago

---

# Repository

Repository:

git@github.com:SpaceCadetOG/Overnight.git

Clone:

git clone git@github.com:SpaceCadetOG/Overnight.git /opt/overnight-strategy

---

# Directory Layout

/opt/overnight-strategy

build/
cmd/
internal/
scripts/
deploy.sh
go.mod

Persistent storage:

/mnt/trading

archive/
backtests/
candles/
executions/
fills/
health/
logs/
marketdata/
models/
orderbook/
recorder/
reports/
snapshots/
state/
trade_windows/

---

# SSD

Mount:

/mnt/trading

Filesystem:

ext4

Label:

HLSSD

Approximate usable capacity:

916 GB

---

# Services

lightercollector.service

Purpose:

Continuous Lighter websocket recorder.

Store:

/mnt/trading/recorder/lighter

---

dailylevels.timer

Runs every day:

05:05 America/Chicago

Generates:

Daily Basket Report

Stores:

/mnt/trading/reports/daily-levels/YYYY-MM-DD.txt

---

dailylevels.service

Runs:

build/dailylevels

Saves report

Calls notify.sh

---

# Notifications

Configuration:

/etc/overnight.env

Variables:

NTFY_URL

NTFY_TOPIC

Example:

NTFY_URL=https://ntfy.sh

NTFY_TOPIC=my-secret-topic

Test:

notify.sh "Pi Online" "Server running."

---

# Build

go mod download

go test ./...

go vet ./...

go build ./cmd/dailylevels

go build ./cmd/lightercollector

go build ./cmd/lighterexecutor

go build ./cmd/dailyplans

---

# Deployment

deploy.sh performs:

git fetch

git reset --hard origin/main

go mod download

go test

go vet

build binaries

restart services

---

# GitHub Workflow

Mac Development

↓

Push

↓

GitHub

↓

Pi

↓

deploy.sh

---

# Daily Runtime

19:00 CT

Collector running

Record websocket

Store recorder

05:00 CT

Session closes

05:05 CT

Generate basket

Save report

Notify phone

Future:

BTC execution

Execution recorder

Fill recorder

Phone alerts

