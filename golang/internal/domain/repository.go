package domain

import "context"

// SourceRepository ソース側（注文+outbox）の永続化を抽象化します。
type SourceRepository interface {
	// CreateOrderWithOutbox 注文行とoutbox行を単一トランザクションで挿入します。
	CreateOrderWithOutbox(ctx context.Context, order Order, event OutboxEvent) error
	// FetchUnpublished 未発行のoutbox行をID昇順で最大limit件取得します。
	FetchUnpublished(ctx context.Context, limit int) ([]OutboxEvent, error)
	// MarkPublished 発行済みフラグを立てます。
	MarkPublished(ctx context.Context, ids []int64) error
}

// TargetRepository ターゲット側（反映先）の永続化を抽象化します。
type TargetRepository interface {
	// ReplicateOrder べき等キー記録と注文反映を単一トランザクションで行います。
	// 同じイベントIDが処理済みの場合はErrDuplicateEventを返します。
	ReplicateOrder(ctx context.Context, in ReplicatedOrder) error
	// FindOrder 反映済み注文を取得します。存在しない場合はErrNotFoundを返します。
	FindOrder(ctx context.Context, orderID string) (ReplicatedOrder, error)
}
