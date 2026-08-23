package ingest_test

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// readStore runs a query under the store's read lock and returns its result.
// Store exposes no raw *sql.DB: every read is ordered against ingest,
// retention, and Close.
func readStore[T any](s *store.Store, fn func(db *sql.DB) (T, error)) (T, error) {
	var out T
	err := s.WithDBRead(func(db *sql.DB) error {
		var err error
		out, err = fn(db)
		return err
	})
	return out, err
}

// TestNewAppenders_ErrorPath verifies that when appender creation fails partway through,
// we close any appenders already created before returning the error (no leak).
func TestNewAppenders_ErrorPath(t *testing.T) {
	t.Parallel()
	s, _ := storetest.New(t)

	tables := []string{"attributes", "nonexistent_table"}
	var appenders map[string]*duckdb.Appender
	err := s.WithConn(func(conn driver.Conn) error {
		var err error
		appenders, err = ingest.NewAppenders(conn, tables)
		return err
	})

	require.Error(t, err)
	assert.Nil(t, appenders)
	assert.True(t, errors.Is(err, ingest.ErrIngestInternal))
	assert.Contains(t, err.Error(), "appender")

	var appenders2 map[string]*duckdb.Appender
	err = s.WithConn(func(conn driver.Conn) error {
		var err error
		appenders2, err = ingest.NewAppenders(conn, []string{"attributes"})
		return err
	})
	require.NoError(t, err)
	ingest.CloseAppenders(appenders2, []string{"attributes"})
}

// TestFlushAppenders_MakesDataVisible verifies that FlushAppenders (not Close)
// makes appended rows visible: we append a log row, flush, query without ever
// calling Close, and assert the row and its attribute references are present.
//
// Reworked for the attribute dictionary. This test used to append the log's
// attribute rows through a second appender on `attributes` (via the deleted
// ingest.IngestAttributes) and assert they were visible. That is no longer how
// attributes are written at all: the dictionary is flushed with SQL inserts
// carrying `on conflict do nothing`, because the appender has no conflict
// handling and a repeated attribute would fail the whole chunk. What rides the
// appender now is the owner's inline attribute_ids array.
//
// So the shape mirrors logs.Ingest exactly -- dictionary first, appender second
// -- and the assertion moves accordingly: the appender-written array must be
// visible after Flush and must resolve against the dictionary. Note that logs
// carries NOT NULL FKs to resources and scopes, so the dictionary flush is
// load-bearing here rather than incidental: without it the appender's own flush
// would fail the FK, which is itself a check that the two halves agree.
func TestFlushAppenders_MakesDataVisible(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	// Build the dictionary side the way logs.Ingest does, so the ids the
	// appender writes are the ids the dictionary rows were keyed by.
	dict := ingest.NewDictionary(nil)

	resource := pcommon.NewResource()
	resource.Attributes().PutStr("service.name", "flush-test")
	resourceID := dict.AddResource(resource)

	scope := pcommon.NewInstrumentationScope()
	scope.SetName("flush-scope")
	scope.SetVersion("v1")
	scopeID := dict.AddScope(scope)

	logAttrs := pcommon.NewMap()
	logAttrs.PutStr("flush_attr", "ok")
	logAttrs.PutStr("key", "value")
	logAttrIDs := dict.AddAttributes(logAttrs, ingest.ScopeLog)
	require.Len(t, logAttrIDs, 2)

	// The appender is deliberately NOT closed inside this callback, and that is
	// the whole design of the test. Close flushes on its way out, so a
	// `defer CloseAppenders` here would publish the row even if FlushAppenders
	// did nothing -- confirmed by neutering FlushAppenders, which this test
	// passed until the close moved out. Reads cannot nest inside WithConn
	// (Store's locks are not reentrant), so the appender is held across two
	// WithConn calls instead: append and flush, read, then close. Both calls
	// get the same dedicated appender connection, so the handle stays valid.
	tables := []string{"logs"}
	logID := duckdb.UUID(uuid.New())
	var appenders map[string]*duckdb.Appender

	err := s.WithConn(func(conn driver.Conn) error {
		if err := dict.Flush(ctx, conn); err != nil {
			return err
		}

		var err error
		appenders, err = ingest.NewAppenders(conn, tables)
		if err != nil {
			return err
		}

		if err := appenders["logs"].AppendRow(
			logID,
			int64(0), int64(0), // Timestamp, ObservedTimestamp
			nil, nil, // TraceID, SpanID
			"INFO", int32(9), // SeverityText, SeverityNumber
			"flush test", "str", // Body, BodyType
			resourceID, scopeID, // ResourceID, ScopeID
			ingest.NonNil(logAttrIDs), // AttributeIDs UUID[]
			uint32(0), uint32(0), "",  // DroppedAttributesCount, Flags, EventName
			"flush-test", // ServiceName VARCHAR (NOT NULL, '' = unknown)
			"", "",       // ResourceSchemaURL, ScopeSchemaURL (batch-level, optional)
		); err != nil {
			return err
		}

		return ingest.FlushAppenders(appenders, tables)
	})
	require.NoError(t, err)

	// Registered after the store-teardown defer, so it runs first (LIFO) and
	// the appender is closed while its connection is still alive.
	defer func() {
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return ingest.CloseAppenders(appenders, tables)
		}))
	}()

	logIDStr := uuid.UUID(logID).String()

	var logCount int
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRowContext(ctx, "select count(*) from logs where id = ?", logIDStr).Scan(&logCount)
	}))
	assert.Equal(t, 1, logCount, "log row must be visible after Flush without Close")

	// The appender-written array must resolve: every id in it is a dictionary
	// row. A join rather than a bare array-length check, so an array of
	// plausible-looking but dangling ids fails here.
	var resolved int
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRowContext(ctx, `
			select count(*)
			from (select unnest(attribute_ids) as id from logs where id = ?) x
			join attributes a on a.id = x.id
		`, logIDStr).Scan(&resolved)
	}))
	assert.Equal(t, 2, resolved, "attribute_ids written by the appender must resolve against the dictionary")

	// Pin the id derivation itself: the row the appender pointed at must be the
	// one ingest.AttributeID names for that (key, value, type, scope).
	flushAttrID := uuid.UUID(ingest.AttributeID("flush_attr", "ok", "string", ingest.ScopeLog)).String()
	var key, value, scope2 string
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRowContext(ctx, `
			select a.key, a.value, a.scope
			from (select unnest(attribute_ids) as id from logs where id = ?) x
			join attributes a on a.id = x.id
			where a.id = ?
		`, logIDStr, flushAttrID).Scan(&key, &value, &scope2)
	}))
	assert.Equal(t, "flush_attr", key)
	assert.Equal(t, "ok", value)
	assert.Equal(t, ingest.ScopeLog, scope2)
}

// TestFlushAppenders_CloseAppenders_NilEmptySafe verifies that FlushAppenders and
// CloseAppenders do not panic when given nil or empty inputs (documented as safe).
func TestFlushAppenders_CloseAppenders_NilEmptySafe(t *testing.T) {
	t.Parallel()
	assert.NotPanics(t, func() { ingest.FlushAppenders(nil, nil) })
	assert.NotPanics(t, func() { ingest.FlushAppenders(nil, []string{"x"}) })
	assert.NotPanics(t, func() { ingest.FlushAppenders(map[string]*duckdb.Appender{}, nil) })
	assert.NotPanics(t, func() { ingest.FlushAppenders(map[string]*duckdb.Appender{}, []string{}) })

	assert.NotPanics(t, func() { ingest.CloseAppenders(nil, nil) })
	assert.NotPanics(t, func() { ingest.CloseAppenders(nil, []string{"x"}) })
	assert.NotPanics(t, func() { ingest.CloseAppenders(map[string]*duckdb.Appender{}, []string{}) })
}
