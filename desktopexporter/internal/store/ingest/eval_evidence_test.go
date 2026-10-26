package ingest

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/require"
)

func evalDeletionFixture(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "atomic-evidence.duckdb")
	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	// The cleanup uses only log identity; use the production Evals DDL and a
	// minimal log table to isolate transaction/failure behavior from ingestion.
	_, err = db.Exec(`create table logs(id uuid primary key, timestamp ubigint)`)
	require.NoError(t, err)
	for _, q := range queries.Tables() {
		if strings.HasSuffix(q.Name, "eval_ingestion.sql") || strings.HasSuffix(q.Name, "eval_experiments.sql") {
			_, err := db.Exec(q.SQL)
			require.NoError(t, err)
		}
	}
	for _, q := range []string{
		`insert into logs values ('00000000-0000-0000-0000-000000000001',1)`,
		`insert into eval_imports values ('import',true)`,
		`insert into eval_transfers values ('import',false,'')`,
		`insert into eval_evidence values ('00000000-0000-0000-0000-000000000001','import')`,
		`insert into eval_parts values ('import',0,0,'digest')`,
		`insert into eval_datasets(id,import_id,envelope,source,format,name,byte_size,row_count,context) values ('dataset','import',0,'json-import','promptfoo-v3','retained-name',1,1,'{"retained":"context"}')`,
		`insert into eval_records values ('import',0,0,1,'{"retained":"source"}')`,
		`insert into eval_experiment_indexes values ('dataset','dataset','ready','','{"retained":"index"}')`,
		`insert into eval_experiments values ('experiment','dataset','adapter','name','key','eval','json-import','promptfoo-v3',null,null,0,0,0,0,0,1)`,
		`insert into eval_experiment_rows values ('dataset',0,'experiment')`,
		`insert into eval_experiment_search values ('dataset',0,'experiment','retained-search')`,
	} {
		_, err := db.Exec(q)
		require.NoError(t, err)
	}
	return db, path
}

type interruptedEvalDeletion struct {
	*sql.Tx
	calls, after int
	cancel       context.CancelFunc
}

func (tx *interruptedEvalDeletion) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	result, err := tx.Tx.ExecContext(ctx, q, args...)
	tx.calls++
	if err == nil && tx.calls == tx.after {
		if tx.cancel != nil {
			tx.cancel()
		} else {
			return result, errors.New("synthetic failure between mutation steps")
		}
	}
	return result, err
}

var evalDeletionTables = []string{"logs", "eval_imports", "eval_transfers", "eval_evidence", "eval_parts", "eval_datasets", "eval_records", "eval_experiments", "eval_experiment_rows", "eval_experiment_search", "eval_experiment_indexes"}

func evalDeletionSnapshot(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, table := range evalDeletionTables {
		var raw string
		require.NoError(t, db.QueryRow(`select coalesce(to_json(list(t))::varchar,'[]') from (select * from `+table+` order by all) t`).Scan(&raw))
		out[table] = raw
	}
	return out
}

func TestEvalDeletionRollsBackCancellationAndFailures(t *testing.T) {
	for _, query := range []string{`truncate table logs`, `delete from logs where id='00000000-0000-0000-0000-000000000001'`, `delete from logs where timestamp<=1`} {
		for _, fault := range []struct {
			name   string
			after  int
			cancel bool
		}{{"cancel-after-delete", 1, true}, {"cancel-mid-cleanup", 4, true}, {"fail-mid-cleanup", 5, false}} {
			t.Run(query+"/"+fault.name, func(t *testing.T) {
				db, path := evalDeletionFixture(t)
				before := evalDeletionSnapshot(t, db)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				tx, err := db.BeginTx(ctx, nil)
				require.NoError(t, err)
				interrupted := &interruptedEvalDeletion{Tx: tx, after: fault.after}
				if fault.cancel {
					interrupted.cancel = cancel
				}
				require.Error(t, deleteLogsAndInvalidate(ctx, interrupted, query))
				require.GreaterOrEqual(t, interrupted.calls, fault.after)
				require.NoError(t, db.Close())
				reopened, err := sql.Open("duckdb", path)
				require.NoError(t, err)
				defer reopened.Close()
				require.Equal(t, before, evalDeletionSnapshot(t, reopened), "evidence and all caches must roll back together, including after reopen")
			})
		}
	}
}

func TestEvalDeletionCommitsInvalidation(t *testing.T) {
	db, path := evalDeletionFixture(t)
	require.NoError(t, DeleteLogs(t.Context(), db, `delete from logs`))
	require.NoError(t, db.Close())
	reopened, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	defer reopened.Close()
	for _, table := range []string{"logs", "eval_evidence", "eval_parts", "eval_records", "eval_experiments", "eval_experiment_rows", "eval_experiment_search", "eval_experiment_indexes"} {
		var n int
		require.NoError(t, reopened.QueryRow(`select count(*) from `+table).Scan(&n))
		require.Zero(t, n, table)
	}
	var complete, dirty bool
	var raw, name string
	require.NoError(t, reopened.QueryRow(`select (select complete from eval_imports),(select dirty from eval_transfers),(select context::varchar from eval_datasets),(select name from eval_datasets)`).Scan(&complete, &dirty, &raw, &name))
	require.False(t, complete)
	require.True(t, dirty)
	require.Equal(t, "{}", raw)
	require.Equal(t, "Imported evaluation", name)
}
