#!/bin/sh
set -eu

package=${1:?package directory is required}
root=${FORENSICS_ROOT:-/srv/trade-forensics}
validator=${PACKAGE_VALIDATOR_BIN:-$root/bin/packagevalidator}
name=$(basename "$package")
result="$root/packages/$name-validation.json"

if "$validator" -package "$package" > "$result.tmp"; then
    mv "$result.tmp" "$result"
    mv "$package" "$root/packages/validated/$name"
    printf '%s\n' "validated package=$name"
else
    status=$?
    mv "$result.tmp" "$result"
    mv "$package" "$root/packages/quarantine/$name"
    printf '%s\n' "quarantined package=$name result=$result" >&2
    exit "$status"
fi
