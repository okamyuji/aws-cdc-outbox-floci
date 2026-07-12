# This file is auto-generated from the current state of the database. Instead
# of editing this file, please use the migrations feature of Active Record to
# incrementally modify your database, and then regenerate this schema definition.
#
# This file is the source Rails uses to define your schema when running `bin/rails
# db:schema:load`. When creating a new database, `bin/rails db:schema:load` tends to
# be faster and is potentially less error prone than running all of your
# migrations from scratch. Old migrations may fail to apply correctly if those
# migrations use external dependencies or application code.
#
# It's strongly recommended that you check this file into your version control system.

ActiveRecord::Schema[8.1].define(version: 2026_07_12_000001) do
  create_table "orders_replica", id: { type: :string, limit: 36 }, charset: "utf8mb4", collation: "utf8mb4_0900_ai_ci", force: :cascade do |t|
    t.decimal "amount", precision: 12, scale: 2, null: false
    t.string "customer_id", limit: 36, null: false
    t.datetime "replicated_at", default: -> { "CURRENT_TIMESTAMP(6)" }, null: false
    t.string "source_event_id", limit: 36, null: false
    t.bigint "source_seq", null: false, unsigned: true
    t.string "status", limit: 32, null: false
  end

  create_table "processed_events", primary_key: "event_id", id: { type: :string, limit: 36 }, charset: "utf8mb4", collation: "utf8mb4_0900_ai_ci", force: :cascade do |t|
    t.datetime "processed_at", default: -> { "CURRENT_TIMESTAMP(6)" }, null: false
  end
end
