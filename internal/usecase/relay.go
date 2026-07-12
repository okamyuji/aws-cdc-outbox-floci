package usecase

import (
	"context"

	"github.com/okamyuji/aws-cdc-outbox-floci/internal/domain"
)

// EventPublisher outboxイベントのストリームへの発行を抽象化します。
// ローカル環境ではKinesisへのDMS互換レコード送信が実装になります。
type EventPublisher interface {
	// Publish イベント群を発行します。全件成功した場合のみnilを返します。
	Publish(ctx context.Context, events []domain.OutboxEvent) error
}

// RelayUsecase outboxの未発行イベントをストリームへ中継するユースケースです。
type RelayUsecase interface {
	// RelayOnce 未発行イベントを1バッチ分中継し、発行件数を返します。
	RelayOnce(ctx context.Context) (int, error)
}

type relayUsecase struct {
	repo      domain.SourceRepository
	publisher EventPublisher
	batchSize int
}

// NewRelayUsecase SourceRepositoryとEventPublisherを注入してRelayUsecaseを返します。
func NewRelayUsecase(repo domain.SourceRepository, publisher EventPublisher, batchSize int) RelayUsecase {
	if batchSize <= 0 {
		batchSize = 100
	}
	return &relayUsecase{repo: repo, publisher: publisher, batchSize: batchSize}
}

func (u *relayUsecase) RelayOnce(ctx context.Context) (int, error) {
	events, err := u.repo.FetchUnpublished(ctx, u.batchSize)
	if err != nil {
		return 0, err
	}
	if len(events) == 0 {
		return 0, nil
	}
	// 発行に失敗した場合はpublishedを更新せず、次回のポーリングで再送する（At-Least-Once）
	if err := u.publisher.Publish(ctx, events); err != nil {
		return 0, err
	}
	ids := make([]int64, 0, len(events))
	for _, e := range events {
		ids = append(ids, e.ID)
	}
	if err := u.repo.MarkPublished(ctx, ids); err != nil {
		return 0, err
	}
	return len(events), nil
}
