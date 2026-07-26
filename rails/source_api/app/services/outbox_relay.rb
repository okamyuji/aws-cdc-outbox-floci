# ローカル環境専用のリレー。outboxをポーリングし、DMSのKinesisターゲット互換
# エンベロープでKinesisへ中継する。stg環境ではAWS DMSがこの役割を担う
class OutboxRelay
  # ストリームへの発行に失敗したことを表す
  class PublishError < StandardError; end

  BATCH_SIZE = 100

  def initialize(kinesis:, stream_name:, logger: Rails.logger)
    @kinesis = kinesis
    @stream_name = stream_name
    @logger = logger
  end

  # 未発行イベントを1バッチ分中継し、発行件数を返す。
  # 発行に失敗した場合はpublishedを更新せず、次回のポーリングで再送する(At-Least-Once)
  def relay_once
    events = OutboxEvent.unpublished_in_order.limit(BATCH_SIZE).to_a
    return 0 if events.empty?

    response = @kinesis.put_records(
      stream_name: @stream_name,
      records: events.map { |event| to_record(event) }
    )
    raise PublishError, "ストリームへの送信に#{response.failed_record_count}件失敗しました" if response.failed_record_count.to_i.positive?

    OutboxEvent.mark_published!(events.map(&:id))
    events.size
  end

  private

  # DMSのKinesisターゲットが出力するJSONエンベロープと同じ形式に変換する
  def to_record(event)
    payload = event.payload.is_a?(String) ? event.payload : event.payload.to_json
    {
      # 同一集約を同一シャードへ寄せる。put_recordsはリクエスト内の順序を保証
      # しないため、これで守れるのはシャードの一致まで。同一集約内の適用順序は
      # ターゲット側のseq比較（巻き戻り防止）で担保する
      partition_key: event.aggregate_id,
      data: {
        data: {
          id: event.id,
          event_id: event.event_id,
          aggregate_id: event.aggregate_id,
          event_type: event.event_type,
          payload: payload,
          created_at: event.created_at.utc.iso8601(6)
        },
        metadata: {
          timestamp: Time.now.utc.iso8601(6),
          "record-type": "data",
          operation: "insert",
          "schema-name": "source_orders",
          "table-name": "outbox"
        }
      }.to_json
    }
  end
end
