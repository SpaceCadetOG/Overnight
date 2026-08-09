variable "aws_region" {
  type        = string
  description = "Required AWS deployment region; no default by design."
}
variable "expected_account_id" {
  type        = string
  description = "Required 12-digit AWS account guardrail."
  validation {
    condition     = can(regex("^[0-9]{12}$", var.expected_account_id))
    error_message = "expected_account_id must be a 12-digit AWS account ID."
  }
}
variable "environment" {
  type = string
  validation {
    condition     = contains(["dev", "prod"], var.environment)
    error_message = "environment must be dev or prod"
  }
}
variable "project_name" {
  type    = string
  default = "overnight-forensics"
}
variable "github_repository" {
  type    = string
  default = "SpaceCadetOG/Overnight"
}
variable "data_lake_bucket_name" {
  type    = string
  default = null
}
variable "audit_bucket_name" {
  type    = string
  default = null
}
variable "budget_email" {
  type     = string
  default  = null
  nullable = true
}
variable "alert_email" {
  type     = string
  default  = null
  nullable = true
}
variable "monthly_budget_usd" {
  type    = number
  default = 5
}
variable "raw_noncurrent_retention_days" {
  type    = number
  default = 30
}
variable "quarantine_retention_days" {
  type    = number
  default = 90
}
variable "reports_retention_days" {
  type    = number
  default = 365
}
variable "queue_visibility_timeout_seconds" {
  type    = number
  default = 900
}
variable "queue_retention_seconds" {
  type    = number
  default = 1209600
}
variable "queue_max_receive_count" {
  type    = number
  default = 5
}
variable "lambda_zip_path" {
  type    = string
  default = "../../dist/awsvalidator.zip"
}
variable "lambda_source_hash" {
  type        = string
  default     = null
  description = "Base64 SHA-256 supplied by CI for the Lambda zip."
}
variable "lambda_memory_mb" {
  type    = number
  default = 512
}
variable "lambda_timeout_seconds" {
  type    = number
  default = 300
}
variable "lambda_reserved_concurrency" {
  type    = number
  default = 1
}
variable "log_retention_days" {
  type    = number
  default = 14
}
variable "fargate_cpu" {
  type    = number
  default = 512
}
variable "fargate_memory" {
  type    = number
  default = 1024
}
variable "enable_private_endpoints" {
  type        = bool
  default     = false
  description = "Interface endpoints incur hourly charges; explicitly enable before running private Fargate tasks."
}
variable "container_images" {
  type = map(string)
  default = {
    normalize          = "public.ecr.aws/docker/library/busybox:1.36"
    reconstruct_l2     = "public.ecr.aws/docker/library/busybox:1.36"
    reconstruct_trades = "public.ecr.aws/docker/library/busybox:1.36"
    features           = "public.ecr.aws/docker/library/busybox:1.36"
    forensics          = "public.ecr.aws/docker/library/busybox:1.36"
    counterfactual     = "public.ecr.aws/docker/library/busybox:1.36"
    data_quality       = "public.ecr.aws/docker/library/busybox:1.36"
    publish            = "public.ecr.aws/docker/library/busybox:1.36"
    shadow_inference   = "public.ecr.aws/docker/library/busybox:1.36"
    daily_report       = "public.ecr.aws/docker/library/busybox:1.36"

  }
}
variable "athena_bytes_scanned_cutoff" {
  type    = number
  default = 1073741824
}
variable "sagemaker_processing_instance_type" {
  type    = string
  default = "ml.m5.large"
}
variable "sagemaker_training_instance_type" {
  type    = string
  default = "ml.m5.large"
}
variable "enable_gpu" {
  type    = bool
  default = false
}
variable "enable_realtime_endpoint" {
  type    = bool
  default = false
  validation {
    condition     = var.enable_realtime_endpoint == false
    error_message = "Real-time inference is intentionally disabled for this foundation."
  }
}
variable "additional_tags" {
  type    = map(string)
  default = {}
}
