package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/schema"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// These synthetic steps exercise the runner without shipping a schema change.
// The second step depends on the first, and replaying either would fail.
func fixtureMigrations() []schema.Migration {
	return []schema.Migration{
		{Version: 28, Name: "028_copy.sql", SQL: `create table migration_samples as
			select span_id, name, attributes from spans`},
		{Version: 29, Name: "029_label.sql", SQL: `alter table migration_samples rename name to label;
			update migration_samples set label = upper(label);
			create index migration_samples_label on migration_samples(label)`},
	}
}

func populatedMigrationDatabase(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "populated.db")
	s := newFileStore(t, path)
	seedSpans(t, s, 3)
	seedLogs(t, s, 3)
	seedDatapoints(t, s, "11111111-1111-1111-1111-111111111111", 3, 0)
	// Include exact integers, bytes and null in native payloads.
	_, err := s.db.Exec(`update spans set attributes = {
		'large': 18446744073709551615::ubigint, 'bytes': '\x00\xFF'::blob,
		'nested': [1, NULL, 3]}::variant`)
	require.NoError(t, err)
	require.NoError(t, s.Close())
	return path
}

func openMigrationDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

func telemetrySnapshot(t *testing.T, db *sql.DB) []string {
	t.Helper()
	var snapshots []string
	for _, table := range []string{"spans", "logs", "metric_records"} {
		var snapshot string
		require.NoError(t, db.QueryRow(`select to_json(list(row_json order by row_json))::varchar from
			(select to_json(t)::varchar row_json from `+table+` t)`).Scan(&snapshot))
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

func TestMigrationsPreserveTelemetryAndDoNotReplay(t *testing.T) {
	path := populatedMigrationDatabase(t)
	db := openMigrationDB(t, path)
	before := telemetrySnapshot(t, db)
	compat, err := initializeSchema(context.Background(), db, path, zap.NewNop(), 29, fixtureMigrations())
	require.NoError(t, err)
	require.Equal(t, SchemaOK, compat)
	require.Equal(t, before, telemetrySnapshot(t, db))
	var copied int
	require.NoError(t, db.QueryRow(`select count(*) from migration_samples m join spans s using(span_id)
		where m.label = upper(s.name) and to_json(m.attributes) = to_json(s.attributes)`).Scan(&copied))
	require.Equal(t, 3, copied)
	var version int
	require.NoError(t, db.QueryRow(schema.ReadVersionQuery).Scan(&version))
	require.Equal(t, 29, version)
	require.NoError(t, db.Close())

	reopened := openMigrationDB(t, path)
	_, err = initializeSchema(context.Background(), reopened, path, zap.NewNop(), 29, fixtureMigrations())
	require.NoError(t, err, "migration SQL is deliberately not idempotent")
	require.Equal(t, before, telemetrySnapshot(t, reopened))

	// Compare the migrated tables, types and indexes to fresh target DDL.
	fresh := newFileStore(t, filepath.Join(t.TempDir(), "fresh.db"))
	t.Cleanup(func() { require.NoError(t, fresh.Close()) })
	_, err = fresh.db.Exec(`create table migration_samples(span_id ubigint, label varchar, attributes variant);
		create index migration_samples_label on migration_samples(label)`)
	require.NoError(t, err)
	for _, query := range []string{
		`select to_json(list(struct_pack(t:=table_name, c:=column_name, ty:=data_type, n:=is_nullable, d:=column_default)
			order by table_name, column_index))::varchar from duckdb_columns() where schema_name = 'main'`,
		`select to_json(list(sql order by index_name))::varchar from duckdb_indexes() where schema_name = 'main'`,
		`select to_json(list(struct_pack(n:=type_name, l:=logical_type) order by type_name))::varchar
			from duckdb_types() where schema_name = 'main' and not internal`,
	} {
		var expected, actual string
		require.NoError(t, fresh.db.QueryRow(query).Scan(&expected))
		require.NoError(t, reopened.QueryRow(query).Scan(&actual))
		require.JSONEq(t, expected, actual)
	}
}

func TestMigrationFailuresRollBackWholeChain(t *testing.T) {
	for _, failure := range []string{"migration", "current DDL", "version stamp"} {
		t.Run(failure, func(t *testing.T) {
			db := openMigrationDB(t, populatedMigrationDatabase(t))
			before := telemetrySnapshot(t, db)
			chain := fixtureMigrations()
			var err error
			if failure == "migration" {
				chain[1].SQL = `update spans set name = 'lost'; create table partial_result(id int);
					select * from missing_migration_input`
				_, err = initializeSchema(context.Background(), db, "fixture.db", zap.NewNop(), 29, chain)
				require.ErrorContains(t, err, "029_label.sql")
				require.ErrorIs(t, err, ErrStoreInitFailed)
			} else if failure == "current DDL" {
				err = applySchema(context.Background(), db, 27, 29, chain,
					[]queries.Statement{{Name: "broken.sql", SQL: "select * from missing_ddl_input"}})
				require.ErrorContains(t, err, "broken.sql")
			} else {
				_, err = db.Exec(`drop table schema_meta;
					create table schema_meta(version integer check(version = 27));
					insert into schema_meta values (27)`)
				require.NoError(t, err)
				_, err = initializeSchema(context.Background(), db, "fixture.db", zap.NewNop(), 29, chain)
				require.ErrorContains(t, err, "update schema version")
			}
			require.Equal(t, before, telemetrySnapshot(t, db))
			var version, partialTables int
			require.NoError(t, db.QueryRow(schema.ReadVersionQuery).Scan(&version))
			require.Equal(t, 27, version)
			require.NoError(t, db.QueryRow(`select count(*) from duckdb_tables()
				where table_name in ('migration_samples', 'partial_result')`).Scan(&partialTables))
			require.Zero(t, partialTables)
			if failure == "version stamp" {
				_, err = db.Exec(`drop table schema_meta; create table schema_meta(version integer);
					insert into schema_meta values (27)`)
				require.NoError(t, err)
			}
			_, err = initializeSchema(context.Background(), db, "fixture.db", zap.NewNop(), 29, fixtureMigrations())
			require.NoError(t, err, "a corrected upgrade can be retried")
		})
	}
}

func TestAmbiguousSchemaMetadataCannotBeMigrated(t *testing.T) {
	for _, values := range []string{"(27), (27)", "(27), (28)", "(NULL)"} {
		t.Run(values, func(t *testing.T) {
			db := openMigrationDB(t, populatedMigrationDatabase(t))
			_, err := db.Exec("delete from schema_meta; insert into schema_meta values " + values)
			require.NoError(t, err)
			_, err = initializeSchema(context.Background(), db, "fixture.db", zap.NewNop(), 29, fixtureMigrations())
			require.ErrorIs(t, err, ErrSchemaIncompatible)
			var tables int
			require.NoError(t, db.QueryRow(`select count(*) from duckdb_tables() where table_name = 'migration_samples'`).Scan(&tables))
			require.Zero(t, tables)
		})
	}
}

func TestFreshSchemaCreationRollsBackDDLAndStamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	db := openMigrationDB(t, path)
	ddl := append(queries.Types(), queries.Tables()...)
	ddl = append(ddl, queries.Statement{Name: "broken.sql", SQL: "select * from missing_ddl_input"})
	err := applySchema(context.Background(), db, 0, schema.Version, nil, ddl)
	require.ErrorContains(t, err, "broken.sql")
	var tables, types int
	require.NoError(t, db.QueryRow(`select count(*) from duckdb_tables() where not internal`).Scan(&tables))
	require.Zero(t, tables, "includes schema_meta")
	require.NoError(t, db.QueryRow(`select count(*) from duckdb_types() where not internal`).Scan(&types))
	require.Zero(t, types)
	_, err = initializeSchema(context.Background(), db, path, zap.NewNop(), schema.Version, nil)
	require.NoError(t, err)
}

func TestMissingMigrationRejectsBeforeMutation(t *testing.T) {
	path := populatedMigrationDatabase(t)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	db := openMigrationDB(t, path)
	_, err = initializeSchema(context.Background(), db, path, zap.NewNop(), 29, fixtureMigrations()[:1])
	require.ErrorContains(t, err, "incomplete migration chain")
	require.NoError(t, db.Close())
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
