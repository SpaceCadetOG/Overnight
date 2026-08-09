output "raw_bucket" {
  value = aws_s3_bucket.raw.id
}
output "validation_bucket" {
  value = aws_s3_bucket.validation.id
}
output "quarantine_bucket" {
  value = aws_s3_bucket.quarantine.id
}
output "jumppi_iam_user" {
  value = aws_iam_user.jumppi.name
}
output "github_actions_role_arn" {
  value = aws_iam_role.github.arn
}
output "validation_queue_url" {
  value = aws_sqs_queue.uploads.id
}
