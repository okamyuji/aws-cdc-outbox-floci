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

variable "target_api_token" {
  description = "delivery LambdaがターゲットAPIへ付与するBearerトークン（空なら無効）"
  type        = string
  default     = ""
  sensitive   = true
}
