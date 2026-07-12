package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithBearerAuth(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	get := func(t *testing.T, url, authorization string) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("リクエストの生成に失敗しました: %v", err)
		}
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("リクエストに失敗しました: %v", err)
		}
		if err := resp.Body.Close(); err != nil {
			t.Errorf("ボディのクローズに失敗しました: %v", err)
		}
		return resp.StatusCode
	}

	t.Run("正常系: 正しいトークンで通過する", func(t *testing.T) {
		srv := httptest.NewServer(WithBearerAuth("secret-token", okHandler))
		defer srv.Close()
		if got := get(t, srv.URL+"/orders", "Bearer secret-token"); got != http.StatusOK {
			t.Errorf("200を期待しました: %d", got)
		}
	})

	t.Run("異常系: トークン不一致と未指定は401を返す", func(t *testing.T) {
		srv := httptest.NewServer(WithBearerAuth("secret-token", okHandler))
		defer srv.Close()
		for _, auth := range []string{"", "Bearer wrong", "Basic secret-token", "secret-token"} {
			if got := get(t, srv.URL+"/orders", auth); got != http.StatusUnauthorized {
				t.Errorf("Authorization=%qで401を期待しました: %d", auth, got)
			}
		}
	})

	t.Run("エッジケース: 401応答のContent-Typeはapplication/json", func(t *testing.T) {
		srv := httptest.NewServer(WithBearerAuth("secret-token", okHandler))
		defer srv.Close()
		resp, err := http.Get(srv.URL + "/orders")
		if err != nil {
			t.Fatalf("リクエストに失敗しました: %v", err)
		}
		if err := resp.Body.Close(); err != nil {
			t.Errorf("ボディのクローズに失敗しました: %v", err)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("application/jsonを期待しました: %s", ct)
		}
	})

	t.Run("エッジケース: healthzは認証なしで通過する", func(t *testing.T) {
		srv := httptest.NewServer(WithBearerAuth("secret-token", okHandler))
		defer srv.Close()
		if got := get(t, srv.URL+"/healthz", ""); got != http.StatusOK {
			t.Errorf("200を期待しました: %d", got)
		}
	})

	t.Run("エッジケース: トークン未設定なら認証なしで通す", func(t *testing.T) {
		srv := httptest.NewServer(WithBearerAuth("", okHandler))
		defer srv.Close()
		if got := get(t, srv.URL+"/orders", ""); got != http.StatusOK {
			t.Errorf("200を期待しました: %d", got)
		}
	})
}
