#!/bin/sh
set -eu

archive_root=${ORACLE_ARCHIVE_ROOT:-/mnt/trading/recorder/lighter}
index_root=${ORACLE_INDEX_ROOT:-/mnt/trading/oracle/index}
oracleindex=${ORACLE_INDEX_BINARY:-/opt/overnight-strategy/current/bin/oracleindex}
report_root=${ORACLE_INDEX_REPORT_ROOT:-/mnt/trading/oracle/reports}
lock_file=${ORACLE_INDEX_LOCK_FILE:-/mnt/trading/oracle/backfill.lock}
readiness_url=${ORACLE_READINESS_URL:-http://127.0.0.1:8083/v1/readiness}
required_books=${ORACLE_REQUIRED_BOOKS:-12}
poll_seconds=${ORACLE_INDEX_HEALTH_POLL_SECONDS:-5}
stable_checks=${ORACLE_INDEX_STABLE_CHECKS:-3}
nice_level=${ORACLE_INDEX_NICE_LEVEL:-19}

mkdir -p "$index_root" "$report_root"
exec 9>"$lock_file"
flock -n 9 || {
	printf '%s\n' "another Oracle index backfill is active" >&2
	exit 1
}

started=$(date -u +%Y%m%dT%H%M%SZ)
report="$report_root/backfill-$started.tsv"
printf 'package\tstarted_at\tfinished_at\tduration_seconds\tpartitions\trecords\tbytes\tresult\n' >"$report"

oracle_ready() {
	body=$(curl --fail --silent --max-time 2 "$readiness_url" 2>/dev/null) || return 1
	printf '%s' "$body" | jq -e \
		--argjson required "$required_books" \
		'.ready == true and .books_ready >= $required and .oracle_books_ready >= $required and .oracle_parity_ready >= $required' \
		>/dev/null
}

wait_for_stable_oracle() {
	checks=0
	while [ "$checks" -lt "$stable_checks" ]; do
		if oracle_ready; then
			checks=$((checks + 1))
		else
			checks=0
		fi
		[ "$checks" -ge "$stable_checks" ] || sleep "$poll_seconds"
	done
}

run_index_guarded() {
	package=$1
	if command -v ionice >/dev/null 2>&1; then
		ionice -c 3 nice -n "$nice_level" "$oracleindex" \
			-root "$archive_root" -index-root "$index_root" -package "$package" &
	else
		nice -n "$nice_level" "$oracleindex" \
			-root "$archive_root" -index-root "$index_root" -package "$package" &
	fi
	child=$!
	paused=0

	trap 'kill -TERM "$child" 2>/dev/null || true; wait "$child" 2>/dev/null || true; exit 130' INT TERM HUP
	while kill -0 "$child" 2>/dev/null; do
		if oracle_ready; then
			if [ "$paused" -eq 1 ]; then
				wait_for_stable_oracle
				kill -CONT "$child" 2>/dev/null || true
				paused=0
				printf '%s\t%s\tRESUMED\n' "$package" "$(date -u +%FT%TZ)" >>"$events_report"
			fi
		elif [ "$paused" -eq 0 ]; then
			kill -STOP "$child" 2>/dev/null || true
			paused=1
			printf '%s\t%s\tPAUSED_NOT_READY\n' "$package" "$(date -u +%FT%TZ)" >>"$events_report"
		fi
		sleep "$poll_seconds"
	done

	set +e
	wait "$child"
	status=$?
	set -e
	trap - INT TERM HUP
	return "$status"
}

events_report="$report.events.tsv"
printf 'package\tat\tevent\n' >"$events_report"

failed=0
find "$archive_root" -name MANIFEST.json -type f -print | sort | while IFS= read -r manifest; do
	package=$(jq -er '.package_id' "$manifest")
	wait_for_stable_oracle
	package_started=$(date -u +%FT%TZ)
	start_epoch=$(date +%s)
	result=INDEXED
	if ! run_index_guarded "$package"; then
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
