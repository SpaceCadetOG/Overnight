#!/bin/sh
set -eu

package=${1:?validated package directory is required}
bucket=${AWS_DATA_LAKE_BUCKET:?AWS_DATA_LAKE_BUCKET is required}
kms_key=${AWS_DATA_KMS_KEY_ARN:?AWS_DATA_KMS_KEY_ARN is required}
manifest="$package/MANIFEST.json"
package_id=$(jq -er .package_id "$manifest")
date=$(jq -er .date "$manifest")
prefix="landing/lighter/date=$date/package=$package_id"

fail_metric() {
    aws cloudwatch put-metric-data --namespace OvernightStrategy/JumpPi --metric-name UploadFailed --value 1 --unit Count >/dev/null 2>&1 || true
}
trap fail_metric HUP INT TERM

# Data objects are uploaded and verified first. The manifest is deliberately
# last because its S3 finalization is the only processing trigger.
jq -c '.files[]' "$manifest" | while IFS= read -r row; do
    relative=$(printf '%s' "$row" | jq -er .path)
    expected_size=$(printf '%s' "$row" | jq -er .compressed_bytes)
    expected_sha=$(printf '%s' "$row" | jq -er .sha256)
    key="$prefix/$relative"
    aws s3 cp "$package/$relative" "s3://$bucket/$key" --only-show-errors --sse aws:kms --sse-kms-key-id "$kms_key" --metadata "sha256=$expected_sha"
    actual_size=$(aws s3api head-object --bucket "$bucket" --key "$key" --query ContentLength --output text)
    actual_sha=$(aws s3api head-object --bucket "$bucket" --key "$key" --query 'Metadata.sha256' --output text)
    if [ "$actual_size" != "$expected_size" ] || [ "$actual_sha" != "$expected_sha" ]; then
        fail_metric
        printf '%s\n' "JumpPi verification failed object=$key" >&2
        exit 1
    fi
done

aws s3 cp "$manifest" "s3://$bucket/$prefix/MANIFEST.json" --only-show-errors --sse aws:kms --sse-kms-key-id "$kms_key"

attempt=0
ack="landing/acknowledgements/$package_id.json"
while [ "$attempt" -lt 60 ]; do
    if aws s3 cp "s3://$bucket/$ack" "$package/CLOUD_VERIFIED.json.tmp" --only-show-errors 2>/dev/null; then
        if jq -e '.status == "VALID" and .shadow_only == true' "$package/CLOUD_VERIFIED.json.tmp" >/dev/null; then
            mv "$package/CLOUD_VERIFIED.json.tmp" "$package/CLOUD_VERIFIED.json"
            printf '%s\n' "AWS validation acknowledged package=$package_id"
            exit 0
        fi
    fi
    attempt=$((attempt + 1))
    sleep 10
done

fail_metric
printf '%s\n' "AWS validation timeout package=$package_id" >&2
exit 1
