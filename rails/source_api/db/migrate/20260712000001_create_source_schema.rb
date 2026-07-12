# ソース側スキーマ。ルートのdb/source_schema.sql(Go側と共通のSSOT)と同じ構造を
# Railsのマイグレーションとして定義する。
# 相違点はordersのupdated_atのON UPDATE句のみ(Railsはupdated_atを自前で更新する
# ため省略。Go実装はordersを更新しないので実害はない)
class CreateSourceSchema < ActiveRecord::Migration[8.1]
  def change
    create_table :orders, id: false do |t|
      t.string :id, limit: 36, null: false, primary_key: true
      t.string :customer_id, limit: 36, null: false
      t.decimal :amount, precision: 12, scale: 2, null: false
      t.string :status, limit: 32, null: false
      # Go実装のINSERTはこの2列を省略しDBデフォルトに依存するため、デフォルト必須
      t.datetime :created_at, null: false, default: -> { "CURRENT_TIMESTAMP(6)" }
      t.datetime :updated_at, null: false, default: -> { "CURRENT_TIMESTAMP(6)" }
    end

    create_table :outbox, id: { type: :bigint, unsigned: true } do |t|
      t.string :event_id, limit: 36, null: false
      t.string :aggregate_id, limit: 36, null: false
      t.string :event_type, limit: 64, null: false
      t.json :payload, null: false
      t.boolean :published, null: false, default: false
      t.datetime :created_at, null: false, default: -> { "CURRENT_TIMESTAMP(6)" }
      t.index :event_id, unique: true, name: "uk_outbox_event_id"
      t.index [ :published, :id ], name: "idx_outbox_published"
    end
  end
end
