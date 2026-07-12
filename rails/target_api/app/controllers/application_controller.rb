class ApplicationController < ActionController::API
  before_action :authenticate!

  private

  # Bearerトークン認証。AUTH_TOKEN未設定時は起動時チェック(initializer)が
  # ローカル環境であることを保証しているため素通しする
  def authenticate!
    token = ENV["AUTH_TOKEN"].to_s
    return if token.empty?

    scheme, value = request.headers["Authorization"].to_s.split(" ", 2)
    return if scheme == "Bearer" && value.present? && ActiveSupport::SecurityUtils.secure_compare(value, token)

    render json: { error: "認証に失敗しました" }, status: :unauthorized
  end
end
