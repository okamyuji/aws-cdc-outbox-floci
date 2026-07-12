require "test_helper"

class OrderTest < ActiveSupport::TestCase
  test "正常系: 注文とoutboxイベントが同一トランザクションで作成される" do
    order = Order.create_with_outbox!(customer_id: "cust-1", amount: "1200.50")

    assert_equal "cust-1", order.customer_id
    assert_equal "created", order.status
    event = OutboxEvent.find_by!(aggregate_id: order.id)
    assert_equal "order.created", event.event_type
    payload = event.payload
    assert_equal order.id, payload["id"]
    assert_equal "1200.50", payload["amount"]
  end

  test "正常系: IDはUUIDv7形式で採番される" do
    order = Order.create_with_outbox!(customer_id: "cust-1", amount: "100")
    assert_match(/\A[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\z/, order.id)
  end

  test "境界値: amountの受理される境界" do
    [ "0", "0.01", "9999999999", "9999999999.99", "1.5" ].each do |amount|
      assert_nothing_raised do
        Order.create_with_outbox!(customer_id: "cust-1", amount: amount)
      end
    end
  end

  test "境界値: amountの拒否される境界" do
    [ "12345678901", "1.234", ".", ".5", "1." ].each do |amount|
      assert_raises(ActiveRecord::RecordInvalid, "amount=#{amount}") do
        Order.create_with_outbox!(customer_id: "cust-1", amount: amount)
      end
    end
  end

  test "異常系: amountが数値以外なら拒否される" do
    [ "", "abc", "-100", "1e3", "100円", " 100", "100 " ].each do |amount|
      assert_raises(ActiveRecord::RecordInvalid, "amount=#{amount}") do
        Order.create_with_outbox!(customer_id: "cust-1", amount: amount)
      end
    end
  end

  test "異常系: customer_idが空なら拒否される" do
    assert_raises(ActiveRecord::RecordInvalid) do
      Order.create_with_outbox!(customer_id: "", amount: "100")
    end
  end

  test "異常系: outboxの挿入が失敗すると注文もロールバックされる" do
    OutboxEvent.define_singleton_method(:create!) { |*| raise "outbox insert failed" }
    assert_raises(RuntimeError) do
      Order.create_with_outbox!(customer_id: "cust-rollback", amount: "100")
    end
    assert_not Order.exists?(customer_id: "cust-rollback")
  ensure
    OutboxEvent.singleton_class.remove_method(:create!)
  end

  test "エッジケース: 記号や日本語を含む顧客IDもペイロードで往復できる" do
    customer_id = %q(cust-"квота"-<注文>&')
    order = Order.create_with_outbox!(customer_id: customer_id, amount: "100")
    payload = OutboxEvent.find_by!(aggregate_id: order.id).payload
    assert_equal customer_id, payload["customer_id"]
  end
end
