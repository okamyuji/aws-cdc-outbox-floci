# Go実装

Go 1.26（標準ライブラリ中心）のクリーンアーキテクチャ実装です。domain/usecase/adapterの各層をすべてインターフェース経由で接続しています。

## 構成

- `internal/domain` エンティティとリポジトリ抽象（他層へ依存しない）
- `internal/usecase` ユースケース（domainの抽象にのみ依存）
- `internal/adapter` MySQL実装・RESTハンドラ・Kinesis発行（interfaceで接続）
- `cmd/` ソースAPI・ターゲットAPI・ローカル用リレー
- `lambda/` fanout（Kinesis→SQS FIFO）とdelivery（SQS→ターゲットAPI）。Rails実装と共通のパイプライン部品

## 起動（ローカル）

```bash
SOURCE_DB_DSN='app:apppassword@tcp(127.0.0.1:7001)/source_orders?parseTime=true' \
  AUTH_TOKEN=local-dev-token go run ./cmd/source-api
TARGET_DB_DSN='app:apppassword@tcp(127.0.0.1:7002)/target_orders?parseTime=true' \
  AUTH_TOKEN=local-dev-token go run ./cmd/target-api
SOURCE_DB_DSN='app:apppassword@tcp(127.0.0.1:7001)/source_orders?parseTime=true' \
  KINESIS_STREAM_NAME=local-cdc-stream AWS_ENDPOINT_URL=http://localhost:4566 \
  AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test AWS_REGION=ap-northeast-1 go run ./cmd/relay
```

## 品質ゲート

```bash
make gate  # gofmt / go vet / golangci-lint / staticcheck / go test(+coverage) / govulncheck
```

repositoryのテストはtestcontainers（mysql:8.0）で実DBに対して実行します。カバレッジはmain関数（配線のみ）を除いて80%以上を目標にしています。
