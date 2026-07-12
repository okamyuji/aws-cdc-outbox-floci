# ターゲット側スキーマ。ルートのdb/target_schema.sql(Go側と共通のSSOT)と同じ構造を
# Railsのマイグレーションとして定義する
class CreateTargetSchema < ActiveRecord::Migration[8.1]
  def change
    create_table :orders_replica, id: false do |t|
      t.string :id, limit: 36, null: false, primary_key: true
      t.string :customer_id, limit: 36, null: false
      t.decimal :amount, precision: 12, scale: 2, null: false
      t.string :status, limit: 32, null: false
      t.string :source_event_id, limit: 36, null: false
      t.bigint :source_seq, unsigned: true, null: false
      t.datetime :replicated_at, null: false, default: -> { "CURRENT_TIMESTAMP(6)" }
    end

    create_table :processed_events, id: false do |t|
      t.string :event_id, limit: 36, null: false, primary_key: true
      t.datetime :processed_at, null: false, default: -> { "CURRENT_TIMESTAMP(6)" }
    end
  end
end
