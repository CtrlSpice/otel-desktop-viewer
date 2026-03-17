package ingest_test

import (
	"context"
	"errors"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/google/uuid"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func setupStore(t *testing.T) (*store.Store, context.Context, func()) {
	t.Helper()
	ctx := context.Background()
	s, err := store.NewStore(ctx, "")
	require.NoError(t, err)
	return s, ctx, func() { s.Close() }
}

// TestNewAppenders_ErrorPath verifies that when appender creation fails partway through,
// we close any appenders already created before returning the error (no leak).
func TestNewAppenders_ErrorPath(t *testing.T) {
	s, _, teardown := setupStore(t)
	defer teardown()

	tables := []string{"attributes", "nonexistent_table"}
	appenders, err := ingest.NewAppenders(s.Conn(), tables)

	require.Error(t, err)
	assert.Nil(t, appenders)
	assert.True(t, errors.Is(err, ingest.ErrIngestInternal))
	assert.Contains(t, err.Error(), "appender")

	appenders2, err := ingest.NewAppenders(s.Conn(), []string{"attributes"})
	require.NoError(t, err)
	ingest.CloseAppenders(appenders2, []string{"attributes"})
}

// TestFlushAppenders_MakesDataVisible verifies that FlushAppenders (not Close) makes rows
// visible: we append a log row and attribute rows, flush, query without ever calling Close,
// and assert both the log and its attributes are present.
func TestFlushAppenders_MakesDataVisible(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	tables := []string{"attributes", "logs"}
	appenders, err := ingest.NewAppenders(s.Conn(), tables)
	require.NoError(t, err)
	defer ingest.CloseAppenders(appenders, tables)

	logID := duckdb.UUID(uuid.New())
	err = appenders["logs"].AppendRow(
		logID,
		int64(0), int64(0), // Timestamp, ObservedTimestamp
		nil, nil,           // TraceID, SpanID
		"INFO", int32(9),   // SeverityText, SeverityNumber
		"flush test", "str", // Body, BodyType
		uint32(0), "scope", "v1", uint32(0), uint32(0), uint32(0), "", "flush test",
	)
	require.NoError(t, err)

	attrs := pcommon.NewMap()
	attrs.PutStr("flush_attr", "ok")
	attrs.PutStr("key", "value")
	err = ingest.IngestAttributes(appenders["attributes"], []ingest.AttributeBatchItem{
		{Attrs: attrs, IDs: ingest.AttributeOwnerIDs{LogID: &logID}, Scope: "log"},
	})
	require.NoError(t, err)

	err = ingest.FlushAppenders(appenders, tables)
	require.NoError(t, err)

	var logCount int
	err = s.DB().QueryRowContext(ctx, "select count(*) from logs").Scan(&logCount)
	require.NoError(t, err)
	assert.Equal(t, 1, logCount, "log row must be visible after Flush without Close")

	logIDStr := uuid.UUID(logID).String()
	var attrCount int
	require.NoError(t, s.DB().QueryRowContext(ctx, "select count(*) from attributes where log_id = ?", logIDStr).Scan(&attrCount))
	assert.Equal(t, 2, attrCount, "attribute rows must be visible after Flush without Close")

	var key, value, scope string
	err = s.DB().QueryRowContext(ctx,
		"select key, value, scope from attributes where log_id = ? and key = ?",
		logIDStr, "flush_attr").Scan(&key, &value, &scope)
	require.NoError(t, err)
	assert.Equal(t, "flush_attr", key)
	assert.Equal(t, "ok", value)
	assert.Equal(t, "log", scope)
}

// TestFlushAppenders_CloseAppenders_NilEmptySafe verifies that FlushAppenders and
// CloseAppenders do not panic when given nil or empty inputs (documented as safe).
func TestFlushAppenders_CloseAppenders_NilEmptySafe(t *testing.T) {
	assert.NotPanics(t, func() { ingest.FlushAppenders(nil, nil) })
	assert.NotPanics(t, func() { ingest.FlushAppenders(nil, []string{"x"}) })
	assert.NotPanics(t, func() { ingest.FlushAppenders(map[string]*duckdb.Appender{}, nil) })
	assert.NotPanics(t, func() { ingest.FlushAppenders(map[string]*duckdb.Appender{}, []string{}) })

	assert.NotPanics(t, func() { ingest.CloseAppenders(nil, nil) })
	assert.NotPanics(t, func() { ingest.CloseAppenders(nil, []string{"x"}) })
	assert.NotPanics(t, func() { ingest.CloseAppenders(map[string]*duckdb.Appender{}, []string{}) })
}
