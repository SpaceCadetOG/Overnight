terraform {
  required_version = ">= 1.8.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }

  }
  backend "s3" {}
}

provider "aws" {
  region = var.aws_region
}
data "aws_caller_identity" "current" {}
data "aws_region" "current" {}
locals {
  name = "trade-forensics-${var.environment}"
}

resource "aws_s3_bucket" "raw" {
  bucket        = "${local.name}-raw-${data.aws_caller_identity.current.account_id}"
  force_destroy = false
}
resource "aws_s3_bucket" "validation" {
  bucket        = "${local.name}-validation-${data.aws_caller_identity.current.account_id}"
  force_destroy = false
}
resource "aws_s3_bucket" "quarantine" {
  bucket        = "${local.name}-quarantine-${data.aws_caller_identity.current.account_id}"
  force_destroy = false
}

resource "aws_s3_bucket_public_access_block" "all" {
  for_each = {
    raw = aws_s3_bucket.raw.id, validation = aws_s3_bucket.validation.id, quarantine = aws_s3_bucket.quarantine.id
  }
  bucket                  = each.value
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}
resource "aws_s3_bucket_versioning" "all" {
  for_each = {
    raw = aws_s3_bucket.raw.id, validation = aws_s3_bucket.validation.id, quarantine = aws_s3_bucket.quarantine.id
  }
  bucket = each.value
  versioning_configuration {
    status = "Enabled"
  }
}
resource "aws_s3_bucket_server_side_encryption_configuration" "all" {
  for_each = {
    raw = aws_s3_bucket.raw.id, validation = aws_s3_bucket.validation.id, quarantine = aws_s3_bucket.quarantine.id
  }
  bucket = each.value
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
    bucket_key_enabled = true
  }
}
resource "aws_s3_bucket_lifecycle_configuration" "raw" {
  bucket     = aws_s3_bucket.raw.id
  depends_on = [aws_s3_bucket_versioning.all]
  rule {
    id     = "remove-old-noncurrent-versions"
    status = "Enabled"
    filter {}
    noncurrent_version_expiration {
      noncurrent_days = 30
    }
  }
}
resource "aws_s3_bucket_lifecycle_configuration" "quarantine" {
  bucket     = aws_s3_bucket.quarantine.id
  depends_on = [aws_s3_bucket_versioning.all]
  rule {
    id     = "expire-quarantine-markers"
    status = "Enabled"
    filter {}
    expiration {
      days = 90
    }
    noncurrent_version_expiration {
      noncurrent_days = 30
    }
  }
}

resource "aws_sqs_queue" "dead_letter" {
  name                      = "${local.name}-validation-dlq"
  message_retention_seconds = 1209600
  sqs_managed_sse_enabled   = true
}
resource "aws_sqs_queue" "uploads" {
  name                       = "${local.name}-uploads"
  visibility_timeout_seconds = 900
  message_retention_seconds  = 1209600
  sqs_managed_sse_enabled    = true
  redrive_policy             = jsonencode({ deadLetterTargetArn = aws_sqs_queue.dead_letter.arn, maxReceiveCount = 5 })
}
data "aws_iam_policy_document" "s3_to_sqs" {
  statement {
    actions   = ["sqs:SendMessage"]
    resources = [aws_sqs_queue.uploads.arn]
    principals {
      type        = "Service"
      identifiers = ["s3.amazonaws.com"]
    }
    condition {
      test     = "ArnEquals"
      variable = "aws:SourceArn"
      values   = [aws_s3_bucket.raw.arn]
    }
  }
}
resource "aws_sqs_queue_policy" "uploads" {
  queue_url = aws_sqs_queue.uploads.id
  policy    = data.aws_iam_policy_document.s3_to_sqs.json
}
resource "aws_s3_bucket_notification" "raw" {
  bucket     = aws_s3_bucket.raw.id
  depends_on = [aws_sqs_queue_policy.uploads]
  queue {
    queue_arn     = aws_sqs_queue.uploads.arn
    events        = ["s3:ObjectCreated:*"]
    filter_suffix = "MANIFEST.json"
  }
}

resource "aws_iam_role" "validator" {
  name               = "${local.name}-validator"
  assume_role_policy = jsonencode({ Version = "2012-10-17", Statement = [{ Effect = "Allow", Principal = { Service = "lambda.amazonaws.com" }, Action = "sts:AssumeRole" }] })
}
data "aws_iam_policy_document" "validator" {
  statement {
    actions   = ["s3:GetObject"]
    resources = ["${aws_s3_bucket.raw.arn}/*"]
  }
  statement {
    actions   = ["s3:PutObject"]
    resources = ["${aws_s3_bucket.validation.arn}/validation/*", "${aws_s3_bucket.quarantine.arn}/quarantine/*"]
  }
  statement {
    actions   = ["sqs:ReceiveMessage", "sqs:DeleteMessage", "sqs:GetQueueAttributes"]
    resources = [aws_sqs_queue.uploads.arn]
  }
  statement {
    actions   = ["logs:CreateLogGroup", "logs:CreateLogStream", "logs:PutLogEvents"]
    resources = ["arn:aws:logs:${var.aws_region}:${data.aws_caller_identity.current.account_id}:*"]
  }
}
resource "aws_iam_role_policy" "validator" {
  role   = aws_iam_role.validator.id
  policy = data.aws_iam_policy_document.validator.json
}
resource "aws_lambda_function" "validator" {
  function_name                  = "${local.name}-package-validator"
  role                           = aws_iam_role.validator.arn
  runtime                        = "provided.al2023"
  handler                        = "bootstrap"
  filename                       = var.lambda_zip_path
  source_code_hash               = filebase64sha256(var.lambda_zip_path)
  timeout                        = 900
  memory_size                    = 1024
  architectures                  = ["arm64"]
  reserved_concurrent_executions = 1
  environment {
    variables = { VALIDATION_BUCKET = aws_s3_bucket.validation.id, QUARANTINE_BUCKET = aws_s3_bucket.quarantine.id }
  }
}
resource "aws_lambda_event_source_mapping" "uploads" {
  event_source_arn = aws_sqs_queue.uploads.arn
  function_name    = aws_lambda_function.validator.arn
  batch_size       = 1
}
resource "aws_cloudwatch_log_group" "validator" {
  name              = "/aws/lambda/${aws_lambda_function.validator.function_name}"
  retention_in_days = 14
}

resource "aws_iam_user" "jumppi" {
  name = "${local.name}-jumppi-uploader"
}
data "aws_iam_policy_document" "jumppi" {
  statement {
    actions   = ["s3:PutObject"]
    resources = ["${aws_s3_bucket.raw.arn}/raw/lighter/*"]
  }
  statement {
    actions   = ["s3:GetObject"]
    resources = ["${aws_s3_bucket.validation.arn}/validation/*"]
  }
  statement {
    actions   = ["s3:ListBucket"]
    resources = [aws_s3_bucket.raw.arn, aws_s3_bucket.validation.arn]
    condition {
      test     = "StringLike"
      variable = "s3:prefix"
      values   = ["raw/lighter/*", "validation/*"]
    }
  }
}
resource "aws_iam_user_policy" "jumppi" {
  user   = aws_iam_user.jumppi.name
  policy = data.aws_iam_policy_document.jumppi.json
}

resource "aws_budgets_budget" "monthly" {
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
    subscriber_email_addresses = [var.budget_email]
  }
  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 80
    threshold_type             = "PERCENTAGE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = [var.budget_email]
  }
  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = [var.budget_email]
  }
}

resource "aws_iam_openid_connect_provider" "github" {
  url             = "https://token.actions.githubusercontent.com"
  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = ["6938fd4d98bab03faadb97b34396831e3780aea1"]
}
data "aws_iam_policy_document" "github_assume" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.github.arn]
    }
    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }
    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:sub"
      values   = ["repo:${var.github_repository}:ref:refs/heads/main"]
    }
  }
}
resource "aws_iam_role" "github" {
  name               = "${local.name}-github-terraform"
  assume_role_policy = data.aws_iam_policy_document.github_assume.json
}
data "aws_iam_policy_document" "github" {
  statement {
    actions   = ["s3:*", "sqs:*", "lambda:*", "logs:*", "budgets:*", "iam:Get*", "iam:List*", "iam:CreateRole", "iam:DeleteRole", "iam:PutRolePolicy", "iam:DeleteRolePolicy", "iam:PassRole", "iam:CreateOpenIDConnectProvider", "iam:DeleteOpenIDConnectProvider", "iam:CreateUser", "iam:DeleteUser", "iam:PutUserPolicy", "iam:DeleteUserPolicy"]
    resources = ["*"]
  }
}
resource "aws_iam_role_policy" "github" {
  role   = aws_iam_role.github.id
  policy = data.aws_iam_policy_document.github.json
}
