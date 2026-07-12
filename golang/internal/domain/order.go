// Package domain エンティティとリポジトリの抽象を定義します。他層への依存を持ちません。
package domain

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// ErrDuplicateEvent 同じイベントIDを既に処理済みであることを表します。
var ErrDuplicateEvent = errors.New("処理済みのイベントです")

// ErrInvalidInput 入力値の検証に失敗したことを表します。
var ErrInvalidInput = errors.New("入力値が不正です")

// ErrNotFound 対象が存在しないことを表します。
var ErrNotFound = errors.New("対象が見つかりません")

var amountPattern = regexp.MustCompile(`^[0-9]{1,10}(\.[0-9]{1,2})?$`)

// Order 注文エンティティです。
type Order struct {
	ID         string    `json:"id"`
	CustomerID string    `json:"customer_id"`
	Amount     string    `json:"amount"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// NewOrder 入力を検証して注文エンティティを生成します。
func NewOrder(customerID, amount string, now time.Time) (Order, error) {
	if customerID == "" {
		return Order{}, fmt.Errorf("%w: customer_idは必須です", ErrInvalidInput)
	}
	if !amountPattern.MatchString(amount) {
		return Order{}, fmt.Errorf("%w: amountは正の数値文字列で指定します", ErrInvalidInput)
	}
	id, err := NewUUID()
	if err != nil {
		return Order{}, err
	}
	return Order{
		ID:         id,
		CustomerID: customerID,
		Amount:     amount,
		Status:     "created",
		CreatedAt:  now,
	}, nil
}

// OutboxEvent outboxテーブルへ書き込むイベントエンティティです。
type OutboxEvent struct {
	ID          int64
	EventID     string
	AggregateID string
	EventType   string
	Payload     string
	CreatedAt   time.Time
}

// NewOrderCreatedEvent 注文作成イベントを生成します。
func NewOrderCreatedEvent(order Order) (OutboxEvent, error) {
	if order.ID == "" {
		return OutboxEvent{}, fmt.Errorf("%w: 注文IDが空です", ErrInvalidInput)
	}
	eventID, err := NewUUID()
	if err != nil {
		return OutboxEvent{}, err
	}
	payload, err := json.Marshal(order)
	if err != nil {
		return OutboxEvent{}, fmt.Errorf("ペイロードのJSON変換に失敗しました: %w", err)
	}
	return OutboxEvent{
		EventID:     eventID,
		AggregateID: order.ID,
		EventType:   "order.created",
		Payload:     string(payload),
	}, nil
}

// ReplicatedOrder ターゲット側へ反映する注文です。
// Seqはソースoutboxテーブルの採番IDで、同一注文内のイベント適用順序の判定に使います。
type ReplicatedOrder struct {
	EventID    string `json:"event_id"`
	OrderID    string `json:"order_id"`
	CustomerID string `json:"customer_id"`
	Amount     string `json:"amount"`
	Status     string `json:"status"`
	Seq        int64  `json:"seq"`
}

// Validate 反映入力を検証します。
func (r ReplicatedOrder) Validate() error {
	if r.EventID == "" {
		return fmt.Errorf("%w: event_idは必須です", ErrInvalidInput)
	}
	if r.OrderID == "" {
		return fmt.Errorf("%w: order_idは必須です", ErrInvalidInput)
	}
	if r.CustomerID == "" || r.Status == "" {
		return fmt.Errorf("%w: customer_idとstatusは必須です", ErrInvalidInput)
	}
	if !amountPattern.MatchString(r.Amount) {
		return fmt.Errorf("%w: amountは正の数値文字列で指定します", ErrInvalidInput)
	}
	if r.Seq <= 0 {
		return fmt.Errorf("%w: seqは正の整数で指定します", ErrInvalidInput)
	}
	return nil
}

// NewUUID UUIDv7形式の識別子を生成します。
// 先頭48ビットがUNIXミリ秒のため時系列でほぼ昇順になり、
// InnoDBのクラスタ化インデックスへの挿入が末尾追記に近づいてページ分割を抑えられます。
func NewUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("乱数の生成に失敗しました: %w", err)
	}
	ms := uint64(time.Now().UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	b[6] = (b[6] & 0x0f) | 0x70
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
