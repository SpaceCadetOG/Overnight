#!/bin/sh
set -eu

artifact_dir=${1:?artifact directory is required}
commit=${2:?git commit is required}
host=${TRADEPI_SSH_HOST:-traderbot@192.168.3.28}
staging="/opt/overnight-strategy/releases/$commit"

ssh "$host" "mkdir -p '$staging'"
scp "$artifact_dir/lightercollector" "$artifact_dir/collectorarchive" "$artifact_dir/packagevalidator" "$artifact_dir/SHA256SUMS" "$host:$staging/"
ssh "$host" "cd '$staging' && sha256sum --check SHA256SUMS --ignore-missing"
ssh "$host" "sudo /opt/overnight-strategy/scripts/activate-release.sh '$commit'"

attempt=0
while [ "$attempt" -lt 30 ]; do
    if ssh "$host" "curl --fail --silent http://127.0.0.1:8082/healthz" | grep -q '"connected":true'; then
        printf '%s\n' "TradePi deployment verified commit=$commit"
        exit 0
    fi
    attempt=$((attempt + 1))
    sleep 2
done

ssh "$host" "sudo /opt/overnight-strategy/scripts/activate-release.sh rollback"
printf '%s\n' "TradePi deployment failed health verification and was rolled back" >&2
exit 1
