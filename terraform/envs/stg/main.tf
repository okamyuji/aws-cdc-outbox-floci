# stg環境（実AWS）
# Aurora MySQL 8.0のbinlogをAWS DMSで読み取り、Kinesisへストリーミングする

terraform {
  required_version = ">= 1.10"

  # tfstateにはdb_master_password等の機微情報が平文で入るため、ローカル保存を避けて
  # 暗号化済みのS3へ置く(S3ネイティブロック使用)。バケットは scripts/tfstate-bucket.sh で作成する
  backend "s3" {
    bucket       = "cdc-outbox-tfstate-018356302326"
    key          = "stg/terraform.tfstate"
    region       = "ap-northeast-1"
    encrypt      = true
    use_lockfile = true
  }

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0"
    }
  }
}

variable "region" {
  description = "AWSリージョン"
  type        = string
  default     = "ap-northeast-1"
}

variable "name_prefix" {
  description = "リソース名のプレフィックス"
  type        = string
  default     = "cdc-stg"
}

variable "db_master_username" {
  description = "Auroraのマスターユーザー名"
  type        = string
  default     = "admin_user"
}

variable "db_master_password" {
  description = "Auroraのマスターパスワード（tfvarsまたは環境変数で指定する）"
  type        = string
  sensitive   = true
}

variable "target_api_url" {
  description = "delivery Lambdaが呼び出すターゲットAPIのベースURL"
  type        = string
}

variable "target_api_token" {
  description = "ターゲットAPIのBearerトークン"
  type        = string
  default     = ""
  sensitive   = true
}

variable "admin_cidr" {
  description = "検証用にDB接続を許可する作業端末のCIDR"
  type        = string
}

provider "aws" {
  region = var.region
}

module "pipeline" {
  source            = "../../modules/pipeline"
  name_prefix       = var.name_prefix
  fanout_zip_path   = "${path.module}/../../../dist/fanout.zip"
  delivery_zip_path = "${path.module}/../../../dist/delivery.zip"
  target_api_url    = var.target_api_url
  target_api_token  = var.target_api_token
}
