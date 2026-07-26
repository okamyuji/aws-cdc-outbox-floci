variable "name_prefix" {
  description = "リソース名のプレフィックス"
  type        = string
}

variable "fanout_zip_path" {
  description = "fanout Lambdaのzipパス"
  type        = string
}

variable "delivery_zip_path" {
  description = "delivery Lambdaのzipパス"
  type        = string
}

variable "target_api_url" {
  description = "ターゲットAPIのベースURL"
  type        = string
}

variable "kinesis_shard_count" {
  description = "Kinesisのシャード数"
  type        = number
  default     = 1
}

variable "kinesis_retention_hours" {
  description = "Kinesisの保持期間（時間）。fanoutの再試行上限超過時に本体を読み直せる猶予でもある"
  type        = number
  default     = 168
}

variable "fanout_failure_bucket_name" {
  description = "fanoutの退避先S3バケット名。S3の名前空間はグローバルに一意なため、衝突する場合に指定する（空なら name_prefix から生成）"
  type        = string
  default     = ""
}

variable "fanout_failure_retention_days" {
  description = "退避したレコードをS3に保持する日数。この期限を過ぎると回収できなくなる"
  type        = number
  default     = 30
}

variable "fanout_iterator_age_threshold_ms" {
  description = "fanoutのIteratorAgeアラームのしきい値（ミリ秒）。既定5分（通常のCDC遅延より十分大きい値にする）"
  type        = number
  default     = 300000
}

variable "target_api_token" {
  description = "delivery LambdaがターゲットAPIへ付与するBearerトークン（空なら無効）"
  type        = string
  default     = ""
  sensitive   = true
}

variable "alarm_actions" {
  description = "DLQ滞留アラームの通知先ARN(SNS等)。空なら通知なしでアラーム状態のみ"
  type        = list(string)
  default     = []
}

variable "sqs_message_retention_seconds" {
  description = "配送キューとDLQのメッセージ保持期間（秒）。既定は上限の14日。DLQの調査と再投入の猶予になる"
  type        = number
  default     = 1209600
}
