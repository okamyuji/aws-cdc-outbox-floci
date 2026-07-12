require "test_helper"

# Kinesisクライアントのフェイク。送信内容を記録する
class FakeKinesis
  Response = Struct.new(:failed_record_count)

  attr_reader :calls

  def initialize(failed_record_count: 0, error: nil)
    @calls = []
    @failed_record_count = failed_record_count
    @error = error
  end

  def put_records(args)
    raise @error if @error

    @calls << args
    Response.new(@failed_record_count)
  end
end

class OutboxRelayTest < ActiveSupport::TestCase
  def create_order
    Order.create_with_outbox!(customer_id: "cust-1", amount: "100")
  end

  test "正常系: 未発行イベントがDMS互換エンベロープで送信され発行済みになる" do
    order = create_order
    kinesis = FakeKinesis.new
    relay = OutboxRelay.new(kinesis: kinesis, stream_name: "test-stream")

    assert_equal 1, relay.relay_once
    assert_empty OutboxEvent.unpublished_in_order.to_a

    record = kinesis.calls.first[:records].first
    assert_equal order.id, record[:partition_key]
    envelope = JSON.parse(record[:data])
    assert_equal "data", envelope["metadata"]["record-type"]
    assert_equal "insert", envelope["metadata"]["operation"]
    assert_equal "outbox", envelope["metadata"]["table-name"]
    assert_equal order.id, envelope["data"]["aggregate_id"]
    assert_kind_of Integer, envelope["data"]["id"]
    payload = JSON.parse(envelope["data"]["payload"])
    assert_equal order.id, payload["id"]
  end

  test "異常系: 送信に失敗したらpublishedを更新しない" do
    create_order
    relay = OutboxRelay.new(kinesis: FakeKinesis.new(error: StandardError.new("kinesis down")), stream_name: "s")
    assert_raises(StandardError) { relay.relay_once }
    assert_equal 1, OutboxEvent.unpublished_in_order.count
  end

  test "異常系: 部分失敗もエラーになり再送対象が残る" do
    create_order
    relay = OutboxRelay.new(kinesis: FakeKinesis.new(failed_record_count: 1), stream_name: "s")
    assert_raises(RuntimeError) { relay.relay_once }
    assert_equal 1, OutboxEvent.unpublished_in_order.count
  end

  test "エッジケース: 未発行が無ければ何もしない" do
    kinesis = FakeKinesis.new
    relay = OutboxRelay.new(kinesis: kinesis, stream_name: "s")
    assert_equal 0, relay.relay_once
    assert_empty kinesis.calls
  end
end
