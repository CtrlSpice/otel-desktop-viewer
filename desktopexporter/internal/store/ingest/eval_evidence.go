package ingest

import (
	"context"
	"database/sql"
)

// InvalidateEvalEvidence repairs missing references under the caller's write
// lock. Normal log deletion uses DeleteLogs to invalidate within its transaction.
// Source and search caches must not outlive the evidence they describe. Transfer
// identities and dataset tombstones survive so an intentional deletion does not
// become a cache miss that resurrects an earlier import.
func InvalidateEvalEvidence(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := invalidateEvalEvidence(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteLogs commits evidence deletion and every dependent invalidation together.
// Cancellation or any statement failure rolls back both sides of the boundary.
// The caller holds the store write lock; it must not already hold a transaction.
func DeleteLogs(ctx context.Context, db *sql.DB, query string, args ...any) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	return deleteLogsAndInvalidate(ctx, tx, query, args...)
}

type evalDeletionTx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	Commit() error
	Rollback() error
}

func deleteLogsAndInvalidate(ctx context.Context, tx evalDeletionTx, query string, args ...any) error {
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return err
	}
	if err := invalidateEvalEvidence(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func invalidateEvalEvidence(ctx context.Context, tx evalDeletionTx) error {
	const missing = `select distinct e.import_id from eval_evidence e where not exists(select 1 from logs l where l.id=e.log_id)`
	for _, q := range []string{
		`update eval_imports set complete=false where id in (` + missing + `)`,
		`update eval_transfers set dirty=true where id in (` + missing + `)`,
		`delete from eval_experiment_search where dataset_id in (select id from eval_datasets where import_id in (` + missing + `))`,
		`delete from eval_experiment_rows where dataset_id in (select id from eval_datasets where import_id in (` + missing + `))`,
		`delete from eval_experiments where dataset_id in (select id from eval_datasets where import_id in (` + missing + `))`,
		`delete from eval_experiment_indexes where dataset_id in (select id from eval_datasets where import_id in (` + missing + `))`,
		`delete from eval_records where import_id in (` + missing + `)`,
		`update eval_datasets set context='{}'::json::variant, name='Imported evaluation', row_count=0, byte_size=0 where import_id in (` + missing + `)`,
		`delete from eval_parts where import_id in (` + missing + `)`,
		`delete from eval_evidence where not exists(select 1 from logs l where l.id=log_id)`,
	} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}
