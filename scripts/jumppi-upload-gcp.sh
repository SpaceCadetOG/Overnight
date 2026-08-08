#!/bin/sh
set -eu

package=${1:?validated package directory is required}
raw_remote=${CLOUD_RAW_REMOTE:?CLOUD_RAW_REMOTE is required}
validation_remote=${CLOUD_VALIDATION_REMOTE:?CLOUD_VALIDATION_REMOTE is required}
name=$(basename "$package")
manifest="$package/MANIFEST.json"
package_id=$(jq -er .package_id "$manifest")
destination="${raw_remote%/}/lighter/$name"

# Upload immutable data first and the manifest last. The manifest finalization
# is the cloud validator's package-ready signal.
rclone copy "$package" "$destination" --immutable --checksum --exclude MANIFEST.json --exclude CLOUD_VERIFIED
rclone copyto "$manifest" "$destination/MANIFEST.json" --immutable --checksum
rclone check "$package" "$destination" --checksum --one-way --exclude CLOUD_VERIFIED

attempt=0
while [ "$attempt" -lt 60 ]; do
    temp="$package/.cloud-validation.tmp"
    if rclone copyto "${validation_remote%/}/validation/$package_id.json" "$temp" 2>/dev/null; then
        if jq -e '.result.valid == true' "$temp" >/dev/null; then
            mv "$temp" "$package/CLOUD_VERIFIED.json"
            printf '%s\n' "cloud validation acknowledged package=$package_id"
            exit 0
        fi
        mv "$temp" "$package/CLOUD_REJECTED.json"
        printf '%s\n' "cloud rejected package=$package_id" >&2
        exit 1
    fi
    attempt=$((attempt + 1))
    sleep 10
done

printf '%s\n' "cloud validation timeout package=$package_id" >&2
exit 1
