# 注文エンティティ。作成時に同一トランザクションでoutboxイベントを記録する
class Order < ApplicationRecord
  AMOUNT_PATTERN = /\A[0-9]{1,10}(\.[0-9]{1,2})?\z/

  self.primary_key = "id"

  validates :customer_id, presence: true
  validates :status, presence: true
  # 入力文字列としての金額。DECIMAL型の丸めで検証がすり抜けないよう文字列のまま
  # 検証する。作成時のみの検証とし、既存レコードの更新を妨げない
  validates :amount_input, format: { with: AMOUNT_PATTERN }, on: :create

  attribute :status, :string, default: "created"

  attr_reader :amount_input

  def amount_input=(value)
    @amount_input = value
    self.amount = value if value.is_a?(String) && value.match?(AMOUNT_PATTERN)
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

  # outboxのpayloadに載せる注文情報。amountはGo実装と同様に入力文字列を
  # そのまま透過させる(DECIMAL経由の"100"→"100.00"正規化をしない)。
  # amount_inputは作成時のみ保持されるメモリ上の値のため、DBから再ロードした
  # インスタンスでの呼び出しは契約違反として即座に失敗させる
  def event_payload
    raise "event_payloadは作成トランザクション内でのみ使用できます" if amount_input.nil?

    {
      id: id,
      customer_id: customer_id,
      amount: amount_input,
      status: status,
      created_at: created_at&.iso8601(6)
    }
  end

  def as_api_json
    event_payload
  end
end
