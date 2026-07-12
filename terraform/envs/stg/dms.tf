# AWS DMS。Auroraのbinlogからoutboxテーブルの変更のみを読み取り、Kinesisへ流す
# ローカル環境ではGo製リレー(cmd/relay)がこの役割を担う

# DMSがVPCリソースを操作するためのアカウント共通ロール。
# この固定名(dms-vpc-role)が存在しないと、サブネットグループ作成が
# 「The IAM Role ... is not configured properly」で失敗する
resource "aws_iam_role" "dms_vpc_role" {
  name = "dms-vpc-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "dms.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy_attachment" "dms_vpc_role" {
  role       = aws_iam_role.dms_vpc_role.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonDMSVPCManagementRole"
}

resource "aws_dms_replication_subnet_group" "main" {
  # IAM反映待ちを含め、ロール整備後に作成する
  depends_on = [aws_iam_role_policy_attachment.dms_vpc_role]

  replication_subnet_group_id          = "${var.name_prefix}-dms-subnet"
  replication_subnet_group_description = "DMS subnet group"
  subnet_ids                           = [aws_subnet.private_a.id, aws_subnet.private_c.id]
}

resource "aws_dms_replication_instance" "main" {
  replication_instance_id = "${var.name_prefix}-dms"
  # dms.t3.microは提供終了(describe-orderable-replication-instancesに無い)。最小はt3.small
  replication_instance_class  = "dms.t3.small"
  allocated_storage           = 20
  replication_subnet_group_id = aws_dms_replication_subnet_group.main.id
  vpc_security_group_ids      = [aws_security_group.dms.id]
  publicly_accessible         = true # 検証用。NATを置かない構成でKinesisエンドポイントへ到達させる
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
  # auroraエンジンのDMSエンドポイントはssl_mode=requireを受け付けない
  # (InvalidParameterCombinationException)。検証では既定のnoneを使い、
  # 暗号化が必要な場合はverify-ca(CA証明書のインポートが必要)を使う
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

  # selectionでoutboxのみを対象にし、object-mappingでKinesisのパーティションキーを
  # 集約ID(aggregate_id)にする。既定のスキーマ.テーブル固定だと複数シャード時に
  # 集約単位の順序分散にならない
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
      },
      {
        rule-type   = "object-mapping"
        rule-id     = "2"
        rule-name   = "outbox-partition-by-aggregate"
        rule-action = "map-record-to-record"
        object-locator = {
          schema-name = "source_orders"
          table-name  = "outbox"
        }
        mapping-parameters = {
          partition-key-type           = "attribute-name"
          partition-key-attribute-name = "aggregate_id"
        }
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
