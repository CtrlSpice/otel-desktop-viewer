package searchindex

import (
	"context"
	"database/sql"
)

type TransactionBeginner interface {
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
}

func DeleteRows(ctx context.Context, db TransactionBeginner, nativeSQL, indexSQL string, args ...any) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if indexSQL != "" {
		if _, err := tx.ExecContext(ctx, indexSQL, args...); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, nativeSQL, args...); err != nil {
		return err
	}
	if err := CollectGarbage(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}
