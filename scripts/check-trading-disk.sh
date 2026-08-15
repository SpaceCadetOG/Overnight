#!/bin/sh
set -eu

mount=${TRADING_MOUNT:-/mnt/trading}
state_root=${OVERNIGHT_STATE_ROOT:-/var/lib/overnight}
warning=${DISK_WARNING_PERCENT:-70}
critical=${DISK_CRITICAL_PERCENT:-80}
emergency=${DISK_EMERGENCY_PERCENT:-90}

mkdir -p "$state_root/health"
used=$(df -P "$mount" | awk 'NR==2 {gsub(/%/, "", $5); print $5}')
available=$(df -Pk "$mount" | awk 'NR==2 {print $4 * 1024}')
sampled_at=$(date -u +%FT%TZ)
level=ok
status=0

if [ "$used" -ge "$emergency" ]; then
    level=emergency
    status=2
elif [ "$used" -ge "$critical" ]; then
    level=critical
    status=1
elif [ "$used" -ge "$warning" ]; then
    level=warning
fi

printf '{"sampled_at":"%s","mount":"%s","used_percent":%s,"available_bytes":%s,"level":"%s"}\n' \
    "$sampled_at" "$mount" "$used" "$available" "$level" \
    > "$state_root/health/disk-latest.json"
printf '%s\n' "trading disk level=$level used=${used}% available_bytes=$available"
exit "$status"
