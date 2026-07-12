class ApplicationController < ActionController::API
  before_action :authenticate!

  private

  # Bearerトークン認証。トークンは起動時にconfig.xへ固定され、実行中の
  # 環境変数変化で認証が静かに無効化されることはない。
  # スキームは「Bearer + 半角スペース1つ」のみを受理する(Go実装と同一の意味論)
  def authenticate!
    token = Rails.configuration.x.auth_token.to_s
    return if token.empty?

    header = request.headers["Authorization"].to_s
    value = header.delete_prefix("Bearer ")
    return if header.start_with?("Bearer ") && value.present? &&
              ActiveSupport::SecurityUtils.secure_compare(value, token)

    render json: { error: "認証に失敗しました" }, status: :unauthorized
  end

  # 文字列として送られた場合のみ値を返す。HashやArray、JSON数値をto_sで
  # 強制文字列化して受理しない(Go実装のJSONデコード厳格性と揃える)
  def string_param(key)
    value = params[key]
    value.is_a?(String) ? value : nil
  end
end
