class OrdersController < ApplicationController
  def show
    order = ReplicatedOrder.find(params[:id])
    render json: order.as_api_json
  rescue ActiveRecord::RecordNotFound
    render json: { error: "注文が見つかりません" }, status: :not_found
  end
end
