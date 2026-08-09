output "aws_account_id" {
  value = data.aws_caller_identity.current.account_id
}
output "aws_region" {
  value = var.aws_region
}
output "data_lake_bucket" {
  value = aws_s3_bucket.lake.id
}
output "data_lake_arn" {
  value = aws_s3_bucket.lake.arn
}
output "data_lake_prefixes" {
  value = { for zone in local.zones : zone => "s3://${aws_s3_bucket.lake.id}/${zone}/" }
}
output "audit_bucket" {
  value = aws_s3_bucket.audit.id
}
output "kms_key_arn" {
  value = aws_kms_key.data.arn
}
output "jumppi_bootstrap_user" {
  value = aws_iam_user.jumppi_bootstrap.name
}
output "validation_queue_url" {
  value = aws_sqs_queue.validation.id
}
output "validation_queue_arn" {
  value = aws_sqs_queue.validation.arn
}
output "validation_dlq_url" {
  value = aws_sqs_queue.validation_dlq.id
}
output "validation_dlq_arn" {
  value = aws_sqs_queue.validation_dlq.arn
}
output "validator_lambda_name" {
  value = aws_lambda_function.validator.function_name
}
output "package_registry_table" {
  value = aws_dynamodb_table.packages.name
}
output "state_machine_arn" {
  value = aws_sfn_state_machine.processing.arn
}
output "ecr_repository_urls" {
  value = { for name, repo in aws_ecr_repository.processing : name => repo.repository_url }
}
output "ecs_cluster_arn" {
  value = aws_ecs_cluster.processing.arn
}
output "ecs_task_definitions" {
  value = { for name, task in aws_ecs_task_definition.processing : name => task.arn }
}
output "glue_databases" {
  value = { for name, db in aws_glue_catalog_database.databases : name => db.name }
}
output "athena_workgroup" {
  value = aws_athena_workgroup.research.name
}
output "athena_output_location" {
  value = "s3://${aws_s3_bucket.lake.id}/athena-results/"
}
output "sagemaker_execution_role_arn" {
  value = aws_iam_role.sagemaker.arn
}
output "model_package_group" {
  value = aws_sagemaker_model_package_group.shadow.model_package_group_name
}
output "reporting_role_arn" {
  value = aws_iam_role.reporting.arn
}
output "cloudwatch_dashboard" {
  value = aws_cloudwatch_dashboard.operations.dashboard_name
}
output "validator_log_group" {
  value = aws_cloudwatch_log_group.validator.name
}
output "workflow_log_group" {
  value = aws_cloudwatch_log_group.workflow.name
}
output "sns_topic_arn" {
  value = aws_sns_topic.operations.arn
}
output "github_deployment_role_arn" {
  value = aws_iam_role.github_deployment.arn
}
output "realtime_inference_enabled" {
  value = var.enable_realtime_endpoint
}
output "shadow_only" {
  value = true
}
