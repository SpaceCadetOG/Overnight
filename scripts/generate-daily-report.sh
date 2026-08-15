#!/bin/sh
set -eu

export TZ=America/Chicago
store=${RESEARCH_STORE:-/mnt/trading/research}
state_root=${OVERNIGHT_STATE_ROOT:-/var/lib/overnight}
report_bin=${DAILY_REPORT_BIN:-/opt/overnight-strategy/current/bin/dailyreport}
day=${1:-$(date +%F)}
report_dir="$state_root/reports/day=$day"

umask 077
mkdir -p "$report_dir"
"$report_bin" -store "$store" -date "$day" -json > "$report_dir/report.json.tmp"
"$report_bin" -store "$store" -date "$day" > "$report_dir/report.txt.tmp"
mv "$report_dir/report.json.tmp" "$report_dir/report.json"
mv "$report_dir/report.txt.tmp" "$report_dir/report.txt"
printf '%s\n' "daily report generated date=$day path=$report_dir"
