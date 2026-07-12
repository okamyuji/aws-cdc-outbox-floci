#!/usr/bin/env bash
# stgのtfstate保存用S3バケットを冪等に作成する(バージョニング+暗号化+公開ブロック)
set -euo pipefail
BUCKET=cdc-outbox-tfstate-018356302326
REGION=ap-northeast-1

if aws s3api head-bucket --bucket "$BUCKET" 2>/dev/null; then
  echo "既に存在します: $BUCKET"
else
  aws s3api create-bucket --bucket "$BUCKET" --region "$REGION" \
    --create-bucket-configuration LocationConstraint="$REGION"
  echo "作成しました: $BUCKET"
fi
aws s3api put-bucket-versioning --bucket "$BUCKET" \
  --versioning-configuration Status=Enabled
aws s3api put-bucket-encryption --bucket "$BUCKET" \
  --server-side-encryption-configuration '{"Rules":[{"ApplyServerSideEncryptionByDefault":{"SSEAlgorithm":"aws:kms"}}]}'
aws s3api put-public-access-block --bucket "$BUCKET" \
  --public-access-block-configuration BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true
echo "設定完了: $BUCKET"
