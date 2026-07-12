require "test_helper"

class OutboxEventTest < ActiveSupport::TestCase
  def create_event(aggregate_id)
    OutboxEvent.create!(
      event_id: SecureRandom.uuid_v7,
      aggregate_id: aggregate_id,
      event_type: "order.created",
      payload: { id: aggregate_id }.to_json
    )
  end

  test "正常系: 未発行イベントがID昇順で取得される" do
    events = 3.times.map { |i| create_event("ord-#{i}") }
    got = OutboxEvent.unpublished_in_order.limit(10)
    assert_equal events.map(&:id), got.map(&:id)
  end

  test "正常系: 発行済みにすると未発行一覧から消える" do
    event = create_event("ord-1")
    OutboxEvent.mark_published!([ event.id ])
    assert_empty OutboxEvent.unpublished_in_order.to_a
  end

  test "エッジケース: 空のID配列では何も更新しない" do
    assert_nothing_raised { OutboxEvent.mark_published!([]) }
  end

  test "異常系: 同じevent_idはUNIQUE制約で拒否される" do
    event = create_event("ord-1")
    assert_raises(ActiveRecord::RecordNotUnique) do
      OutboxEvent.create!(
        event_id: event.event_id,
        aggregate_id: "ord-2",
        event_type: "order.created",
        payload: "{}"
      )
    end
  end
end
