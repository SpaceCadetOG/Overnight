terraform {
  required_version = ">= 1.15.0"
  required_providers {
    aws = {
      source = "hashicorp/aws", version = "~> 6.0"
    }

  }
  backend "s3" {}
}

provider "aws" {
  region = var.aws_region
  default_tags {
    tags = local.required_tags
  }
}

data "aws_caller_identity" "current" {}
data "aws_partition" "current" {}
data "aws_availability_zones" "available" {
  state = "available"
}

check "correct_account" {
  assert {
    condition     = data.aws_caller_identity.current.account_id == var.expected_account_id
    error_message = "Authenticated AWS account does not match expected_account_id."
  }
}

locals {
  name = "${var.project_name}-${var.environment}"
  required_tags = merge(var.additional_tags, {
    Project     = var.project_name
    Environment = var.environment
    ManagedBy   = "Terraform"
    DataClass   = "TradeForensics"
    TradingUse  = "Prohibited"

  })
  zones        = toset(["landing", "raw", "quarantine", "normalized", "features", "forensics", "datasets", "models", "reports", "athena-results"])
  budget_email = var.budget_email != null && trimspace(var.budget_email) != "" ? trimspace(var.budget_email) : null
  alert_email  = var.alert_email != null && trimspace(var.alert_email) != "" ? trimspace(var.alert_email) : null
}
