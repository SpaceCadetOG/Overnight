#!/bin/sh
set -eu

release=${1:?release, rollback, or version is required}
activator_version=2
if [ "$release" = version ]; then
    printf '%s\n' "$activator_version"
    exit 0
fi
root=/opt/overnight-strategy
current="$root/current"
previous="$root/previous"
required="lightercollector dailyplans dailylevels dailyreport eodexport tradedashboard collectorarchive packagevalidator lighterexecutor traderuntime recordercert"

mkdir -p "$root/releases" /var/lib/overnight
exec 9>"$root/deploy-lock"
flock -n 9 || {
    printf '%s\n' "another Overnight deployment is active" >&2
    exit 1
}

if [ "$release" = rollback ]; then
    target=$(readlink -f "$previous")
    [ -n "$target" ] && [ -d "$target" ]
else
    target="$root/releases/$release"
    for binary in $required; do [ -x "$target/bin/$binary" ]; done
    [ -x "$target/scripts/archive-and-upload.sh" ]
    [ -x "$target/scripts/check-recorder-health.sh" ]
    [ -x "$target/scripts/check-trading-disk.sh" ]
    [ -x "$target/scripts/generate-daily-report.sh" ]
    [ -f "$target/systemd/lightercollector.service" ]
    [ -s "$target/BUILD.json" ]
    (cd "$target" && sha256sum --check SHA256SUMS)
fi

old=$(readlink -f "$current" || true)
ln -sfn "$target" "$current.new"
mv -Tf "$current.new" "$current"
if [ -n "$old" ] && [ -d "$old" ] && [ "$old" != "$target" ]; then ln -sfn "$old" "$previous"; fi

for unit in "$current"/systemd/*.service "$current"/systemd/*.timer "$current"/systemd/*.target; do
    [ -f "$unit" ] || continue
    install -m 0644 "$unit" "/etc/systemd/system/$(basename "$unit")"
done
install -d -m 0755 /etc/systemd/journald.conf.d
install -m 0644 "$current/journald/overnight.conf" /etc/systemd/journald.conf.d/overnight.conf
systemctl daemon-reload
systemctl enable overnight-recorder.target overnight-trading.target overnight-operations.target
systemctl enable lightercollector.service traderuntime.service dailyplans.timer dailylevels.timer dailyreport.timer eodexport.timer lighterarchive.timer overnight-health.timer overnight-disk-guard.timer
systemctl restart lightercollector.service
systemctl restart traderuntime.service
systemctl restart dailyplans.timer dailylevels.timer dailyreport.timer eodexport.timer lighterarchive.timer overnight-health.timer overnight-disk-guard.timer
