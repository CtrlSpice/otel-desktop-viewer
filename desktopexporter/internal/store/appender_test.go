package store

import (
	"testing"

	"github.com/google/uuid"
	"github.com/marcboeker/go-duckdb/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// TestNewAppenders_ErrorPath verifies that when appender creation fails partway through,
// we close any appenders already created before returning the error (no leak).
func TestNewAppenders_ErrorPath(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	// First table exists, second does not — so first NewAppender succeeds, second fails.
	tables := []string{"attributes", "nonexistent_table"}
	appenders, err := NewAppenders(helper.Store.conn, tables)

	require.Error(t, err)
	assert.Nil(t, appenders)
	assert.Contains(t, err.Error(), "failed to create appender")

	// Store should still be usable (cleanup closed only the appenders we created, not the conn).
	appenders2, err := NewAppenders(helper.Store.conn, []string{"attributes"})
	require.NoError(t, err)
	CloseAppenders(appenders2, []string{"attributes"})
}

// TestFlushAppenders_MakesDataVisible verifies that FlushAppenders (not Close) makes rows
// visible: we append a log row and attribute rows, flush, query without ever calling Close,
// and assert both the log and its attributes are present.
func TestFlushAppenders_MakesDataVisible(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	tables := []string{"attributes", "logs"}
	appenders, err := NewAppenders(helper.Store.conn, tables)
	require.NoError(t, err)
	defer CloseAppenders(appenders, tables)

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
	err = IngestAttributes(appenders["attributes"], []AttributeBatchItem{
		{Attrs: attrs, IDs: AttributeOwnerIDs{LogID: &logID}, Scope: "log"},
	})
	require.NoError(t, err)

	err = FlushAppenders(appenders, tables)
	require.NoError(t, err)
	// Do not close yet — query with the store's db. In DuckDB, flush makes data visible
	// to other connections using the same (in-memory) database.
	var logCount int
	err = helper.Store.db.QueryRowContext(helper.Ctx, "select count(*) from logs").Scan(&logCount)
	require.NoError(t, err)
	assert.Equal(t, 1, logCount, "log row must be visible after Flush without Close")

	logIDStr := uuid.UUID(logID).String()
	attrCount := countRows(t, helper, "SELECT COUNT(*) FROM attributes WHERE log_id = ?", logIDStr)
	assert.Equal(t, 2, attrCount, "attribute rows must be visible after Flush without Close")

	// Assert the specific attribute key/values are present
	var key, value, scope string
	err = helper.Store.db.QueryRowContext(helper.Ctx,
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
	assert.NotPanics(t, func() { FlushAppenders(nil, nil) })
	assert.NotPanics(t, func() { FlushAppenders(nil, []string{"x"}) })
	assert.NotPanics(t, func() { FlushAppenders(map[string]*duckdb.Appender{}, nil) })
	assert.NotPanics(t, func() { FlushAppenders(map[string]*duckdb.Appender{}, []string{}) })

	assert.NotPanics(t, func() { CloseAppenders(nil, nil) })
	assert.NotPanics(t, func() { CloseAppenders(nil, []string{"x"}) })
	assert.NotPanics(t, func() { CloseAppenders(map[string]*duckdb.Appender{}, []string{}) })
}
