package logs_test

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"database/sql/driver"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
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

func countRows(t *testing.T, s *store.Store, ctx context.Context, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRowContext(ctx, query, args...).Scan(&n)
	}))
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
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return logs.Search(ctx, db, 0, maxNano, nil)
	})
	assert.NoError(t, err)
	var entries []logSummaryJSON
	assert.NoError(t, json.Unmarshal(raw, &entries))
	return entries
}

func TestSearchLogsLimit(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	baseTime := time.Now().UnixNano()
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, createTestLogsPdata(baseTime), s.FlushedIDs())
	}))

	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return logs.SearchWithLimit(ctx, db, 0, 1<<63-1, nil, 2)
	})
	require.NoError(t, err)
	var entries []logSummaryJSON
	require.NoError(t, json.Unmarshal(raw, &entries))
	require.Len(t, entries, 2)
	require.Equal(t, []string{"ERROR", "WARN"}, []string{entries[0].SeverityText, entries[1].SeverityText})

	_, err = readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return logs.SearchWithLimit(ctx, db, 0, 1<<63-1, nil, 0)
	})
	require.ErrorIs(t, err, logs.ErrInvalidLogLimit)
}

// getLogFull fetches the full LogData for one log via logs.Get and
// unmarshals it into the rich fixture struct used by the detail-
// shape assertions below.
func getLogFull(t *testing.T, s *store.Store, ctx context.Context, id string) logEntryJSON {
	t.Helper()
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return logs.Get(ctx, db, id)
	})
	assert.NoError(t, err)
	var entry logEntryJSON
	assert.NoError(t, json.Unmarshal(raw, &entry))
	return entry
}

// parseWireTimestamp decodes a varchar-encoded int64 nanosecond timestamp
// from the JSON wire format.
func parseWireTimestamp(t *testing.T, s string) int64 {
	t.Helper()
	n, err := strconv.ParseInt(s, 10, 64)
	require.NoError(t, err)
	return n
}

// logSummaryJSON mirrors the shape that logs.Search now returns:
// lightweight card-shaped projection without bodies/attributes/etc.
// `id` is in the wire payload but never rendered to users (tool-
// minted UUID for keying/selection/detail-fetch only).
type logSummaryJSON struct {
	ID             string `json:"id"`
	Timestamp      string `json:"timestamp"` // varchar-encoded int64 ns
	SeverityText   string `json:"severityText"`
	SeverityNumber int32  `json:"severityNumber"`
	ServiceName    string `json:"serviceName"`
	BodyPreview    string `json:"bodyPreview"`
}

// logEntryJSON mirrors the full LogData shape returned by logs.Get.
// Used by tests that assert on detail-page fields (body, attributes,
// resource, scope, flags, eventName, dropped counts, etc).
type logEntryJSON struct {
	ID                     string          `json:"id"`
	Timestamp              string          `json:"timestamp"`         // varchar-encoded int64 ns
	ObservedTimestamp      string          `json:"observedTimestamp"` // varchar-encoded int64 ns
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
	t.Parallel()
	s, ctx := storetest.New(t)

	baseTime := time.Now().UnixNano()
	ldata := createTestLogsPdata(baseTime)
	err := s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, ldata, s.FlushedIDs())
	})
	assert.NoError(t, err)

	entries := searchLogsAll(t, s, ctx)
	assert.Len(t, entries, 3)

	// Order: newest first by effective time — ERROR (t+150ms), WARN (t+100ms), INFO (t+0)
	assert.Equal(t, "ERROR", entries[0].SeverityText)
	assert.Equal(t, "WARN", entries[1].SeverityText)
	assert.Equal(t, "INFO", entries[2].SeverityText)
}

// TestEmptyLogs verifies handling of empty log lists and empty store.
func TestEmptyLogs(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	err := s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, plog.NewLogs(), s.FlushedIDs())
	})
	assert.NoError(t, err)

	entries := searchLogsAll(t, s, ctx)
	assert.Empty(t, entries)
}

// TestClearLogs verifies that all logs can be cleared from the store, and pins
// the two-step contract the attribute dictionary introduced.
//
// Clear used to delete the log's attribute rows along with the logs, because
// each row belonged to exactly one log. Dictionary rows are shared with spans
// and metrics, so Clear can no longer tell whether a row it just abandoned is
// dead; it deliberately leaves them, and ingest.SweepOrphans is the only thing
// that decides. Asserting that they survive Clear is the new contract, not a
// dropped assertion. See spans.TestClearTraces for the same shape.
func TestClearLogs(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	baseTime := time.Now().UnixNano()
	ldata := createTestLogsPdata(baseTime)
	err := s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, ldata, s.FlushedIDs())
	})
	assert.NoError(t, err)

	entries := searchLogsAll(t, s, ctx)
	assert.Len(t, entries, 3)

	// Snapshot rather than a non-empty check, so a Clear that deleted only some
	// of the dictionary would still be caught.
	attrsBefore := countRows(t, s, ctx, "select count(*) from attributes")
	assert.Greater(t, attrsBefore, 0)
	assert.Greater(t, countRows(t, s, ctx, "select count(*) from attributes where scope = 'log'"), 0)

	err = s.WithDBWrite(func(db *sql.DB) error {
		return logs.Clear(ctx, db)
	})
	assert.NoError(t, err)

	entries = searchLogsAll(t, s, ctx)
	assert.Empty(t, entries)
	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from logs"))
	assert.Equal(t, attrsBefore, countRows(t, s, ctx, "select count(*) from attributes"),
		"Clear must not delete attribute rows: they are shared with spans and metrics")
	assert.Greater(t, countRows(t, s, ctx, "select count(*) from resources"), 0)
	assert.Greater(t, countRows(t, s, ctx, "select count(*) from scopes"), 0)

	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		return ingest.SweepOrphans(ctx, db, s.FlushedIDs())
	}))

	// Logs were the only signal ingested, so after the sweep nothing is
	// referenced and all three tables empty out.
	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from attributes"))
	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from resources"))
	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from scopes"))
}

// TestLogSuite runs a comprehensive suite on the same three-log dataset.
func TestLogSuite(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	baseTime := time.Now().UnixNano()
	ldata := createTestLogsPdata(baseTime)
	err := s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, ldata, s.FlushedIDs())
	})
	assert.NoError(t, err, "failed to ingest test logs")

	t.Run("LogOrdering", func(t *testing.T) {
		entries := searchLogsAll(t, s, ctx)
		assert.Len(t, entries, 3)
		assert.Equal(t, "ERROR", entries[0].SeverityText)
		assert.Equal(t, "WARN", entries[1].SeverityText)
		assert.Equal(t, "INFO", entries[2].SeverityText)
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
		assert.Equal(t, "Operation warning", entries[1].BodyPreview)
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
		assert.Equal(t, baseTime+150*int64(time.Millisecond), parseWireTimestamp(t, entries[0].Timestamp))
		assert.NotEmpty(t, entries[1].Timestamp)
		assert.NotEmpty(t, entries[2].Timestamp)

		// Full LogData preserves both fields separately.
		full0 := getLogFull(t, s, ctx, entries[0].ID)
		assert.Equal(t, int64(0), parseWireTimestamp(t, full0.Timestamp))
		assert.Equal(t, baseTime+150*int64(time.Millisecond), parseWireTimestamp(t, full0.ObservedTimestamp))
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
		_, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.Get(ctx, db, "00000000-0000-0000-0000-000000000000")
		})
		assert.Error(t, err)
		assert.ErrorIs(t, err, logs.ErrLogIDNotFound)
	})
}

// attributeDef mirrors one entry of the GetLogAttributes payload, which the
// attribute_def_json macro renders.
type attributeDef struct {
	Name           string `json:"name"`
	AttributeScope string `json:"attributeScope"`
	Type           string `json:"type"`
}

func getLogAttributeDefs(t *testing.T, s *store.Store, ctx context.Context, startTime, endTime int64) []attributeDef {
	t.Helper()
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return logs.GetLogAttributes(ctx, db, startTime, endTime)
	})
	require.NoError(t, err)
	var out []attributeDef
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

// TestGetLogAttributes pins a deliberate behaviour change: GetLogAttributes
// accepts a time range and ignores it, returning every log-side attribute
// definition (scopes resource, scope, log) the store knows about.
//
// It used to window, because attribute rows were per-log and could be joined
// back to a timestamp. The dictionary has no owner and therefore no time: one
// row covers every log that ever carried that (key, value, type), so there is
// nothing to filter on. Answering from the few hundred dictionary rows is also
// why this got cheap enough to stop windowing in the first place.
//
// The old test asserted that an out-of-range window returned "[]", which is now
// exactly backwards. Two assertions replace it, and both fail if the function
// silently reverted to windowing:
//
//  1. An out-of-range window returns the same set as a covering one.
//  2. A window that covers only the first batch still reports attributes that
//     only the second, much later batch introduced.
//
// (1) alone would be weak: the fixture's second record has timestamp 0, so a
// windowed implementation asked for [0,1] would return a non-empty subset
// rather than nothing. (2) has no such escape -- those keys exist only outside
// the window.
func TestGetLogAttributes(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	baseTime := time.Now().UnixNano()
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, createTestLogsPdata(baseTime), s.FlushedIDs())
	}))

	startTime := baseTime - int64(time.Hour)
	endTime := baseTime + int64(time.Hour)
	attributes := getLogAttributeDefs(t, s, ctx, startTime, endTime)

	// The full set for this fixture, exactly. Compared as a set rather than a
	// slice: the query orders by (key, scope), and the two log.list entries tie
	// on both, so their relative order is not something the function promises.
	assert.ElementsMatch(t, []attributeDef{
		{Name: "log.bool", AttributeScope: "log", Type: "bool"},
		{Name: "log.float", AttributeScope: "log", Type: "float64"},
		{Name: "log.int", AttributeScope: "log", Type: "int64"},
		{Name: "log.list", AttributeScope: "log", Type: "string[]"},
		{Name: "log.list", AttributeScope: "log", Type: "int64[]"},
		{Name: "log.string", AttributeScope: "log", Type: "string"},
		{Name: "service.name", AttributeScope: "resource", Type: "string"},
		{Name: "service.version", AttributeScope: "resource", Type: "string"},
	}, attributes)

	// The ordering the function does promise: non-decreasing by (key, scope).
	for i := 1; i < len(attributes); i++ {
		prev, cur := attributes[i-1], attributes[i]
		assert.LessOrEqual(t, prev.Name+"\x00"+prev.AttributeScope, cur.Name+"\x00"+cur.AttributeScope,
			"results must be ordered by key then scope")
	}

	// (1) The range is ignored, not merely generous.
	assert.ElementsMatch(t, attributes, getLogAttributeDefs(t, s, ctx, 0, 1),
		"GetLogAttributes must ignore its time range")

	// (2) A second batch two hours later, carrying keys the first batch never
	// used -- including a scope-scoped one, so all three scopes are covered.
	laterTime := baseTime + int64(2*time.Hour)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, createTestLogsPdataN(laterTime, 1), s.FlushedIDs())
	}))

	// Queried with the ORIGINAL window, which ends an hour before that batch.
	names := map[string]string{}
	for _, a := range getLogAttributeDefs(t, s, ctx, startTime, endTime) {
		names[a.Name] = a.AttributeScope
	}
	assert.Equal(t, "log", names["log.index"], "attributes outside the window must still be reported")
	assert.Equal(t, "log", names["flush_test"])
	assert.Equal(t, "resource", names["resource.key"])
	assert.Equal(t, "scope", names["scope.key"], "scope-scoped attributes belong to the log-side set")
}

// TestDeleteLogsByIDs verifies that multiple logs can be deleted by their IDs.
func TestDeleteLogsByIDs(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	baseTime := time.Now().UnixNano()
	ldata := createTestLogsPdata(baseTime)
	err := s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, ldata, s.FlushedIDs())
	})
	assert.NoError(t, err)

	entries := searchLogsAll(t, s, ctx)
	assert.Len(t, entries, 3)

	idsToDelete := []any{entries[0].ID, entries[1].ID}
	err = s.WithDBWrite(func(db *sql.DB) error {
		return logs.DeleteLogsByIDs(ctx, db, idsToDelete)
	})
	assert.NoError(t, err)

	entries = searchLogsAll(t, s, ctx)
	assert.Len(t, entries, 1)
}

// TestDeleteLogsByIDs_Empty verifies that deleting with an empty list is a no-op.
func TestDeleteLogsByIDs_Empty(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	err := s.WithDBWrite(func(db *sql.DB) error {
		return logs.DeleteLogsByIDs(ctx, db, []any{})
	})
	assert.NoError(t, err)
}

// TestIngestLogs_LargeBatchStaysConsistent ingests more logs in one call than
// the flush interval and asserts every record landed with its attributes
// intact. As with the spans version, it does not claim to test the flush
// itself -- that has no observable behaviour -- only that a batch larger than
// the interval ingests consistently. Sized from the constant: this said 250
// against an interval that had been raised to 500.
func TestIngestLogs_LargeBatchStaysConsistent(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	baseTime := time.Now().UnixNano()
	const batchSize = logs.FlushInterval + 1
	ldata := createTestLogsPdataN(baseTime, batchSize)
	err := s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, ldata, s.FlushedIDs())
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

func TestIngest_CanceledContext(t *testing.T) {
	t.Parallel()
	s, _ := storetest.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, createTestLogsPdataN(time.Now().UnixNano(), 1), s.FlushedIDs())
	})
	require.ErrorIs(t, err, context.Canceled)
}

func TestIngest_CanceledDuringIngest(t *testing.T) {
	t.Parallel()
	s, _ := storetest.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	ldata := createTestLogsPdataN(time.Now().UnixNano(), 100)

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.WithConn(func(conn driver.Conn) error {
			return logs.Ingest(ctx, conn, ldata, s.FlushedIDs())
		})
	}()
	cancel()

	err := <-errCh
	require.ErrorIs(t, err, context.Canceled)
}

// TestSearchLogs tests logs.Search with various query types.
func TestSearchLogs(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	baseTime := time.Now().UnixNano()
	ldata := createTestLogsPdata(baseTime)
	err := s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, ldata, s.FlushedIDs())
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
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.Search(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.Len(t, entries, 1)
		assert.Equal(t, "ERROR", entries[0].SeverityText)
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
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.Search(ctx, db, startTime, endTime, query)
		})
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
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.Search(ctx, db, startTime, endTime, query)
		})
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
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.Search(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.Len(t, entries, 1, "global search for span ID hex should match one log")
		assert.Equal(t, "ERROR", entries[0].SeverityText)
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
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.Search(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.Empty(t, entries)
	})

	// The search bar's ID-paste completion suggests `traceID = <32-hex>` and
	// `spanID = <16-hex>` field queries in wire form (no dashes). Both
	// columns compare as wire-form strings (mapLogFieldExpression), with
	// values dash-stripped and lowercased by the search package.
	t.Run("Field_TraceID_WireForm", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q3c",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{Name: "traceID", SearchScope: "field"},
				FieldOperator: "=",
				Value:         "00000000000000000000000000000099",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.Search(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.Len(t, entries, 3, "wire-form trace ID should match all logs in the trace")
	})

	t.Run("Field_SpanID_WireForm", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q3d",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{Name: "spanID", SearchScope: "field"},
				FieldOperator: "=",
				Value:         "0000000000000002",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.Search(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.Len(t, entries, 1, "wire-form span ID should match its log, not a cast error")
		assert.Equal(t, "ERROR", entries[0].SeverityText)
	})

	// Malformed IDs are a search that finds nothing, not a uuid cast error
	// bubbling up as -32603 (issue #276).
	t.Run("Field_TraceID_Garbage", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q3e",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{Name: "traceID", SearchScope: "field"},
				FieldOperator: "=",
				Value:         "not-a-trace",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.Search(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err, "garbage trace ID must not surface a cast error")
		assert.Empty(t, parseSummaries(raw))
	})

	t.Run("Field_SpanID_Garbage", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q3f",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{Name: "spanID", SearchScope: "field"},
				FieldOperator: "=",
				Value:         "zz-definitely-not-hex",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.Search(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err, "garbage span ID must not surface a cast error")
		assert.Empty(t, parseSummaries(raw))
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
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.Search(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.Len(t, entries, 1)
		assert.Equal(t, "ERROR", entries[0].SeverityText)
	})

	// spanID compares in wire form (16-char hex, as served by the API). The
	// padded dashed-uuid storage form is an internal detail and no longer
	// matches -- it only ever worked as a side effect of the raw uuid-column
	// comparison that broke wire-form input.
	t.Run("Field_SpanID", func(t *testing.T) {
		query := &search.QueryNode{
			ID:   "q5",
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{Name: "spanID", SearchScope: "field"},
				FieldOperator: "=",
				Value:         "0000000000000001",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.Search(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.Len(t, entries, 1)
		assert.Equal(t, "INFO", entries[0].SeverityText)
	})

	// Dashed uuid input keeps working: the search package strips dashes
	// before binding, so it compares equal to the wire-form column.
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
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.Search(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.Len(t, entries, 3, "all three logs share the same trace")
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
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.Search(ctx, db, startTime, endTime, query)
		})
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
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.Search(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.Len(t, entries, 1)
		assert.Equal(t, "WARN", entries[0].SeverityText)
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
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.Search(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		entries := parseSummaries(raw)
		assert.Len(t, entries, 1)
		assert.Equal(t, "INFO", entries[0].SeverityText)
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
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.Search(ctx, db, startTime, endTime, query)
		})
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
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.Search(ctx, db, startTime, endTime, query)
		})
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
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.Search(ctx, db, startTime, endTime, query)
		})
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
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.Search(ctx, db, startTime, endTime, query)
		})
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
	t.Parallel()
	s, ctx := storetest.New(t)

	baseTime := time.Now().UnixNano()
	err := s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, createTestLogsPdata(baseTime), s.FlushedIDs())
	})
	require.NoError(t, err)

	// The path to the source of truth changed with the dictionary: there is no
	// log-keyed resource attribute row left to left-join, so it resolves through
	// resource_id -> resources.attribute_ids -> attributes, which attr_value
	// does in one step. logs.resource_id is NOT NULL with an FK, so the inner
	// join cannot drop a row; attr_value yields NULL for an absent key, hence
	// the coalesce against the column's '' default. Mirrors the spans test.
	mismatches := countRows(t, s, ctx, `
		select count(*) from logs l
		join resources r on r.id = l.resource_id
		where l.service_name <> coalesce(attr_value(r.attribute_ids, 'service.name'), '')
	`)
	assert.Equal(t, 0, mismatches,
		"logs.service_name must equal the source resource attribute (or '' when absent)")

	// Guard against a vacuous pass: zero mismatches over zero joined rows would
	// look identical to the invariant holding.
	joined := countRows(t, s, ctx, `
		select count(*) from logs l join resources r on r.id = l.resource_id
	`)
	assert.Equal(t, countRows(t, s, ctx, "select count(*) from logs"), joined,
		"every log must resolve to a resource row, or the check above passes vacuously")
	assert.Greater(t, joined, 0)
}
