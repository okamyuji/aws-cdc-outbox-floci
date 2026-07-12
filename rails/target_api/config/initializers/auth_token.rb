# fail-closed: 明示的にローカル環境を宣言した場合のみ無認証を許容する。
# テスト環境はテストコード側でAUTH_TOKENを制御するため対象外
if ENV["AUTH_TOKEN"].to_s.empty? && ENV["APP_ENV"] != "local" && !Rails.env.test?
  abort("AUTH_TOKENが未設定です。ローカルで無認証にする場合はAPP_ENV=localを設定してください")
end
