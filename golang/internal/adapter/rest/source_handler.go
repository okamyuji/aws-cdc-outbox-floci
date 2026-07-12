// Package rest usecase層のインターフェースにのみ依存するHTTPハンドラを提供します。
package rest

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/okamyuji/aws-cdc-outbox-floci/internal/domain"
	"github.com/okamyuji/aws-cdc-outbox-floci/internal/usecase"
)

// SourceHandler 注文APIのHTTPハンドラです。
type SourceHandler struct {
	uc     usecase.OrderUsecase
	logger *slog.Logger
}

// NewSourceHandler OrderUsecaseとロガーからSourceHandlerを組み立てます。
func NewSourceHandler(uc usecase.OrderUsecase, logger *slog.Logger) *SourceHandler {
	return &SourceHandler{uc: uc, logger: logger}
}

// Register ルーティングを登録します。
func (h *SourceHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /orders", h.createOrder)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func (h *SourceHandler) createOrder(w http.ResponseWriter, r *http.Request) {
	var in usecase.CreateOrderInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONError(w, h.logger, http.StatusBadRequest, "リクエストボディが不正です")
		return
	}
	order, err := h.uc.CreateOrder(r.Context(), in)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidInput) {
			writeJSONError(w, h.logger, http.StatusBadRequest, "入力値が不正です")
			return
		}
		h.logger.Error("注文作成に失敗しました", "error", err)
		writeJSONError(w, h.logger, http.StatusInternalServerError, "内部エラーが発生しました")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(order); err != nil {
		h.logger.Error("レスポンスの書き込みに失敗しました", "error", err)
	}
}

func writeJSONError(w http.ResponseWriter, logger *slog.Logger, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message}); err != nil {
		logger.Error("エラーレスポンスの書き込みに失敗しました", "error", err)
	}
}
