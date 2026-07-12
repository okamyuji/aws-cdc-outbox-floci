# IMPROVEMENT_BACKLOG

敵対的レビュー（3並列、K=2合議）で単独指摘に留まった項目の記録です。次回改修の候補であり、現時点では未対応です。

## 2026-07-12 再レビュー

- delivery Lambdaのグループ単位保留により、毒メッセージと同一グループの健全な後続メッセージも受信回数を同時に消費し、先頭と一緒にDLQへ退避される（レビュー3再レビュー・High単独）。順序保証を優先した意図的なトレードオフとしてコードコメントに明文化済み。DLQ滞留の監視アラームと再投入手順の整備、またはグループ単位のCircuit Breakerが将来の改善候補

## 2026-07-12 初回レビュー

- persistence_test.goのサブテストが共有DB状態に依存している。`t.Parallel()`追加やサブテスト順序変更で壊れる可能性があるため、サブテストごとのTRUNCATE等で分離する（レビュー3・High単独）
- Kinesis PutRecordsの部分失敗時にバッチ全体を再送する挙動について、「部分失敗→全件再送→べき等性で吸収」の経路を通しで検証するテストがない（レビュー3・Medium単独）
- envelope.ParseOutboxInsertがPayloadの非空検証をしていない。fanout側（SequenceNumberを持つ最も早い地点）で破損を検知しログに残すのが望ましい（レビュー3・Low単独）
- Terraformのstg環境がローカルバックエンドのため、applyするとdb_master_passwordを含むtfstateが端末に平文で残る。実運用前にS3バックエンド+暗号化へ移行する（レビュー2・Medium単独）
- 認証ミドルウェアはトークン未設定時に素通し（fail-open）。stgでの設定漏れを検知する起動時警告または必須化を検討する → 対応済み（2026-07-12）。APP_ENV=local以外ではAUTH_TOKEN必須で起動失敗するfail-closedに変更
- delivery Lambda側のTARGET_API_TOKEN（Terraform変数）とターゲットAPI側のAUTH_TOKEN（OS環境変数）が別経路管理で、一致を保証する仕組みがない。片方の設定漏れで「無認証化」または「全件401でDLQ滞留」が静かに起きる。API側のデプロイ定義をIaCへ載せる際に単一のシークレット参照へ統合する（レビュー2再レビュー・Medium）
