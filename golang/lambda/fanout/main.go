// fanout Lambda。KinesisのDMS互換レコードを解析し、SQS FIFOキューへ転送します。
// MessageGroupIdに集約ID、MessageDeduplicationIdにイベントIDを使い、順序と重複排除を担保します。
package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/okamyuji/aws-cdc-outbox-floci/internal/envelope"
)

// SQSAPI 使用するSQS操作を抽象化します。
type SQSAPI interface {
	SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
}

// Handler fanout Lambdaの依存を保持します。
type Handler struct {
	client   SQSAPI
	queueURL string
	logger   *slog.Logger
}

// DeliveryMessage SQSへ渡す配送メッセージです。
// Seqはソースoutboxの採番IDで、ターゲット側の順序逆行防止に使います。
type DeliveryMessage struct {
	EventID     string `json:"event_id"`
	AggregateID string `json:"aggregate_id"`
	EventType   string `json:"event_type"`
	Payload     string `json:"payload"`
	Seq         int64  `json:"seq"`
}

// HandleKinesis Kinesisイベントを処理し、最初の失敗レコードを部分バッチ応答で返します。
// Kinesisのイベントソースマッピングは報告したシーケンス番号以降を丸ごと再試行するため、
// 最初の失敗の時点で処理を打ち切る。後続を処理してしまうと、再試行時に同一集約の
// イベントが順序の入れ替わった状態でSQSへ二重投入される。
func (h *Handler) HandleKinesis(ctx context.Context, event events.KinesisEvent) (events.KinesisEventResponse, error) {
	for _, rec := range event.Records {
		row, ok, err := envelope.ParseOutboxInsert(rec.Kinesis.Data)
		if err != nil {
			h.logger.Error("レコードの解析に失敗しました", "error", err, "sequence", rec.Kinesis.SequenceNumber)
			return failFrom(rec), nil
		}
		if !ok {
			// outboxへのINSERT以外は対象外として読み飛ばす
			continue
		}
		body, err := json.Marshal(DeliveryMessage{
			EventID:     row.EventID,
			AggregateID: row.AggregateID,
			EventType:   row.EventType,
			Payload:     row.Payload,
			Seq:         row.ID,
		})
		if err != nil {
			h.logger.Error("メッセージのJSON変換に失敗しました", "error", err, "sequence", rec.Kinesis.SequenceNumber)
			return failFrom(rec), nil
		}
		if _, err := h.client.SendMessage(ctx, &sqs.SendMessageInput{
			QueueUrl:               aws.String(h.queueURL),
			MessageBody:            aws.String(string(body)),
			MessageGroupId:         aws.String(row.AggregateID),
			MessageDeduplicationId: aws.String(row.EventID),
		}); err != nil {
			h.logger.Error("SQSへの送信に失敗しました", "error", err, "event_id", row.EventID)
			return failFrom(rec), nil
		}
	}
	return events.KinesisEventResponse{}, nil
}

// failFrom 指定レコード以降の再試行を要求する部分バッチ応答を返します。
func failFrom(rec events.KinesisEventRecord) events.KinesisEventResponse {
	return events.KinesisEventResponse{
		BatchItemFailures: []events.KinesisBatchItemFailure{
			{ItemIdentifier: rec.Kinesis.SequenceNumber},
		},
	}
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	queueURL := os.Getenv("DELIVERY_QUEUE_URL")
	if queueURL == "" {
		logger.Error("DELIVERY_QUEUE_URLが未設定です")
		os.Exit(1)
	}
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		logger.Error("AWS設定の読み込みに失敗しました", "error", err)
		os.Exit(1)
	}
	h := &Handler{client: sqs.NewFromConfig(cfg), queueURL: queueURL, logger: logger}
	lambda.Start(h.HandleKinesis)
}
