# 認証トークンは起動時に一度だけ読み込んで固定する。
# fail-closed: production環境ではAUTH_TOKEN必須(未設定なら起動失敗)。
# 設計判断: 無認証を許容するのはdevelopment/testのみ。stgを含む外部到達可能な
# 環境は必ずRAILS_ENV=productionで起動する運用を前提とする(rails/README.md参照)
Rails.application.config.x.auth_token = ENV["AUTH_TOKEN"].to_s

if Rails.application.config.x.auth_token.empty? && Rails.env.production?
  abort("AUTH_TOKENが未設定です。productionでは必須です")
end
