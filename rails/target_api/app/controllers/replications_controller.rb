class ReplicationsController < ApplicationController
  def create
    event_id = params[:event_id].to_s
    # X-Idempotency-Keyを優先し、ボディのevent_idと不一致なら拒否する
    header_key = request.headers["X-Idempotency-Key"].to_s
    if header_key.present?
      if event_id.present? && event_id != header_key
        return render json: { error: "X-Idempotency-Keyとevent_idが一致しません" }, status: :bad_request
      end

      event_id = header_key
    end

    ReplicatedOrder.replicate!(
      event_id: event_id,
      order_id: params[:order_id].to_s,
      customer_id: params[:customer_id].to_s,
      amount: params[:amount].to_s,
      status: params[:status].to_s,
      seq: parse_seq(params[:seq])
    )
    head :created
  rescue ReplicatedOrder::DuplicateEvent
    Rails.logger.info("重複イベントを読み飛ばしました event_id=#{event_id}")
    head :ok
  rescue ReplicatedOrder::InvalidInput => e
    Rails.logger.warn("入力値が不正です: #{e.message}")
    render json: { error: "入力値が不正です" }, status: :bad_request
  end

  private

  # seqはJSON数値でも数字文字列でも受け付け、それ以外はnil(検証エラー)にする
  def parse_seq(value)
    return value if value.is_a?(Integer)
    return value.to_i if value.to_s.match?(/\A\d+\z/)

    nil
  end
end
