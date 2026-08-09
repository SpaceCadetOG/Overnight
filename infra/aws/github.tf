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
resource "aws_iam_role" "github_deployment" {
  name                 = "${local.name}-github-deployment"
  assume_role_policy   = data.aws_iam_policy_document.github_assume.json
  max_session_duration = 3600
}
data "aws_iam_policy_document" "github_deployment" {
  statement {
    actions   = ["s3:*", "sqs:*", "lambda:*", "logs:*", "cloudwatch:*", "sns:*", "events:*", "states:*", "ecs:*", "ecr:*", "dynamodb:*", "glue:*", "athena:*", "sagemaker:*", "secretsmanager:*", "kms:*", "ec2:*", "cloudtrail:*", "budgets:*"]
    resources = ["*"]
  }
  statement {
    actions   = ["iam:Get*", "iam:List*", "iam:CreateRole", "iam:DeleteRole", "iam:PutRolePolicy", "iam:DeleteRolePolicy", "iam:AttachRolePolicy", "iam:DetachRolePolicy", "iam:PassRole", "iam:CreateOpenIDConnectProvider", "iam:DeleteOpenIDConnectProvider", "iam:CreateUser", "iam:DeleteUser", "iam:PutUserPolicy", "iam:DeleteUserPolicy", "iam:TagRole", "iam:TagUser"]
    resources = ["arn:${data.aws_partition.current.partition}:iam::${data.aws_caller_identity.current.account_id}:role/${local.name}-*", "arn:${data.aws_partition.current.partition}:iam::${data.aws_caller_identity.current.account_id}:user/${local.name}-*", "arn:${data.aws_partition.current.partition}:iam::${data.aws_caller_identity.current.account_id}:oidc-provider/token.actions.githubusercontent.com"]
  }
}
resource "aws_iam_role_policy" "github_deployment" {
  role   = aws_iam_role.github_deployment.id
  policy = data.aws_iam_policy_document.github_deployment.json
}
