#!/usr/bin/env bash
# ソースAuroraのbinlog保持期間を設定する（冪等）。
#
# なぜ必要か: Aurora MySQLのbinlog保持期間はクラスタパラメータグループでは設定できず、
# 既定は保持しない設定（NULL）。DMSが止まっている間に必要なbinlogが削除されると、
# チェックポイントからCDCを再開できず、その区間のoutboxイベントが欠落する。
# terraform apply のあとに必ず実行する。
#
# 使い方:
#   scripts/stg-binlog-retention.sh <endpoint> <user> <password> [hours]
set -euo pipefail

ENDPOINT="${1:?ソースAuroraのライターエンドポイントを指定してください}"
DB_USER="${2:?DBユーザーを指定してください}"
DB_PASSWORD="${3:?DBパスワードを指定してください}"
# 既定168時間(7日)。「許容するDMSタスク停止時間 + 最大CDC遅延」より長い値にする
HOURS="${4:-168}"

mysql_exec() {
  mysql --host="$ENDPOINT" --user="$DB_USER" --password="$DB_PASSWORD" \
    --batch --skip-column-names --execute="$1"
}

echo "binlog保持期間を ${HOURS} 時間に設定します (${ENDPOINT})"
mysql_exec "CALL mysql.rds_set_configuration('binlog retention hours', ${HOURS});"

echo "設定を確認します"
# rds_show_configuration の出力はタブ区切りで、設定名は "binlog retention hours" と
# スペースを含む。awkの既定の区切り（スペースも含む）では設定名が分割されてしまうため、
# 区切りをタブに固定する
CURRENT=$(mysql_exec "CALL mysql.rds_show_configuration;" | awk -F'\t' '$1 == "binlog retention hours" { print $2 }')

if [ "$CURRENT" != "$HOURS" ]; then
  echo "ERROR: 設定が反映されていません (現在値: ${CURRENT:-NULL})" >&2
  exit 1
fi
echo "OK: binlog retention hours = ${CURRENT}"
