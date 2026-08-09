# Overnight Strategy AWS foundation

TradePi remains autonomous and has no AWS identity. JumpPi validates and buffers sealed packages, uploads data objects to `landing/`, verifies S3 size/checksum metadata, and uploads `MANIFEST.json` last. That final manifest alone triggers SQS and the lightweight Lambda validator.

Valid packages are conditionally registered in DynamoDB, copied into `raw/`, acknowledged under `landing/acknowledgements/`, and passed to the Step Functions/Fargate processing shell. Invalid packages receive a quarantine record. Duplicate package IDs do not start another workflow.

## Data lake

One private, versioned, KMS-encrypted bucket contains:

```text
landing/ raw/ quarantine/ normalized/ features/ forensics/
datasets/ models/ reports/ athena-results/
```

Landing and raw objects cannot be deleted through normal IAM calls. Object Lock is intentionally disabled. Current raw objects have no expiry.

## Bootstrap

1. Choose the AWS account and region.
2. Authenticate locally using AWS SSO.
3. Create the encrypted/versioned Terraform state bucket using `backend-bootstrap.tf.example` as a guide.
4. Build the Lambda artifact:

```bash
mkdir -p dist
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -trimpath -o dist/bootstrap ./cmd/awsvalidator
(cd dist && zip awsvalidator.zip bootstrap)
```

5. Initialize and create a reviewed plan:

```bash
terraform -chdir=infra/aws init \
  -backend-config="bucket=STATE_BUCKET" \
  -backend-config="key=trade-forensics/prod.tfstate" \
  -backend-config="region=SELECTED_REGION" \
  -backend-config="use_lockfile=true"

TF_VAR_aws_region=SELECTED_REGION \
TF_VAR_expected_account_id=TWELVE_DIGIT_ACCOUNT_ID \
TF_VAR_environment=prod \
terraform -chdir=infra/aws plan \
  -var-file=environments/prod.tfvars \
  -out=production.tfplan
```

6. Review `terraform show production.tfplan`, then apply that exact saved plan.

## GitHub

The first local apply creates the repository/main-bound OIDC deployment role. Configure the protected `aws-production` GitHub environment with:

- Variables: `AWS_TERRAFORM_ROLE_ARN`, `AWS_TERRAFORM_STATE_BUCKET`, `AWS_REGION`, `AWS_ACCOUNT_ID`
- Optional secrets: `AWS_BUDGET_EMAIL`, `AWS_ALERT_EMAIL`
- Required reviewer before the apply job

GitHub creates a saved Terraform plan, uploads its human-readable form, then waits for environment approval before applying that exact plan.

## JumpPi

Terraform creates a bootstrap IAM user but no access key. If temporary static credentials are necessary:

1. Create one access key after apply.
2. Store it only in JumpPi's root-owned AWS configuration (`0600`).
3. Export `AWS_DATA_LAKE_BUCKET` and `AWS_DATA_KMS_KEY_ARN` from Terraform outputs.
4. Run `scripts/jumppi-upload-aws.sh /srv/trade-forensics/packages/validated/date=YYYY-MM-DD`.
5. Rotate the key after the acceptance test and replace it with temporary certificate-backed credentials.

The JumpPi principal cannot delete data, use derived prefixes, start ML/training, approve models, access secrets, or modify infrastructure.

## Processing and ML activation

Fargate task definitions are safe placeholder shells. Private interface endpoints are disabled by default because they create hourly charges. Enable `enable_private_endpoints=true` and replace every placeholder container image with a reviewed immutable digest before running the acceptance workflow.

SageMaker real-time inference and GPU use are disabled. Models remain manual-approval and shadow-only.

See [SECURITY_REVIEW.md](SECURITY_REVIEW.md) and [COST_ESTIMATE.md](COST_ESTIMATE.md).
