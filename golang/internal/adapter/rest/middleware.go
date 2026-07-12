package rest

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// WithBearerAuth Bearerトークン認証ミドルウェアを返します。
// tokenが空の場合は認証なしで通します（ローカル検証用の明示的なオプトアウト）。
// /healthzは監視経路のため常に認証対象外です。
func WithBearerAuth(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			if _, err := w.Write([]byte(`{"error":"認証に失敗しました"}` + "\n")); err != nil {
				return
			}
			return
		}
		next.ServeHTTP(w, r)
	})
}
