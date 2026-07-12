// Package persistence domain層リポジトリのMySQL実装を提供します。
package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/okamyuji/aws-cdc-outbox-floci/internal/domain"
)

type sourceMySQL struct {
	db *sql.DB
}

// NewSourceMySQL MySQL実装のSourceRepositoryを返します。
func NewSourceMySQL(db *sql.DB) domain.SourceRepository {
	return &sourceMySQL{db: db}
}

func (r *sourceMySQL) CreateOrderWithOutbox(ctx context.Context, order domain.Order, event domain.OutboxEvent) error {
	return withTx(ctx, r.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO orders (id, customer_id, amount, status) VALUES (?, ?, ?, ?)`,
			order.ID, order.CustomerID, order.Amount, order.Status,
		); err != nil {
			return fmt.Errorf("注文の挿入に失敗しました: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO outbox (event_id, aggregate_id, event_type, payload) VALUES (?, ?, ?, ?)`,
			event.EventID, event.AggregateID, event.EventType, event.Payload,
		); err != nil {
			return fmt.Errorf("outboxの挿入に失敗しました: %w", err)
		}
		return nil
	})
}

func (r *sourceMySQL) FetchUnpublished(ctx context.Context, limit int) (events []domain.OutboxEvent, retErr error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, event_id, aggregate_id, event_type, payload, created_at
		   FROM outbox WHERE published = 0 ORDER BY id ASC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("未発行outboxの取得に失敗しました: %w", err)
	}
	defer closeRows(rows, &retErr)

	for rows.Next() {
		var e domain.OutboxEvent
		if err := rows.Scan(&e.ID, &e.EventID, &e.AggregateID, &e.EventType, &e.Payload, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("outbox行の読み取りに失敗しました: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("outbox行の走査に失敗しました: %w", err)
	}
	return events, nil
}

func (r *sourceMySQL) MarkPublished(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	query := fmt.Sprintf(`UPDATE outbox SET published = 1 WHERE id IN (%s)`, placeholders)
	if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("発行済みフラグの更新に失敗しました: %w", err)
	}
	return nil
}
