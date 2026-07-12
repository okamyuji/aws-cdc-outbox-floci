# AWS DMS。Auroraのbinlogからoutboxテーブルの変更のみを読み取り、Kinesisへ流す
# ローカル環境ではGo製リレー(cmd/relay)がこの役割を担う

resource "aws_dms_replication_subnet_group" "main" {
  replication_subnet_group_id          = "${var.name_prefix}-dms-subnet"
  replication_subnet_group_description = "DMS subnet group"
  subnet_ids                           = [aws_subnet.private_a.id, aws_subnet.private_c.id]
}

resource "aws_dms_replication_instance" "main" {
  replication_instance_id     = "${var.name_prefix}-dms"
  replication_instance_class  = "dms.t3.micro"
  allocated_storage           = 20
  replication_subnet_group_id = aws_dms_replication_subnet_group.main.id
  vpc_security_group_ids      = [aws_security_group.dms.id]
  publicly_accessible         = false
  multi_az                    = false
}

resource "aws_dms_endpoint" "source" {
  endpoint_id   = "${var.name_prefix}-source-aurora"
  endpoint_type = "source"
  engine_name   = "aurora"
  server_name   = aws_rds_cluster.source.endpoint
  port          = 3306
  database_name = "source_orders"
  username      = var.db_master_username
  password      = var.db_master_password
  ssl_mode      = "require"
}

# DMSがKinesisへ書き込むためのIAMロール
data "aws_iam_policy_document" "dms_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["dms.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "dms_kinesis" {
  name               = "${var.name_prefix}-dms-kinesis-role"
  assume_role_policy = data.aws_iam_policy_document.dms_assume.json
}

data "aws_iam_policy_document" "dms_kinesis" {
  statement {
    actions = [
      "kinesis:PutRecord",
      "kinesis:PutRecords",
      "kinesis:DescribeStream",
    ]
    resources = [module.pipeline.kinesis_stream_arn]
  }
}

resource "aws_iam_role_policy" "dms_kinesis" {
  name   = "${var.name_prefix}-dms-kinesis-policy"
  role   = aws_iam_role.dms_kinesis.id
  policy = data.aws_iam_policy_document.dms_kinesis.json
}

resource "aws_dms_endpoint" "kinesis_target" {
  endpoint_id   = "${var.name_prefix}-target-kinesis"
  endpoint_type = "target"
  engine_name   = "kinesis"

  kinesis_settings {
    stream_arn                     = module.pipeline.kinesis_stream_arn
    message_format                 = "json"
    service_access_role_arn        = aws_iam_role.dms_kinesis.arn
    include_partition_value        = true
    partition_include_schema_table = false
  }
}

# outboxテーブルのINSERTのみをCDC対象にする
resource "aws_dms_replication_task" "outbox_cdc" {
  replication_task_id      = "${var.name_prefix}-outbox-cdc"
  replication_instance_arn = aws_dms_replication_instance.main.replication_instance_arn
  source_endpoint_arn      = aws_dms_endpoint.source.endpoint_arn
  target_endpoint_arn      = aws_dms_endpoint.kinesis_target.endpoint_arn
  migration_type           = "cdc"

  table_mappings = jsonencode({
    rules = [
      {
        rule-type = "selection"
        rule-id   = "1"
        rule-name = "select-outbox"
        object-locator = {
          schema-name = "source_orders"
          table-name  = "outbox"
        }
        rule-action = "include"
      }
    ]
  })

  replication_task_settings = jsonencode({
    TargetMetadata = {
      TargetSchema = ""
    }
    Logging = {
      EnableLogging = true
    }
  })
}
