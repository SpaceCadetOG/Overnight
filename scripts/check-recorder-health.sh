#!/bin/sh
set -eu

endpoint=${RECORDER_HEALTH_URL:-http://127.0.0.1:8082/healthz}
state_root=${OVERNIGHT_STATE_ROOT:-/var/lib/overnight}
expected_books=${EXPECTED_BOOKS:-12}
max_age_seconds=${RECORDER_MAX_AGE_SECONDS:-30}

mkdir -p "$state_root/health"
payload=$(curl --fail --silent --max-time 5 "$endpoint")
sampled_at=$(date -u +%FT%TZ)
printf '{"sampled_at":"%s","health":%s}\n' "$sampled_at" "$payload" >> "$state_root/health/recorder-health.jsonl"

printf '%s' "$payload" | jq -e \
    --argjson expected "$expected_books" \
    --argjson max_age "$max_age_seconds" '
      .connected == true and
      .books_ready == $expected and
      .nonce_gaps == 0 and
      (.crossed_books // 0) == 0 and
      (.invalid_levels // 0) == 0 and
      ((now - (.last_event | sub("\\.[0-9]+Z$"; "Z") | fromdateiso8601)) <= $max_age)
    ' >/dev/null

printf '%s\n' "recorder health PASS books=$expected_books sampled_at=$sampled_at"
