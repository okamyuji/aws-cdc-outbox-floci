require "test_helper"

class ReplicationsControllerTest < ActionDispatch::IntegrationTest
  def valid_body(overrides = {})
    {
      event_id: SecureRandom.uuid_v7,
      order_id: SecureRandom.uuid_v7,
      customer_id: "c1",
      amount: "100",
      status: "created",
      seq: 1
    }.merge(overrides)
  end

  test "正常系: 反映は201、照会は200を返す" do
    body = valid_body
    post "/orders/replicate", params: body, as: :json
    assert_response :created
    get "/orders/#{body[:order_id]}"
    assert_response :ok
    got = JSON.parse(response.body)
    assert_equal body[:order_id], got["order_id"]
    assert_equal "100.00", got["amount"]
    assert_equal 1, got["seq"]
  end

  test "正常系: X-Idempotency-Keyがevent_idより優先される" do
    body = valid_body.except(:event_id)
    key = SecureRandom.uuid_v7
    post "/orders/replicate", params: body, as: :json, headers: { "X-Idempotency-Key" => key }
    assert_response :created
    get "/orders/#{body[:order_id]}"
    assert_equal key, JSON.parse(response.body)["event_id"]
  end

  test "異常系: ヘッダとevent_idの不一致は400を返す" do
    post "/orders/replicate", params: valid_body, as: :json,
      headers: { "X-Idempotency-Key" => "different-key" }
    assert_response :bad_request
  end

  test "正常系: 重複イベントは200を返し状態は変わらない" do
    body = valid_body(amount: "5000.00")
    post "/orders/replicate", params: body, as: :json
    assert_response :created
    post "/orders/replicate", params: body.merge(amount: "1.00"), as: :json
    assert_response :ok
    get "/orders/#{body[:order_id]}"
    assert_equal "5000.00", JSON.parse(response.body)["amount"]
  end

  test "異常系: 検証エラーは400を返す" do
    post "/orders/replicate", params: valid_body(seq: 0), as: :json
    assert_response :bad_request
    post "/orders/replicate", params: valid_body(amount: "abc"), as: :json
    assert_response :bad_request
  end

  test "異常系: 存在しない注文の照会は404を返す" do
    get "/orders/00000000-0000-7000-8000-000000000000"
    assert_response :not_found
  end

  test "異常系: AUTH_TOKEN設定時は認証なしアクセスが401になる" do
    ENV["AUTH_TOKEN"] = "secret-token"
    post "/orders/replicate", params: valid_body, as: :json
    assert_response :unauthorized
    get "/healthz"
    assert_response :ok
  ensure
    ENV.delete("AUTH_TOKEN")
  end
end
