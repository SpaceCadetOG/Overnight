# Cost estimate

This is an engineering estimate before an account/region-specific Terraform plan. Confirm it with AWS Pricing Calculator after selecting the region.

## Idle foundation

- Customer-managed KMS key: approximately USD 1/month before rotations.
- S3: proportional to stored data; at roughly USD 0.023/GB-month in common US regions, 100 GB is about USD 2.30/month before requests.
- ECS cluster, ECR repositories, task definitions, Lambda, SQS, Step Functions, DynamoDB on-demand, Glue databases, and SageMaker role/model group: near zero while unused, excluding small metadata/log storage.
- CloudWatch alarms/dashboard/logs and CloudTrail S3 storage can add a few dollars after free allowances depending on account plan and volume.
- Interface VPC endpoints: disabled by default because their hourly cost would dominate this small system.

Expected idle total before meaningful data: roughly USD 1–5/month, plus S3 and any CloudWatch usage.

## Initial daily operation

One 12-asset package/day invokes Lambda once and then up to ten short Fargate tasks. At 0.5 vCPU/1 GB, ten one-minute tasks per day remain small, but actual cost depends on processing duration. Athena is capped at 1 GiB scanned per query by default. SageMaker jobs do not exist until explicitly started.

The Terraform budget defaults to USD 5 for development and USD 10 for production, with 50%, 80%, and 100% notifications when an email is supplied.
