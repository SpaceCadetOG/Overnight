# Overnight Strategy — Run While Away Guide

This is the operating checklist for TradePi. Follow it in order. Do not enable
funded execution until the paper soak, notifications, recorder, authenticated
account check, and flat-account checks all pass.

## What runs automatically

TradePi runs these independent services:

| Component | Purpose | Schedule |
|---|---|---|
| `lightercollector` | L2 books, trades, ticker, statistics and liquidations for 12 markets | Continuous |
| `traderuntime` | All 12 paper lifecycles and gated BTC/ETH live lifecycles | Continuous |
| `dailyplans` | Independent 05:00 CT market map and plan for each asset | 05:00 CT |
| `eodexport` | Trade forensics and shadow-only ML package | 16:05 CT |
| `lighterarchive` | Archive/upload the closed recorder day | 00:15 CT |

The 12 markets are BTC, ETH, SOL, HYPE, LIT, XAU, XAG, LINK, AAVE, UNI, ZEC,
and BNB. All 12 run in paper. Only BTC and ETH have funded authority.

The live baseline is fixed:

1. Submit the entry between 05:00 and 05:05 CT.
2. After a reconciled fill, protect 100% with a stop and place TP1 for 50%.
3. After TP1, move the remaining 50% stop to the actual-fill breakeven and
   place TP2.
4. Cancel surviving sibling orders after the position closes.
5. At 16:00 CT, cancel an unfilled entry or flatten any remaining position.

## 1. Connect to TradePi

From the Mac:

```bash
ssh traderbot@192.168.3.28
```

All commands in the remaining sections run on TradePi unless stated otherwise.

## 2. Confirm the protected environment

The services read `/etc/overnight.env` first and
`/etc/overnight-strategy.env` second. The second file takes precedence.

Never print or paste the private key. Check only whether required names exist:

```bash
sudo sh -c '
file=/etc/overnight-strategy.env
[ -f "$file" ] || file=/etc/overnight.env
for key in LIGHTER_BASE_URL LIGHTER_WS_URL LIGHTER_ACCOUNT_INDEX LIGHTER_API_KEY_INDEX LIGHTER_API_PRIVATE_KEY LIGHTER_CHAIN_ID LIGHTER_EXECUTOR_TOKEN NTFY_URL NTFY_TOPIC KILL_SWITCH ENABLE_FUNDED_EXECUTION; do
  grep -q "^${key}=" "$file" && echo "$key: SET" || echo "$key: MISSING"
done
'
```

Every name must show `SET`. Values should include:

```text
KILL_SWITCH=false
ENABLE_FUNDED_EXECUTION=false
```

Use `false` for funded execution throughout the dress rehearsal.

## 3. Test the existing ntfy phone channel

This sends a harmless test using the protected values without displaying them:

```bash
sudo sh -c '
file=/etc/overnight-strategy.env
[ -f "$file" ] || file=/etc/overnight.env
set -a
. "$file"
set +a
curl --fail --silent --show-error \
  -H "Title: Overnight Strategy Test" \
  -H "Priority: high" \
  -H "Tags: white_check_mark" \
  --data "TradePi notifications are connected." \
  "${NTFY_URL%/}/${NTFY_TOPIC}"
'
```

Do not continue until the phone receives the message.

## 4. Confirm the installed release

```bash
readlink -f /opt/overnight-strategy/current
sha256sum --check /opt/overnight-strategy/current/SHA256SUMS
```

Expected: every checksum reports `OK`.

## 5. Start the safe paper dress rehearsal

Make certain funded execution remains off, then restart the services:

```bash
sudo systemctl daemon-reload
sudo systemctl enable lightercollector.service traderuntime.service
sudo systemctl enable dailyplans.timer eodexport.timer lighterarchive.timer
sudo systemctl restart lightercollector.service traderuntime.service
sudo systemctl restart dailyplans.timer eodexport.timer lighterarchive.timer
```

The phone should receive recorder/runtime startup notifications.

## 6. Verify everything is running

```bash
systemctl is-active lightercollector.service
systemctl is-active traderuntime.service
systemctl is-active dailyplans.timer
systemctl is-active eodexport.timer
systemctl is-active lighterarchive.timer
```

Every line must say `active`.

Check the next scheduled jobs:

```bash
systemctl list-timers dailyplans.timer eodexport.timer lighterarchive.timer
```

Expected local schedules:

```text
05:00 CT  daily plans
16:05 CT  EOD export
00:15 CT  recorder archive
```

## 7. Verify recorder health

```bash
curl --fail --silent http://127.0.0.1:8082/healthz
```

Required launch conditions:

```text
connected: true
books_ready: 12
markets: 12
nonce_gaps: 0
```

Reconnects are not automatically a failure. A reconnect is acceptable only
when the collector restores snapshots and nonce gaps remain zero.

## 8. Verify the Lighter account

```bash
cd /opt/overnight-strategy
/opt/overnight-strategy/build/lighterexecutor -check
```

Required:

```text
public_connectivity: PASS
authenticated_account_snapshot: PASS
balances_equity: PASS
positions: PASS
active_orders: PASS
recent_fills_trades: PASS
private_websocket: PASS
BTC and ETH payload validation: PASS
```

Before activation, the account must also show zero unexpected positions and
zero unexpected active orders.

## 9. Watch the control room

Compact, continuously updating view:

```bash
sudo systemctl start tradedashboard-compact.service
journalctl -fu tradedashboard-compact.service
```

Detailed hourly view:

```bash
sudo systemctl start tradedashboard-detailed.service
journalctl -fu tradedashboard-detailed.service
```

Press `Ctrl-C` to leave the display. That does not stop the bot.

Runtime events:

```bash
journalctl -fu traderuntime.service
```

Recorder events:

```bash
journalctl -fu lightercollector.service
```

## 10. What the phone reports

ntfy sends:

- Recorder and runtime startup/restart.
- Runtime degradation or connection errors.
- Paper state changes for every market.
- BTC/ETH live entry submission.
- Live fill and protection-state changes.
- Hourly session totals.
- EOD package PASS or FAIL.

Treat an urgent degradation or EOD FAIL as requiring review. Notification
delivery is intentionally not a trading dependency; recording and lifecycle
management continue if ntfy is temporarily unavailable.

## 11. Paper test-day checklist

Before 05:00 CT:

```text
[ ] Recorder connected
[ ] Book coverage 12/12
[ ] Nonce gaps zero
[ ] Runtime active
[ ] Timers active
[ ] ntfy test received
[ ] ENABLE_FUNDED_EXECUTION=false
[ ] Account check PASS
```

At 05:00–05:05 CT:

```text
[ ] Twelve independent plans created
[ ] Twelve paper opportunities visible
[ ] BTC and ETH also remain paper-only
[ ] No funded order exists
[ ] Phone receives session updates
```

After 05:05 CT:

```text
[ ] Waiting, filled, TP1, breakeven, TP2, stop and no-fill states update
[ ] Dashboard matches journal records
[ ] Recorder remains 12/12 with zero nonce gaps
```

After 16:05 CT:

```text
[ ] Every opportunity has a terminal or explicitly open/expiry outcome
[ ] EOD notification received
[ ] EOD data quality PASS
[ ] Twelve ML rows exported with shadow_only=true
[ ] No funded orders were submitted
```

## 12. Enable BTC and ETH live trading

Do this only after the complete dress rehearsal passes and the account is flat.

Open the protected environment:

```bash
sudoedit /etc/overnight-strategy.env
```

Set:

```text
KILL_SWITCH=false
ENABLE_FUNDED_EXECUTION=true
```

Then run the account check again:

```bash
cd /opt/overnight-strategy
/opt/overnight-strategy/build/lighterexecutor -check
```

If it passes, restart only the runtime:

```bash
sudo systemctl restart traderuntime.service
systemctl is-active traderuntime.service
journalctl -n 50 --no-pager -u traderuntime.service
```

The startup notification must say that the BTC/ETH funded route is enabled.
New funded entries still cannot occur outside 05:00–05:05 CT.

## 13. Safe pause: stop new entries without abandoning protection

Set the kill switch:

```bash
sudoedit /etc/overnight-strategy.env
```

Change:

```text
KILL_SWITCH=true
```

Restart the runtime so it reloads the value:

```bash
sudo systemctl restart traderuntime.service
```

The kill switch blocks new entries. Existing exchange-side reduce-only
protection remains in place. Confirm account positions and active orders with:

```bash
/opt/overnight-strategy/build/lighterexecutor -check
```

Do not stop the runtime while a live position is open unless the exchange-side
stop and target orders have been independently confirmed.

## 14. Return to paper-only mode

Edit the protected file:

```text
ENABLE_FUNDED_EXECUTION=false
KILL_SWITCH=false
```

Then:

```bash
sudo systemctl restart traderuntime.service
```

All 12 paper strategies continue; BTC/ETH funded submissions stop.

## 15. Power loss and reboot recovery

After TradePi reboots, systemd automatically restarts the recorder and runtime
and catches persistent timers. Verify:

```bash
systemctl is-active lightercollector.service traderuntime.service
systemctl list-timers dailyplans.timer eodexport.timer lighterarchive.timer
curl --fail --silent http://127.0.0.1:8082/healthz
/opt/overnight-strategy/build/lighterexecutor -check
```

The runtime uses deterministic client-order indexes and durable lifecycle
states to avoid creating a second order for the same opportunity after restart.

## 16. Remote five-minute check while away

From the Mac:

```bash
ssh traderbot@192.168.3.28 '
echo SERVICES
systemctl is-active lightercollector.service traderuntime.service dailyplans.timer eodexport.timer lighterarchive.timer
echo RECORDER
curl --fail --silent http://127.0.0.1:8082/healthz
echo RECENT_RUNTIME
journalctl -n 20 --no-pager -u traderuntime.service
'
```

Healthy means services are active, recorder connected, 12 books ready, zero
nonce gaps, and no repeated runtime errors.

## 17. Troubleshooting

### No phone notification

1. Repeat the ntfy test in section 3.
2. Confirm `NTFY_URL` and `NTFY_TOPIC` are set in one protected environment.
3. Restart `lightercollector` and `traderuntime`.
4. Check `journalctl -n 100 -u traderuntime.service`.

### Recorder not connected or fewer than 12 books

```bash
sudo systemctl restart lightercollector.service
sleep 10
curl --fail --silent http://127.0.0.1:8082/healthz
```

Do not enable funded execution until 12/12 coverage returns.

### Runtime is failed

```bash
systemctl status traderuntime.service --no-pager
journalctl -n 200 --no-pager -u traderuntime.service
sudo systemctl restart traderuntime.service
```

### Daily plans did not run

```bash
systemctl status dailyplans.timer --no-pager
journalctl -n 200 --no-pager -u dailyplans.service
sudo systemctl start dailyplans.service
```

Manual start is safe because plan and order identities are deterministic. Do
not manually start a delayed funded session outside the entry window.

### EOD export failed

```bash
journalctl -n 200 --no-pager -u eodexport.service
sudo systemctl start eodexport.service
```

An EOD failure does not grant permission to delete or rewrite raw recorder data.

## 18. Emergency rules

- Never paste the Lighter private key into chat, logs, or screenshots.
- Never delete recorder or journal data to clear an error.
- Never enable a research asset for funded execution.
- Never bypass the 05:00–05:05 CT entry window.
- Never treat an inferred liquidation as confirmed.
- Never let ML output alter the frozen baseline; all ML remains shadow-only.
- If account state, local state, and notifications disagree, set
  `KILL_SWITCH=true`, inspect the exchange account directly, and keep funded
  execution disabled until reconciliation passes.

## Final unattended-operation gate

The bot is ready to run while away only when every item is true:

```text
[ ] Verified checksums
[ ] Recorder service active
[ ] Runtime service active
[ ] Timers active with correct CT schedules
[ ] 12/12 books ready
[ ] Zero nonce gaps
[ ] Authenticated account check PASS
[ ] No unexpected position or order
[ ] ntfy phone test received
[ ] Full paper test day PASS
[ ] EOD and ML export PASS
[ ] Restart/reboot recovery tested
[ ] BTC/ETH-only live authority verified
[ ] Kill switch tested
```

If any item fails, remain in paper-only mode.
