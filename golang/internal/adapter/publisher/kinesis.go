// Package publisher EventPublisherのKinesis実装を提供します。
// DMSのKinesisターゲットと同じJSONエンベロープで送信し、下流を環境非依存にします。
package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	"github.com/aws/aws-sdk-go-v2/service/kinesis/types"

	"github.com/okamyuji/aws-cdc-outbox-floci/internal/domain"
	"github.com/okamyuji/aws-cdc-outbox-floci/internal/envelope"
	"github.com/okamyuji/aws-cdc-outbox-floci/internal/usecase"
)

// KinesisAPI 使用するKinesis操作を抽象化します。
type KinesisAPI interface {
	PutRecords(ctx context.Context, params *kinesis.PutRecordsInput, optFns ...func(*kinesis.Options)) (*kinesis.PutRecordsOutput, error)
}

type kinesisPublisher struct {
	client     KinesisAPI
	streamName string
	now        func() time.Time
}

// NewKinesisPublisher KinesisクライアントとストリームからEventPublisherを返します。
func NewKinesisPublisher(client KinesisAPI, streamName string) usecase.EventPublisher {
	return &kinesisPublisher{client: client, streamName: streamName, now: time.Now}
}

func (p *kinesisPublisher) Publish(ctx context.Context, events []domain.OutboxEvent) error {
	entries := make([]types.PutRecordsRequestEntry, 0, len(events))
	for _, e := range events {
		rec, err := envelope.NewInsertRecord(envelope.OutboxRow{
			ID:          e.ID,
			EventID:     e.EventID,
			AggregateID: e.AggregateID,
			EventType:   e.EventType,
			Payload:     e.Payload,
			CreatedAt:   e.CreatedAt.UTC().Format(time.RFC3339Nano),
		}, p.now())
		if err != nil {
			return err
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return fmt.Errorf("レコードのJSON変換に失敗しました: %w", err)
		}
		entries = append(entries, types.PutRecordsRequestEntry{
			Data: data,
			// 同一集約は同一シャードに載せて順序を守る
			PartitionKey: aws.String(e.AggregateID),
		})
	}
	out, err := p.client.PutRecords(ctx, &kinesis.PutRecordsInput{
		StreamName: aws.String(p.streamName),
		Records:    entries,
	})
	if err != nil {
		return fmt.Errorf("ストリームへの送信に失敗しました: %w", err)
	}
	if out.FailedRecordCount != nil && *out.FailedRecordCount > 0 {
		return fmt.Errorf("ストリームへの送信に%d件失敗しました", *out.FailedRecordCount)
	}
	return nil
}
