# 不正なJSONボディへの400応答をGo実装と同じ形式に揃えるRackミドルウェア。
# JSONパースはコントローラ到達前のParamsParserで失敗するため、rescue_fromでは
# 捕捉できずここで処理する。環境によってはParseErrorが別の例外にラップされて
# 届くため、cause連鎖も確認する
class JsonParseErrorHandler
  def initialize(app)
    @app = app
  end

  def call(env)
    @app.call(env)
  rescue StandardError => e
    raise unless parse_error?(e)

    [ 400, { "Content-Type" => "application/json" },
      [ { error: "リクエストボディが不正です" }.to_json ] ]
  end

  private

  def parse_error?(error)
    current = error
    while current
      return true if current.is_a?(ActionDispatch::Http::Parameters::ParseError)

      current = current.cause
    end
    false
  end
end
