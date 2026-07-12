package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/okamyuji/aws-cdc-outbox-floci/internal/domain"
)

type targetMySQL struct {
	db *sql.DB
}

// NewTargetMySQL MySQL実装のTargetRepositoryを返します。
func NewTargetMySQL(db *sql.DB) domain.TargetRepository {
	return &targetMySQL{db: db}
}

func (r *targetMySQL) ReplicateOrder(ctx context.Context, in domain.ReplicatedOrder) error {
	return withTx(ctx, r.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`INSERT IGNORE INTO processed_events (event_id) VALUES (?)`, in.EventID)
		if err != nil {
			return fmt.Errorf("べき等キーの記録に失敗しました: %w", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("べき等キー記録結果の取得に失敗しました: %w", err)
		}
		if affected == 0 {
			return domain.ErrDuplicateEvent
		}
		// source_seq（ソースoutboxのID）が既存より大きいときだけ上書きし、
		// 順序が逆転して届いた古いイベントによる状態の巻き戻りを防ぐ。
		// source_seqの更新は最後に行い、各IFの比較が更新前の値を参照するようにする
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO orders_replica (id, customer_id, amount, status, source_event_id, source_seq)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON DUPLICATE KEY UPDATE
			   customer_id     = IF(VALUES(source_seq) > source_seq, VALUES(customer_id), customer_id),
			   amount          = IF(VALUES(source_seq) > source_seq, VALUES(amount), amount),
			   status          = IF(VALUES(source_seq) > source_seq, VALUES(status), status),
			   source_event_id = IF(VALUES(source_seq) > source_seq, VALUES(source_event_id), source_event_id),
			   source_seq      = IF(VALUES(source_seq) > source_seq, VALUES(source_seq), source_seq)`,
			in.OrderID, in.CustomerID, in.Amount, in.Status, in.EventID, in.Seq,
		); err != nil {
			return fmt.Errorf("注文の反映に失敗しました: %w", err)
		}
		return nil
	})
}

func (r *targetMySQL) FindOrder(ctx context.Context, orderID string) (domain.ReplicatedOrder, error) {
	var out domain.ReplicatedOrder
	err := r.db.QueryRowContext(ctx,
		`SELECT source_event_id, id, customer_id, amount, status, source_seq FROM orders_replica WHERE id = ?`,
		orderID,
	).Scan(&out.EventID, &out.OrderID, &out.CustomerID, &out.Amount, &out.Status, &out.Seq)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ReplicatedOrder{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.ReplicatedOrder{}, fmt.Errorf("注文の取得に失敗しました: %w", err)
	}
	return out, nil
}
