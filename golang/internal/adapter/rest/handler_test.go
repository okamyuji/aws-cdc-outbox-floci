package rest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/okamyuji/aws-cdc-outbox-floci/internal/domain"
	"github.com/okamyuji/aws-cdc-outbox-floci/internal/usecase"
)

var testLogger = slog.New(slog.NewJSONHandler(io.Discard, nil))

// fakeOrderUsecase OrderUsecaseのフェイク実装です。
type fakeOrderUsecase struct {
	order domain.Order
	err   error
}

func (f *fakeOrderUsecase) CreateOrder(_ context.Context, _ usecase.CreateOrderInput) (domain.Order, error) {
	return f.order, f.err
}

func TestSourceHandlerCreateOrder(t *testing.T) {
	newServer := func(uc usecase.OrderUsecase) *httptest.Server {
		mux := http.NewServeMux()
		NewSourceHandler(uc, testLogger).Register(mux)
		return httptest.NewServer(mux)
	}

	t.Run("正常な注文作成は201を返す", func(t *testing.T) {
		srv := newServer(&fakeOrderUsecase{order: domain.Order{ID: "ord-1"}})
		defer srv.Close()
		resp, err := http.Post(srv.URL+"/orders", "application/json",
			strings.NewReader(`{"customer_id":"c1","amount":"100"}`))
		if err != nil {
			t.Fatalf("リクエストに失敗しました: %v", err)
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Errorf("ボディのクローズに失敗しました: %v", err)
			}
		}()
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("201を期待しました: %d", resp.StatusCode)
		}
	})

	t.Run("不正なJSONは400を返す", func(t *testing.T) {
		srv := newServer(&fakeOrderUsecase{})
		defer srv.Close()
		resp, err := http.Post(srv.URL+"/orders", "application/json", strings.NewReader("{broken"))
		if err != nil {
			t.Fatalf("リクエストに失敗しました: %v", err)
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Errorf("ボディのクローズに失敗しました: %v", err)
			}
		}()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("400を期待しました: %d", resp.StatusCode)
		}
	})

	t.Run("検証エラーは400、その他のエラーは500", func(t *testing.T) {
		cases := []struct {
			err  error
			want int
		}{
			{domain.ErrInvalidInput, http.StatusBadRequest},
			{errors.New("db down"), http.StatusInternalServerError},
		}
		for _, c := range cases {
			srv := newServer(&fakeOrderUsecase{err: c.err})
			resp, err := http.Post(srv.URL+"/orders", "application/json",
				strings.NewReader(`{"customer_id":"c1","amount":"100"}`))
			if err != nil {
				t.Fatalf("リクエストに失敗しました: %v", err)
			}
			if resp.StatusCode != c.want {
				t.Errorf("%dを期待しました: %d", c.want, resp.StatusCode)
			}
			if err := resp.Body.Close(); err != nil {
				t.Errorf("ボディのクローズに失敗しました: %v", err)
			}
			srv.Close()
		}
	})

	t.Run("healthzは200を返す", func(t *testing.T) {
		srv := newServer(&fakeOrderUsecase{})
		defer srv.Close()
		resp, err := http.Get(srv.URL + "/healthz")
		if err != nil {
			t.Fatalf("リクエストに失敗しました: %v", err)
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Errorf("ボディのクローズに失敗しました: %v", err)
			}
		}()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("200を期待しました: %d", resp.StatusCode)
		}
	})
}

// fakeReplicationUsecase ReplicationUsecaseのフェイク実装です。
type fakeReplicationUsecase struct {
	replicateErr error
	getErr       error
	got          domain.ReplicatedOrder
	received     *domain.ReplicatedOrder
}

func (f *fakeReplicationUsecase) Replicate(_ context.Context, in domain.ReplicatedOrder) error {
	f.received = &in
	return f.replicateErr
}

func (f *fakeReplicationUsecase) GetOrder(_ context.Context, _ string) (domain.ReplicatedOrder, error) {
	return f.got, f.getErr
}

func TestTargetHandlerReplicate(t *testing.T) {
	newServer := func(uc usecase.ReplicationUsecase) *httptest.Server {
		mux := http.NewServeMux()
		NewTargetHandler(uc, testLogger).Register(mux)
		return httptest.NewServer(mux)
	}
	validBody := `{"event_id":"ev-1","order_id":"o1","customer_id":"c1","amount":"100","status":"created","seq":1}`

	post := func(t *testing.T, url, body, idempotencyKey string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, url+"/orders/replicate", strings.NewReader(body))
		if err != nil {
			t.Fatalf("リクエストの生成に失敗しました: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if idempotencyKey != "" {
			req.Header.Set("X-Idempotency-Key", idempotencyKey)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("リクエストに失敗しました: %v", err)
		}
		t.Cleanup(func() {
			if err := resp.Body.Close(); err != nil {
				t.Errorf("ボディのクローズに失敗しました: %v", err)
			}
		})
		return resp
	}

	t.Run("正常な反映は201を返す", func(t *testing.T) {
		srv := newServer(&fakeReplicationUsecase{})
		defer srv.Close()
		if resp := post(t, srv.URL, validBody, ""); resp.StatusCode != http.StatusCreated {
			t.Errorf("201を期待しました: %d", resp.StatusCode)
		}
	})

	t.Run("X-Idempotency-Keyがevent_idより優先される", func(t *testing.T) {
		uc := &fakeReplicationUsecase{}
		srv := newServer(uc)
		defer srv.Close()
		body := `{"order_id":"o1","customer_id":"c1","amount":"100","status":"created","seq":1}`
		if resp := post(t, srv.URL, body, "ev-header"); resp.StatusCode != http.StatusCreated {
			t.Fatalf("201を期待しました: %d", resp.StatusCode)
		}
		if uc.received == nil || uc.received.EventID != "ev-header" {
			t.Errorf("ヘッダのイベントIDが使われていません: %+v", uc.received)
		}
	})

	t.Run("ヘッダとevent_idの不一致は400を返す", func(t *testing.T) {
		srv := newServer(&fakeReplicationUsecase{})
		defer srv.Close()
		if resp := post(t, srv.URL, validBody, "different-key"); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("400を期待しました: %d", resp.StatusCode)
		}
	})

	t.Run("重複イベントは200を返す", func(t *testing.T) {
		srv := newServer(&fakeReplicationUsecase{replicateErr: domain.ErrDuplicateEvent})
		defer srv.Close()
		if resp := post(t, srv.URL, validBody, ""); resp.StatusCode != http.StatusOK {
			t.Errorf("200を期待しました: %d", resp.StatusCode)
		}
	})

	t.Run("検証エラーは400、その他のエラーは500", func(t *testing.T) {
		cases := []struct {
			err  error
			want int
		}{
			{domain.ErrInvalidInput, http.StatusBadRequest},
			{errors.New("db down"), http.StatusInternalServerError},
		}
		for _, c := range cases {
			srv := newServer(&fakeReplicationUsecase{replicateErr: c.err})
			if resp := post(t, srv.URL, validBody, ""); resp.StatusCode != c.want {
				t.Errorf("%dを期待しました: %d", c.want, resp.StatusCode)
			}
			srv.Close()
		}
	})

	t.Run("注文取得は200、未存在は404を返す", func(t *testing.T) {
		srv := newServer(&fakeReplicationUsecase{got: domain.ReplicatedOrder{OrderID: "o1"}})
		defer srv.Close()
		resp, err := http.Get(srv.URL + "/orders/o1")
		if err != nil {
			t.Fatalf("リクエストに失敗しました: %v", err)
		}
		if err := resp.Body.Close(); err != nil {
			t.Errorf("ボディのクローズに失敗しました: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("200を期待しました: %d", resp.StatusCode)
		}

		srv2 := newServer(&fakeReplicationUsecase{getErr: domain.ErrNotFound})
		defer srv2.Close()
		resp2, err := http.Get(srv2.URL + "/orders/missing")
		if err != nil {
			t.Fatalf("リクエストに失敗しました: %v", err)
		}
		if err := resp2.Body.Close(); err != nil {
			t.Errorf("ボディのクローズに失敗しました: %v", err)
		}
		if resp2.StatusCode != http.StatusNotFound {
			t.Errorf("404を期待しました: %d", resp2.StatusCode)
		}
	})
}
