# 検証ログ

stg（実AWS）での実測記録です。記事とREADMEの記述の根拠成果物として残します。環境はいずれも検証後に`terraform destroy`済みです。

## 2026-07-12 Go実装ラウンド（apply3〜5 → destroy）

構成はAurora MySQL 8.0（source/target、Serverless v2 0.5〜1.0 ACU）+ DMS（dms.t3.small、CDC専用タスク）+ 共有パイプライン。

観測事実は次のとおりです。

- `binlog_format=ROW` / `binlog_row_image=FULL` / `log_bin=ON` をSHOW VARIABLESで確認し、`binlog retention hours`を24に設定
- outboxへ「コミットする注文」と「ロールバックする注文」を1件ずつINSERTした結果、DMSのtable statisticsはinserts=1、Kinesisのデータレコードもコミット分の1件のみ（ロールバック分は非伝播）
- CDC開始時に`record-type: control`のレコード（awsdms_apply_exceptions / outboxのcreate-table）が2件先行
- 実データレコード（IDは検証用に手動採番したUUIDv7形式の固定値）:

```json
{
  "data": {
    "id": 1,
    "event_id": "0198a001-0000-7000-8000-00000000e001",
    "aggregate_id": "0198a001-0000-7000-8000-000000000001",
    "event_type": "order.created",
    "payload": "{\"id\": \"0198a001-0000-7000-8000-000000000001\", \"amount\": \"3980.00\", \"status\": \"created\", \"customer_id\": \"stg-cust-1\"}",
    "published": 0,
    "created_at": "2026-07-12T09:48:04.596147Z"
  },
  "metadata": {
    "timestamp": "2026-07-12T09:48:04.337444Z",
    "record-type": "data",
    "operation": "insert",
    "partition-key-type": "schema-table",
    "schema-name": "source_orders",
    "table-name": "outbox",
    "transaction-id": 17179870950
  }
}
```

- Kinesisレコードのパーティションキーは既定で`source_orders.outbox`（スキーマ.テーブル）
- fanout Lambda（ローカルと同一バイナリ）がこのレコードを解釈し、SQS FIFOに`MessageGroupId=0198a001-...0001`、`MessageDeduplicationId=event_id`、`seq=1`のメッセージを生成
- delivery Lambdaの宛先を到達不能URL（target.invalid）にした状態でESMを有効化すると、maxReceiveCount=5により5回の配信試行後にメッセージが`cdc-stg-delivery-dlq.fifo`へ退避（DLQのApproximateNumberOfMessages=1を確認）
- 構築時に当たった失敗: `dms-vpc-role`不在（AccessDeniedFault: The IAM Role ... is not configured properly）、auroraエンジンで`ssl_mode=require`不可（InvalidParameterCombinationException）、`dms.t3.micro`提供終了（Invalid ReplicationInstance class。orderableの最小はdms.t3.small）
- destroy完了: Resources: 36 destroyed。RDS/DMS/Kinesis/SQS/VPCの残存ゼロをCLIで確認

## 2026-07-12 Rails実装ラウンド（apply_rails → destroy2）

同一のTerraform定義を再適用し、両端をRails 8.1（ローカルでRAILS_ENV=production起動、AUTH_TOKEN設定、stgのAuroraへ接続）に差し替え。

- Rails source APIへのPOSTで注文を作成（応答: `{"id":"019f55e0-65e5-...","amount":"7980.00",...}`）
- DMS→Kinesisの実レコードを取得し、`data.payload`はRailsが書いたoutbox行そのもの、エンベロープ構造はGoラウンドと同一
- fanout LambdaがSQS FIFOへ変換したメッセージ: `group=019f55e0-...`（注文ID）、`seq=1`
- そのメッセージをdelivery Lambdaと同じリクエスト形式でRails target API（stgターゲットAuroraへ接続）に配送: 1回目201、同一べき等キーの2回目200
- stgターゲットAuroraの実データ: `orders_replica`に1行（amount=7980.00, source_seq=1）、`processed_events`は1件
- stgで機械的に流したのはSQSまでで、SQS→delivery LambdaのESM連携はローカルE2Eで検証済みのため、stgでは同形式の手動配送で代替
- destroy2完了後、残存リソースゼロを確認

## ローカル（floci）の代表実測

- Go実装: POSTから約3秒でリレー→Kinesis→fanout→SQS FIFO→delivery→ターゲットAPIを通過し反映
- Rails実装: 同一のPlaywright E2Eスイート7件がGo実装と同様に全件パス
- production相当設定の不正JSONボディ: `{"error":"リクエストボディが不正です"}` / 400 / application/json を実測（JsonParseErrorHandlerの挿入位置修正後）

## 2026-07-12 オブジェクトマッピング検証ラウンド（apply_om → destroy3）

DMSタスクにobject-mappingを追加し、Kinesisのパーティションキーを集約ID（aggregate_id）へ変更した検証です。

- 実測結果: データレコードのPartitionKeyが`om-verify-order-0001`（=aggregate_id）になり、エンベロープの`data`構造は全列を保持したまま不変。fanout Lambdaはそのまま解釈し、SQS FIFOに`group=aggregate_id`のメッセージを生成
- 構文の落とし穴を2つ実測: `partition-key-type: attribute-name`では(1)パラメータ名は`partition-key-name`（`partition-key-attribute-name`はInvalidParameterValueException）、(2)`attribute-mappings`の併記が必須（無いと「Attribute mappings are empty」で拒否）。ソース列を素通しでマップすれば要件を満たす
- DLQ滞留アラーム（delivery/fanout）が両方作成されOK状態であることを確認
- tfstateはS3バックエンド（cdc-outbox-tfstate-018356302326、暗号化+バージョニング+公開ブロック）へ移行済みで、このラウンドからS3上のstateで運用
- destroy完了後、残存リソースゼロを確認（tfstateバケットは意図的に残置）
