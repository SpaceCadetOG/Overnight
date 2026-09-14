#!/bin/sh
set -eu

base_url="${1:-http://127.0.0.1:8083}"
asset="${2:-BTC}"
from="${3:-2026-09-10T14:00:00Z}"
to="${4:-2026-09-10T14:05:00Z}"

call() {
    title="$1"
    path="$2"
    filter="$3"
    echo "=== $title ==="
    body="$(curl -fsS "$base_url$path")"
    printf '%s\n' "$body" | jq "$filter"
}

encoded_from="$(printf '%s' "$from" | jq -sRr @uri)"
encoded_to="$(printf '%s' "$to" | jq -sRr @uri)"
range="asset=$asset&from=$encoded_from&to=$encoded_to"

call "CANDLES" "/v1/candles?$range&interval=1m" '{schema_version,type,asset,from,to,quality,packages,count:(.candles|length),first:.candles[0]}'
call "VOLUME PROFILE" "/v1/profiles?$range" '{schema_version,type,profile_model,asset,from,to,quality,packages,excluded_events,profile:{trades:.profile.trades,poc:.profile.poc,vah:.profile.vah,val:.profile.val,vwap:.profile.vwap,hvns:.profile.hvns,lvns:.profile.lvns}}'
call "FOOTPRINT" "/v1/footprints?$range&interval=1m" '{schema_version,type,asset,quality,count:(.footprints|length),first:.footprints[0]}'
call "ORDER FLOW" "/v1/order-flow?$range" '{schema_version,type,asset,trades,buy_volume,sell_volume,total_volume,delta,cvd,delta_rate_per_second,quality,packages}'
call "HEATMAP" "/v1/heatmap?$range" '{schema_version,model,asset,quality,count:(.levels|length),levels:(.levels[:5])}'
call "LIQUIDITY CORRELATION" "/v1/liquidity?$range" '{schema_version,model,asset,quality,count,events:(.events[:5]),disclaimer}'
call "OPEN INTEREST" "/v1/open-interest?$range" '{schema_version,asset,quality,count,points:(.points[:5])}'
call "SESSIONS" "/v1/sessions?at=$encoded_to" '{schema_version,session_definition_version,as_of,sessions:[.sessions[]|{id,type,utc_start,utc_end,status}]}'
call "LEVELS" "/v1/levels?$range" '{schema_version,session_definition_version,zone_model,asset,quality,count:(.levels|length),levels}'
call "ZONES" "/v1/zones?$range" '{schema_version,session_definition_version,zone_model,asset,quality,count:(.zones|length),zones}'
call "BOOK AT" "/v1/books/$asset/at?at=$encoded_to" '{schema_version,asset,at,quality,venue_nonce,oracle_sequence,depth,best_bid:.bids[0],best_ask:.asks[0],checksum}'

echo "ORACLE HISTORICAL ANALYTICS ACCEPTANCE: PASS"
