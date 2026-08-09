resource "aws_dynamodb_table" "packages" {
  name                        = "${local.name}-package-registry"
  billing_mode                = "PAY_PER_REQUEST"
  hash_key                    = "package_id"
  deletion_protection_enabled = true
  attribute {
    name = "package_id"
    type = "S"
  }
  point_in_time_recovery {
    enabled = true
  }
  server_side_encryption {
    enabled     = true
    kms_key_arn = aws_kms_key.data.arn
  }
  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_sqs_queue" "validation_dlq" {
  name                      = "${local.name}-validation-dlq"
  message_retention_seconds = var.queue_retention_seconds
  kms_master_key_id         = aws_kms_key.data.arn
}
resource "aws_sqs_queue" "validation" {
  name                       = "${local.name}-validation"
  visibility_timeout_seconds = var.queue_visibility_timeout_seconds
  message_retention_seconds  = var.queue_retention_seconds
  kms_master_key_id          = aws_kms_key.data.arn
  redrive_policy             = jsonencode({ deadLetterTargetArn = aws_sqs_queue.validation_dlq.arn, maxReceiveCount = var.queue_max_receive_count })
}
data "aws_iam_policy_document" "s3_validation_queue" {
  statement {
    actions   = ["sqs:SendMessage"]
    resources = [aws_sqs_queue.validation.arn]
    principals {
      type        = "Service"
      identifiers = ["s3.amazonaws.com"]
    }
    condition {
      test     = "ArnEquals"
      variable = "aws:SourceArn"
      values   = [aws_s3_bucket.lake.arn]
    }
    condition {
      test     = "StringEquals"
      variable = "aws:SourceAccount"
      values   = [data.aws_caller_identity.current.account_id]
    }
  }
}
resource "aws_sqs_queue_policy" "validation" {
  queue_url = aws_sqs_queue.validation.id
  policy    = data.aws_iam_policy_document.s3_validation_queue.json
}
resource "aws_s3_bucket_notification" "landing_manifest" {
  bucket     = aws_s3_bucket.lake.id
  depends_on = [aws_sqs_queue_policy.validation]
  queue {
    queue_arn     = aws_sqs_queue.validation.arn
    events        = ["s3:ObjectCreated:*"]
    filter_prefix = "landing/"
    filter_suffix = "MANIFEST.json"
  }
}

resource "aws_iam_role" "validator" {
  name               = "${local.name}-validator"
  assume_role_policy = jsonencode({ Version = "2012-10-17", Statement = [{ Effect = "Allow", Principal = { Service = "lambda.amazonaws.com" }, Action = "sts:AssumeRole" }] })
}
data "aws_iam_policy_document" "validator" {
  statement {
    actions   = ["s3:GetObject", "s3:GetObjectVersion", "s3:ListBucketVersions"]
    resources = [aws_s3_bucket.lake.arn, "${aws_s3_bucket.lake.arn}/landing/*"]
  }
  statement {
    actions   = ["s3:PutObject"]
    resources = ["${aws_s3_bucket.lake.arn}/landing/acknowledgements/*"]
  }
  statement {
    actions   = ["s3:GetObject", "s3:GetObjectVersion", "s3:PutObject"]
    resources = ["${aws_s3_bucket.lake.arn}/raw/*", "${aws_s3_bucket.lake.arn}/quarantine/*"]
  }
  statement {
    actions   = ["dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:UpdateItem", "dynamodb:ConditionCheckItem"]
    resources = [aws_dynamodb_table.packages.arn]
  }
  statement {
    actions   = ["sqs:ReceiveMessage", "sqs:DeleteMessage", "sqs:GetQueueAttributes"]
    resources = [aws_sqs_queue.validation.arn]
  }
  statement {
    actions   = ["states:StartExecution"]
    resources = [aws_sfn_state_machine.processing.arn]
  }
  statement {
    actions   = ["kms:Decrypt", "kms:Encrypt", "kms:GenerateDataKey"]
    resources = [aws_kms_key.data.arn]
  }
  statement {
    actions   = ["logs:CreateLogStream", "logs:PutLogEvents"]
    resources = ["${aws_cloudwatch_log_group.validator.arn}:*"]
  }
  statement {
    actions   = ["cloudwatch:PutMetricData"]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "cloudwatch:namespace"
      values   = ["OvernightStrategy/DataQuality"]
    }
  }
}
resource "aws_iam_role_policy" "validator" {
  role   = aws_iam_role.validator.id
  policy = data.aws_iam_policy_document.validator.json
}
resource "aws_cloudwatch_log_group" "validator" {
  name              = "/aws/lambda/${local.name}-package-validator"
  retention_in_days = var.log_retention_days
  kms_key_id        = aws_kms_key.data.arn
}
resource "aws_lambda_function" "validator" {
  function_name                  = "${local.name}-package-validator"
  role                           = aws_iam_role.validator.arn
  runtime                        = "provided.al2023"
  handler                        = "bootstrap"
  architectures                  = ["arm64"]
  filename                       = var.lambda_zip_path
  source_code_hash               = var.lambda_source_hash
  timeout                        = var.lambda_timeout_seconds
  memory_size                    = var.lambda_memory_mb
  reserved_concurrent_executions = var.lambda_reserved_concurrency
  environment {
    variables = { DATA_LAKE_BUCKET = aws_s3_bucket.lake.id, PACKAGE_REGISTRY_TABLE = aws_dynamodb_table.packages.name, STATE_MACHINE_ARN = aws_sfn_state_machine.processing.arn, LANDING_PREFIX = "landing/", RAW_PREFIX = "raw/", QUARANTINE_PREFIX = "quarantine/" }
  }
  depends_on = [aws_cloudwatch_log_group.validator]
}
resource "aws_lambda_event_source_mapping" "validation" {
  event_source_arn        = aws_sqs_queue.validation.arn
  function_name           = aws_lambda_function.validator.arn
  batch_size              = 1
  function_response_types = ["ReportBatchItemFailures"]
}

resource "aws_iam_user" "jumppi_bootstrap" {
  name          = "${local.name}-jumppi-bootstrap"
  force_destroy = false
}
data "aws_iam_policy_document" "jumppi" {
  statement {
    actions   = ["s3:PutObject", "s3:GetObject", "s3:GetObjectVersion", "s3:GetObjectAttributes"]
    resources = ["${aws_s3_bucket.lake.arn}/landing/*"]
  }
  statement {
    actions   = ["s3:ListBucket", "s3:ListBucketVersions"]
    resources = [aws_s3_bucket.lake.arn]
    condition {
      test     = "StringLike"
      variable = "s3:prefix"
      values   = ["landing/*"]
    }
  }
  statement {
    actions   = ["kms:Encrypt", "kms:Decrypt", "kms:GenerateDataKey"]
    resources = [aws_kms_key.data.arn]
  }
  statement {
    actions   = ["cloudwatch:PutMetricData"]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "cloudwatch:namespace"
      values   = ["OvernightStrategy/JumpPi"]
    }
  }
}
resource "aws_iam_user_policy" "jumppi" {
  user   = aws_iam_user.jumppi_bootstrap.name
  policy = data.aws_iam_policy_document.jumppi.json
}
