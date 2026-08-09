# Security-policy review

Reviewed scope: AWS foundation only. This stack contains no exchange clients, trading credentials, order permissions, or network path into TradePi.

## Enforced boundaries

- TradePi receives no AWS principal.
- JumpPi's bootstrap principal is limited to `landing/`, KMS use for that data, and one custom metric namespace. It cannot delete objects, read raw/derived zones, invoke processing, access SageMaker, or change infrastructure.
- Lambda reads landing, writes only raw/quarantine, conditionally updates the package registry, and starts only the processing state machine.
- Processing tasks read validated data and write derived analytical prefixes. They cannot write landing/raw/models or use SageMaker promotion APIs.
- SageMaker reads datasets/features and writes only ML datasets, models, and shadow reports. It has no production trading or secrets access.
- Reporting is read-oriented and limited to analytical prefixes plus Athena results.
- GitHub may assume its deployment role only from `SpaceCadetOG/Overnight` on `main`; pull requests receive no AWS role.
- All objects are private, versioned, TLS-only, and KMS encrypted. Landing/raw deletion is explicitly denied. Object Lock is not enabled.
- DynamoDB point-in-time recovery and deletion protection are enabled. Permanent buckets and KMS-backed data resources use deletion safeguards.
- Model package approval is manual and every task/model foundation is tagged or configured `shadow_only=true`.

## Deliberate exceptions and follow-up

- The initial JumpPi IAM user has no access key managed by Terraform. If used for bootstrap, create one manually, keep it root-readable on JumpPi, rotate after the acceptance test, then replace it with a certificate-based temporary credential mechanism.
- The GitHub deployment policy spans the services Terraform manages. Its trust policy is narrowly bound to the repository/main branch and production environment approval must remain enabled.
- Interface VPC endpoints are disabled by default because they create hourly charges. Enable them before executing private Fargate tasks; the idle task definitions themselves do not run.
- CloudTrail management-event logging is enabled. Data-event selectors can be added after measuring their cost.
- Production Glue/Iceberg tables must be applied from reviewed, versioned application schemas; crawlers are not granted authority to define production contracts.

## Destructive controls

No Object Lock, irreversible retention, automatic current raw deletion, `force_destroy`, real-time endpoint, GPU, or automatic model approval is enabled.
