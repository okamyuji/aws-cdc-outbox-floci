package rest

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/okamyuji/aws-cdc-outbox-floci/internal/domain"
	"github.com/okamyuji/aws-cdc-outbox-floci/internal/usecase"
)

// TargetHandler ターゲットAPIのHTTPハンドラです。
type TargetHandler struct {
	uc     usecase.ReplicationUsecase
	logger *slog.Logger
}

// NewTargetHandler ReplicationUsecaseとロガーからTargetHandlerを組み立てます。
func NewTargetHandler(uc usecase.ReplicationUsecase, logger *slog.Logger) *TargetHandler {
	return &TargetHandler{uc: uc, logger: logger}
}

// Register ルーティングを登録します。
func (h *TargetHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /orders/replicate", h.replicate)
	mux.HandleFunc("GET /orders/{id}", h.getOrder)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func (h *TargetHandler) replicate(w http.ResponseWriter, r *http.Request) {
	var in domain.ReplicatedOrder
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONError(w, h.logger, http.StatusBadRequest, "リクエストボディが不正です")
		return
	}
	// X-Idempotency-Keyを優先し、ボディのevent_idと不一致なら拒否する
	if key := r.Header.Get("X-Idempotency-Key"); key != "" {
		if in.EventID != "" && in.EventID != key {
			writeJSONError(w, h.logger, http.StatusBadRequest, "X-Idempotency-Keyとevent_idが一致しません")
			return
		}
		in.EventID = key
	}
	if err := h.uc.Replicate(r.Context(), in); err != nil {
		switch {
		case errors.Is(err, domain.ErrDuplicateEvent):
			h.logger.Info("重複イベントを読み飛ばしました", "event_id", in.EventID)
			w.WriteHeader(http.StatusOK)
		case errors.Is(err, domain.ErrInvalidInput):
			writeJSONError(w, h.logger, http.StatusBadRequest, "入力値が不正です")
		default:
			h.logger.Error("注文反映に失敗しました", "error", err, "event_id", in.EventID)
			writeJSONError(w, h.logger, http.StatusInternalServerError, "内部エラーが発生しました")
		}
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (h *TargetHandler) getOrder(w http.ResponseWriter, r *http.Request) {
	out, err := h.uc.GetOrder(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrInvalidInput) {
			writeJSONError(w, h.logger, http.StatusNotFound, "注文が見つかりません")
			return
		}
		h.logger.Error("注文取得に失敗しました", "error", err)
		writeJSONError(w, h.logger, http.StatusInternalServerError, "内部エラーが発生しました")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		h.logger.Error("レスポンスの書き込みに失敗しました", "error", err)
	}
}
