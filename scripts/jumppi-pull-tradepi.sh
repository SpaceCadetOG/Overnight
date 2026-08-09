#!/bin/sh
set -eu

export TZ=America/Chicago
root=${FORENSICS_ROOT:-/srv/trade-forensics}
day=${1:-$(date -d yesterday +%F)}
remote=${TRADEPI_REMOTE:-traderbot@192.168.3.28}
identity=${TRADEPI_TRANSFER_KEY:-/home/trader2/.ssh/tradepi_transfer}
name="lighter-$day"
pending="$root/packages/pending/$name"
temporary="$pending.tmp"
validated="$root/packages/validated/$name"
quarantine="$root/packages/quarantine/$name"

if [ -d "$validated" ] || [ -d "$quarantine" ]; then
    printf '%s\n' "package already classified name=$name"
    exit 0
fi

mkdir -p "$root/packages/pending" "$root/packages/validated" "$root/packages/quarantine"
if [ -e "$temporary" ]; then
    printf '%s\n' "stale temporary package requires review path=$temporary" >&2
    exit 1
fi
mkdir -p "$temporary"

rsync -a --checksum --partial --delay-updates \
    --include='*/' \
    --include='*.zst' \
    --include='MANIFEST.json' \
    --include='SHA256SUMS' \
    --exclude='*' \
    -e "ssh -i $identity -o IdentitiesOnly=yes -o BatchMode=yes" \
    "$remote:/mnt/trading/recorder/lighter/date=$day/" \
    "$temporary/"

test -s "$temporary/MANIFEST.json"
mv "$temporary" "$pending"
exec "$root/bin/jumppi-validate-package.sh" "$pending"
