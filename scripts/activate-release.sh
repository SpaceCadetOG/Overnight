#!/bin/sh
set -eu

release=${1:?release or rollback is required}
root=/opt/overnight-strategy
current="$root/current"
previous="$root/previous"

if [ "$release" = rollback ]; then
    target=$(readlink -f "$previous")
    [ -n "$target" ] && [ -d "$target" ]
else
    target="$root/releases/$release"
    [ -x "$target/lightercollector" ]
    [ -x "$target/collectorarchive" ]
fi

old=$(readlink -f "$current" || true)
ln -sfn "$target" "$current.new"
mv -Tf "$current.new" "$current"
if [ -n "$old" ] && [ -d "$old" ]; then ln -sfn "$old" "$previous"; fi
ln -sfn "$current/lightercollector" "$root/build/lightercollector"
ln -sfn "$current/collectorarchive" "$root/build/collectorarchive"
ln -sfn "$current/packagevalidator" "$root/build/packagevalidator"
systemctl restart lightercollector.service
