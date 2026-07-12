# 注文エンティティ。作成時に同一トランザクションでoutboxイベントを記録する
class Order < ApplicationRecord
  AMOUNT_PATTERN = /\A[0-9]{1,10}(\.[0-9]{1,2})?\z/

  self.primary_key = "id"

  validates :customer_id, presence: true
  validates :status, presence: true
  validates :amount_input, format: { with: AMOUNT_PATTERN }, allow_nil: false

  attribute :status, :string, default: "created"

  # 入力文字列としての金額。DECIMAL型の丸めで検証がすり抜けないよう文字列のまま検証する
  attr_reader :amount_input

  def amount_input=(value)
    @amount_input = value.to_s
    self.amount = value if @amount_input.match?(AMOUNT_PATTERN)
  end

  # 注文とoutboxイベントを単一トランザクションで作成する
  def self.create_with_outbox!(customer_id:, amount:)
    transaction do
      order = new(customer_id: customer_id)
      order.amount_input = amount
      order.id = SecureRandom.uuid_v7
      order.save!
      OutboxEvent.create!(
        event_id: SecureRandom.uuid_v7,
        aggregate_id: order.id,
        event_type: "order.created",
        payload: order.event_payload
      )
      order
    end
  end

  # outboxのpayloadに載せる注文情報（Go実装とキーを揃える）
  def event_payload
    {
      id: id,
      customer_id: customer_id,
      amount: format("%.2f", amount),
      status: status,
      created_at: created_at&.iso8601(6)
    }
  end

  def as_api_json
    event_payload
  end
end
