#!/bin/sh
set -eu

release=${1:?release or rollback is required}
root=/opt/overnight-strategy
current="$root/current"
previous="$root/previous"
required="lightercollector dailyplans eodexport tradedashboard collectorarchive packagevalidator lighterexecutor"

if [ "$release" = rollback ]; then
    target=$(readlink -f "$previous")
    [ -n "$target" ] && [ -d "$target" ]
else
    target="$root/releases/$release"
    for binary in $required; do [ -x "$target/$binary" ]; done
    [ -x "$target/scripts/archive-and-upload.sh" ]
    [ -f "$target/systemd/lightercollector.service" ]
    (cd "$target" && sha256sum --check SHA256SUMS)
fi

old=$(readlink -f "$current" || true)
ln -sfn "$target" "$current.new"
mv -Tf "$current.new" "$current"
if [ -n "$old" ] && [ -d "$old" ]; then ln -sfn "$old" "$previous"; fi
mkdir -p "$root/build"
for binary in $required; do ln -sfn "$current/$binary" "$root/build/$binary"; done
ln -sfn "$current/scripts/archive-and-upload.sh" "$root/scripts/archive-and-upload.sh"

for unit in "$current"/systemd/*.service "$current"/systemd/*.timer; do
    [ -f "$unit" ] || continue
    install -m 0644 "$unit" "/etc/systemd/system/$(basename "$unit")"
done
systemctl daemon-reload
systemctl enable lightercollector.service dailyplans.timer eodexport.timer lighterarchive.timer
systemctl restart lightercollector.service
systemctl restart dailyplans.timer eodexport.timer lighterarchive.timer
