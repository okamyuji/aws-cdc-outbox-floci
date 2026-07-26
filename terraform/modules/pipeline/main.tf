# CDCイベント配送パイプライン
# Kinesis(CDCイベント) -> fanout Lambda -> SQS FIFO -> delivery Lambda -> ターゲットAPI

# 保持期間はfanoutの再試行上限超過時の回収猶予でもある。退避先(SQS)にはメタデータ
# しか残らず、本体はこのストリームから読み直す必要があるため、既定の24時間より長く取る
resource "aws_kinesis_stream" "cdc" {
  name             = "${var.name_prefix}-cdc-stream"
  shard_count      = var.kinesis_shard_count
  retention_period = var.kinesis_retention_hours
}

# 配送用FIFOキュー。コンテンツベース重複排除は使わず、明示的なDeduplicationIdで制御する。
# message_retention_secondsは既定4日。DLQへ退避したメッセージは原因調査と再投入まで
# 残す必要があるため、両キューとも上限の14日を明示する（未指定のまま4日で消えると、
# 「DLQに残るので失われない」という設計の前提が崩れる）
resource "aws_sqs_queue" "delivery_dlq" {
  name                      = "${var.name_prefix}-delivery-dlq.fifo"
  fifo_queue                = true
  message_retention_seconds = var.sqs_message_retention_seconds
}

resource "aws_sqs_queue" "delivery" {
  name                      = "${var.name_prefix}-delivery.fifo"
  fifo_queue                = true
  message_retention_seconds = var.sqs_message_retention_seconds
  # delivery Lambdaのタイムアウト(30秒)の6倍。AWSの推奨に合わせ、関数が処理中の
  # メッセージが他のポーラーへ再配信されないようにする
  visibility_timeout_seconds = 180
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.delivery_dlq.arn
    maxReceiveCount     = 5
  })
}

# fanoutが再試行上限を超えたレコードの退避先。
# 退避先にSQSを指定するとメタデータ（シャードIDと開始・終了シーケンス番号）しか残らず、
# 本体はKinesisの保持期間内に読み直すしかない。S3を指定するとレコード本体が保存され、
# 保持期間に依存せず再投入できる。バケット名はグローバルに一意である必要がある
resource "aws_s3_bucket" "fanout_failures" {
  bucket        = var.fanout_failure_bucket_name != "" ? var.fanout_failure_bucket_name : "${var.name_prefix}-fanout-failures"
  force_destroy = true
}

resource "aws_s3_bucket_public_access_block" "fanout_failures" {
  bucket                  = aws_s3_bucket.fanout_failures.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# 退避レコードは調査と再投入のための一時保管。無期限に残すとコストと管理対象が増える
resource "aws_s3_bucket_lifecycle_configuration" "fanout_failures" {
  bucket = aws_s3_bucket.fanout_failures.id

  rule {
    id     = "expire-failed-records"
    status = "Enabled"

    filter {}

    expiration {
      days = var.fanout_failure_retention_days
    }
  }
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
    resources = [aws_sqs_queue.delivery.arn]
  }
  # 再試行上限を超えたレコード本体の退避先。
  # ESMのS3退避には PutObject に加えて バケットに対する ListBucket が要る
  statement {
    actions   = ["s3:PutObject"]
    resources = ["${aws_s3_bucket.fanout_failures.arn}/*"]
  }
  statement {
    actions   = ["s3:ListBucket"]
    resources = [aws_s3_bucket.fanout_failures.arn]
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
# 上限超過時の退避先（S3。レコード本体が保存され再投入できる）を設定する
resource "aws_lambda_event_source_mapping" "kinesis_to_fanout" {
  event_source_arn = aws_kinesis_stream.cdc.arn
  function_name    = aws_lambda_function.fanout.arn
  # LATESTだと、ESMの作成・更新中（ポーリング開始まで数分かかる）にKinesisへ
  # 入ったレコードを読み飛ばしうる。AWSは「イベントを取りこぼさないためには
  # TRIM_HORIZONかAT_TIMESTAMPを指定する」としており、少なくとも1回届ける
  # という前提を守るためTRIM_HORIZONにする。
  # 代償として、ESMを作り直すと保持期間内のレコードを読み直すが、
  # ターゲット側の冪等キーで重複は排除される
  starting_position       = "TRIM_HORIZON"
  batch_size              = 100
  function_response_types = ["ReportBatchItemFailures"]
  maximum_retry_attempts  = 10

  destination_config {
    on_failure {
      destination_arn = aws_s3_bucket.fanout_failures.arn
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

# DLQ滞留の監視。1件でも退避されたらALARMにする
# (退避は設計上「異常の可視化」なので、放置検知を最優先にする)
resource "aws_cloudwatch_metric_alarm" "delivery_dlq_depth" {
  alarm_name          = "${var.name_prefix}-delivery-dlq-depth"
  alarm_description   = "配送DLQにメッセージが滞留しています。原因解消後にリドライブしてください"
  namespace           = "AWS/SQS"
  metric_name         = "ApproximateNumberOfMessagesVisible"
  statistic           = "Maximum"
  period              = 60
  evaluation_periods  = 1
  threshold           = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"
  alarm_actions       = var.alarm_actions

  dimensions = {
    QueueName = aws_sqs_queue.delivery_dlq.name
  }
}

# S3退避にはキュー滞留のようなメトリクスが無いため、退避に至る手前の
# 「fanoutがシャードを進められていない」状態を検知する。
#
# ここでErrorsを使ってはいけない。fanoutは失敗レコードを部分バッチ応答で返し、
# ハンドラ自体はnilエラーで正常終了するため、AWS/Lambda Errorsは加算されない。
# 一方IteratorAgeは、再試行でシャードが進まない限り単調に増える。
# 再試行上限に達してS3へ退避されるより前に鳴る唯一の標準メトリクス。
resource "aws_cloudwatch_metric_alarm" "fanout_iterator_age" {
  alarm_name          = "${var.name_prefix}-fanout-iterator-age"
  alarm_description   = "fanoutがKinesisシャードを進められていません。放置すると再試行上限超過でS3へ退避されます"
  namespace           = "AWS/Lambda"
  metric_name         = "IteratorAge"
  statistic           = "Maximum"
  period              = 60
  evaluation_periods  = 3
  threshold           = var.fanout_iterator_age_threshold_ms
  comparison_operator = "GreaterThanThreshold"
  treat_missing_data  = "notBreaching"
  alarm_actions       = var.alarm_actions

  dimensions = {
    FunctionName = aws_lambda_function.fanout.function_name
  }
}
