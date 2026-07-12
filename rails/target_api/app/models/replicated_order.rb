# 反映先の注文。べき等キー記録と順序逆行の巻き戻り防止を持つ
class ReplicatedOrder < ApplicationRecord
  self.table_name = "orders_replica"
  self.primary_key = "id"

  AMOUNT_PATTERN = /\A[0-9]{1,10}(\.[0-9]{1,2})?\z/

  # 同じイベントIDを既に処理済みであることを表す
  class DuplicateEvent < StandardError; end
  # 入力値の検証に失敗したことを表す
  class InvalidInput < StandardError; end

  # べき等キー記録と注文反映を単一トランザクションで行う。
  # 同じイベントIDが処理済みの場合はDuplicateEventを送出する。
  # source_seq(ソースoutboxのID)が既存より大きいときだけ上書きし、
  # 順序が逆転して届いた古いイベントによる状態の巻き戻りを防ぐ
  def self.replicate!(event_id:, order_id:, customer_id:, amount:, status:, seq:)
    validate_input!(event_id:, order_id:, customer_id:, amount:, status:, seq:)

    transaction do
      begin
        ProcessedEvent.create!(event_id: event_id)
      rescue ActiveRecord::RecordNotUnique
        raise DuplicateEvent
      end

      connection.execute(sanitize_sql_array([ <<~SQL, order_id, customer_id, amount, status, event_id, seq ]))
        INSERT INTO orders_replica (id, customer_id, amount, status, source_event_id, source_seq)
        VALUES (?, ?, ?, ?, ?, ?)
        ON DUPLICATE KEY UPDATE
          customer_id     = IF(VALUES(source_seq) > source_seq, VALUES(customer_id), customer_id),
          amount          = IF(VALUES(source_seq) > source_seq, VALUES(amount), amount),
          status          = IF(VALUES(source_seq) > source_seq, VALUES(status), status),
          source_event_id = IF(VALUES(source_seq) > source_seq, VALUES(source_event_id), source_event_id),
          source_seq      = IF(VALUES(source_seq) > source_seq, VALUES(source_seq), source_seq)
      SQL
    end
  end

  def self.validate_input!(event_id:, order_id:, customer_id:, amount:, status:, seq:)
    raise InvalidInput, "event_idは必須です" if event_id.blank?
    raise InvalidInput, "order_idは必須です" if order_id.blank?
    raise InvalidInput, "customer_idとstatusは必須です" if customer_id.blank? || status.blank?
    raise InvalidInput, "amountは正の数値文字列で指定します" unless amount.to_s.match?(AMOUNT_PATTERN)
    raise InvalidInput, "seqは正の整数で指定します" unless seq.is_a?(Integer) && seq.positive?
  end

  def as_api_json
    {
      event_id: source_event_id,
      order_id: id,
      customer_id: customer_id,
      amount: format("%.2f", amount),
      status: status,
      seq: source_seq
    }
  end
end
