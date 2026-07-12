# ローカル環境（floci）
# RDS MySQLは実MySQL 8.0コンテナ、LambdaはDockerコンテナとして起動される

terraform {
  required_version = ">= 1.10"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0"
    }
  }
}

variable "floci_endpoint" {
  description = "flociのエンドポイント"
  type        = string
  default     = "http://localhost:4566"
}

variable "target_api_url" {
  description = "delivery LambdaコンテナからみたターゲットAPIのURL"
  type        = string
  default     = "http://host.docker.internal:8082"
}

variable "target_api_token" {
  description = "ターゲットAPIのBearerトークン"
  type        = string
  default     = ""
  sensitive   = true
}

provider "aws" {
  region     = "ap-northeast-1"
  access_key = "test"
  secret_key = "test"

  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true
  s3_use_path_style           = true

  endpoints {
    iam     = var.floci_endpoint
    kinesis = var.floci_endpoint
    lambda  = var.floci_endpoint
    rds     = var.floci_endpoint
    sqs     = var.floci_endpoint
    sts     = var.floci_endpoint
    s3      = var.floci_endpoint
    logs    = var.floci_endpoint
  }
}

module "pipeline" {
  source            = "../../modules/pipeline"
  name_prefix       = "local"
  fanout_zip_path   = "${path.module}/../../../dist/fanout.zip"
  delivery_zip_path = "${path.module}/../../../dist/delivery.zip"
  target_api_url    = var.target_api_url
  target_api_token  = var.target_api_token
}

output "kinesis_stream_name" {
  description = "CDCイベントストリーム名"
  value       = module.pipeline.kinesis_stream_name
}

output "delivery_queue_url" {
  description = "配送用FIFOキューURL"
  value       = module.pipeline.delivery_queue_url
}
