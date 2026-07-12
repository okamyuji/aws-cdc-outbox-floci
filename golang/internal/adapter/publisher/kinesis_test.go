package publisher

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"

	"github.com/okamyuji/aws-cdc-outbox-floci/internal/domain"
	"github.com/okamyuji/aws-cdc-outbox-floci/internal/envelope"
)

// fakeKinesis KinesisAPIのフェイク実装です。
type fakeKinesis struct {
	inputs      []*kinesis.PutRecordsInput
	err         error
	failedCount int32
}

func (f *fakeKinesis) PutRecords(_ context.Context, params *kinesis.PutRecordsInput, _ ...func(*kinesis.Options)) (*kinesis.PutRecordsOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.inputs = append(f.inputs, params)
	return &kinesis.PutRecordsOutput{FailedRecordCount: aws.Int32(f.failedCount)}, nil
}

func TestKinesisPublisherPublish(t *testing.T) {
	events := []domain.OutboxEvent{
		{ID: 1, EventID: "ev-1", AggregateID: "ord-1", EventType: "order.created", Payload: `{"id":"ord-1"}`, CreatedAt: time.Now()},
		{ID: 2, EventID: "ev-2", AggregateID: "ord-2", EventType: "order.created", Payload: `{"id":"ord-2"}`, CreatedAt: time.Now()},
	}

	t.Run("DMS互換エンベロープと集約IDのパーティションキーで送信される", func(t *testing.T) {
		client := &fakeKinesis{}
		pub := NewKinesisPublisher(client, "outbox-stream")
		if err := pub.Publish(context.Background(), events); err != nil {
			t.Fatalf("エラーは想定外です: %v", err)
		}
		if len(client.inputs) != 1 || len(client.inputs[0].Records) != 2 {
			t.Fatalf("1回のPutRecordsで2レコードを期待しました")
		}
		rec := client.inputs[0].Records[0]
		if *rec.PartitionKey != "ord-1" {
			t.Errorf("パーティションキーが不正です: %s", *rec.PartitionKey)
		}
		row, ok, err := envelope.ParseOutboxInsert(rec.Data)
		if err != nil || !ok {
			t.Fatalf("エンベロープの解析に失敗しました: ok=%v err=%v", ok, err)
		}
		if row.EventID != "ev-1" {
			t.Errorf("イベントIDが不正です: %s", row.EventID)
		}
	})

	t.Run("API呼び出しの失敗はエラーになる", func(t *testing.T) {
		pub := NewKinesisPublisher(&fakeKinesis{err: errors.New("kinesis down")}, "outbox-stream")
		if err := pub.Publish(context.Background(), events); err == nil {
			t.Error("エラーを期待しました")
		}
	})

	t.Run("部分失敗もエラーになる", func(t *testing.T) {
		pub := NewKinesisPublisher(&fakeKinesis{failedCount: 1}, "outbox-stream")
		if err := pub.Publish(context.Background(), events); err == nil {
			t.Error("エラーを期待しました")
		}
	})

	t.Run("エンベロープはJSONとして往復可能", func(t *testing.T) {
		rec, err := envelope.NewInsertRecord(envelope.OutboxRow{EventID: "e", AggregateID: "a"}, time.Now())
		if err != nil {
			t.Fatalf("生成に失敗しました: %v", err)
		}
		if _, err := json.Marshal(rec); err != nil {
			t.Errorf("JSON変換に失敗しました: %v", err)
		}
	})
}
