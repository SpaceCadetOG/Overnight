# AWS deployment

This stack preserves the edge/cloud boundary:

- TradePi records and executes without AWS dependencies.
- JumpPi locally validates sealed packages, uploads immutable objects, and waits for an independent AWS validation acknowledgement.
- S3 stores raw, validation, and quarantine data in separate versioned buckets.
- S3 emits only completed-manifest notifications to SQS.
- A concurrency-limited Lambda downloads the sealed package, verifies checksums and row counts, and writes a validation or quarantine result.
- GitHub Actions uses OIDC. No long-lived AWS key is stored in GitHub.

## One-time bootstrap

1. Authenticate an administrator locally with AWS SSO.
2. Create the dedicated Terraform state bucket using `backend-bootstrap.tf.example` as a guide.
3. Build `dist/awsvalidator.zip` and apply this stack locally once.
4. Add the Terraform output role ARN and state settings as GitHub environment variables:
   - `AWS_TERRAFORM_ROLE_ARN`
   - `AWS_TERRAFORM_STATE_BUCKET`
   - `AWS_REGION`
5. Add `AWS_BUDGET_EMAIL` as an environment secret.
6. Require approval on the `aws-production` GitHub environment.

Terraform intentionally does not create an access key for JumpPi. After deployment, create one key for the output IAM user, install it only in JumpPi's root-owned rclone configuration, test it, and never copy it to TradePi or GitHub.

The raw bucket has no automatic current-object deletion. Quarantine markers expire after 90 days, old object versions after 30 days, and validator logs after 14 days. The default budget is USD 5/month with 50%, 80%, and 100% alerts.
