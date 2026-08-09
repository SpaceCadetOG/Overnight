resource "aws_iam_role" "step_functions" {
  name               = "${local.name}-workflow"
  assume_role_policy = jsonencode({ Version = "2012-10-17", Statement = [{ Effect = "Allow", Principal = { Service = "states.amazonaws.com" }, Action = "sts:AssumeRole" }] })
}
data "aws_iam_policy_document" "step_functions" {
  statement {
    actions   = ["ecs:RunTask"]
    resources = values(aws_ecs_task_definition.processing)[*].arn
  }
  statement {
    actions   = ["iam:PassRole"]
    resources = [aws_iam_role.task_execution.arn, aws_iam_role.processing.arn]
  }
  statement {
    actions   = ["events:PutTargets", "events:PutRule", "events:DescribeRule"]
    resources = ["arn:${data.aws_partition.current.partition}:events:${var.aws_region}:${data.aws_caller_identity.current.account_id}:rule/StepFunctionsGetEventsForECSTaskRule"]
  }
  statement {
    actions   = ["dynamodb:UpdateItem"]
    resources = [aws_dynamodb_table.packages.arn]
  }
  statement {
    actions   = ["logs:CreateLogDelivery", "logs:GetLogDelivery", "logs:UpdateLogDelivery", "logs:DeleteLogDelivery", "logs:ListLogDeliveries", "logs:PutResourcePolicy", "logs:DescribeResourcePolicies", "logs:DescribeLogGroups"]
    resources = ["*"]
  }
  statement {
    actions   = ["sns:Publish"]
    resources = [aws_sns_topic.operations.arn]
  }
}
resource "aws_iam_role_policy" "step_functions" {
  role   = aws_iam_role.step_functions.id
  policy = data.aws_iam_policy_document.step_functions.json
}
resource "aws_cloudwatch_log_group" "workflow" {
  name              = "/aws/states/${local.name}-processing"
  retention_in_days = var.log_retention_days
  kms_key_id        = aws_kms_key.data.arn
}
resource "aws_sfn_state_machine" "processing" {
  name       = "${local.name}-processing"
  role_arn   = aws_iam_role.step_functions.arn
  type       = "STANDARD"
  definition = templatefile("${path.module}/workflow.asl.json.tftpl", { cluster = aws_ecs_cluster.processing.arn, subnets = jsonencode(aws_subnet.private[*].id), security_groups = jsonencode([aws_security_group.tasks.id]), normalize = aws_ecs_task_definition.processing["normalize"].arn, reconstruct_l2 = aws_ecs_task_definition.processing["reconstruct_l2"].arn, reconstruct_trades = aws_ecs_task_definition.processing["reconstruct_trades"].arn, features = aws_ecs_task_definition.processing["features"].arn, forensics = aws_ecs_task_definition.processing["forensics"].arn, counterfactual = aws_ecs_task_definition.processing["counterfactual"].arn, data_quality = aws_ecs_task_definition.processing["data_quality"].arn, publish = aws_ecs_task_definition.processing["publish"].arn, shadow_inference = aws_ecs_task_definition.processing["shadow_inference"].arn, daily_report = aws_ecs_task_definition.processing["daily_report"].arn, registry = aws_dynamodb_table.packages.name, alerts = aws_sns_topic.operations.arn })
  logging_configuration {
    log_destination        = "${aws_cloudwatch_log_group.workflow.arn}:*"
    include_execution_data = true
    level                  = "ERROR"
  }
}
