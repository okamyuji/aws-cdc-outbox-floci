package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/okamyuji/aws-cdc-outbox-floci/internal/envelope"
)

// fakeSQS SQSAPIのフェイク実装です。
type fakeSQS struct {
	sent []*sqs.SendMessageInput
	err  error
}

func (f *fakeSQS) SendMessage(_ context.Context, params *sqs.SendMessageInput, _ ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.sent = append(f.sent, params)
	return &sqs.SendMessageOutput{}, nil
}

func kinesisRecord(t *testing.T, seq string, row envelope.OutboxRow) events.KinesisEventRecord {
	t.Helper()
	rec, err := envelope.NewInsertRecord(row, time.Now())
	if err != nil {
		t.Fatalf("レコード生成に失敗しました: %v", err)
	}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("JSON変換に失敗しました: %v", err)
	}
	return events.KinesisEventRecord{
		Kinesis: events.KinesisRecord{Data: data, SequenceNumber: seq},
	}
}

func TestHandleKinesis(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	row := envelope.OutboxRow{
		ID: 1, EventID: "ev-1", AggregateID: "ord-1", EventType: "order.created",
		Payload: `{"id":"ord-1"}`, CreatedAt: time.Now().Format(time.RFC3339Nano),
	}

	t.Run("outboxのINSERTがFIFO属性付きでSQSへ転送される", func(t *testing.T) {
		client := &fakeSQS{}
		h := &Handler{client: client, queueURL: "http://queue", logger: logger}
		resp, err := h.HandleKinesis(context.Background(), events.KinesisEvent{
			Records: []events.KinesisEventRecord{kinesisRecord(t, "seq-1", row)},
		})
		if err != nil {
			t.Fatalf("エラーは想定外です: %v", err)
		}
		if len(resp.BatchItemFailures) != 0 {
			t.Errorf("失敗0件を期待しました: %+v", resp.BatchItemFailures)
		}
		if len(client.sent) != 1 {
			t.Fatalf("送信1件を期待しました: %d件", len(client.sent))
		}
		sent := client.sent[0]
		if *sent.MessageGroupId != "ord-1" || *sent.MessageDeduplicationId != "ev-1" {
			t.Errorf("FIFO属性が不正です: group=%s dedup=%s", *sent.MessageGroupId, *sent.MessageDeduplicationId)
		}
		var msg DeliveryMessage
		if err := json.Unmarshal([]byte(*sent.MessageBody), &msg); err != nil {
			t.Fatalf("メッセージの解析に失敗しました: %v", err)
		}
		if msg.Seq != 1 {
			t.Errorf("seqが伝播していません: %d", msg.Seq)
		}
	})

	t.Run("SQS送信失敗は部分バッチ失敗として返し、後続レコードを処理しない", func(t *testing.T) {
		client := &fakeSQS{err: errors.New("sqs down")}
		h := &Handler{client: client, queueURL: "http://queue", logger: logger}
		row2 := row
		row2.ID = 2
		row2.EventID = "ev-2"
		resp, err := h.HandleKinesis(context.Background(), events.KinesisEvent{
			Records: []events.KinesisEventRecord{
				kinesisRecord(t, "seq-1", row),
				kinesisRecord(t, "seq-2", row2),
			},
		})
		if err != nil {
			t.Fatalf("エラーは想定外です: %v", err)
		}
		if len(resp.BatchItemFailures) != 1 || resp.BatchItemFailures[0].ItemIdentifier != "seq-1" {
			t.Errorf("seq-1のみの失敗を期待しました: %+v", resp.BatchItemFailures)
		}
		// 失敗以降のレコードへ送信を試みると、再試行時に順序が崩れるため送信してはならない
		if len(client.sent) != 0 {
			t.Errorf("失敗後の送信は0件であるべきです: %d件", len(client.sent))
		}
	})

	t.Run("解析不能なレコードは部分バッチ失敗として返す", func(t *testing.T) {
		h := &Handler{client: &fakeSQS{}, queueURL: "http://queue", logger: logger}
		resp, err := h.HandleKinesis(context.Background(), events.KinesisEvent{
			Records: []events.KinesisEventRecord{
				{Kinesis: events.KinesisRecord{Data: []byte("{broken"), SequenceNumber: "seq-x"}},
			},
		})
		if err != nil {
			t.Fatalf("エラーは想定外です: %v", err)
		}
		if len(resp.BatchItemFailures) != 1 {
			t.Errorf("失敗1件を期待しました: %+v", resp.BatchItemFailures)
		}
	})

	t.Run("outbox以外のテーブルは送信せず成功扱い", func(t *testing.T) {
		client := &fakeSQS{}
		h := &Handler{client: client, queueURL: "http://queue", logger: logger}
		raw := []byte(`{"data":{},"metadata":{"record-type":"data","operation":"insert","table-name":"orders"}}`)
		resp, err := h.HandleKinesis(context.Background(), events.KinesisEvent{
			Records: []events.KinesisEventRecord{
				{Kinesis: events.KinesisRecord{Data: raw, SequenceNumber: "seq-2"}},
			},
		})
		if err != nil || len(resp.BatchItemFailures) != 0 || len(client.sent) != 0 {
			t.Errorf("読み飛ばしを期待しました: err=%v failures=%d sent=%d", err, len(resp.BatchItemFailures), len(client.sent))
		}
	})
}
