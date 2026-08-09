locals {
  glue_databases = toset(["raw_metadata", "normalized", "features", "forensics", "research", "ml", "monitoring"])
}
resource "aws_glue_catalog_database" "databases" {
  for_each    = local.glue_databases
  name        = "${replace(local.name, "-", "_")}_${each.value}"
  description = "Versioned ${each.value} contracts; production tables are declared, not crawler-inferred"
}
resource "aws_athena_workgroup" "research" {
  name          = "${local.name}-research"
  force_destroy = false
  configuration {
    enforce_workgroup_configuration    = true
    publish_cloudwatch_metrics_enabled = true
    bytes_scanned_cutoff_per_query     = var.athena_bytes_scanned_cutoff
    result_configuration {
      output_location = "s3://${aws_s3_bucket.lake.id}/athena-results/"
      encryption_configuration {
        encryption_option = "SSE_KMS"
        kms_key_arn       = aws_kms_key.data.arn
      }
    }

  }
}

resource "aws_iam_role" "sagemaker" {
  name               = "${local.name}-sagemaker-shadow"
  assume_role_policy = jsonencode({ Version = "2012-10-17", Statement = [{ Effect = "Allow", Principal = { Service = "sagemaker.amazonaws.com" }, Action = "sts:AssumeRole" }] })
}
data "aws_iam_policy_document" "sagemaker" {
  statement {
    actions   = ["s3:GetObject", "s3:ListBucket"]
    resources = [aws_s3_bucket.lake.arn, "${aws_s3_bucket.lake.arn}/datasets/*", "${aws_s3_bucket.lake.arn}/features/*"]
  }
  statement {
    actions   = ["s3:PutObject"]
    resources = ["${aws_s3_bucket.lake.arn}/models/*", "${aws_s3_bucket.lake.arn}/reports/shadow/*", "${aws_s3_bucket.lake.arn}/datasets/ml/*"]
  }
  statement {
    actions   = ["kms:Decrypt", "kms:Encrypt", "kms:GenerateDataKey"]
    resources = [aws_kms_key.data.arn]
  }
  statement {
    actions   = ["ecr:GetAuthorizationToken"]
    resources = ["*"]
  }
  statement {
    actions   = ["ecr:BatchGetImage", "ecr:GetDownloadUrlForLayer", "ecr:BatchCheckLayerAvailability"]
    resources = values(aws_ecr_repository.processing)[*].arn
  }
  statement {
    actions   = ["logs:CreateLogStream", "logs:PutLogEvents"]
    resources = ["${aws_cloudwatch_log_group.sagemaker.arn}:*"]
  }
  statement {
    actions   = ["cloudwatch:PutMetricData"]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "cloudwatch:namespace"
      values   = ["OvernightStrategy/ML"]
    }
  }
}
resource "aws_iam_role_policy" "sagemaker" {
  role   = aws_iam_role.sagemaker.id
  policy = data.aws_iam_policy_document.sagemaker.json
}
resource "aws_cloudwatch_log_group" "sagemaker" {
  name              = "/overnight/${var.environment}/sagemaker"
  retention_in_days = var.log_retention_days
  kms_key_id        = aws_kms_key.data.arn
}
resource "aws_sagemaker_model_package_group" "shadow" {
  model_package_group_name        = "${local.name}-shadow-models"
  model_package_group_description = "Manual approval only; every package is shadow-only"
  tags                            = { shadow_only = "true", approval = "manual" }
}

resource "aws_iam_role" "reporting" {
  name               = "${local.name}-reporting"
  assume_role_policy = jsonencode({ Version = "2012-10-17", Statement = [{ Effect = "Allow", Principal = { Service = ["quicksight.amazonaws.com", "athena.amazonaws.com"] }, Action = "sts:AssumeRole" }] })
}
data "aws_iam_policy_document" "reporting" {
  statement {
    actions   = ["athena:GetQueryExecution", "athena:GetQueryResults", "athena:StartQueryExecution", "athena:StopQueryExecution"]
    resources = [aws_athena_workgroup.research.arn]
  }
  statement {
    actions   = ["glue:GetDatabase", "glue:GetDatabases", "glue:GetTable", "glue:GetTables", "glue:GetPartitions"]
    resources = ["*"]
  }
  statement {
    actions   = ["s3:GetObject", "s3:ListBucket", "s3:PutObject"]
    resources = [aws_s3_bucket.lake.arn, "${aws_s3_bucket.lake.arn}/normalized/*", "${aws_s3_bucket.lake.arn}/features/*", "${aws_s3_bucket.lake.arn}/forensics/*", "${aws_s3_bucket.lake.arn}/reports/*", "${aws_s3_bucket.lake.arn}/athena-results/*"]
  }
  statement {
    actions   = ["kms:Decrypt", "kms:Encrypt", "kms:GenerateDataKey"]
    resources = [aws_kms_key.data.arn]
  }
}
resource "aws_iam_role_policy" "reporting" {
  role   = aws_iam_role.reporting.id
  policy = data.aws_iam_policy_document.reporting.json
}
