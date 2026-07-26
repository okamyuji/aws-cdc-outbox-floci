# source_seqはソースoutboxのid由来で、Goのint64とRailsの受理上限(2^63-1)で受ける。
# ソース側のoutbox.idを符号付きへ揃えたのに合わせる
class ChangeSourceSeqToSignedBigint < ActiveRecord::Migration[8.1]
  def up
    execute "ALTER TABLE orders_replica MODIFY source_seq BIGINT NOT NULL"
  end

  def down
    execute "ALTER TABLE orders_replica MODIFY source_seq BIGINT UNSIGNED NOT NULL"
  end
end
