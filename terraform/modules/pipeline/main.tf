# CDCイベント配送パイプライン
# Kinesis(CDCイベント) -> fanout Lambda -> SQS FIFO -> delivery Lambda -> ターゲットAPI

resource "aws_kinesis_stream" "cdc" {
  name             = "${var.name_prefix}-cdc-stream"
  shard_count      = var.kinesis_shard_count
  retention_period = 24
}

# 配送用FIFOキュー。コンテンツベース重複排除は使わず、明示的なDeduplicationIdで制御する
resource "aws_sqs_queue" "delivery_dlq" {
  name       = "${var.name_prefix}-delivery-dlq.fifo"
  fifo_queue = true
}

resource "aws_sqs_queue" "delivery" {
  name                       = "${var.name_prefix}-delivery.fifo"
  fifo_queue                 = true
  visibility_timeout_seconds = 60
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.delivery_dlq.arn
    maxReceiveCount     = 5
  })
}

# fanoutが再試行上限を超えたレコードのメタデータ退避先
resource "aws_sqs_queue" "fanout_dlq" {
  name = "${var.name_prefix}-fanout-dlq"
}

data "aws_iam_policy_document" "lambda_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "fanout" {
  name               = "${var.name_prefix}-fanout-role"
  assume_role_policy = data.aws_iam_policy_document.lambda_assume.json
}

data "aws_iam_policy_document" "fanout" {
  statement {
    actions = [
      "kinesis:GetRecords",
      "kinesis:GetShardIterator",
      "kinesis:DescribeStream",
      "kinesis:DescribeStreamSummary",
      "kinesis:ListShards",
      "kinesis:ListStreams",
    ]
    resources = [aws_kinesis_stream.cdc.arn]
  }
  statement {
    actions   = ["sqs:SendMessage"]
    resources = [aws_sqs_queue.delivery.arn, aws_sqs_queue.fanout_dlq.arn]
  }
  statement {
    actions   = ["logs:CreateLogGroup", "logs:CreateLogStream", "logs:PutLogEvents"]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "fanout" {
  name   = "${var.name_prefix}-fanout-policy"
  role   = aws_iam_role.fanout.id
  policy = data.aws_iam_policy_document.fanout.json
}

resource "aws_iam_role" "delivery" {
  name               = "${var.name_prefix}-delivery-role"
  assume_role_policy = data.aws_iam_policy_document.lambda_assume.json
}

data "aws_iam_policy_document" "delivery" {
  statement {
    actions = [
      "sqs:ReceiveMessage",
      "sqs:DeleteMessage",
      "sqs:GetQueueAttributes",
    ]
    resources = [aws_sqs_queue.delivery.arn]
  }
  statement {
    actions   = ["logs:CreateLogGroup", "logs:CreateLogStream", "logs:PutLogEvents"]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "delivery" {
  name   = "${var.name_prefix}-delivery-policy"
  role   = aws_iam_role.delivery.id
  policy = data.aws_iam_policy_document.delivery.json
}

resource "aws_lambda_function" "fanout" {
  function_name    = "${var.name_prefix}-fanout"
  role             = aws_iam_role.fanout.arn
  runtime          = "provided.al2023"
  handler          = "bootstrap"
  architectures    = ["arm64"]
  filename         = var.fanout_zip_path
  source_code_hash = filebase64sha256(var.fanout_zip_path)
  timeout          = 30

  environment {
    variables = {
      DELIVERY_QUEUE_URL = aws_sqs_queue.delivery.url
    }
  }
}

resource "aws_lambda_function" "delivery" {
  function_name    = "${var.name_prefix}-delivery"
  role             = aws_iam_role.delivery.arn
  runtime          = "provided.al2023"
  handler          = "bootstrap"
  architectures    = ["arm64"]
  filename         = var.delivery_zip_path
  source_code_hash = filebase64sha256(var.delivery_zip_path)
  timeout          = 30

  environment {
    variables = {
      TARGET_API_URL   = var.target_api_url
      TARGET_API_TOKEN = var.target_api_token
    }
  }
}

# Kinesis -> fanout。部分バッチ応答で失敗レコード以降を再試行する。
# 解析不能な毒レコードでシャード全体が止まり続けないよう、再試行上限と
# 上限超過時の退避先（fanout_dlq）を設定する
resource "aws_lambda_event_source_mapping" "kinesis_to_fanout" {
  event_source_arn        = aws_kinesis_stream.cdc.arn
  function_name           = aws_lambda_function.fanout.arn
  starting_position       = "LATEST"
  batch_size              = 100
  function_response_types = ["ReportBatchItemFailures"]
  maximum_retry_attempts  = 10

  destination_config {
    on_failure {
      destination_arn = aws_sqs_queue.fanout_dlq.arn
    }
  }
}

# SQS FIFO -> delivery。部分バッチ応答で失敗メッセージのみ再配信する
resource "aws_lambda_event_source_mapping" "sqs_to_delivery" {
  event_source_arn        = aws_sqs_queue.delivery.arn
  function_name           = aws_lambda_function.delivery.arn
  batch_size              = 10
  function_response_types = ["ReportBatchItemFailures"]
}
