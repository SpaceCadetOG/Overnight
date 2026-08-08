#!/bin/sh
set -eu

artifact_dir=${1:?artifact directory is required}
commit=${2:?git commit is required}
host=${TRADEPI_SSH_HOST:-traderbot@192.168.3.28}
staging="/opt/overnight-strategy/releases/$commit"
required="lightercollector dailyplans eodexport tradedashboard collectorarchive packagevalidator lighterexecutor"

for binary in $required; do
    [ -x "$artifact_dir/$binary" ] || {
        printf '%s\n' "missing release binary: $artifact_dir/$binary" >&2
        exit 1
    }
done
[ -f "$artifact_dir/systemd/lightercollector.service" ] || {
    printf '%s\n' "release is missing systemd definitions" >&2
    exit 1
}
[ -x "$artifact_dir/scripts/archive-and-upload.sh" ] || {
    printf '%s\n' "release is missing archive lifecycle script" >&2
    exit 1
}

ssh "$host" "mkdir -p '$staging'"
scp -r "$artifact_dir/." "$host:$staging/"
ssh "$host" "cd '$staging' && sha256sum --check SHA256SUMS"
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
