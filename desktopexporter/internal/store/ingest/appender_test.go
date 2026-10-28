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

	tables := []string{"logs", "nonexistent_table"}
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
		appenders2, err = ingest.NewAppenders(conn, []string{"logs"})
		return err
	})
	require.NoError(t, err)
	ingest.CloseAppenders(appenders2, []string{"logs"})
}

// TestFlushAppenders_MakesDataVisible verifies that FlushAppenders (not Close)
// makes an appended log and all of its owned payloads visible.
func TestFlushAppenders_MakesDataVisible(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	resource := pcommon.NewResource()
	resource.Attributes().PutStr("service.name", "flush-test")
	resourceInput, err := ingest.ResourceInput(resource, "")
	require.NoError(t, err)

	scope := pcommon.NewInstrumentationScope()
	scope.SetName("flush-scope")
	scope.SetVersion("v1")
	scopeInput, err := ingest.ScopeInput(scope, "")
	require.NoError(t, err)

	logAttrs := pcommon.NewMap()
	logAttrs.PutStr("flush_attr", "ok")
	logAttrs.PutStr("key", "value")
	attributes, err := ingest.AttributeBytes(logAttrs)
	require.NoError(t, err)

	// Keep the appender open so Close cannot make the row visible. Reads cannot
	// nest inside WithConn, so the dedicated connection retains the appender
	// across append, read, and close calls.
	tables := []string{"logs"}
	logRef := duckdb.UUID(uuid.New())
	var appenders map[string]*duckdb.Appender

	err = s.WithConn(func(conn driver.Conn) error {
		var err error
		appenders, err = ingest.NewAppenders(conn, tables)
		if err != nil {
			return err
		}

		body, err := ingest.ValueBytes(pcommon.NewValueStr("flush test"))
		if err != nil {
			return err
		}
		if err := appenders["logs"].AppendRow(
			logRef,
			int64(0), int64(0), // Timestamp, ObservedTimestamp
			nil, nil, // TraceID, SpanID
			"INFO", int32(9), // SeverityText, SeverityNumber
			body, // Body Parquet Variant bytes
			resourceInput, scopeInput,
			attributes,
			uint32(0), uint32(0), "", // DroppedAttributesCount, Flags, EventName
			"flush-test", // ServiceName VARCHAR (NOT NULL, '' = unknown)
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

	logRefStr := uuid.UUID(logRef).String()

	var logCount int
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRowContext(ctx, "select count(*) from logs where id = ?", logRefStr).Scan(&logCount)
	}))
	assert.Equal(t, 1, logCount, "log row must be visible after Flush without Close")

	var resolved int
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRowContext(ctx, `
			select len(variant_keys(attributes)) from logs where id = ?
		`, logRefStr).Scan(&resolved)
	}))
	assert.Equal(t, 2, resolved, "attributes must be visible with their owner after Flush")

	// Pin the id derivation itself: the row the appender pointed at must be the
	// one ingest.AttributeID names for that canonical key/value payload.
	var value string
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRowContext(ctx, `
			select otel_value_json(attributes.flush_attr)::varchar from logs where id = ?
		`, logRefStr).Scan(&value)
	}))
	assert.JSONEq(t, `{"kind":"string","value":"ok"}`, value)
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

// DuckDB 2 drops the buffered rows of a failed flush. The transaction must also
// discard an earlier successful flush, and a fresh batch must still succeed on
// the same dedicated connection.
func TestFlushFailureRollsBackEarlierTablesAndAllowsRetry(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		_, err := db.ExecContext(ctx, `
			create table flush_first (id integer primary key);
			create table flush_second (id integer primary key)`)
		return err
	}))
	tables := []string{"flush_second", "flush_first"}
	batch := func(duplicate bool) error {
		return s.WithConn(func(conn driver.Conn) error {
			return ingest.InTransaction(ctx, conn, func() (err error) {
				appenders, err := ingest.NewAppenders(conn, tables)
				if err != nil {
					return err
				}
				defer func() {
					err = errors.Join(err, ingest.CloseAppenders(appenders, tables))
				}()
				if err := appenders["flush_first"].AppendRow(int32(7)); err != nil {
					return err
				}
				if err := appenders["flush_second"].AppendRow(int32(11)); err != nil {
					return err
				}
				if duplicate {
					if err := appenders["flush_second"].AppendRow(int32(11)); err != nil {
						return err
					}
				}
				return ingest.FlushAppenders(appenders, tables)
			})
		})
	}
	counts := func() []int {
		t.Helper()
		var first, second int
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			return db.QueryRowContext(ctx, `select
				(select count(*) from flush_first),
				(select count(*) from flush_second)`).Scan(&first, &second)
		}))
		return []int{first, second}
	}
	err := batch(true)
	require.ErrorIs(t, err, ingest.ErrIngestInternal)
	require.Contains(t, err.Error(), "FlushAppenders")
	require.Equal(t, []int{0, 0}, counts())
	require.NoError(t, batch(false))
	require.Equal(t, []int{1, 1}, counts())
}
