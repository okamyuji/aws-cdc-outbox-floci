# aws-cdc-outbox-floci

Aurora MySQL 8.0のTransactional OutboxをCDCで別マイクロサービスへ連携する検証リポジトリです。ローカルは[floci](https://floci.io/)でAWSをエミュレートし、stgは実AWS（Aurora MySQL + AWS DMS）を使います。アプリケーションはGo実装（`golang/`）とRails実装（`rails/`）の2系統があり、REST APIの契約・インフラ・E2Eテストを共有します。

## アーキテクチャ

```text
ソースAPI ── 単一TXで orders + outbox へINSERT
      │
      ▼ binlog / ポーリング
[stg: AWS DMS CDC | local: リレー]
      │ DMS互換JSONエンベロープ
      ▼
Kinesis Data Streams ── fanout Lambda ── SQS FIFO ── delivery Lambda
                                                          │ X-Idempotency-Key
                                                          ▼
                                                  ターゲットAPI ── MySQL反映
```

- ローカルとstgの違いはoutbox変更の検知部だけです。ローカルはリレー（outboxポーリング）、stgはDMSがbinlogを読みます。どちらも同じDMS互換エンベロープでKinesisへ流すため、下流のLambda・APIは共通です
- SQS FIFOのMessageGroupIdに集約ID（注文ID）、MessageDeduplicationIdにイベントIDを使い、順序維持と重複排除を担保します
- ターゲットAPIは`X-Idempotency-Key`（イベントID）を`processed_events`テーブルに記録して重複を排除し、outbox ID由来の`seq`で順序逆行の巻き戻りを防ぎます

## ディレクトリ構成

| パス | 役割 |
| --- | --- |
| `db/` | スキーマのSSOT（Go/Rails共通） |
| `terraform/` | パイプラインモジュールとlocal（floci）/stg（実AWS）環境（言語非依存） |
| `e2e/` | Playwright E2E。Go実装・Rails実装のどちらに対しても同一スイートが通る |
| `scripts/` | ローカルDB作成などの補助スクリプト |
| `golang/` | Go実装（API 2つ+リレー+Lambda）。詳細は[golang/README.md](golang/README.md) |
| `rails/` | Rails 8.1実装（API 2つ+リレー）。詳細は[rails/README.md](rails/README.md) |
| `dist/` | Lambda zipのビルド出力（`make build-lambda`） |

Lambda（fanout/delivery）はパイプラインの一部としてGoで実装しており、Rails実装を使う場合も共通で動きます。

## 品質ゲート

```bash
make gate       # golang gate + rails gate + terraform validate
make precommit  # pre-commit用の軽量サブセット（lefthookが実行）
make e2e        # Playwright E2E（ローカル環境の起動が前提）
```

## ローカル環境

```bash
docker compose up -d           # floci起動
./scripts/local-db.sh          # RDS MySQL 2台の作成とスキーマ適用
make build-lambda              # Lambda zipのビルド
cd terraform/envs/local && terraform apply -var target_api_token=local-dev-token
```

この後、Go実装またはRails実装のAPIサーバー群を起動します（各READMEを参照）。どちらを起動しても`make e2e`の同一スイートで検証できます。

## stg環境（実AWS）

`terraform/envs/stg`にAurora MySQL 8.0（ソース・ターゲット）、DMS（binlog CDC→Kinesis）、共通パイプラインの定義があります。適用は費用が発生するため手動で行います。実測済みの検証手順は次のとおりです。

1. `terraform apply`（`admin_cidr`に作業端末のIPを指定）
2. 両Auroraへ`db/*.sql`を適用し、`binlog_format=ROW`と`binlog retention hours`を確認
3. `aws dms start-replication-task`でCDCタスクを起動
4. outboxへテストデータを投入（ロールバック分はKinesisへ流れないことも確認）
5. Kinesisの実レコード（DMSエンベロープ）とSQS FIFOのメッセージを突き合わせ
6. `terraform destroy`

## 既知の制約（stg/DMS）

- アカウントに固定名の`dms-vpc-role`が必要です（無いとサブネットグループ作成が「is not configured properly」で失敗）。Terraformで作成しています
- `aurora`エンジンのDMSエンドポイントは`ssl_mode = "require"`非対応です
- `dms.t3.micro`は提供終了。最小クラスは`dms.t3.small`です
- Kinesisターゲットの既定パーティションキーは`スキーマ.テーブル`です。複数シャードで集約単位に分散させるにはオブジェクトマッピングが必要です

## 既知の制約（floci）

- flociは`DescribeDBInstances`の`dbi-resource-id`フィルタ未実装のため、Terraform AWS provider v5系の`aws_db_instance`が使えません。ローカルのDB作成のみ`scripts/local-db.sh`（AWS CLI）で行います
- flociのRDSプロキシはTLS非対応のため、mysqlクライアントは`--ssl-mode=DISABLED`で接続します
- macOSはAirPlayが7000番を使うため、RDSプロキシポートは7001始まりにしています
