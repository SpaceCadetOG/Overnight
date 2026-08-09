resource "aws_sns_topic" "operations" {
  name              = "${local.name}-operations"
  kms_master_key_id = "alias/aws/sns"
}
resource "aws_sns_topic_subscription" "email" {
  count     = local.alert_email == null ? 0 : 1
  topic_arn = aws_sns_topic.operations.arn
  protocol  = "email"
  endpoint  = local.alert_email
}

resource "aws_cloudwatch_metric_alarm" "validation_errors" {
  alarm_name          = "${local.name}-lambda-errors"
  namespace           = "AWS/Lambda"
  metric_name         = "Errors"
  dimensions          = { FunctionName = aws_lambda_function.validator.function_name }
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  threshold           = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  alarm_actions       = [aws_sns_topic.operations.arn]
  treat_missing_data  = "notBreaching"
}
resource "aws_cloudwatch_metric_alarm" "lambda_throttles" {
  alarm_name          = "${local.name}-lambda-throttles"
  namespace           = "AWS/Lambda"
  metric_name         = "Throttles"
  dimensions          = { FunctionName = aws_lambda_function.validator.function_name }
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  threshold           = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  alarm_actions       = [aws_sns_topic.operations.arn]
  treat_missing_data  = "notBreaching"
}
resource "aws_cloudwatch_metric_alarm" "queue_depth" {
  alarm_name          = "${local.name}-validation-queue-depth"
  namespace           = "AWS/SQS"
  metric_name         = "ApproximateNumberOfMessagesVisible"
  dimensions          = { QueueName = aws_sqs_queue.validation.name }
  statistic           = "Maximum"
  period              = 300
  evaluation_periods  = 2
  threshold           = 3
  comparison_operator = "GreaterThanThreshold"
  alarm_actions       = [aws_sns_topic.operations.arn]
  treat_missing_data  = "notBreaching"
}
resource "aws_cloudwatch_metric_alarm" "queue_age" {
  alarm_name          = "${local.name}-validation-oldest-message"
  namespace           = "AWS/SQS"
  metric_name         = "ApproximateAgeOfOldestMessage"
  dimensions          = { QueueName = aws_sqs_queue.validation.name }
  statistic           = "Maximum"
  period              = 300
  evaluation_periods  = 1
  threshold           = 900
  comparison_operator = "GreaterThanThreshold"
  alarm_actions       = [aws_sns_topic.operations.arn]
  treat_missing_data  = "notBreaching"
}
resource "aws_cloudwatch_metric_alarm" "dead_letter" {
  alarm_name          = "${local.name}-validation-dlq"
  namespace           = "AWS/SQS"
  metric_name         = "ApproximateNumberOfMessagesVisible"
  dimensions          = { QueueName = aws_sqs_queue.validation_dlq.name }
  statistic           = "Maximum"
  period              = 300
  evaluation_periods  = 1
  threshold           = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  alarm_actions       = [aws_sns_topic.operations.arn]
  treat_missing_data  = "notBreaching"
}
resource "aws_cloudwatch_metric_alarm" "workflow_failed" {
  alarm_name          = "${local.name}-workflow-failed"
  namespace           = "AWS/States"
  metric_name         = "ExecutionsFailed"
  dimensions          = { StateMachineArn = aws_sfn_state_machine.processing.arn }
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  threshold           = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  alarm_actions       = [aws_sns_topic.operations.arn]
  treat_missing_data  = "notBreaching"
}
resource "aws_cloudwatch_metric_alarm" "missing_daily_package" {
  alarm_name          = "${local.name}-missing-daily-package"
  namespace           = "OvernightStrategy/DataQuality"
  metric_name         = "DailyPackageMissing"
  statistic           = "Maximum"
  period              = 86400
  evaluation_periods  = 1
  threshold           = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  alarm_actions       = [aws_sns_topic.operations.arn]
  treat_missing_data  = "missing"
}
resource "aws_cloudwatch_metric_alarm" "checksum_mismatch" {
  alarm_name          = "${local.name}-checksum-mismatch"
  namespace           = "OvernightStrategy/DataQuality"
  metric_name         = "ChecksumMismatch"
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  threshold           = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  alarm_actions       = [aws_sns_topic.operations.arn]
  treat_missing_data  = "notBreaching"
}
resource "aws_cloudwatch_metric_alarm" "quarantined" {
  alarm_name          = "${local.name}-quarantined-package"
  namespace           = "OvernightStrategy/DataQuality"
  metric_name         = "PackageQuarantined"
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  threshold           = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  alarm_actions       = [aws_sns_topic.operations.arn]
  treat_missing_data  = "notBreaching"
}
resource "aws_cloudwatch_metric_alarm" "processing_failure" {
  alarm_name          = "${local.name}-fargate-task-failure"
  namespace           = "OvernightStrategy/Processing"
  metric_name         = "TaskFailure"
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  threshold           = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  alarm_actions       = [aws_sns_topic.operations.arn]
  treat_missing_data  = "notBreaching"
}
resource "aws_cloudwatch_metric_alarm" "ml_failure" {
  alarm_name          = "${local.name}-ml-job-failure"
  namespace           = "OvernightStrategy/ML"
  metric_name         = "JobFailure"
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  threshold           = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  alarm_actions       = [aws_sns_topic.operations.arn]
  treat_missing_data  = "notBreaching"
}

resource "aws_cloudwatch_dashboard" "operations" {
  dashboard_name = "${local.name}-operations"
  dashboard_body = jsonencode({ widgets = [{ type = "metric", x = 0, y = 0, width = 12, height = 6, properties = { title = "Validation queue", region = var.aws_region, metrics = [["AWS/SQS", "ApproximateNumberOfMessagesVisible", "QueueName", aws_sqs_queue.validation.name], [".", "ApproximateAgeOfOldestMessage", ".", "."]] } }, { type = "metric", x = 12, y = 0, width = 12, height = 6, properties = { title = "Validator", region = var.aws_region, metrics = [["AWS/Lambda", "Errors", "FunctionName", aws_lambda_function.validator.function_name], [".", "Throttles", ".", "."]] } }, { type = "metric", x = 0, y = 6, width = 12, height = 6, properties = { title = "Workflow", region = var.aws_region, metrics = [["AWS/States", "ExecutionsFailed", "StateMachineArn", aws_sfn_state_machine.processing.arn], [".", "ExecutionsSucceeded", ".", "."]] } }] })
}

data "aws_iam_policy_document" "audit_bucket" {
  statement {
    actions   = ["s3:GetBucketAcl"]
    resources = [aws_s3_bucket.audit.arn]
    principals {
      type        = "Service"
      identifiers = ["cloudtrail.amazonaws.com"]
    }
  }
  statement {
    actions   = ["s3:PutObject"]
    resources = ["${aws_s3_bucket.audit.arn}/AWSLogs/${data.aws_caller_identity.current.account_id}/*"]
    principals {
      type        = "Service"
      identifiers = ["cloudtrail.amazonaws.com"]
    }
    condition {
      test     = "StringEquals"
      variable = "s3:x-amz-acl"
      values   = ["bucket-owner-full-control"]
    }
  }
}
resource "aws_s3_bucket_policy" "audit" {
  bucket = aws_s3_bucket.audit.id
  policy = data.aws_iam_policy_document.audit_bucket.json
}
resource "aws_cloudtrail" "audit" {
  name                          = "${local.name}-audit"
  s3_bucket_name                = aws_s3_bucket.audit.id
  include_global_service_events = true
  is_multi_region_trail         = true
  enable_log_file_validation    = true
  kms_key_id                    = aws_kms_key.data.arn
  depends_on                    = [aws_s3_bucket_policy.audit]
}

resource "aws_budgets_budget" "monthly" {
  count        = local.budget_email == null ? 0 : 1
  name         = "${local.name}-monthly-cost"
  budget_type  = "COST"
  limit_amount = tostring(var.monthly_budget_usd)
  limit_unit   = "USD"
  time_unit    = "MONTHLY"
  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 50
    threshold_type             = "PERCENTAGE"
    notification_type          = "FORECASTED"
    subscriber_email_addresses = [local.budget_email]
  }
  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 80
    threshold_type             = "PERCENTAGE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = [local.budget_email]
  }
  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = [local.budget_email]
  }
}
