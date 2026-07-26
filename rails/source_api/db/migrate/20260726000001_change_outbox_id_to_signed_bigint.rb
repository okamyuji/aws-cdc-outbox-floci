# outbox.idはseqとして下流へ運ばれ、Goのint64とRailsの受理上限(2^63-1)で受ける。
# BIGINT UNSIGNEDのままだとスキーマ上は2^64-1まで採番できてしまい、
# 契約側が受け取れない値の行を作れる。既存DBにも効くよう前進マイグレーションで揃える
class ChangeOutboxIdToSignedBigint < ActiveRecord::Migration[8.1]
  def up
    execute "ALTER TABLE outbox MODIFY id BIGINT NOT NULL AUTO_INCREMENT"
  end

  def down
    execute "ALTER TABLE outbox MODIFY id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT"
  end
end
