package logs_test

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"database/sql/driver"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

func setupStore(t *testing.T) (*store.Store, context.Context, func()) {
	t.Helper()
	ctx := context.Background()
	s, err := store.NewStore(ctx, "")
	require.NoError(t, err)
	return s, ctx, func() { s.Close() }
}

func countRows(t *testing.T, db *sql.DB, ctx context.Context, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRowContext(ctx, query, args...).Scan(&n))
	return n
}

// mustDecodeTraceID decodes a 32-char hex string to 16 bytes (trace ID).
func mustDecodeTraceIDLogs(s string) [16]byte {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 16 {
		panic("invalid trace ID hex: " + s)
	}
	var out [16]byte
	copy(out[:], b)
	return out
}

// mustDecodeSpanID decodes a 16-char hex string to 8 bytes (span ID).
func mustDecodeSpanIDLogs(s string) [8]byte {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 8 {
		panic("invalid span ID hex: " + s)
	}
	var out [8]byte
	copy(out[:], b)
	return out
}

// createTestLogsPdata builds plog.Logs with three log records: span 0001 (INFO, body map), span 0002 (ERROR, body string, timestamp 0), span 0007 (WARN).
func createTestLogsPdata(baseTime int64) plog.Logs {
	logs := plog.NewLogs()
	rl := logs.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", "test-service")
	rl.Resource().Attributes().PutStr("service.version", "1.0.0")
	sl := rl.ScopeLogs().AppendEmpty()
	sl.Scope().SetName("test-scope")
	sl.Scope().SetVersion("v1.0.0")

	// Span 0001: INFO, body as map, full timestamp
	rec0 := sl.LogRecords().AppendEmpty()
	rec0.SetTimestamp(pcommon.Timestamp(baseTime))
	rec0.SetObservedTimestamp(pcommon.Timestamp(baseTime + 100*int64(time.Millisecond)))
	rec0.SetTraceID(mustDecodeTraceIDLogs("00000000000000000000000000000099"))
	rec0.SetSpanID(mustDecodeSpanIDLogs("0000000000000001"))
	rec0.SetSeverityText("INFO")
	rec0.SetSeverityNumber(plog.SeverityNumberInfo)
	rec0.Body().SetEmptyMap()
	rec0.Body().Map().PutStr("message", "Operation started")
	details := rec0.Body().Map().PutEmptyMap("details")
	details.PutStr("operation", "op-a")
	details.PutStr("status", "starting")
	rec0.Attributes().PutStr("log.string", "log-a")
	rec0.Attributes().PutInt("log.int", 42)
	rec0.Attributes().PutDouble("log.float", 3.14)
	rec0.Attributes().PutBool("log.bool", true)
	arr := rec0.Attributes().PutEmptySlice("log.list")
	arr.AppendEmpty().SetStr("one")
	arr.AppendEmpty().SetStr("two")
	arr.AppendEmpty().SetStr("three")
	rec0.SetEventName("event.a")

	// Span 0002: ERROR, body string, timestamp 0 (fallback to observed)
	rec1 := sl.LogRecords().AppendEmpty()
	rec1.SetTimestamp(0)
	rec1.SetObservedTimestamp(pcommon.Timestamp(baseTime + 150*int64(time.Millisecond)))
	rec1.SetTraceID(mustDecodeTraceIDLogs("00000000000000000000000000000099"))
	rec1.SetSpanID(mustDecodeSpanIDLogs("0000000000000002"))
	rec1.SetSeverityText("ERROR")
	rec1.SetSeverityNumber(plog.SeverityNumberError)
	rec1.Body().SetStr("Operation failed")
	rec1.Attributes().PutStr("log.string", "log-b")
	rec1.Attributes().PutInt("log.int", 24)
	rec1.Attributes().PutDouble("log.float", 2.71)
	rec1.Attributes().PutBool("log.bool", false)
	arr1 := rec1.Attributes().PutEmptySlice("log.list")
	arr1.AppendEmpty().SetInt(1)
	arr1.AppendEmpty().SetInt(2)
	arr1.AppendEmpty().SetInt(3)
	arr1.AppendEmpty().SetInt(4)
	arr1.AppendEmpty().SetInt(5)
	rec1.SetDroppedAttributesCount(1)
	rec1.SetFlags(plog.LogRecordFlags(1))
	rec1.SetEventName("event.b")

	// Span 0007: WARN
	rec2 := sl.LogRecords().AppendEmpty()
	rec2.SetTimestamp(pcommon.Timestamp(baseTime + 100*int64(time.Millisecond)))
	rec2.SetObservedTimestamp(pcommon.Timestamp(baseTime + 200*int64(time.Millisecond)))
	rec2.SetTraceID(mustDecodeTraceIDLogs("00000000000000000000000000000099"))
	rec2.SetSpanID(mustDecodeSpanIDLogs("0000000000000007"))
	rec2.SetSeverityText("WARN")
	rec2.SetSeverityNumber(plog.SeverityNumberWarn)
	rec2.Body().SetStr("Operation warning")
	rec2.Attributes().PutStr("log.string", "log-c")
	rec2.SetEventName("event.c")

	return logs
}

// createTestLogsPdataN builds plog.Logs with n log records (one resource/scope), each with
// resource, scope, and log attributes. Used to exercise the flushIntervalLogs codepath
// and attribute flushing by ingesting >= 100 logs in one call.
func createTestLogsPdataN(baseTime int64, n int) plog.Logs {
	logs := plog.NewLogs()
	rl := logs.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", "test-service")
	rl.Resource().Attributes().PutStr("resource.key", "resource.val")
	sl := rl.ScopeLogs().AppendEmpty()
	sl.Scope().SetName("test-scope")
	sl.Scope().SetVersion("v1.0.0")
	sl.Scope().Attributes().PutStr("scope.key", "scope.val")
	for i := 0; i < n; i++ {
		rec := sl.LogRecords().AppendEmpty()
		rec.SetTimestamp(pcommon.Timestamp(baseTime + int64(i)))
		rec.SetObservedTimestamp(pcommon.Timestamp(baseTime + int64(i)))
		rec.SetSeverityText("INFO")
		rec.SetSeverityNumber(plog.SeverityNumberInfo)
		rec.Body().SetStr("log message")
		rec.Attributes().PutStr("log.index", fmt.Sprintf("%d", i))
		rec.Attributes().PutStr("flush_test", "ok")
	}
	return logs
}

// searchLogsAll returns logs.Search with a wide time range and nil query to get all log summaries.
func searchLogsAll(t *testing.T, s *store.Store, ctx context.Context) []logSummaryJSON {
	t.Helper()
	const maxNano = 1<<63 - 1
	raw, err := logs.Search(ctx, s.DB(), 0, maxNano, nil)
	assert.NoError(t, err)
	var entries []logSummaryJSON
	assert.NoError(t, json.Unmarshal(raw, &entries))
	return entries
}

// getLogFull fetches the full LogData for one log via logs.Get and
// unmarshals it into the rich fixture struct used by the detail-
// shape assertions below.
func getLogFull(t *testing.T, s *store.Store, ctx context.Context, id string) logEntryJSON {
	t.Helper()
	raw, err := logs.Get(ctx, s.DB(), id)
	assert.NoError(t, err)
	var entry logEntryJSON
	assert.NoError(t, json.Unmarshal(raw, &entry))
	return entry
}

// logSummaryJSON mirrors the shape that logs.Search now returns:
// lightweight card-shaped projection without bodies/attributes/etc.
// `id` is in the wire payload but never rendered to users (tool-
// minted UUID for keying/selection/detail-fetch only).
type logSummaryJSON struct {
	ID             string  `json:"id"`
	Timestamp      int64   `json:"timestamp"`
	TraceID        *string `json:"traceID"`
	SpanID         *string `json:"spanID"`
	SeverityText   string  `json:"severityText"`
	SeverityNumber int32   `json:"severityNumber"`
	ServiceName    string  `json:"serviceName"`
	BodyPreview    string  `json:"bodyPreview"`
	BodyTruncated  bool    `json:"bodyTruncated"`
	BodyType       string  `json:"bodyType"`
}

// logEntryJSON mirrors the full LogData shape returned by logs.Get.
// Used by tests that assert on detail-page fields (body, attributes,
// resource, scope, flags, eventName, dropped counts, etc).
type logEntryJSON struct {
	ID                     string          `json:"id"`
	Timestamp              int64           `json:"timestamp"`
	ObservedTimestamp      int64           `json:"observedTimestamp"`
	TraceID                string          `json:"traceID"`
	SpanID                 string          `json:"spanID"`
	SeverityText           string          `json:"severityText"`
	SeverityNumber         int32           `json:"severityNumber"`
	Body                   string          `json:"body"`
	BodyType               string          `json:"bodyType"`
	Resource               resourceLogJSON `json:"resource"`
	Scope                  scopeLogJSON    `json:"scope"`
	DroppedAttributesCount uint32          `json:"droppedAttributesCount"`
	Flags                  uint32          `json:"flags"`
	EventName              string          `json:"eventName"`
	Attributes             []attrKeyValue  `json:"attributes"`
}

type resourceLogJSON struct {
	Attributes             []attrKeyValue `json:"attributes"`
	DroppedAttributesCount uint32         `json:"droppedAttributesCount"`
}

type scopeLogJSON struct {
	Name                   string         `json:"name"`
	Version                string         `json:"version"`
	Attributes             []attrKeyValue `json:"attributes"`
	DroppedAttributesCount uint32         `json:"droppedAttributesCount"`
}

type attrKeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Type  string `json:"type"`
}

// attrMap returns a map key -> value for easier assertions.
func attrMap(attrs []attrKeyValue) map[string]string {
	m := make(map[string]string)
	for _, a := range attrs {
		m[a.Key] = a.Value
	}
	return m
}

// TestLogOrdering verifies that logs are returned newest-first by effective time (timestamp or observedTimestamp).
func TestLogOrdering(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	baseTime := time.Now().UnixNano()
	ldata := createTestLogsPdata(baseTime)
	err := s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, ldata)
	})
	assert.NoError(t, err)

	entries := searchLogsAll(t, s, ctx)
	assert.Len(t, entries, 3)

	// Order: newest first by effective time — 0002 (t+150ms), 0007 (t+100ms), 0001 (t+0)
	assert.Equal(t, "0000000000000002", spanIDOrEmpty(entries[0].SpanID))
	assert.Equal(t, "0000000000000007", spanIDOrEmpty(entries[1].SpanID))
	assert.Equal(t, "0000000000000001", spanIDOrEmpty(entries[2].SpanID))
}

// spanIDOrEmpty unwraps the nullable SpanID for tests that want plain
// string comparison against the fixture's known span IDs.
func spanIDOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// TestEmptyLogs verifies handling of empty log lists and empty store.
func TestEmptyLogs(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	err := s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, plog.NewLogs())
	})
	assert.NoError(t, err)

	entries := searchLogsAll(t, s, ctx)
	assert.Empty(t, entries)
}

// TestClearLogs verifies that all logs can be cleared from the store, including child attributes.
func TestClearLogs(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	baseTime := time.Now().UnixNano()
	ldata := createTestLogsPdata(baseTime)
	err := s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, ldata)
	})
	assert.NoError(t, err)

	entries := searchLogsAll(t, s, ctx)
	assert.Len(t, entries, 3)
	assert.Greater(t, countRows(t, s.DB(), ctx, "select count(*) from attributes where log_id is not null"), 0)

	err = logs.Clear(ctx, s.DB())
	assert.NoError(t, err)

	entries = searchLogsAll(t, s, ctx)
	assert.Empty(t, entries)
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from attributes where log_id is not null"))
}

// TestLogSuite runs a comprehensive suite on the same three-log dataset.
func TestLogSuite(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	baseTime := time.Now().UnixNano()
	ldata := createTestLogsPdata(baseTime)
	err := s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, ldata)
	})
	assert.NoError(t, err, "failed to ingest test logs")

	t.Run("LogOrdering", func(t *testing.T) {
		entries := searchLogsAll(t, s, ctx)
		assert.Len(t, entries, 3)
		assert.Equal(t, "0000000000000002", spanIDOrEmpty(entries[0].SpanID))
		assert.Equal(t, "0000000000000007", spanIDOrEmpty(entries[1].SpanID))
		assert.Equal(t, "0000000000000001", spanIDOrEmpty(entries[2].SpanID))
	})

	t.Run("LogSeverity", func(t *testing.T) {
		entries := searchLogsAll(t, s, ctx)
		assert.Equal(t, "ERROR", entries[0].SeverityText)
		assert.Equal(t, int32(plog.SeverityNumberError), entries[0].SeverityNumber)
		assert.Equal(t, "WARN", entries[1].SeverityText)
		assert.Equal(t, "INFO", entries[2].SeverityText)
		assert.Equal(t, int32(plog.SeverityNumberInfo), entries[2].SeverityNumber)
	})

	t.Run("LogBodyPreviewFromSummary", func(t *testing.T) {
		// Summary carries server-truncated bodyPreview; full body
		// lives on the detail fetch (TestLogBodyFromDetail below).
		entries := searchLogsAll(t, s, ctx)
		assert.Equal(t, "Operation failed", entries[0].BodyPreview)
		assert.False(t, entries[0].BodyTruncated)
		assert.Equal(t, "Operation warning", entries[1].BodyPreview)
		assert.False(t, entries[1].BodyTruncated)
	})

	t.Run("LogBodyFromDetail", func(t *testing.T) {
		entries := searchLogsAll(t, s, ctx)
		full0 := getLogFull(t, s, ctx, entries[0].ID)
		assert.Equal(t, "Operation failed", full0.Body)
		full1 := getLogFull(t, s, ctx, entries[1].ID)
		assert.Equal(t, "Operation warning", full1.Body)
		full2 := getLogFull(t, s, ctx, entries[2].ID)
		assert.Contains(t, full2.Body, "Operation started")
	})

	t.Run("LogServiceNameFromSummary", func(t *testing.T) {
		// service_name is denormalized onto every log row and
		// surfaced directly on the summary; tests don't need to
		// dig through resource attributes.
		entries := searchLogsAll(t, s, ctx)
		for _, e := range entries {
			assert.Equal(t, "test-service", e.ServiceName)
		}
	})

	t.Run("LogTimestamp", func(t *testing.T) {
		entries := searchLogsAll(t, s, ctx)
		// Summary `timestamp` is coalesced: prefers Timestamp,
		// falls back to ObservedTimestamp when timestamp = 0.
		// Entry 0 (the ERROR with Timestamp=0) therefore reports
		// the observed_timestamp on the summary.
		assert.Equal(t, baseTime+150*int64(time.Millisecond), entries[0].Timestamp)
		assert.NotZero(t, entries[1].Timestamp)
		assert.NotZero(t, entries[2].Timestamp)

		// Full LogData preserves both fields separately.
		full0 := getLogFull(t, s, ctx, entries[0].ID)
		assert.Equal(t, int64(0), full0.Timestamp)
		assert.Equal(t, baseTime+150*int64(time.Millisecond), full0.ObservedTimestamp)
	})

	t.Run("LogResource", func(t *testing.T) {
		entries := searchLogsAll(t, s, ctx)
		full0 := getLogFull(t, s, ctx, entries[0].ID)
		resMap := attrMap(full0.Resource.Attributes)
		assert.Equal(t, "test-service", resMap["service.name"])
		assert.Equal(t, "1.0.0", resMap["service.version"])
		full2 := getLogFull(t, s, ctx, entries[2].ID)
		assert.Equal(t, uint32(0), full2.Resource.DroppedAttributesCount)
	})

	t.Run("LogScope", func(t *testing.T) {
		entries := searchLogsAll(t, s, ctx)
		for i := range entries {
			full := getLogFull(t, s, ctx, entries[i].ID)
			assert.Equal(t, "test-scope", full.Scope.Name)
			assert.Equal(t, "v1.0.0", full.Scope.Version)
		}
	})

	t.Run("LogAttributes", func(t *testing.T) {
		entries := searchLogsAll(t, s, ctx)
		full0 := getLogFull(t, s, ctx, entries[0].ID)
		attrs0 := attrMap(full0.Attributes)
		assert.Equal(t, "log-b", attrs0["log.string"])
		assert.Equal(t, "24", attrs0["log.int"])
		assert.Equal(t, "2.71", attrs0["log.float"])
		assert.Equal(t, "false", attrs0["log.bool"])

		full2 := getLogFull(t, s, ctx, entries[2].ID)
		attrs2 := attrMap(full2.Attributes)
		assert.Equal(t, "log-a", attrs2["log.string"])
		assert.Equal(t, "42", attrs2["log.int"])
		assert.Equal(t, "3.14", attrs2["log.float"])
		assert.Equal(t, "true", attrs2["log.bool"])
	})

	t.Run("LogMetadata", func(t *testing.T) {
		entries := searchLogsAll(t, s, ctx)
		full0 := getLogFull(t, s, ctx, entries[0].ID)
		assert.Equal(t, uint32(1), full0.DroppedAttributesCount)
		assert.Equal(t, uint32(1), full0.Flags)
		assert.Equal(t, "event.b", full0.EventName)
		full1 := getLogFull(t, s, ctx, entries[1].ID)
		assert.Equal(t, "event.c", full1.EventName)
		full2 := getLogFull(t, s, ctx, entries[2].ID)
		assert.Equal(t, "event.a", full2.EventName)
	})

	t.Run("LogGetNotFound", func(t *testing.T) {
		_, err := logs.Get(ctx, s.DB(), "00000000-0000-0000-0000-000000000000")
		assert.Error(t, err)
		assert.ErrorIs(t, err, logs.ErrLogIDNotFound)
	})
}

func TestGetLogAttributes(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	baseTime := time.Now().UnixNano()
	ldata := createTestLogsPdata(baseTime)
	err := s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, ldata)
	})
	require.NoError(t, err)

	startTime := baseTime - int64(time.Hour)
	endTime := baseTime + int64(time.Hour)
	raw, err := logs.GetLogAttributes(ctx, s.DB(), startTime, endTime)
	assert.NoError(t, err)

	var attributes []struct {
		Name           string `json:"name"`
		AttributeScope string `json:"attributeScope"`
		Type           string `json:"type"`
	}
	assert.NoError(t, json.Unmarshal(raw, &attributes))
	assert.NotEmpty(t, attributes, "should have discovered log attributes")

	byScope := make(map[string][]string)
	for _, a := range attributes {
		byScope[a.AttributeScope] = append(byScope[a.AttributeScope], a.Name)
	}

	assert.Contains(t, byScope["resource"], "service.name")
	assert.Contains(t, byScope["resource"], "service.version")
	assert.Contains(t, byScope["log"], "log.string")
	assert.Contains(t, byScope["log"], "log.int")
	assert.Contains(t, byScope["log"], "log.float")
	assert.Contains(t, byScope["log"], "log.bool")
	assert.Contains(t, byScope["log"], "log.list")

	// Out-of-range query returns empty
	rawEmpty, err := logs.GetLogAttributes(ctx, s.DB(), 0, 1)
	assert.NoError(t, err)
	assert.Equal(t, json.RawMessage("[]"), rawEmpty)
}

// TestDeleteLogByID verifies that a single log can be deleted by its ID, including child attributes.
func TestDeleteLogByID(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	baseTime := time.Now().UnixNano()
	ldata := createTestLogsPdata(baseTime)
	err := s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, ldata)
	})
	assert.NoError(t, err)

	entries := searchLogsAll(t, s, ctx)
	assert.Len(t, entries, 3)

	targetID := entries[0].ID
	assert.NotEmpty(t, targetID)

	attrsBefore := countRows(t, s.DB(), ctx, "select count(*) from attributes where log_id = ?", targetID)
	assert.Greater(t, attrsBefore, 0, "target log should have attributes")

	err = logs.DeleteLogByID(ctx, s.DB(), targetID)
	assert.NoError(t, err)

	entries = searchLogsAll(t, s, ctx)
	assert.Len(t, entries, 2)
	for _, e := range entries {
		assert.NotEqual(t, targetID, e.ID)
	}

	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from attributes where log_id = ?", targetID))
}

// TestDeleteLogsByIDs verifies that multiple logs can be deleted by their IDs.
func TestDeleteLogsByIDs(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	baseTime := time.Now().UnixNano()
	ldata := createTestLogsPdata(baseTime)
	err := s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, ldata)
	})
	assert.NoError(t, err)

	entries := searchLogsAll(t, s, ctx)
	assert.Len(t, entries, 3)

	idsToDelete := []any{entries[0].ID, entries[1].ID}
	err = logs.DeleteLogsByIDs(ctx, s.DB(), idsToDelete)
	assert.NoError(t, err)

	entries = searchLogsAll(t, s, ctx)
	assert.Len(t, entries, 1)
}

// TestDeleteLogsByIDs_Empty verifies that deleting with an empty list is a no-op.
func TestDeleteLogsByIDs_Empty(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	err := logs.DeleteLogsByIDs(ctx, s.DB(), []any{})
	assert.NoError(t, err)
}

// TestIngestLogs_FlushInterval exercises the flushIntervalLogs codepath by ingesting
// a few hundred logs in one call (flush runs when logCount % 100 == 0). All logs
// have resource, scope, and log attributes; we assert they were flushed correctly.
func TestIngestLogs_FlushInterval(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	baseTime := time.Now().UnixNano()
	const batchSize = 250
	ldata := createTestLogsPdataN(baseTime, batchSize)
	err := s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, ldata)
	})
	assert.NoError(t, err)

	entries := searchLogsAll(t, s, ctx)
	assert.Len(t, entries, batchSize)

	// Summaries don't carry attributes; fetch each row's full
	// LogData via logs.Get and key the resulting map by log.index.
	byIndex := make(map[string]logEntryJSON)
	for _, e := range entries {
		full := getLogFull(t, s, ctx, e.ID)
		m := attrMap(full.Attributes)
		byIndex[m["log.index"]] = full
	}

	// Assert attributes on first (before any flush), 99th (before flush at 100), 100th (at flush), 249th (after multiple flushes).
	for _, idx := range []string{"0", "99", "100", "249"} {
		e, ok := byIndex[idx]
		assert.True(t, ok, "entry with log.index %s", idx)
		resourceAttrs := attrMap(e.Resource.Attributes)
		assert.Equal(t, "test-service", resourceAttrs["service.name"], "resource.service.name for index %s", idx)
		assert.Equal(t, "resource.val", resourceAttrs["resource.key"], "resource.key for index %s", idx)
		scopeAttrs := attrMap(e.Scope.Attributes)
		assert.Equal(t, "scope.val", scopeAttrs["scope.key"], "scope.key for index %s", idx)
		logAttrs := attrMap(e.Attributes)
		assert.Equal(t, idx, logAttrs["log.index"], "log.index for index %s", idx)
		assert.Equal(t, "ok", logAttrs["flush_test"], "flush_test for index %s", idx)
	}
}

// TestSearchLogs tests logs.Search with various query types.
func TestSearchLogs(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	baseTime := time.Now().UnixNano()
	ldata := createTestLogsPdata(baseTime)
	err := s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, ldata)
	})
	assert.NoError(t, err)

	startTime := baseTime - 24*int64(time.Hour)
	endTime := baseTime + 24*int64(time.Hour)

	parseSummaries := func(raw json.RawMessage) []logSummaryJSON {
		var e []logSummaryJSON
		assert.NoError(t, json.Unmarshal(raw, &e))
		return e
	}

	t.Run("GlobalSearch_Body", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q1",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{SearchScope: "global"},
				FieldOperator: "CONTAINS",
				Value:         "Operation failed",
			},
		}
		raw, err := logs.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.Len(t, entries, 1)
		assert.Equal(t, "0000000000000002", spanIDOrEmpty(entries[0].SpanID))
	})

	t.Run("GlobalSearch_EventName", func(t *testing.T) {
		// eventName isn't on the summary, but searching for it
		// against the full log row still works -- we just need
		// to fetch the matched log's detail to verify the field.
		query := &search.QueryNode{
			ID:   "q2",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{SearchScope: "global"},
				FieldOperator: "CONTAINS",
				Value:         "event.a",
			},
		}
		raw, err := logs.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.NotEmpty(t, entries)
		full := getLogFull(t, s, ctx, entries[0].ID)
		assert.Equal(t, "event.a", full.EventName)
	})

	t.Run("GlobalSearch_TraceID", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q3a",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{SearchScope: "global"},
				FieldOperator: "CONTAINS",
				Value:         "00000000000000000000000000000099",
			},
		}
		raw, err := logs.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.Len(t, entries, 3, "global search for trace ID hex should match all logs")
	})

	t.Run("GlobalSearch_SpanID", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q3b",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{SearchScope: "global"},
				FieldOperator: "CONTAINS",
				Value:         "0000000000000002",
			},
		}
		raw, err := logs.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.Len(t, entries, 1, "global search for span ID hex should match one log")
		assert.Equal(t, "0000000000000002", spanIDOrEmpty(entries[0].SpanID))
	})

	t.Run("GlobalSearch_NoResults", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q3",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{SearchScope: "global"},
				FieldOperator: "CONTAINS",
				Value:         "nonexistent-log-text-xyz",
			},
		}
		raw, err := logs.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.Empty(t, entries)
	})

	t.Run("Field_SeverityText", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q4",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{Name: "severityText", SearchScope: "field"},
				FieldOperator: "=",
				Value:         "ERROR",
			},
		}
		raw, err := logs.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.Len(t, entries, 1)
		assert.Equal(t, "ERROR", entries[0].SeverityText)
	})

	t.Run("Field_SpanID", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q5",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{Name: "spanID", SearchScope: "field"},
				FieldOperator: "=",
				Value:         "00000000-0000-0000-0000-000000000001",
			},
		}
		raw, err := logs.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.Len(t, entries, 1)
		assert.Equal(t, "0000000000000001", spanIDOrEmpty(entries[0].SpanID))
	})

	t.Run("Field_TraceID", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q5b",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{Name: "traceID", SearchScope: "field"},
				FieldOperator: "=",
				Value:         "00000000-0000-0000-0000-000000000099",
			},
		}
		raw, err := logs.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.Len(t, entries, 3, "all three logs share the same trace")
		for _, e := range entries {
			assert.NotNil(t, e.TraceID)
			assert.Equal(t, "00000000000000000000000000000099", *e.TraceID)
		}
	})

	t.Run("Field_SeverityNumber", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q5c",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{Name: "severityNumber", SearchScope: "field"},
				FieldOperator: "=",
				Value:         "17", // plog.SeverityNumberError
			},
		}
		raw, err := logs.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.Len(t, entries, 1)
		assert.Equal(t, "ERROR", entries[0].SeverityText)
		assert.Equal(t, int32(17), entries[0].SeverityNumber)
	})

	t.Run("Field_Body", func(t *testing.T) {
		// Search predicate runs against the full body column; the
		// summary only carries the preview. The fixture's body
		// fits in 200 chars so we can verify against bodyPreview
		// without a detail fetch.
		query := &search.QueryNode{
			ID:   "q5d",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{Name: "body", SearchScope: "field"},
				FieldOperator: "CONTAINS",
				Value:         "Operation warning",
			},
		}
		raw, err := logs.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.Len(t, entries, 1)
		assert.Equal(t, "0000000000000007", spanIDOrEmpty(entries[0].SpanID))
		assert.Contains(t, entries[0].BodyPreview, "Operation warning")
	})

	t.Run("Field_EventName", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q5e",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{Name: "eventName", SearchScope: "field"},
				FieldOperator: "=",
				Value:         "event.a",
			},
		}
		raw, err := logs.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.Len(t, entries, 1)
		assert.Equal(t, "0000000000000001", spanIDOrEmpty(entries[0].SpanID))
		full := getLogFull(t, s, ctx, entries[0].ID)
		assert.Equal(t, "event.a", full.EventName)
	})

	t.Run("Field_ScopeName", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q5f",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{Name: "scope.name", SearchScope: "field"},
				FieldOperator: "=",
				Value:         "test-scope",
			},
		}
		raw, err := logs.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.Len(t, entries, 3)
		// scope.name lives on the full row; assert via one detail fetch.
		full := getLogFull(t, s, ctx, entries[0].ID)
		assert.Equal(t, "test-scope", full.Scope.Name)
	})

	t.Run("Field_ScopeVersion", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q5g",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{Name: "scope.version", SearchScope: "field"},
				FieldOperator: "=",
				Value:         "v1.0.0",
			},
		}
		raw, err := logs.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.Len(t, entries, 3)
		full := getLogFull(t, s, ctx, entries[0].ID)
		assert.Equal(t, "v1.0.0", full.Scope.Version)
	})

	t.Run("Attribute_LogString", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q6",
			Type: "condition",
			Query: &search.Query{
				Field: &search.FieldDefinition{
					Name:           "log.string",
					SearchScope:    "attribute",
					AttributeScope: "log",
					Type:           "string",
				},
				FieldOperator: "=",
				Value:         "log-b",
			},
		}
		raw, err := logs.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.Len(t, entries, 1)
		full := getLogFull(t, s, ctx, entries[0].ID)
		assert.Equal(t, "log-b", attrMap(full.Attributes)["log.string"])
	})

	t.Run("Attribute_Resource", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q7",
			Type: "condition",
			Query: &search.Query{
				Field: &search.FieldDefinition{
					Name:           "service.name",
					SearchScope:    "attribute",
					AttributeScope: "resource",
					Type:           "string",
				},
				FieldOperator: "CONTAINS",
				Value:         "test-service",
			},
		}
		raw, err := logs.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.NotEmpty(t, entries)
		// service_name is denormalized onto the summary too, so
		// no detail fetch needed for the value assertion.
		assert.Equal(t, "test-service", entries[0].ServiceName)
	})
}

// TestLogs_ServiceNameDenormStaysConsistent mirrors the spans/streams
// invariant: logs.service_name (the denormalized hot-filter column)
// must equal the source-of-truth resource attribute value for every
// log row. If a future change writes only the column, only the
// attribute, or writes inconsistent values, this test fails. We rely
// on the standard fixture which stamps service.name = test-service on
// the resource for every record.
func TestLogs_ServiceNameDenormStaysConsistent(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	baseTime := time.Now().UnixNano()
	err := s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, createTestLogsPdata(baseTime))
	})
	require.NoError(t, err)

	mismatches := countRows(t, s.DB(), ctx, `
		select count(*) from logs l
		left join attributes a
		     on a.log_id = l.id
		    and a.scope = 'resource'
		    and a.key = 'service.name'
		where l.service_name <> coalesce(a.value, '')
	`)
	assert.Equal(t, 0, mismatches,
		"logs.service_name must equal the source resource attribute (or '' when absent)")
}
