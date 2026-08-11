package store

import (
	"context"
	"database/sql"
	"path/filepath"
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

// A file stamped with a different version must be reported, not silently used.
// This is the case that becomes a hard failure once enforcement is switched on.
func TestVersionMismatchIsReported(t *testing.T) {
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

	reopened, logs := openWithLogs(t, path)
	defer reopened.Close()

	assert.Equal(t, SchemaMismatch, reopened.SchemaCompatibility())
	require.Equal(t, 1, logs.Len(), "a mismatch must be reported to the user")

	entry := logs.All()[0]
	fields := entry.ContextMap()
	assert.Contains(t, fields["database"], "future.db", "name the file")
	assert.Equal(t, int64(schema.Version+1), fields["file_version"])
	assert.Contains(t, fields["remedy"], "--db", "say what to do about it")
}

// A file holding data but carrying no stamp predates versioning. It must not be
// stamped on sight: doing so would assert a compatibility nobody checked and
// destroy the only evidence of where the file came from.
func TestPreVersioningDatabaseIsNotStamped(t *testing.T) {
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
			values (?::uuid, ?::uuid,
				'eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee'::uuid,
				'ffffffff-ffff-ffff-ffff-ffffffffffff'::uuid,
				[]::uuid[])`,
			"11111111-1111-1111-1111-111111111111",
			"22222222-2222-2222-2222-222222222222")
		return err
	})
	require.NoError(t, err)
	require.NoError(t, s.Close())

	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	_, err = db.Exec(`drop table schema_meta`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	reopened, logs := openWithLogs(t, path)
	defer reopened.Close()

	assert.Equal(t, SchemaPreVersioning, reopened.SchemaCompatibility())
	require.Equal(t, 1, logs.Len())
	assert.Contains(t, logs.All()[0].ContextMap()["database"], "legacy.db")

	// And it stays unstamped, so reopening reports the same thing rather than
	// quietly deciding the file is fine the second time.
	require.NoError(t, reopened.WithDBRead(func(db *sql.DB) error {
		var stamped sql.NullInt64
		if err := db.QueryRow(schema.ReadVersionQuery).Scan(&stamped); err != nil {
			return err
		}
		assert.False(t, stamped.Valid, "a pre-versioning file must not be stamped on sight")
		return nil
	}))
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

	// The open still fails: warn-only means the check explains the failure
	// rather than preventing it. Enforcement (returning this as an error) is
	// the switch to flip once the schema settles.
	require.Error(t, err, "an incompatible file cannot be opened by this build")

	// The point of the ordering: the version warning is emitted *before* the
	// failure, so the opaque index error has an explanation attached. With the
	// check moved after the table/index loops, this open fails identically but
	// logs nothing -- which is what the mutation check confirms.
	require.Equal(t, 1, logs.Len(),
		"the version warning must be logged before anything builds on the old schema")
	entry := logs.All()[0].ContextMap()
	assert.Contains(t, entry["database"], "order.db")
	assert.Contains(t, entry["remedy"], "--db")
}
