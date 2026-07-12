// delivery Lambda。SQS FIFOキューのメッセージをターゲットAPIへPOSTします。
// X-Idempotency-Keyにイベント IDを載せ、少なくとも1回配信の重複をターゲット側で排除できるようにします。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

// DeliveryMessage fanout Lambdaが送る配送メッセージです。
type DeliveryMessage struct {
	EventID     string `json:"event_id"`
	AggregateID string `json:"aggregate_id"`
	EventType   string `json:"event_type"`
	Payload     string `json:"payload"`
	Seq         int64  `json:"seq"`
}

// orderPayload outboxのペイロード（注文JSON）です。
type orderPayload struct {
	ID         string `json:"id"`
	CustomerID string `json:"customer_id"`
	Amount     string `json:"amount"`
	Status     string `json:"status"`
}

// replicateRequest ターゲットAPIへのリクエストボディです。
type replicateRequest struct {
	EventID    string `json:"event_id"`
	OrderID    string `json:"order_id"`
	CustomerID string `json:"customer_id"`
	Amount     string `json:"amount"`
	Status     string `json:"status"`
	Seq        int64  `json:"seq"`
}

// Handler delivery Lambdaの依存を保持します。
type Handler struct {
	client    *http.Client
	targetURL string
	authToken string
	logger    *slog.Logger
}

// HandleSQS SQSイベントを処理し、失敗メッセージを部分バッチ応答で返します。
// あるメッセージが失敗したら、同一MessageGroupIdの後続メッセージは処理せずに
// 失敗として返す。後続を先に配送すると同一集約内の適用順序が入れ替わるためで、
// 失敗として返した分はFIFOキューが順序を保ったまま再配信する。
// 再試行してもエラーが続くメッセージはmaxReceiveCount超過でDLQへ退避される。
//
// 設計判断: 先頭が恒久的に失敗する（毒メッセージ）場合、同一グループの後続も
// 受信回数を同時に消費するため、先頭と一緒にDLQへ退避される。これは「順序を
// 崩して後続だけ先に適用する」ことを避けるための意図的なトレードオフで、
// 後続はDLQに残るため失われない。原因解消後にDLQからの再投入で順序どおり
// 回復する運用を前提とする。
func (h *Handler) HandleSQS(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
	var failures []events.SQSBatchItemFailure
	failedGroups := map[string]bool{}
	for _, rec := range event.Records {
		group := rec.Attributes["MessageGroupId"]
		if group != "" && failedGroups[group] {
			h.logger.Info("同一グループの先行メッセージが失敗したため後続を保留します",
				"message_id", rec.MessageId, "group", group)
			failures = append(failures, events.SQSBatchItemFailure{ItemIdentifier: rec.MessageId})
			continue
		}
		if err := h.deliver(ctx, rec.Body); err != nil {
			h.logger.Error("配送に失敗しました", "error", err, "message_id", rec.MessageId)
			failures = append(failures, events.SQSBatchItemFailure{ItemIdentifier: rec.MessageId})
			if group != "" {
				failedGroups[group] = true
			}
		}
	}
	return events.SQSEventResponse{BatchItemFailures: failures}, nil
}

func (h *Handler) deliver(ctx context.Context, body string) error {
	var msg DeliveryMessage
	if err := json.Unmarshal([]byte(body), &msg); err != nil {
		return fmt.Errorf("配送メッセージの解析に失敗しました: %w", err)
	}
	var order orderPayload
	if err := json.Unmarshal([]byte(msg.Payload), &order); err != nil {
		return fmt.Errorf("注文ペイロードの解析に失敗しました: %w", err)
	}
	reqBody, err := json.Marshal(replicateRequest{
		EventID:    msg.EventID,
		OrderID:    order.ID,
		CustomerID: order.CustomerID,
		Amount:     order.Amount,
		Status:     order.Status,
		Seq:        msg.Seq,
	})
	if err != nil {
		return fmt.Errorf("リクエストのJSON変換に失敗しました: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.targetURL+"/orders/replicate", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("リクエストの生成に失敗しました: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Idempotency-Key", msg.EventID)
	if h.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+h.authToken)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("ターゲットAPIの呼び出しに失敗しました: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			h.logger.Error("レスポンスボディのクローズに失敗しました", "error", cerr)
		}
	}()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return fmt.Errorf("レスポンスボディの読み取りに失敗しました: %w", err)
	}

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		return nil
	}
	// 4xxを含む全ての異常応答を失敗として返す。無条件に破棄すると追跡不能な
	// イベント消失になるため、maxReceiveCount超過によるDLQ退避に一本化する
	return fmt.Errorf("ターゲットAPIが異常応答を返しました: status=%d body=%s", resp.StatusCode, string(respBody))
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	targetURL := os.Getenv("TARGET_API_URL")
	if targetURL == "" {
		logger.Error("TARGET_API_URLが未設定です")
		os.Exit(1)
	}
	h := &Handler{
		client:    &http.Client{Timeout: 10 * time.Second},
		targetURL: targetURL,
		authToken: os.Getenv("TARGET_API_TOKEN"),
		logger:    logger,
	}
	lambda.Start(h.HandleSQS)
}
