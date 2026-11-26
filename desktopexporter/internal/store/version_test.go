package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

func TestSchemaInitializationHonorsCancellation(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = initializeSchema(ctx, db, "", zap.NewNop(), schema.Version, nil)
	require.ErrorIs(t, err, context.Canceled)

	var schemaMetaTables int
	require.NoError(t, db.QueryRow(schema.SchemaMetaTableExistsQuery).Scan(&schemaMetaTables))
	assert.Zero(t, schemaMetaTables)
}

// A file stamped with a different version must be refused before ingest or DDL.
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

	// A newer file requires a newer executable, not a destructive reset.
	assert.Contains(t, err.Error(), "future.db", "name the file")
	assert.Contains(t, err.Error(), "--db", "say what to do about it")
	assert.Contains(t, err.Error(), "upgrade the viewer")

	require.Equal(t, 1, logs.Len(), "and it must be logged, not only returned")
	fields := logs.All()[0].ContextMap()
	assert.Equal(t, int64(schema.Version+1), fields["file_version"])
	assert.Equal(t, int64(schema.Version), fields["expected_version"])
}

func TestOlderDatabasesAreRefusedWithoutModification(t *testing.T) {
	for _, version := range []int{21, 22, 23, 24, 25, 26} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), fmt.Sprintf("v%d.db", version))
			s := newFileStore(t, path)
			require.NoError(t, s.Close())
			db, err := sql.Open("duckdb", path)
			require.NoError(t, err)
			_, err = db.Exec(`update schema_meta set version = ?`, version)
			require.NoError(t, err)
			require.NoError(t, db.Close())
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			_, err = NewStore(context.Background(), path, zap.NewNop())
			require.ErrorIs(t, err, ErrSchemaIncompatible)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, before, after)
			assert.Greater(t, schema.Version, version)
		})
	}
}

// Version 15 wrote zero for both absent and supplied-zero histogram statistics.
// Pin that exact predecessor because those states cannot be reconstructed.
func TestVersionFifteenDatabaseIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v15.db")

	s := newFileStore(t, path)
	require.NoError(t, s.Close())

	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	_, err = db.Exec(`delete from schema_meta`)
	require.NoError(t, err)
	_, err = db.Exec(`insert into schema_meta (version) values (15)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, err = NewStore(context.Background(), path, zap.NewNop())
	require.ErrorIs(t, err, ErrSchemaIncompatible,
		"a database with ambiguous histogram optional statistics must be refused")
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
	seedOwners(t, s)
	err := s.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec(`
			insert into spans (trace_id, span_id, resource, scope, attributes)
			values (?::uuid, 1::ubigint, seed_resource(), seed_scope(), '{}'::json::variant)`,
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

func TestVersion19DatabaseIsRejectedWithoutMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "version-19.db")
	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	_, err = db.Exec(schema.VersionTableQuery)
	require.NoError(t, err)
	_, err = db.Exec(schema.StampVersionQuery, 19)
	require.NoError(t, err)
	_, err = db.Exec(`create table telemetry_sample (id uuid primary key, schema_url varchar)`)
	require.NoError(t, err)
	_, err = db.Exec(`insert into telemetry_sample values (uuid(), 'https://example.test/resource/v1')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	before, err := os.ReadFile(path)
	require.NoError(t, err)
	_, err = NewStore(context.Background(), path, zap.NewNop())
	require.ErrorIs(t, err, ErrSchemaIncompatible)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after, "rejecting a version-19 database must not rewrite stored telemetry")
}

func TestVersion17DatabaseIsRejectedWithoutMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "version-17.db")
	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	_, err = db.Exec(schema.VersionTableQuery)
	require.NoError(t, err)
	_, err = db.Exec(schema.StampVersionQuery, 17)
	require.NoError(t, err)
	_, err = db.Exec(`create table telemetry_sample (id uuid, parent_id uuid, payload blob)`)
	require.NoError(t, err)
	_, err = db.Exec(`insert into telemetry_sample values (uuid(), uuid(), ?::blob)`, []byte{0, 1, 2, 255})
	require.NoError(t, err)
	require.NoError(t, db.Close())

	before, err := os.ReadFile(path)
	require.NoError(t, err)
	_, err = NewStore(context.Background(), path, zap.NewNop())
	require.ErrorIs(t, err, ErrSchemaIncompatible)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after, "rejecting a version-17 database must not rewrite stored telemetry")
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
	_, err = db.Exec(`create table business_metrics (id integer)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	s, err := NewStore(context.Background(), path, zap.NewNop())
	require.NoError(t, err)
	defer s.Close()
	assert.Equal(t, SchemaOK, s.SchemaCompatibility())

	var unrelatedTables int
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRow(`select count(*) from duckdb_tables() where table_name = 'business_metrics'`).Scan(&unrelatedTables)
	}))
	assert.Equal(t, 1, unrelatedTables)
}

func TestAutomaticOpenPreservesUnrelatedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "current.duckdb")
	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	_, err = db.Exec(`create macro attr_id(k, v) as 'mine'; create table schema_meta(version integer); insert into schema_meta values (27)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	for _, createDefault := range []bool{false, true} {
		// true simulates another file appearing after path resolution.
		_, err = OpenViewerDatabase(t.Context(), path, createDefault, nil)
		require.ErrorIs(t, err, ErrSchemaIncompatible)
		require.ErrorContains(t, err, path)
		after, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, before, after)
	}
	db, err = sql.Open("duckdb", path)
	require.NoError(t, err)
	defer db.Close()
	var result string
	require.NoError(t, db.QueryRow(`select attr_id('k', 'v')`).Scan(&result))
	require.Equal(t, "mine", result)
}

func TestManagedReopenRecoversCommittedRowUnderOriginalCatalog(t *testing.T) {
	const env = "VIEWER_TEST_CRASH_DATABASE"
	ctx := t.Context()
	if path := os.Getenv(env); path != "" {
		s, err := OpenViewerDatabase(ctx, path, false, nil)
		require.NoError(t, err)
		_, err = s.db.ExecContext(ctx, `insert into capture.main.capture_marker values (42)`)
		require.NoError(t, err)
		os.Exit(0) // Skip store shutdown and leave the committed row in the WAL.
	}
	path := filepath.Join(t.TempDir(), "capture.duckdb")
	s := newFileStore(t, path)
	_, err := s.db.ExecContext(ctx, `create table capture.main.capture_marker(value integer)`)
	require.NoError(t, err)
	require.NoError(t, s.Close())
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestManagedReopenRecoversCommittedRowUnderOriginalCatalog$")
	cmd.Env = append(os.Environ(), env+"="+path)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	wal, err := os.Stat(path + ".wal")
	require.NoError(t, err)
	require.Positive(t, wal.Size())
	s, err = OpenViewerDatabase(ctx, path, false, nil)
	require.NoError(t, err)
	defer s.Close()
	var value int
	require.NoError(t, s.db.QueryRowContext(ctx, `select value from capture.main.capture_marker`).Scan(&value))
	require.Equal(t, 42, value)
}

func TestExplicitOpenDoesNotCreateAFileAtAnotherSpelling(t *testing.T) {
	if filepath.Separator == '\\' {
		t.Skip("backslash is the native directory separator")
	}
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "build"), 0700))
	literal := filepath.Join(dir, `build\run.duckdb`)
	_, err := OpenExplicitDatabase(t.Context(), literal, nil)
	require.ErrorContains(t, err, "backslash")
	require.NoFileExists(t, literal)
	files, err := os.ReadDir(filepath.Join(dir, "build"))
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestPublishNewDatabaseRefusesExistingFile(t *testing.T) {
	dir := t.TempDir()
	staged := filepath.Join(dir, "staged")
	path := filepath.Join(dir, "capture.duckdb")
	require.NoError(t, os.WriteFile(staged, []byte("first complete capture"), 0600))
	require.NoError(t, publishNewDatabase(staged, path))
	other := filepath.Join(dir, "other")
	require.NoError(t, os.WriteFile(other, []byte("second complete capture"), 0600))
	require.ErrorIs(t, publishNewDatabase(other, path), os.ErrExist)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "first complete capture", string(got))
}

func TestResumeDoesNotRecreateDatabaseMovedAfterInspection(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	if runtime.GOOS != "windows" {
		alias := filepath.Join(t.TempDir(), "alias")
		require.NoError(t, os.Symlink(dir, alias))
		dir = alias
	}
	path := filepath.Join(dir, "remembered.duckdb")
	moved := filepath.Join(dir, "moved.duckdb")
	s := newFileStore(t, path)
	require.NoError(t, s.Close())

	// Exercise the two production steps in order, moving the file exactly
	// between the read-only inspection and the writable open.
	engine, err := newCaptureEngine(ctx, path)
	require.NoError(t, err)
	defer engine.control.Close()
	pin, err := engine.inspectFile(ctx, path)
	require.NoError(t, err)
	require.NoError(t, os.Rename(path, moved))
	journal := []byte("existing journal must not be removed")
	require.NoError(t, os.WriteFile(path+".wal", journal, 0600))
	original, err := os.ReadFile(moved)
	require.NoError(t, err)
	s, err = openStore(ctx, path, nil, pin)
	if s != nil {
		require.NoError(t, s.Close())
	}
	require.ErrorContains(t, err, path)
	_, err = os.Lstat(path)
	require.ErrorIs(t, err, os.ErrNotExist, "resume created a replacement database")
	remaining, err := os.ReadFile(path + ".wal")
	require.NoError(t, err)
	require.Equal(t, journal, remaining)
	after, err := os.ReadFile(moved)
	require.NoError(t, err)
	require.Equal(t, original, after)
}

func TestResumeRejectsReplacementBeforeJournalRecovery(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	path := filepath.Join(dir, "remembered.duckdb")
	other := filepath.Join(dir, "other.duckdb")
	require.NoError(t, newFileStore(t, path).Close())
	require.NoError(t, newFileStore(t, other).Close())
	engine, err := newCaptureEngine(ctx, path)
	require.NoError(t, err)
	defer engine.control.Close()
	pin, err := engine.inspectFile(ctx, path)
	require.NoError(t, err)
	require.NoError(t, os.Rename(path, path+".moved"))
	require.NoError(t, os.Rename(other, path))
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	journal := []byte("replacement journal must remain unchanged")
	require.NoError(t, os.WriteFile(path+".wal", journal, 0600))
	s, err := openStore(ctx, path, nil, pin)
	if s != nil {
		require.NoError(t, s.Close())
	}
	require.ErrorContains(t, err, "database file changed")
	require.ErrorContains(t, err, path)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
	remaining, err := os.ReadFile(path + ".wal")
	require.NoError(t, err)
	require.Equal(t, journal, remaining)
}

func TestPhysicalFileHasOneWriterAcrossNames(t *testing.T) {
	for _, mode := range []string{"direct", "resume", "literal percent"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "original.duckdb")
			if mode == "literal percent" {
				path = filepath.Join(dir, "100% original.duckdb")
			}
			require.NoError(t, newFileStore(t, path).Close())
			alias := filepath.Join(dir, "alias.duckdb")
			require.NoError(t, os.Link(path, alias))
			var active *Store
			var err error
			if mode == "resume" {
				active, err = OpenViewerDatabase(t.Context(), path, false, nil)
			} else {
				active, err = NewStore(t.Context(), path, nil)
			}
			require.NoError(t, err)
			defer active.Close()
			before, changed, _ := active.CommitSnapshot()
			require.NoError(t, active.SelectDatabase(t.Context(), alias, func() error {
				return errors.New("must not publish a second name for an open database")
			}))
			after, stillWatching, _ := active.CommitSnapshot()
			require.Equal(t, before, after)
			require.True(t, changed == stillWatching)
			_, err = NewStore(t.Context(), alias, nil)
			require.ErrorIs(t, err, ErrFileInUse)
			_, err = OpenViewerDatabase(t.Context(), alias, false, nil)
			require.ErrorIs(t, err, ErrFileInUse)
			if mode == "resume" {
				checkExternalWriter(t, path, "locked")
			}

			require.NoError(t, active.WithDBWrite(func(db *sql.DB) error {
				_, err := db.Exec("create table ownership_marker as select 42 as value")
				return err
			}))
			require.NoError(t, active.Close())
			if mode == "resume" {
				checkExternalWriter(t, path, "available")
			}
			reopened, err := OpenViewerDatabase(t.Context(), alias, false, nil)
			require.NoError(t, err)
			defer reopened.Close()
			var marker int
			require.NoError(t, reopened.WithDBRead(func(db *sql.DB) error {
				return db.QueryRow("select value from ownership_marker").Scan(&marker)
			}))
			require.Equal(t, 42, marker)
		})
	}
}

func checkExternalWriter(t *testing.T, path, want string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestExternalWriterProbe$")
	cmd.Env = append(os.Environ(), "VIEWER_TEST_WRITER_PATH="+path, "VIEWER_TEST_WRITER_STATE="+want)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
}

func TestExternalWriterProbe(t *testing.T) {
	path := os.Getenv("VIEWER_TEST_WRITER_PATH")
	if path == "" {
		t.Skip("child process for the native file-lock test")
	}
	db, err := sql.Open("duckdb", path)
	if err == nil {
		defer db.Close()
		err = db.PingContext(t.Context())
	}
	if os.Getenv("VIEWER_TEST_WRITER_STATE") == "locked" {
		require.Error(t, err, "a separate process acquired the active file")
	} else {
		require.NoError(t, err)
	}
}

func TestReselectRejectsReplacementAtActivePath(t *testing.T) {
	dir := t.TempDir()
	path, other := filepath.Join(dir, "active.duckdb"), filepath.Join(dir, "other.duckdb")
	require.NoError(t, newFileStore(t, other).Close())
	active := newFileStore(t, path)
	defer active.Close()
	require.NoError(t, os.Rename(path, path+".moved"))
	require.NoError(t, os.Rename(other, path))
	err := active.SelectDatabase(t.Context(), path, func() error {
		t.Fatal("replacement must not become the remembered database")
		return nil
	})
	require.ErrorContains(t, err, "database file changed")
}

func TestConcurrentOpensOfHardLinksAdmitOneStore(t *testing.T) {
	dir := t.TempDir()
	path, alias := filepath.Join(dir, "first.duckdb"), filepath.Join(dir, "second.duckdb")
	require.NoError(t, newFileStore(t, path).Close())
	require.NoError(t, os.Link(path, alias))
	type result struct {
		s   *Store
		err error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for _, name := range []string{path, alias} {
		go func() {
			<-start
			s, err := OpenViewerDatabase(t.Context(), name, false, nil)
			results <- result{s, err}
		}()
	}
	close(start)
	var admitted int
	for range 2 {
		result := <-results
		if result.s != nil {
			defer result.s.Close() // Keep the winner open until both attempts finish.
			admitted++
			require.NoError(t, result.err)
		} else {
			require.ErrorIs(t, result.err, ErrFileInUse)
		}
	}
	require.Equal(t, 1, admitted)
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
// The test uses an incompatible table shape so DDL would fail if the version
// check ran too late.
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
