data "aws_iam_policy_document" "kms" {
  statement {
    sid       = "AccountAdministration"
    actions   = ["kms:*"]
    resources = ["*"]
    principals {
      type        = "AWS"
      identifiers = ["arn:${data.aws_partition.current.partition}:iam::${data.aws_caller_identity.current.account_id}:root"]
    }
  }
  statement {
    sid       = "AWSServiceEncryption"
    actions   = ["kms:Encrypt", "kms:Decrypt", "kms:ReEncrypt*", "kms:GenerateDataKey*", "kms:DescribeKey"]
    resources = ["*"]
    principals {
      type        = "Service"
      identifiers = ["logs.${var.aws_region}.amazonaws.com", "cloudtrail.amazonaws.com"]
    }
    condition {
      test     = "StringEquals"
      variable = "aws:SourceAccount"
      values   = [data.aws_caller_identity.current.account_id]
    }
  }
}
resource "aws_kms_key" "data" {
  description             = "Overnight Strategy data lake encryption"
  enable_key_rotation     = true
  deletion_window_in_days = 30
  policy                  = data.aws_iam_policy_document.kms.json
}
resource "aws_kms_alias" "data" {
  name          = "alias/${local.name}-data"
  target_key_id = aws_kms_key.data.key_id
}

resource "aws_s3_bucket" "lake" {
  bucket        = coalesce(var.data_lake_bucket_name, "${local.name}-lake-${data.aws_caller_identity.current.account_id}")
  force_destroy = false
  lifecycle {
    prevent_destroy = true
  }
}
resource "aws_s3_bucket_public_access_block" "lake" {
  bucket                  = aws_s3_bucket.lake.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}
resource "aws_s3_bucket_versioning" "lake" {
  bucket = aws_s3_bucket.lake.id
  versioning_configuration {
    status = "Enabled"
  }
}
resource "aws_s3_bucket_server_side_encryption_configuration" "lake" {
  bucket = aws_s3_bucket.lake.id
  rule {
    apply_server_side_encryption_by_default {
      kms_master_key_id = aws_kms_key.data.arn
      sse_algorithm     = "aws:kms"
    }
    bucket_key_enabled = true
  }
}
resource "aws_s3_bucket_lifecycle_configuration" "lake" {
  bucket     = aws_s3_bucket.lake.id
  depends_on = [aws_s3_bucket_versioning.lake]
  rule {
    id     = "raw-noncurrent"
    status = "Enabled"
    filter {
      prefix = "raw/"
    }
    noncurrent_version_expiration {
      noncurrent_days = var.raw_noncurrent_retention_days
    }
  }
  rule {
    id     = "quarantine"
    status = "Enabled"
    filter {
      prefix = "quarantine/"
    }
    expiration {
      days = var.quarantine_retention_days
    }
    noncurrent_version_expiration {
      noncurrent_days = 30
    }
  }
  rule {
    id     = "reports"
    status = "Enabled"
    filter {
      prefix = "reports/"
    }
    expiration {
      days = var.reports_retention_days
    }
    noncurrent_version_expiration {
      noncurrent_days = 30
    }
  }
  rule {
    id     = "athena-results"
    status = "Enabled"
    filter {
      prefix = "athena-results/"
    }
    expiration {
      days = 30
    }
    noncurrent_version_expiration {
      noncurrent_days = 7
    }
  }
}

data "aws_iam_policy_document" "lake" {
  statement {
    sid       = "DenyInsecureTransport"
    effect    = "Deny"
    actions   = ["s3:*"]
    resources = [aws_s3_bucket.lake.arn, "${aws_s3_bucket.lake.arn}/*"]
    principals {
      type        = "*"
      identifiers = ["*"]
    }
    condition {
      test     = "Bool"
      variable = "aws:SecureTransport"
      values   = ["false"]
    }
  }
  statement {
    sid       = "DenyPermanentSourceDeletion"
    effect    = "Deny"
    actions   = ["s3:DeleteObject", "s3:DeleteObjectVersion"]
    resources = ["${aws_s3_bucket.lake.arn}/landing/*", "${aws_s3_bucket.lake.arn}/raw/*"]
    principals {
      type        = "*"
      identifiers = ["*"]
    }
  }
}
resource "aws_s3_bucket_policy" "lake" {
  bucket = aws_s3_bucket.lake.id
  policy = data.aws_iam_policy_document.lake.json
}

resource "aws_s3_object" "zone_markers" {
  for_each               = local.zones
  bucket                 = aws_s3_bucket.lake.id
  key                    = "${each.value}/"
  content                = ""
  kms_key_id             = aws_kms_key.data.arn
  server_side_encryption = "aws:kms"
  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_s3_bucket" "audit" {
  bucket        = coalesce(var.audit_bucket_name, "${local.name}-audit-${data.aws_caller_identity.current.account_id}")
  force_destroy = false
  lifecycle {
    prevent_destroy = true
  }
}
resource "aws_s3_bucket_public_access_block" "audit" {
  bucket                  = aws_s3_bucket.audit.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}
resource "aws_s3_bucket_versioning" "audit" {
  bucket = aws_s3_bucket.audit.id
  versioning_configuration {
    status = "Enabled"
  }
}
resource "aws_s3_bucket_server_side_encryption_configuration" "audit" {
  bucket = aws_s3_bucket.audit.id
  rule {
    apply_server_side_encryption_by_default {
      kms_master_key_id = aws_kms_key.data.arn
      sse_algorithm     = "aws:kms"
    }
  }
}
