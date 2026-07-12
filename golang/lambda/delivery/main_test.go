package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
)

func sqsRecord(t *testing.T, messageID string) events.SQSMessage {
	t.Helper()
	body, err := json.Marshal(DeliveryMessage{
		EventID:     "ev-1",
		AggregateID: "ord-1",
		EventType:   "order.created",
		Payload:     `{"id":"ord-1","customer_id":"c1","amount":"100","status":"created"}`,
	})
	if err != nil {
		t.Fatalf("JSON変換に失敗しました: %v", err)
	}
	return events.SQSMessage{
		MessageId:  messageID,
		Body:       string(body),
		Attributes: map[string]string{"MessageGroupId": "ord-1"},
	}
}

func newHandler(targetURL string) *Handler {
	return &Handler{
		client:    &http.Client{Timeout: 2 * time.Second},
		targetURL: targetURL,
		logger:    slog.New(slog.NewJSONHandler(io.Discard, nil)),
	}
}

func TestHandleSQS(t *testing.T) {
	t.Run("ターゲットAPIへべき等キー付きでPOSTされる", func(t *testing.T) {
		var gotKey atomic.Value
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotKey.Store(r.Header.Get("X-Idempotency-Key"))
			var req replicateRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("リクエストの解析に失敗しました: %v", err)
			}
			if req.OrderID != "ord-1" || req.EventID != "ev-1" {
				t.Errorf("リクエスト内容が不正です: %+v", req)
			}
			w.WriteHeader(http.StatusCreated)
		}))
		defer srv.Close()

		h := newHandler(srv.URL)
		resp, err := h.HandleSQS(context.Background(), events.SQSEvent{
			Records: []events.SQSMessage{sqsRecord(t, "msg-1")},
		})
		if err != nil || len(resp.BatchItemFailures) != 0 {
			t.Fatalf("成功を期待しました: err=%v failures=%+v", err, resp.BatchItemFailures)
		}
		if gotKey.Load() != "ev-1" {
			t.Errorf("X-Idempotency-Keyが不正です: %v", gotKey.Load())
		}
	})

	t.Run("5xx応答は部分バッチ失敗として返し再配信に任せる", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		h := newHandler(srv.URL)
		resp, err := h.HandleSQS(context.Background(), events.SQSEvent{
			Records: []events.SQSMessage{sqsRecord(t, "msg-1")},
		})
		if err != nil {
			t.Fatalf("エラーは想定外です: %v", err)
		}
		if len(resp.BatchItemFailures) != 1 || resp.BatchItemFailures[0].ItemIdentifier != "msg-1" {
			t.Errorf("msg-1の失敗を期待しました: %+v", resp.BatchItemFailures)
		}
	})

	t.Run("4xx応答も部分バッチ失敗として返しDLQ退避に委ねる", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer srv.Close()

		h := newHandler(srv.URL)
		resp, err := h.HandleSQS(context.Background(), events.SQSEvent{
			Records: []events.SQSMessage{sqsRecord(t, "msg-1")},
		})
		if err != nil {
			t.Fatalf("エラーは想定外です: %v", err)
		}
		if len(resp.BatchItemFailures) != 1 {
			t.Errorf("失敗1件を期待しました: %+v", resp.BatchItemFailures)
		}
	})

	t.Run("同一グループの先行失敗後は後続を配送せず失敗として返す", func(t *testing.T) {
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		h := newHandler(srv.URL)
		resp, err := h.HandleSQS(context.Background(), events.SQSEvent{
			Records: []events.SQSMessage{sqsRecord(t, "msg-1"), sqsRecord(t, "msg-2")},
		})
		if err != nil {
			t.Fatalf("エラーは想定外です: %v", err)
		}
		if len(resp.BatchItemFailures) != 2 {
			t.Fatalf("2件の失敗を期待しました: %+v", resp.BatchItemFailures)
		}
		// 後続メッセージはターゲットAPIへ到達してはならない（順序逆転防止）
		if hits.Load() != 1 {
			t.Errorf("API呼び出しは1回であるべきです: %d回", hits.Load())
		}
	})

	t.Run("認証トークン設定時はAuthorizationヘッダが付与される", func(t *testing.T) {
		var gotAuth atomic.Value
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth.Store(r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusCreated)
		}))
		defer srv.Close()

		h := newHandler(srv.URL)
		h.authToken = "secret-token"
		if _, err := h.HandleSQS(context.Background(), events.SQSEvent{
			Records: []events.SQSMessage{sqsRecord(t, "msg-1")},
		}); err != nil {
			t.Fatalf("エラーは想定外です: %v", err)
		}
		if gotAuth.Load() != "Bearer secret-token" {
			t.Errorf("Authorizationヘッダが不正です: %v", gotAuth.Load())
		}
	})

	t.Run("接続不能は部分バッチ失敗として返す", func(t *testing.T) {
		h := newHandler("http://127.0.0.1:1")
		resp, err := h.HandleSQS(context.Background(), events.SQSEvent{
			Records: []events.SQSMessage{sqsRecord(t, "msg-1")},
		})
		if err != nil {
			t.Fatalf("エラーは想定外です: %v", err)
		}
		if len(resp.BatchItemFailures) != 1 {
			t.Errorf("失敗1件を期待しました: %+v", resp.BatchItemFailures)
		}
	})

	t.Run("不正なメッセージボディは部分バッチ失敗として返す", func(t *testing.T) {
		h := newHandler("http://127.0.0.1:1")
		resp, err := h.HandleSQS(context.Background(), events.SQSEvent{
			Records: []events.SQSMessage{{MessageId: "msg-x", Body: "{broken"}},
		})
		if err != nil {
			t.Fatalf("エラーは想定外です: %v", err)
		}
		if len(resp.BatchItemFailures) != 1 {
			t.Errorf("失敗1件を期待しました: %+v", resp.BatchItemFailures)
		}
	})
}
