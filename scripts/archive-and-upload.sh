#!/bin/sh
set -eu

export TZ=America/Chicago
root=${COLLECTOR_ROOT:-/mnt/trading/recorder/lighter}
day=${1:-$(date -d yesterday +%F)}
archive_bin=${COLLECTOR_ARCHIVE_BIN:-/opt/overnight-strategy/current/bin/collectorarchive}

# Keep source JSONL on TradePi. Compression and local validation are not
# sufficient authority to delete recorder data; cloud acknowledgement is the
# later deletion gate.
"$archive_bin" -root "$root" -date "$day" -remove-raw=false

day_dir="$root/date=$day"
if [ -z "${CLOUD_REMOTE:-}" ]; then
    printf '%s\n' "cloud upload pending for date=$day: CLOUD_REMOTE is not configured"
    exit 0
fi
if ! command -v rclone >/dev/null 2>&1; then
    printf '%s\n' "cloud upload failed for date=$day: rclone is not installed" >&2
    exit 1
fi

destination="${CLOUD_REMOTE%/}/raw/lighter/date=$day"
rclone copy "$day_dir" "$destination" --immutable --checksum
rclone check "$day_dir" "$destination" --checksum --one-way
date -u +%FT%TZ > "$day_dir/CLOUD_VERIFIED"
printf '%s\n' "cloud upload verified date=$day destination=$destination"
