require "test_helper"

class ReplicatedOrderTest < ActiveSupport::TestCase
  def valid_attrs(overrides = {})
    {
      event_id: SecureRandom.uuid_v7,
      order_id: SecureRandom.uuid_v7,
      customer_id: "cust-1",
      amount: "980.00",
      status: "created",
      seq: 1
    }.merge(overrides)
  end

  test "正常系: 反映した注文が取得できる" do
    attrs = valid_attrs
    ReplicatedOrder.replicate!(**attrs)
    order = ReplicatedOrder.find(attrs[:order_id])
    assert_equal attrs[:event_id], order.source_event_id
    assert_equal "980.00", format("%.2f", order.amount)
    assert_equal 1, order.source_seq
  end

  test "異常系: 同じイベントIDの2回目はDuplicateEventで反映されない" do
    attrs = valid_attrs
    ReplicatedOrder.replicate!(**attrs)
    assert_raises(ReplicatedOrder::DuplicateEvent) do
      ReplicatedOrder.replicate!(**attrs.merge(amount: "1.00"))
    end
    assert_equal "980.00", format("%.2f", ReplicatedOrder.find(attrs[:order_id]).amount)
  end

  test "正常系: 順序が逆転して届いた古いイベントでは上書きされない" do
    attrs = valid_attrs(seq: 20, status: "shipped")
    ReplicatedOrder.replicate!(**attrs)
    older = valid_attrs(order_id: attrs[:order_id], seq: 10, status: "created", amount: "1.00")
    assert_nothing_raised { ReplicatedOrder.replicate!(**older) }
    order = ReplicatedOrder.find(attrs[:order_id])
    assert_equal "shipped", order.status
    assert_equal 20, order.source_seq
    assert_equal "980.00", format("%.2f", order.amount)
  end

  test "エッジケース: 同一seqのイベントでは上書きされない" do
    attrs = valid_attrs(seq: 5)
    ReplicatedOrder.replicate!(**attrs)
    same = valid_attrs(order_id: attrs[:order_id], seq: 5, amount: "2.00")
    ReplicatedOrder.replicate!(**same)
    assert_equal "980.00", format("%.2f", ReplicatedOrder.find(attrs[:order_id]).amount)
  end

  test "境界値: seqは1以上の整数のみ受理される" do
    [ 0, -1, nil, "abc", "5", 2**63 ].each do |seq|
      assert_raises(ReplicatedOrder::InvalidInput, "seq=#{seq.inspect}") do
        ReplicatedOrder.replicate!(**valid_attrs(seq: seq))
      end
    end
    assert_nothing_raised { ReplicatedOrder.replicate!(**valid_attrs(seq: 2**63 - 1)) }
  end

  test "境界値: amountの受理と拒否" do
    [ "0", "0.01", "9999999999.99" ].each do |amount|
      assert_nothing_raised do
        ReplicatedOrder.replicate!(**valid_attrs(amount: amount))
      end
    end
    [ "12345678901", "1.234", "", "abc", "-1" ].each do |amount|
      assert_raises(ReplicatedOrder::InvalidInput, "amount=#{amount}") do
        ReplicatedOrder.replicate!(**valid_attrs(amount: amount))
      end
    end
  end

  test "異常系: 必須項目が欠けるとInvalidInput" do
    [ { event_id: "" }, { order_id: "" }, { customer_id: "" }, { status: "" } ].each do |override|
      assert_raises(ReplicatedOrder::InvalidInput, override.inspect) do
        ReplicatedOrder.replicate!(**valid_attrs(override))
      end
    end
  end
end
