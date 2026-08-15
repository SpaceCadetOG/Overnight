#!/bin/sh
set -eu

artifact_dir=${1:?artifact directory is required}
commit=${2:?git commit is required}
build_id=${3:-manual}
host=${TRADEPI_SSH_HOST:-traderbot@192.168.3.28}
release_id="$commit-$build_id"
release_root=/opt/overnight-strategy/releases
staging="$release_root/$release_id"
temporary="$release_root/.staging-$release_id"
required="lightercollector dailyplans dailylevels dailyreport eodexport tradedashboard collectorarchive packagevalidator lighterexecutor traderuntime recordercert"

for binary in $required; do
    [ -x "$artifact_dir/bin/$binary" ] || {
        printf '%s\n' "missing release binary: $artifact_dir/bin/$binary" >&2
        exit 1
    }
done
[ -s "$artifact_dir/BUILD.json" ] || {
    printf '%s\n' "release is missing BUILD.json" >&2
    exit 1
}
[ -f "$artifact_dir/systemd/lightercollector.service" ] || {
    printf '%s\n' "release is missing systemd definitions" >&2
    exit 1
}
[ -x "$artifact_dir/scripts/archive-and-upload.sh" ] &&
[ -x "$artifact_dir/scripts/check-recorder-health.sh" ] &&
[ -x "$artifact_dir/scripts/check-trading-disk.sh" ] &&
[ -x "$artifact_dir/scripts/generate-daily-report.sh" ] || {
    printf '%s\n' "release is missing archive lifecycle script" >&2
    exit 1
}

ssh "$host" "test \"\$(sudo /opt/overnight-strategy/scripts/activate-release.sh version)\" = 2" || {
    printf '%s\n' "TradePi activator v2 is not installed; perform the documented one-time bootstrap" >&2
    exit 1
}

ssh "$host" "test ! -e '$staging' && test ! -e '$temporary' && mkdir -p '$temporary'"
scp -r "$artifact_dir/." "$host:$temporary/"
ssh "$host" "cd '$temporary' && sha256sum --check SHA256SUMS && test \"\$(jq -r .commit BUILD.json)\" = '$commit' && test \"\$(jq -r .target BUILD.json)\" = linux/arm64 && mv '$temporary' '$staging'"
ssh "$host" "sudo /opt/overnight-strategy/scripts/activate-release.sh '$release_id'"

attempt=0
while [ "$attempt" -lt 30 ]; do
    if ssh "$host" "curl --fail --silent http://127.0.0.1:8082/healthz | jq -e '.connected == true and .books_ready == 12 and .nonce_gaps == 0 and (.crossed_books // 0) == 0 and (.invalid_levels // 0) == 0' >/dev/null && systemctl is-active --quiet lightercollector.service traderuntime.service"; then
        printf '%s\n' "TradePi deployment verified release=$release_id"
        exit 0
    fi
    attempt=$((attempt + 1))
    sleep 2
done

ssh "$host" "sudo /opt/overnight-strategy/scripts/activate-release.sh rollback"
printf '%s\n' "TradePi deployment failed health verification and was rolled back" >&2
exit 1
