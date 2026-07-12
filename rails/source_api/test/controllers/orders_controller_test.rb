require "test_helper"

class OrdersControllerTest < ActionDispatch::IntegrationTest
  setup do
    @original_token = Rails.configuration.x.auth_token
  end

  teardown do
    Rails.configuration.x.auth_token = @original_token
  end

  test "正常系: 注文作成は201と注文JSONを返す" do
    post "/orders", params: { customer_id: "c1", amount: "1980.00" }, as: :json
    assert_response :created
    body = JSON.parse(response.body)
    assert_equal "c1", body["customer_id"]
    assert_equal "1980.00", body["amount"]
    assert_equal "created", body["status"]
  end

  test "正常系: amountの入力文字列がそのまま応答に透過される" do
    post "/orders", params: { customer_id: "c1", amount: "10" }, as: :json
    assert_response :created
    assert_equal "10", JSON.parse(response.body)["amount"]
  end

  test "異常系: 検証エラーは400を返す" do
    post "/orders", params: { customer_id: "", amount: "100" }, as: :json
    assert_response :bad_request
    post "/orders", params: { customer_id: "c1", amount: "abc" }, as: :json
    assert_response :bad_request
  end

  test "異常系: 文字列以外の型のパラメータは400を返す" do
    post "/orders", params: { customer_id: { x: 1 }, amount: "100" }, as: :json
    assert_response :bad_request
    post "/orders", params: { customer_id: "c1", amount: 100 }, as: :json
    assert_response :bad_request
  end

  test "異常系: 不正なJSONボディはGo実装と同じ形式の400を返す" do
    post "/orders", params: "{broken", headers: { "Content-Type" => "application/json" }
    assert_response :bad_request
    assert_equal "リクエストボディが不正です", JSON.parse(response.body)["error"]
  end

  test "正常系: healthzは認証なしで200を返す" do
    get "/healthz"
    assert_response :ok
  end

  test "異常系: トークン設定時は不一致と未指定と非正規スキームが401になる" do
    Rails.configuration.x.auth_token = "secret-token"
    post "/orders", params: { customer_id: "c1", amount: "100" }, as: :json
    assert_response :unauthorized
    post "/orders", params: { customer_id: "c1", amount: "100" }, as: :json,
      headers: { "Authorization" => "Bearer wrong" }
    assert_response :unauthorized
    post "/orders", params: { customer_id: "c1", amount: "100" }, as: :json,
      headers: { "Authorization" => "secret-token" }
    assert_response :unauthorized
    post "/orders", params: { customer_id: "c1", amount: "100" }, as: :json,
      headers: { "Authorization" => "Bearer\tsecret-token" }
    assert_response :unauthorized
  end

  test "正常系: トークン設定時に正しいBearerトークンで通過する" do
    Rails.configuration.x.auth_token = "secret-token"
    post "/orders", params: { customer_id: "c1", amount: "100" }, as: :json,
      headers: { "Authorization" => "Bearer secret-token" }
    assert_response :created
  end

  test "エッジケース: トークン設定時もhealthzは認証なしで通過する" do
    Rails.configuration.x.auth_token = "secret-token"
    get "/healthz"
    assert_response :ok
  end
end
