#!/usr/bin/env bash
# ローカル環境（floci）のRDS MySQLを冪等に作成し、スキーマを適用する
#
# 背景: flociはDescribeDBInstancesのdbi-resource-idフィルタを未実装のため、
# Terraform AWS provider v5系のaws_db_instanceは読み取りに失敗する。
# ローカルのDB作成のみCLIで行い、パイプラインはTerraformで管理する。
set -euo pipefail

export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_DEFAULT_REGION=ap-northeast-1
export AWS_ENDPOINT_URL=${AWS_ENDPOINT_URL:-http://localhost:4566}

DB_USER=app
DB_PASSWORD=apppassword
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

create_instance() {
  local id=$1 dbname=$2
  # flociは未知の識別子でもエラーではなく空リストを返すため、件数で存在判定する
  local count
  count=$(aws rds describe-db-instances --db-instance-identifier "$id" \
    --query 'length(DBInstances)' --output text 2>/dev/null || echo 0)
  if [ "$count" != "0" ] && [ "$count" != "None" ]; then
    echo "既に存在します: $id"
    return
  fi
  aws rds create-db-instance \
    --db-instance-identifier "$id" \
    --db-instance-class db.t3.micro \
    --engine mysql \
    --engine-version 8.0 \
    --master-username "$DB_USER" \
    --master-user-password "$DB_PASSWORD" \
    --db-name "$dbname" \
    --allocated-storage 20 > /dev/null
  echo "作成しました: $id"
}

wait_available() {
  local id=$1
  for _ in $(seq 1 60); do
    local status
    status=$(aws rds describe-db-instances --db-instance-identifier "$id" \
      --query 'DBInstances[0].DBInstanceStatus' --output text)
    if [ "$status" = "available" ]; then
      return
    fi
    sleep 2
  done
  echo "起動待ちがタイムアウトしました: $id" >&2
  exit 1
}

port_of() {
  aws rds describe-db-instances --db-instance-identifier "$1" \
    --query 'DBInstances[0].Endpoint.Port' --output text
}

apply_schema() {
  local port=$1 schema=$2
  # flociのJDBCプロキシはTLS非対応のためSSLを無効化する
  docker run --rm -i mysql:8.0 mysql --ssl-mode=DISABLED \
    -h host.docker.internal -P "$port" -u "$DB_USER" -p"$DB_PASSWORD" < "$schema"
}

wait_mysql() {
  local port=$1
  for _ in $(seq 1 30); do
    if docker run --rm mysql:8.0 mysqladmin ping --ssl-mode=DISABLED \
      -h host.docker.internal -P "$port" -u "$DB_USER" -p"$DB_PASSWORD" --silent > /dev/null 2>&1; then
      return
    fi
    sleep 2
  done
  echo "MySQLの応答待ちがタイムアウトしました: port=$port" >&2
  exit 1
}

create_instance cdc-source source_orders
create_instance cdc-target target_orders
wait_available cdc-source
wait_available cdc-target

SOURCE_PORT=$(port_of cdc-source)
TARGET_PORT=$(port_of cdc-target)
wait_mysql "$SOURCE_PORT"
wait_mysql "$TARGET_PORT"

apply_schema "$SOURCE_PORT" "$REPO_ROOT/db/source_schema.sql"
apply_schema "$TARGET_PORT" "$REPO_ROOT/db/target_schema.sql"

echo "SOURCE_DB_DSN=$DB_USER:$DB_PASSWORD@tcp(127.0.0.1:$SOURCE_PORT)/source_orders?parseTime=true"
echo "TARGET_DB_DSN=$DB_USER:$DB_PASSWORD@tcp(127.0.0.1:$TARGET_PORT)/target_orders?parseTime=true"
