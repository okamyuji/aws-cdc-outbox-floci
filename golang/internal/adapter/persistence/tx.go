package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// withTx トランザクション境界を管理します。fnがエラーを返した場合はロールバックし、
// ロールバック自体の失敗もerrors.Joinで呼び出し元へ伝えます。
func withTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("トランザクションの開始に失敗しました: %w", err)
	}
	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			return errors.Join(err, fmt.Errorf("ロールバックにも失敗しました: %w", rbErr))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("コミットに失敗しました: %w", err)
	}
	return nil
}

// closeRows rows.Closeの失敗を呼び出し元のエラーへ合成します。
func closeRows(rows *sql.Rows, retErr *error) {
	if err := rows.Close(); err != nil {
		*retErr = errors.Join(*retErr, fmt.Errorf("結果セットのクローズに失敗しました: %w", err))
	}
}
