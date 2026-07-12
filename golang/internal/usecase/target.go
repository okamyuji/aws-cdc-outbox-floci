package usecase

import (
	"context"

	"github.com/okamyuji/aws-cdc-outbox-floci/internal/domain"
)

// ReplicationUsecase ターゲット側の注文反映ユースケースを抽象化します。
type ReplicationUsecase interface {
	// Replicate べき等に注文を反映します。重複はdomain.ErrDuplicateEventを返します。
	Replicate(ctx context.Context, in domain.ReplicatedOrder) error
	// GetOrder 反映済み注文を取得します。
	GetOrder(ctx context.Context, orderID string) (domain.ReplicatedOrder, error)
}

type replicationUsecase struct {
	repo domain.TargetRepository
}

// NewReplicationUsecase TargetRepositoryを注入してReplicationUsecaseを返します。
func NewReplicationUsecase(repo domain.TargetRepository) ReplicationUsecase {
	return &replicationUsecase{repo: repo}
}

func (u *replicationUsecase) Replicate(ctx context.Context, in domain.ReplicatedOrder) error {
	if err := in.Validate(); err != nil {
		return err
	}
	return u.repo.ReplicateOrder(ctx, in)
}

func (u *replicationUsecase) GetOrder(ctx context.Context, orderID string) (domain.ReplicatedOrder, error) {
	if orderID == "" {
		return domain.ReplicatedOrder{}, domain.ErrInvalidInput
	}
	return u.repo.FindOrder(ctx, orderID)
}
