# 認証トークンは起動時に一度だけ読み込んで固定する。
# fail-closed: production環境ではAUTH_TOKEN必須(未設定なら起動失敗)。
# development/testは開発体験のため未設定(無認証)を許容する
Rails.application.config.x.auth_token = ENV["AUTH_TOKEN"].to_s

if Rails.application.config.x.auth_token.empty? && Rails.env.production?
  abort("AUTH_TOKENが未設定です。productionでは必須です")
end
