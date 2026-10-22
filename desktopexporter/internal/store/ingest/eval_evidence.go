package ingest

import (
	"context"
	"database/sql"
)

// InvalidateEvalEvidence runs under the caller's write lock after log deletion.
// Source and search caches must not outlive the evidence they describe. Transfer
// identities and dataset tombstones survive so an intentional deletion does not
// become a cache miss that resurrects an earlier import.
func InvalidateEvalEvidence(ctx context.Context, db *sql.DB) error {
	const missing = `select distinct e.import_id from eval_evidence e where not exists(select 1 from logs l where l.id=e.log_id)`
	for _, q := range []string{
		`update eval_imports set complete=false where id in (` + missing + `)`,
		`update eval_transfers set dirty=true where id in (` + missing + `)`,
		`delete from eval_experiment_search where dataset_id in (select id from eval_datasets where import_id in (` + missing + `))`,
		`delete from eval_experiment_rows where dataset_id in (select id from eval_datasets where import_id in (` + missing + `))`,
		`delete from eval_experiments where dataset_id in (select id from eval_datasets where import_id in (` + missing + `))`,
		`delete from eval_experiment_indexes where dataset_id in (select id from eval_datasets where import_id in (` + missing + `))`,
		`delete from eval_records where import_id in (` + missing + `)`,
		`update eval_datasets set context='{}', name='Imported evaluation', row_count=0, byte_size=0 where import_id in (` + missing + `)`,
		`delete from eval_parts where import_id in (` + missing + `)`,
		`delete from eval_evidence where not exists(select 1 from logs l where l.id=log_id)`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}
