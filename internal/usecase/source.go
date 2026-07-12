// Package usecase アプリケーションのユースケースを定義します。domain層の抽象にのみ依存します。
package usecase

import (
	"context"
	"time"

	"github.com/okamyuji/aws-cdc-outbox-floci/internal/domain"
)

// CreateOrderInput 注文作成の入力です。
type CreateOrderInput struct {
	CustomerID string `json:"customer_id"`
	Amount     string `json:"amount"`
}

// OrderUsecase 注文作成のユースケースを抽象化します。
type OrderUsecase interface {
	// CreateOrder 注文を作成し、同一トランザクションでoutboxイベントを記録します。
	CreateOrder(ctx context.Context, in CreateOrderInput) (domain.Order, error)
}

type orderUsecase struct {
	repo domain.SourceRepository
	now  func() time.Time
}

// NewOrderUsecase SourceRepositoryを注入してOrderUsecaseを返します。
func NewOrderUsecase(repo domain.SourceRepository) OrderUsecase {
	return &orderUsecase{repo: repo, now: time.Now}
}

func (u *orderUsecase) CreateOrder(ctx context.Context, in CreateOrderInput) (domain.Order, error) {
	order, err := domain.NewOrder(in.CustomerID, in.Amount, u.now())
	if err != nil {
		return domain.Order{}, err
	}
	event, err := domain.NewOrderCreatedEvent(order)
	if err != nil {
		return domain.Order{}, err
	}
	if err := u.repo.CreateOrderWithOutbox(ctx, order, event); err != nil {
		return domain.Order{}, err
	}
	return order, nil
}
