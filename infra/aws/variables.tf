variable "aws_region" {
  type    = string
  default = "us-east-2"
}
variable "environment" {
  type    = string
  default = "prod"
}
variable "github_repository" {
  type    = string
  default = "SpaceCadetOG/Overnight"
}
variable "budget_email" {
  type = string
}
variable "monthly_budget_usd" {
  type    = number
  default = 5
}
variable "lambda_zip_path" {
  type    = string
  default = "../../dist/awsvalidator.zip"
}
