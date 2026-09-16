#!/bin/sh
set -eu

archive_root=${ORACLE_ARCHIVE_ROOT:-/mnt/trading/recorder/lighter}
index_root=${ORACLE_INDEX_ROOT:-/mnt/trading/oracle/index}
oracleindex=${ORACLE_INDEX_BINARY:-/opt/overnight-strategy/current/bin/oracleindex}
report_root=${ORACLE_INDEX_REPORT_ROOT:-/mnt/trading/oracle/reports}
lock_file=${ORACLE_INDEX_LOCK_FILE:-/mnt/trading/oracle/backfill.lock}

mkdir -p "$index_root" "$report_root"
exec 9>"$lock_file"
flock -n 9 || {
	printf '%s\n' "another Oracle index backfill is active" >&2
	exit 1
}

started=$(date -u +%Y%m%dT%H%M%SZ)
report="$report_root/backfill-$started.tsv"
printf 'package\tstarted_at\tfinished_at\tduration_seconds\tpartitions\trecords\tbytes\tresult\n' >"$report"

failed=0
find "$archive_root" -name MANIFEST.json -type f -print | sort | while IFS= read -r manifest; do
	package=$(jq -er '.package_id' "$manifest")
	package_started=$(date -u +%FT%TZ)
	start_epoch=$(date +%s)
	result=INDEXED
	if ! "$oracleindex" -root "$archive_root" -index-root "$index_root" -package "$package"; then
		result=FAILED
		failed=$((failed + 1))
	fi
	finish_epoch=$(date +%s)
	package_finished=$(date -u +%FT%TZ)
	index_manifest="$index_root/$package/INDEX.json"
	partitions=0
	records=0
	bytes=0
	if [ -s "$index_manifest" ]; then
		partitions=$(jq '[.partitions[]] | length' "$index_manifest")
		records=$(jq '[.partitions[].records] | add // 0' "$index_manifest")
		bytes=$(jq '[.partitions[].bytes] | add // 0' "$index_manifest")
	fi
	printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
		"$package" "$package_started" "$package_finished" "$((finish_epoch - start_epoch))" \
		"$partitions" "$records" "$bytes" "$result" >>"$report"
done

# The loop runs in a subshell on POSIX shells, so derive failure state from the
# durable report rather than relying on the loop's local counter.
failed=$(awk -F '\t' 'NR > 1 && $8 == "FAILED" { count++ } END { print count + 0 }' "$report")
printf 'Oracle index backfill complete report=%s failed=%s\n' "$report" "$failed"
[ "$failed" -eq 0 ]
