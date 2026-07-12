# Rails実装

Rails 8.1（API mode）/ Ruby 3.4のVanilla Rails実装です。`source_api`（注文+outbox）と`target_api`（べき等な反映）の2アプリで、Go実装とREST APIの契約を揃えています。

## 構成

- `source_api/` 注文API。`Order.create_with_outbox!`が単一トランザクションでorders+outboxへ書き込み、`OutboxRelay`（`bin/rails outbox:relay`）がローカル環境のCDC代替としてKinesisへ中継します
- `target_api/` 反映API。`ReplicatedOrder.replicate!`が`processed_events`によるべき等化と`source_seq`による順序逆行の巻き戻り防止を単一トランザクションで行います

## 前提

テストと開発にはMySQLが必要です（CIのrails-ci.ymlと同じ想定）。

```bash
docker run -d --name cdc-rails-mysql -e MYSQL_ALLOW_EMPTY_PASSWORD=true -p 3306:3306 mysql:8.0
```

## 起動（ローカル、floci環境のDBに向ける）

```bash
cd source_api
RAILS_ENV=production SECRET_KEY_BASE=$(bin/rails secret) \
  DB_HOST=127.0.0.1 DB_PORT=7001 DB_USER=app DB_PASSWORD=apppassword DB_SSL_DISABLED=1 \
  AUTH_TOKEN=local-dev-token bin/rails server -p 8081

cd target_api
RAILS_ENV=production SECRET_KEY_BASE=$(bin/rails secret) \
  DB_HOST=127.0.0.1 DB_PORT=7002 DB_USER=app DB_PASSWORD=apppassword DB_SSL_DISABLED=1 \
  AUTH_TOKEN=local-dev-token bin/rails server -p 8082

# リレー（source_api内で実行）
KINESIS_STREAM_NAME=local-cdc-stream AWS_ENDPOINT_URL=http://localhost:4566 \
  AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test AWS_REGION=ap-northeast-1 \
  (上記DB/認証環境変数と合わせて) bin/rails outbox:relay
```

APP_ENVがlocal以外でAUTH_TOKEN未設定の場合は起動に失敗します（fail-closed）。

## 品質ゲート

```bash
make gate  # rubocop / brakeman / bundler-audit / minitest（両アプリ）
```

テストはRailsのトランザクショナルテストで分離され、正常系・異常系・境界値・エッジケースを含みます。
