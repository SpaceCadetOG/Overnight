#!/bin/sh
set -eu

base_url="${1:-http://127.0.0.1:8083}"
asset="${2:-BTC}"

echo "=== HEALTH ==="
curl -fsS "$base_url/healthz" | jq .

echo "=== READINESS ==="
readiness="$(curl -fsS "$base_url/v1/readiness")"
printf '%s\n' "$readiness" | jq .
printf '%s\n' "$readiness" | jq -e '.ready == true and .collector_connected == true and .books_ready == 12 and .oracle_books_ready == 12 and .oracle_parity_ready == 12 and (.oracle_last_error == "" or .oracle_last_error == null)' >/dev/null

echo "=== INSTRUMENTS ==="
instruments="$(curl -fsS "$base_url/v1/instruments")"
printf '%s\n' "$instruments" | jq '{schema_version,count,assets:[.instruments[].asset]}'
printf '%s\n' "$instruments" | jq -e '.count == 12' >/dev/null

echo "=== BOOK ==="
book="$(curl -fsS "$base_url/v1/books/$asset")"
printf '%s\n' "$book" | jq '{schema_version,asset,timestamp,venue_nonce,oracle_sequence,depth,bids:(.bids|length),asks:(.asks|length),best_bid:.bids[0],best_ask:.asks[0],quality,checksum}'
printf '%s\n' "$book" | jq -e '.quality == "CERTIFIED" and (.bids|length)>0 and (.asks|length)>0 and .oracle_sequence>0 and .venue_nonce>0' >/dev/null

echo "=== MARKET ==="
market="$(curl -fsS "$base_url/v1/market/$asset")"
printf '%s\n' "$market" | jq '{schema_version,asset,as_of,book_ready,book_depth:.book.depth,book_quality:.book.quality,latest_streams:(.latest|keys)}'
printf '%s\n' "$market" | jq -e '.book_ready == true and .book.quality == "CERTIFIED"' >/dev/null

echo "=== INPUT VALIDATION ==="
code="$(curl -sS -o /tmp/oracle-invalid-response.json -w '%{http_code}' "$base_url/v1/events?from=2026-09-14T18:00:00Z&to=2026-09-14T19:00:00Z&stream=trade")"
test "$code" = "400"
jq . /tmp/oracle-invalid-response.json

echo "ORACLE LIVE REST ACCEPTANCE: PASS"
