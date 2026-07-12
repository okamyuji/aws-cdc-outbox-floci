require "test_helper"

class OrdersControllerTest < ActionDispatch::IntegrationTest
  test "正常系: 注文作成は201と注文JSONを返す" do
    post "/orders", params: { customer_id: "c1", amount: "1980.00" }, as: :json
    assert_response :created
    body = JSON.parse(response.body)
    assert_equal "c1", body["customer_id"]
    assert_equal "1980.00", body["amount"]
    assert_equal "created", body["status"]
  end

  test "異常系: 検証エラーは400を返す" do
    post "/orders", params: { customer_id: "", amount: "100" }, as: :json
    assert_response :bad_request
    post "/orders", params: { customer_id: "c1", amount: "abc" }, as: :json
    assert_response :bad_request
  end

  test "正常系: healthzは認証なしで200を返す" do
    get "/healthz"
    assert_response :ok
  end

  test "異常系: AUTH_TOKEN設定時はトークン不一致と未指定が401になる" do
    ENV["AUTH_TOKEN"] = "secret-token"
    post "/orders", params: { customer_id: "c1", amount: "100" }, as: :json
    assert_response :unauthorized
    post "/orders", params: { customer_id: "c1", amount: "100" }, as: :json,
      headers: { "Authorization" => "Bearer wrong" }
    assert_response :unauthorized
    post "/orders", params: { customer_id: "c1", amount: "100" }, as: :json,
      headers: { "Authorization" => "secret-token" }
    assert_response :unauthorized
  ensure
    ENV.delete("AUTH_TOKEN")
  end

  test "正常系: AUTH_TOKEN設定時に正しいBearerトークンで通過する" do
    ENV["AUTH_TOKEN"] = "secret-token"
    post "/orders", params: { customer_id: "c1", amount: "100" }, as: :json,
      headers: { "Authorization" => "Bearer secret-token" }
    assert_response :created
  ensure
    ENV.delete("AUTH_TOKEN")
  end

  test "エッジケース: AUTH_TOKEN設定時もhealthzは認証なしで通過する" do
    ENV["AUTH_TOKEN"] = "secret-token"
    get "/healthz"
    assert_response :ok
  ensure
    ENV.delete("AUTH_TOKEN")
  end
end
