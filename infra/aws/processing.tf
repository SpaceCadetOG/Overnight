resource "aws_vpc" "processing" {
  cidr_block           = "10.42.0.0/16"
  enable_dns_support   = true
  enable_dns_hostnames = true
}
resource "aws_subnet" "private" {
  count                   = 2
  vpc_id                  = aws_vpc.processing.id
  cidr_block              = cidrsubnet(aws_vpc.processing.cidr_block, 8, count.index)
  availability_zone       = data.aws_availability_zones.available.names[count.index]
  map_public_ip_on_launch = false
}
resource "aws_route_table" "private" {
  vpc_id = aws_vpc.processing.id
}
resource "aws_route_table_association" "private" {
  count          = 2
  subnet_id      = aws_subnet.private[count.index].id
  route_table_id = aws_route_table.private.id
}
resource "aws_security_group" "tasks" {
  name        = "${local.name}-tasks"
  vpc_id      = aws_vpc.processing.id
  description = "No inbound access; outbound TLS only"
  ingress {
    from_port = 443
    to_port   = 443
    protocol  = "tcp"
    self      = true
  }
  egress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
}
resource "aws_vpc_endpoint" "s3" {
  vpc_id            = aws_vpc.processing.id
  service_name      = "com.amazonaws.${var.aws_region}.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = [aws_route_table.private.id]
}
resource "aws_vpc_endpoint" "dynamodb" {
  vpc_id            = aws_vpc.processing.id
  service_name      = "com.amazonaws.${var.aws_region}.dynamodb"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = [aws_route_table.private.id]
}
locals {
  interface_endpoint_services = toset(["ecr.api", "ecr.dkr", "logs", "secretsmanager", "kms", "sts", "states"])
}
resource "aws_vpc_endpoint" "interfaces" {
  for_each            = var.enable_private_endpoints ? local.interface_endpoint_services : toset([])
  vpc_id              = aws_vpc.processing.id
  service_name        = "com.amazonaws.${var.aws_region}.${each.value}"
  vpc_endpoint_type   = "Interface"
  private_dns_enabled = true
  subnet_ids          = aws_subnet.private[*].id
  security_group_ids  = [aws_security_group.tasks.id]
}

resource "aws_ecr_repository" "processing" {
  for_each             = toset(keys(var.container_images))
  name                 = "${local.name}/${replace(each.value, "_", "-")}"
  image_tag_mutability = "IMMUTABLE"
  force_delete         = false
  encryption_configuration {
    encryption_type = "KMS"
    kms_key         = aws_kms_key.data.arn
  }
  image_scanning_configuration {
    scan_on_push = true
  }
}
resource "aws_ecr_lifecycle_policy" "processing" {
  for_each   = aws_ecr_repository.processing
  repository = each.value.name
  policy     = jsonencode({ rules = [{ rulePriority = 1, description = "Retain 20 images", selection = { tagStatus = "any", countType = "imageCountMoreThan", countNumber = 20 }, action = { type = "expire" } }] })
}
resource "aws_ecs_cluster" "processing" {
  name = "${local.name}-processing"
  setting {
    name  = "containerInsights"
    value = "enabled"
  }
}

resource "aws_iam_role" "task_execution" {
  name               = "${local.name}-task-execution"
  assume_role_policy = jsonencode({ Version = "2012-10-17", Statement = [{ Effect = "Allow", Principal = { Service = "ecs-tasks.amazonaws.com" }, Action = "sts:AssumeRole" }] })
}
resource "aws_iam_role_policy_attachment" "task_execution" {
  role       = aws_iam_role.task_execution.name
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}
resource "aws_iam_role" "processing" {
  name               = "${local.name}-processing"
  assume_role_policy = aws_iam_role.task_execution.assume_role_policy
}
data "aws_iam_policy_document" "processing" {
  statement {
    actions   = ["s3:GetObject", "s3:GetObjectVersion"]
    resources = ["${aws_s3_bucket.lake.arn}/raw/*", "${aws_s3_bucket.lake.arn}/normalized/*", "${aws_s3_bucket.lake.arn}/features/*", "${aws_s3_bucket.lake.arn}/forensics/*", "${aws_s3_bucket.lake.arn}/datasets/*"]
  }
  statement {
    actions   = ["s3:PutObject"]
    resources = ["${aws_s3_bucket.lake.arn}/normalized/*", "${aws_s3_bucket.lake.arn}/features/*", "${aws_s3_bucket.lake.arn}/forensics/*", "${aws_s3_bucket.lake.arn}/datasets/*", "${aws_s3_bucket.lake.arn}/reports/*"]
  }
  statement {
    actions   = ["dynamodb:GetItem", "dynamodb:UpdateItem"]
    resources = [aws_dynamodb_table.packages.arn]
  }
  statement {
    actions   = ["kms:Decrypt", "kms:Encrypt", "kms:GenerateDataKey"]
    resources = [aws_kms_key.data.arn]
  }
  statement {
    actions   = ["cloudwatch:PutMetricData"]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "cloudwatch:namespace"
      values   = ["OvernightStrategy/Processing", "OvernightStrategy/DataQuality"]
    }
  }
}
resource "aws_iam_role_policy" "processing" {
  role   = aws_iam_role.processing.id
  policy = data.aws_iam_policy_document.processing.json
}
resource "aws_secretsmanager_secret" "processing" {
  name                    = "${local.name}/processing-placeholder"
  description             = "Non-trading processing secrets only; exchange credentials are prohibited"
  kms_key_id              = aws_kms_key.data.arn
  recovery_window_in_days = 30
}

resource "aws_cloudwatch_log_group" "tasks" {
  for_each          = toset(keys(var.container_images))
  name              = "/overnight/${var.environment}/tasks/${each.value}"
  retention_in_days = var.log_retention_days
  kms_key_id        = aws_kms_key.data.arn
}
resource "aws_ecs_task_definition" "processing" {
  for_each                 = var.container_images
  family                   = "${local.name}-${replace(each.key, "_", "-")}"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = tostring(var.fargate_cpu)
  memory                   = tostring(var.fargate_memory)
  execution_role_arn       = aws_iam_role.task_execution.arn
  task_role_arn            = aws_iam_role.processing.arn
  container_definitions    = jsonencode([{ name = each.key, image = each.value, essential = true, command = ["sh", "-c", "echo processing-shell:${each.key}"], environment = [{ name = "SHADOW_ONLY", value = "true" }, { name = "DATA_LAKE_BUCKET", value = aws_s3_bucket.lake.id }], logConfiguration = { logDriver = "awslogs", options = { "awslogs-group" = aws_cloudwatch_log_group.tasks[each.key].name, "awslogs-region" = var.aws_region, "awslogs-stream-prefix" = "ecs" } } }])
}
