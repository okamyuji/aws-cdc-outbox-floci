# aws-cdc-outbox-floci

Aurora MySQL 8.0のTransactional OutboxをCDCで別マイクロサービスへ連携する検証リポジトリです。ローカルは[floci](https://floci.io/)でAWSをエミュレートし、stgは実AWS（Aurora MySQL + AWS DMS）を使います。

## アーキテクチャ

```text
ソースAPI (Go) ── 単一TXで orders + outbox へINSERT
      │
      ▼ binlog / ポーリング
[stg: AWS DMS CDC | local: cmd/relay]
      │ DMS互換JSONエンベロープ
      ▼
Kinesis Data Streams ── fanout Lambda ── SQS FIFO ── delivery Lambda
                                                          │ X-Idempotency-Key
                                                          ▼
                                             ターゲットAPI (Go) ── MySQL反映
```

- ローカルとstgの違いはoutbox変更の検知部だけです。ローカルはGo製リレー（outboxポーリング）、stgはDMSがbinlogを読みます。どちらも同じDMS互換エンベロープでKinesisへ流すため、下流のLambda・APIは共通です。
- SQS FIFOのMessageGroupIdに集約ID（注文ID）、MessageDeduplicationIdにイベントIDを使い、順序維持と重複排除を担保します。
- ターゲットAPIは`X-Idempotency-Key`（イベントID）を`processed_events`テーブルに記録し、少なくとも1回配信の重複を排除します。

## 構成

- `internal/domain` エンティティとリポジトリ抽象（他層へ依存しない）
- `internal/usecase` ユースケース（domainの抽象にのみ依存）
- `internal/adapter` MySQL実装・RESTハンドラ・Kinesis発行（interfaceで接続）
- `cmd/` ソースAPI・ターゲットAPI・ローカル用リレー
- `lambda/` fanout（Kinesis→SQS FIFO）とdelivery（SQS→ターゲットAPI）
- `terraform/` パイプラインモジュールとlocal（floci）/stg（実AWS）環境
- `e2e/` Playwright（APIリクエスト）によるE2E

## ローカル環境の起動

```bash
docker compose up -d           # floci起動
./scripts/local-db.sh          # RDS MySQL 2台の作成とスキーマ適用
make build-lambda              # Lambda zipのビルド
cd terraform/envs/local && terraform apply  # パイプライン構築

# 3プロセスを起動（DSNはlocal-db.shの出力を使う）
SOURCE_DB_DSN='app:apppassword@tcp(127.0.0.1:7001)/source_orders?parseTime=true' go run ./cmd/source-api
TARGET_DB_DSN='app:apppassword@tcp(127.0.0.1:7002)/target_orders?parseTime=true' go run ./cmd/target-api
SOURCE_DB_DSN='...' KINESIS_STREAM_NAME=local-cdc-stream AWS_ENDPOINT_URL=http://localhost:4566 \
  AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test AWS_REGION=ap-northeast-1 go run ./cmd/relay
```

### 疎通確認（記事中の実測ログの再現手順）

```bash
curl -s -X POST http://localhost:8081/orders \
  -H 'Authorization: Bearer local-dev-token' -H 'Content-Type: application/json' \
  -d '{"customer_id":"cust-e2e-1","amount":"1980.00"}'
# 返ってきたidで数秒後にターゲット側を照会する
curl -s -H 'Authorization: Bearer local-dev-token' http://localhost:8082/orders/<id>
```

## 品質ゲート

```bash
make gate   # gofmt / go vet / golangci-lint / go test(+coverage) / govulncheck / terraform validate
make e2e    # Playwright E2E（ローカル環境の起動が前提）
```

repositoryのテストはtestcontainers（mysql:8.0）で実DBに対して実行します。

## stg環境

`terraform/envs/stg`にAurora MySQL 8.0（ソース・ターゲット）、DMS（binlog CDC→Kinesis）、共通パイプラインの定義があります。適用は費用が発生するため手動で行います。

```bash
cd terraform/envs/stg
terraform init
terraform plan -var db_master_password=... -var target_api_url=https://...
```

## 既知の制約

- flociは`DescribeDBInstances`の`dbi-resource-id`フィルタ未実装のため、Terraform AWS provider v5系の`aws_db_instance`が使えません。ローカルのDB作成のみ`scripts/local-db.sh`（AWS CLI）で行います。
- flociのRDSプロキシはTLS非対応のため、mysqlクライアントは`--ssl-mode=DISABLED`で接続します。
- macOSはAirPlayが7000番を使うため、RDSプロキシポートは7001始まりにしています。
