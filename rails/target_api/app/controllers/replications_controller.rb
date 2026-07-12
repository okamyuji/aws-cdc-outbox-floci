class ReplicationsController < ApplicationController
  def create
    event_id = string_param(:event_id).to_s
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
      order_id: string_param(:order_id).to_s,
      customer_id: string_param(:customer_id).to_s,
      amount: string_param(:amount),
      status: string_param(:status).to_s,
      seq: params[:seq]
    )
    head :created
  rescue ReplicatedOrder::DuplicateEvent
    Rails.logger.info("重複イベントを読み飛ばしました event_id=#{event_id.inspect}")
    head :ok
  rescue ReplicatedOrder::InvalidInput => e
    Rails.logger.warn("入力値が不正です: #{e.message.inspect}")
    render json: { error: "入力値が不正です" }, status: :bad_request
  end
end
