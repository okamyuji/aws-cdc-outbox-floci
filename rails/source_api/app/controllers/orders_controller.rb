class OrdersController < ApplicationController
  def create
    order = Order.create_with_outbox!(
      customer_id: string_param(:customer_id).to_s,
      amount: string_param(:amount)
    )
    render json: order.as_api_json, status: :created
  rescue ActiveRecord::RecordInvalid => e
    Rails.logger.warn("注文の検証に失敗しました: #{e.message.inspect}")
    render json: { error: "入力値が不正です" }, status: :bad_request
  end
end
