package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func newFileStore(t *testing.T, path string) *Store {
	t.Helper()
	s, _ := openWithLogs(t, path)
	return s
}

// openWithLogs opens a store and captures whatever it logged while doing so, so
// tests can assert on the message a user would actually see rather than on an
// accessor that exists only for tests.
func openWithLogs(t *testing.T, path string) (*Store, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zap.WarnLevel)
	s, err := NewStore(context.Background(), path, zap.New(core))
	require.NoError(t, err)
	return s, logs
}

// A brand-new database gets stamped, so the next open recognises it rather than
// treating it as pre-versioning.
func TestNewDatabaseIsStampedAndClean(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")

	s, logs := openWithLogs(t, path)
	assert.Equal(t, SchemaOK, s.SchemaCompatibility())
	assert.Zero(t, logs.Len(), "a fresh database should warn about nothing")
	require.NoError(t, s.Close())

	reopened, reopenLogs := openWithLogs(t, path)
	defer reopened.Close()
	assert.Equal(t, SchemaOK, reopened.SchemaCompatibility())
	assert.Zero(t, reopenLogs.Len(), "reopening our own database should warn about nothing")
}

// In-memory stores are created fresh every time, so they are always clean.
func TestInMemoryStoreIsClean(t *testing.T) {
	s, logs := openWithLogs(t, "")
	defer s.Close()
	assert.Equal(t, SchemaOK, s.SchemaCompatibility())
	assert.Zero(t, logs.Len())
}

// A file stamped with a different version must be refused, not silently used.
//
// This was warn-only through the rewrite, while the schema was still moving.
// Now it is the thing standing between an incompatible file and the opaque
// failure it would otherwise produce -- an appender column-count error partway
// through an ingest, or an index built against a column that is not there.
func TestVersionMismatchIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.db")

	s := newFileStore(t, path)
	require.NoError(t, s.Close())

	// Rewrite the stamp to a version this build does not know.
	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	_, err = db.Exec(`delete from schema_meta`)
	require.NoError(t, err)
	_, err = db.Exec(`insert into schema_meta (version) values (?)`, schema.Version+1)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	core, logs := observer.New(zap.WarnLevel)
	reopened, err := NewStore(context.Background(), path, zap.New(core))
	require.Error(t, err, "an incompatible file must not open")
	require.ErrorIs(t, err, ErrSchemaIncompatible,
		"callers need to tell this apart from a genuine store failure")
	if reopened != nil {
		reopened.Close()
	}

	// The error has to say which file and what to do, since there is no
	// migration path -- the user's only move is to delete it or point --db
	// elsewhere.
	assert.Contains(t, err.Error(), "future.db", "name the file")
	assert.Contains(t, err.Error(), "--db", "say what to do about it")

	require.Equal(t, 1, logs.Len(), "and it must be logged, not only returned")
	fields := logs.All()[0].ContextMap()
	assert.Equal(t, int64(schema.Version+1), fields["file_version"])
	assert.Equal(t, int64(schema.Version), fields["expected_version"])
}

// Version 10 stored every exemplar value as DOUBLE. Pin that exact predecessor
// so this incompatible schema transition cannot lose its bump.
func TestVersionTenDatabaseIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v10.db")

	s := newFileStore(t, path)
	require.NoError(t, s.Close())

	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	_, err = db.Exec(`delete from schema_meta`)
	require.NoError(t, err)
	_, err = db.Exec(`insert into schema_meta (version) values (10)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, err = NewStore(context.Background(), path, zap.NewNop())
	require.ErrorIs(t, err, ErrSchemaIncompatible,
		"a database with DOUBLE-only exemplar values must be refused")
}

// Version 9 stored 8-byte span IDs as zero-padded UUIDs. Pin that exact
// predecessor so this incompatible schema transition cannot lose its bump.
func TestVersionNineDatabaseIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v9.db")

	s := newFileStore(t, path)
	require.NoError(t, s.Close())

	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	_, err = db.Exec(`delete from schema_meta`)
	require.NoError(t, err)
	_, err = db.Exec(`insert into schema_meta (version) values (9)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, err = NewStore(context.Background(), path, zap.NewNop())
	require.ErrorIs(t, err, ErrSchemaIncompatible,
		"a database with UUID-backed span IDs must be refused")
}

// A file holding data but carrying no stamp predates versioning, so its shape
// is unknown and it must be refused too.
//
// It must also not be stamped on the way out: stamping would assert a
// compatibility nobody checked and destroy the only evidence of where the file
// came from, so a second open would wrongly call it fine.
func TestPreVersioningDatabaseIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")

	// Build a database the way a pre-versioning build would leave it: real
	// tables with a row in spans, and no schema_meta at all.
	s := newFileStore(t, path)
	err := s.WithDBWrite(func(db *sql.DB) error {
		// spans.resource_id / scope_id are NOT NULL FKs, so the owner rows
		// have to exist first.
		if _, err := db.Exec(`
			insert into resources (id, attribute_ids)
			values ('eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee'::uuid, []::uuid[])`); err != nil {
			return err
		}
		if _, err := db.Exec(`
			insert into scopes (id, name, version, attribute_ids)
			values ('ffffffff-ffff-ffff-ffff-ffffffffffff'::uuid, '', '', []::uuid[])`); err != nil {
			return err
		}
		_, err := db.Exec(`
			insert into spans (trace_id, span_id, resource_id, scope_id, attribute_ids)
			values (?::uuid, 1::ubigint,
				'eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee'::uuid,
				'ffffffff-ffff-ffff-ffff-ffffffffffff'::uuid,
				[]::uuid[])`,
			"11111111-1111-1111-1111-111111111111")
		return err
	})
	require.NoError(t, err)
	require.NoError(t, s.Close())

	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	_, err = db.Exec(`drop table schema_meta`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	core, logs := observer.New(zap.WarnLevel)
	_, err = NewStore(context.Background(), path, zap.New(core))
	require.ErrorIs(t, err, ErrSchemaIncompatible)
	assert.Contains(t, err.Error(), "legacy.db")
	require.Equal(t, 1, logs.Len())

	// And its metadata table stays absent, so a second open reports the same
	// thing rather than quietly deciding the file is fine.
	raw, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	defer raw.Close()
	var schemaMetaTables int
	require.NoError(t, raw.QueryRow(schema.SchemaMetaTableExistsQuery).Scan(&schemaMetaTables))
	assert.Zero(t, schemaMetaTables, "a pre-versioning file must not be stamped on sight")
}

func TestIncompatibleDatabaseIsRejectedWithoutMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.db")
	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	_, err = db.Exec(schema.VersionTableQuery)
	require.NoError(t, err)
	_, err = db.Exec(schema.StampVersionQuery, schema.Version+1)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	before, err := os.ReadFile(path)
	require.NoError(t, err)
	_, err = NewStore(context.Background(), path, zap.NewNop())
	require.ErrorIs(t, err, ErrSchemaIncompatible)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after, "rejecting an incompatible database must not mutate its file")
}

func TestEmptySchemaMetadataIsRejectedWithoutMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty-schema-meta.db")
	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	_, err = db.Exec(schema.VersionTableQuery)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	before, err := os.ReadFile(path)
	require.NoError(t, err)
	_, err = NewStore(context.Background(), path, zap.NewNop())
	require.ErrorIs(t, err, ErrSchemaIncompatible)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after, "empty metadata must not be stamped as a fresh database")
}

func TestMalformedSchemaMetadataIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "malformed-schema-meta.db")
	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	_, err = db.Exec(`create table schema_meta (unexpected varchar)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	before, err := os.ReadFile(path)
	require.NoError(t, err)
	_, err = NewStore(context.Background(), path, zap.NewNop())
	require.ErrorIs(t, err, ErrSchemaIncompatible)
	assert.Contains(t, err.Error(), "malformed schema metadata")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after, "malformed metadata must remain untouched")
}

func TestSchemaMetadataWithCoercibleVersionIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "coercible-schema-meta.db")
	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	_, err = db.Exec(`create table schema_meta (version varchar, extra integer)`)
	require.NoError(t, err)
	_, err = db.Exec(`insert into schema_meta values ('12', 1)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, err = NewStore(context.Background(), path, zap.NewNop())
	require.ErrorIs(t, err, ErrSchemaIncompatible)
}

func TestUnversionedTelemetryDatabaseIsRejectedWithoutMutation(t *testing.T) {
	for table, columns := range map[string]string{
		"spans": `trace_id uuid, span_id uuid, resource_dropped_attributes_count uinteger,
			scope_dropped_attributes_count uinteger`,
		"logs": `trace_id uuid, observed_timestamp bigint, resource_dropped_attributes_count uinteger,
			scope_dropped_attributes_count uinteger`,
		"metric_ingests": `id uuid, stream_id uuid`,
	} {
		t.Run(table, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "legacy-empty.db")
			db, err := sql.Open("duckdb", path)
			require.NoError(t, err)
			_, err = db.Exec(`create table ` + table + ` (` + columns + `)`)
			require.NoError(t, err)
			require.NoError(t, db.Close())

			before, err := os.ReadFile(path)
			require.NoError(t, err)
			_, err = NewStore(context.Background(), path, zap.NewNop())
			require.ErrorIs(t, err, ErrSchemaIncompatible)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, before, after, "an unversioned telemetry database must remain untouched")
		})
	}
}

func TestCompatibleStampedDatabaseInitializes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compatible.db")
	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	_, err = db.Exec(schema.VersionTableQuery)
	require.NoError(t, err)
	_, err = db.Exec(schema.StampVersionQuery, schema.Version)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	s, err := NewStore(context.Background(), path, zap.NewNop())
	require.NoError(t, err)
	defer s.Close()
	assert.Equal(t, SchemaOK, s.SchemaCompatibility())
}

func TestConcurrentFirstOpenStampsOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	const openers = 4
	start := make(chan struct{})
	errs := make(chan error, openers)
	var wg sync.WaitGroup
	for range openers {
		wg.Go(func() {
			<-start
			s, err := NewStore(context.Background(), path, zap.NewNop())
			if err == nil {
				err = s.Close()
			}
			errs <- err
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	defer db.Close()
	var stamps int
	require.NoError(t, db.QueryRow(`select count(*) from schema_meta`).Scan(&stamps))
	assert.Equal(t, 1, stamps)
}

func TestUnrelatedDatabaseInitializes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unrelated.db")
	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	_, err = db.Exec(`create table metrics (id integer)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	s, err := NewStore(context.Background(), path, zap.NewNop())
	require.NoError(t, err)
	defer s.Close()
	assert.Equal(t, SchemaOK, s.SchemaCompatibility())

	var unrelatedTables int
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRow(`select count(*) from duckdb_tables() where table_name = 'metrics'`).Scan(&unrelatedTables)
	}))
	assert.Equal(t, 1, unrelatedTables)
}

func TestCustomSchemaNamesDoNotAffectCompatibilityInspection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom-schema.db")
	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	_, err = db.Exec(`create schema other`)
	require.NoError(t, err)
	_, err = db.Exec(`create table other.spans (id integer)`)
	require.NoError(t, err)
	_, err = db.Exec(`create table other.schema_meta (version integer)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	s, err := NewStore(context.Background(), path, zap.NewNop())
	require.NoError(t, err)
	defer s.Close()
	assert.Equal(t, SchemaOK, s.SchemaCompatibility())
}

// The version check has to run before the table and index loops.
//
// The scenario that makes this matter: a file whose `spans` table predates a
// column that a current index needs. `create table if not exists` leaves the old
// table alone, so index creation then fails against a column that is not there.
// Check first and the user gets a version message; check afterwards and they get
// "failed to create index N" with no hint about why.
//
// Mutation-checked: moving checkSchemaVersion below the table/index loops in
// NewStore makes this test fail. An earlier version of this test asserted only
// the returned compatibility, which survived that mutation and proved nothing.
func TestVersionCheckRunsBeforeTableCreation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "order.db")

	// A spans table shaped like an older schema: no service_name, which
	// idx_spans_service needs.
	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	_, err = db.Exec(`create table spans (trace_id uuid, span_id uuid primary key)`)
	require.NoError(t, err)
	_, err = db.Exec(`insert into spans values (?::uuid, ?::uuid)`,
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222")
	require.NoError(t, err)
	_, err = db.Exec(schema.VersionTableQuery)
	require.NoError(t, err)
	_, err = db.Exec(`insert into schema_meta (version) values (?)`, schema.Version+99)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	core, logs := observer.New(zap.WarnLevel)
	_, err = NewStore(context.Background(), path, zap.New(core))

	// The version check rejects the file before DDL can fail against its old
	// shape, so callers get the compatibility error rather than an index error.
	require.Error(t, err, "an incompatible file cannot be opened by this build")

	// The error remains logged with the database and remedy for users who start
	// the application from an environment that does not display returned errors.
	require.Equal(t, 1, logs.Len(),
		"the version warning must be logged before anything builds on the old schema")
	entry := logs.All()[0].ContextMap()
	assert.Contains(t, entry["database"], "order.db")
	assert.Contains(t, entry["remedy"], "--db")
}

// The upgrade path this bump exists for.
//
// Version 1 shipped on main, stamped onto the owner-keyed attributes schema.
// Had the dictionary rewrite kept Version = 1, an existing database would have
// matched on the number and been read as compatible -- and then failed as an
// appender column-count error partway through an ingest, or an index built
// against a column that no longer exists. Exactly the opaque failure the
// version check was built to prevent, defeated by not bumping.
//
// Deliberately hardcodes 1 rather than deriving it: the point is the specific
// version that exists in the wild, and a derived value would follow future
// bumps and stop testing anything.
func TestDatabaseFromPreviousReleaseIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v1.db")

	s := newFileStore(t, path)
	require.NoError(t, s.Close())

	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	_, err = db.Exec(`delete from schema_meta`)
	require.NoError(t, err)
	_, err = db.Exec(`insert into schema_meta (version) values (1)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, err = NewStore(context.Background(), path, zap.NewNop())
	require.ErrorIs(t, err, ErrSchemaIncompatible,
		"a database from the previous release must be refused, not silently reused")
	assert.Contains(t, err.Error(), "--db", "and must say what to do about it")
}
