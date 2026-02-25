package store

import (
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

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

// searchLogsAll returns SearchLogs with a wide time range and nil query to get all logs.
func searchLogsAll(t *testing.T, helper *TestHelper) []logEntryJSON {
	t.Helper()
	const maxNano = 1<<63 - 1
	raw, err := helper.Store.SearchLogs(helper.Ctx, 0, maxNano, nil)
	assert.NoError(t, err)
	var entries []logEntryJSON
	assert.NoError(t, json.Unmarshal(raw, &entries))
	return entries
}

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
	DroppedAttributesCount  uint32          `json:"droppedAttributesCount"`
	Flags                  uint32          `json:"flags"`
	EventName              string          `json:"eventName"`
	Attributes             []attrKeyValue  `json:"attributes"`
}

type resourceLogJSON struct {
	Attributes             []attrKeyValue `json:"attributes"`
	DroppedAttributesCount uint32         `json:"droppedAttributesCount"`
}

type scopeLogJSON struct {
	Name                   string        `json:"name"`
	Version                string        `json:"version"`
	Attributes             []attrKeyValue `json:"attributes"`
	DroppedAttributesCount uint32        `json:"droppedAttributesCount"`
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
	helper, teardown := SetupTest(t)
	defer teardown()

	baseTime := time.Now().UnixNano()
	logs := createTestLogsPdata(baseTime)
	err := helper.Store.IngestLogs(helper.Ctx, logs)
	assert.NoError(t, err)

	entries := searchLogsAll(t, helper)
	assert.Len(t, entries, 3)

	// Order: newest first by effective time — 0002 (t+150ms), 0007 (t+100ms), 0001 (t+0)
	assert.Equal(t, "0000000000000002", entries[0].SpanID)
	assert.Equal(t, "0000000000000007", entries[1].SpanID)
	assert.Equal(t, "0000000000000001", entries[2].SpanID)
}

// TestEmptyLogs verifies handling of empty log lists and empty store.
func TestEmptyLogs(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	err := helper.Store.IngestLogs(helper.Ctx, plog.NewLogs())
	assert.NoError(t, err)

	entries := searchLogsAll(t, helper)
	assert.Empty(t, entries)
}

// TestClearLogs verifies that all logs can be cleared from the store.
func TestClearLogs(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	baseTime := time.Now().UnixNano()
	logs := createTestLogsPdata(baseTime)
	err := helper.Store.IngestLogs(helper.Ctx, logs)
	assert.NoError(t, err)

	entries := searchLogsAll(t, helper)
	assert.Len(t, entries, 3)

	err = helper.Store.ClearLogs(helper.Ctx)
	assert.NoError(t, err)

	entries = searchLogsAll(t, helper)
	assert.Empty(t, entries)
}

// TestLogSuite runs a comprehensive suite on the same three-log dataset.
func TestLogSuite(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	baseTime := time.Now().UnixNano()
	logs := createTestLogsPdata(baseTime)
	err := helper.Store.IngestLogs(helper.Ctx, logs)
	assert.NoError(t, err, "failed to ingest test logs")

	t.Run("LogOrdering", func(t *testing.T) {
		entries := searchLogsAll(t, helper)
		assert.Len(t, entries, 3)
		assert.Equal(t, "0000000000000002", entries[0].SpanID)
		assert.Equal(t, "0000000000000007", entries[1].SpanID)
		assert.Equal(t, "0000000000000001", entries[2].SpanID)
	})

	t.Run("LogSeverity", func(t *testing.T) {
		entries := searchLogsAll(t, helper)
		assert.Equal(t, "ERROR", entries[0].SeverityText)
		assert.Equal(t, int32(plog.SeverityNumberError), entries[0].SeverityNumber)
		assert.Equal(t, "WARN", entries[1].SeverityText)
		assert.Equal(t, "INFO", entries[2].SeverityText)
		assert.Equal(t, int32(plog.SeverityNumberInfo), entries[2].SeverityNumber)
	})

	t.Run("LogBody", func(t *testing.T) {
		entries := searchLogsAll(t, helper)
		assert.Equal(t, "Operation failed", entries[0].Body)
		assert.Equal(t, "Operation warning", entries[1].Body)
		assert.Contains(t, entries[2].Body, "Operation started")
	})

	t.Run("LogTimestamp", func(t *testing.T) {
		entries := searchLogsAll(t, helper)
		assert.Equal(t, int64(0), entries[0].Timestamp)
		assert.Equal(t, baseTime+150*int64(time.Millisecond), entries[0].ObservedTimestamp)
		assert.NotZero(t, entries[1].Timestamp)
		assert.NotZero(t, entries[2].Timestamp)
	})

	t.Run("LogResource", func(t *testing.T) {
		entries := searchLogsAll(t, helper)
		resMap := attrMap(entries[0].Resource.Attributes)
		assert.Equal(t, "test-service", resMap["service.name"])
		assert.Equal(t, "1.0.0", resMap["service.version"])
		assert.Equal(t, uint32(0), entries[2].Resource.DroppedAttributesCount)
	})

	t.Run("LogScope", func(t *testing.T) {
		entries := searchLogsAll(t, helper)
		for i := range entries {
			assert.Equal(t, "test-scope", entries[i].Scope.Name)
			assert.Equal(t, "v1.0.0", entries[i].Scope.Version)
		}
	})

	t.Run("LogAttributes", func(t *testing.T) {
		entries := searchLogsAll(t, helper)
		attrs0 := attrMap(entries[0].Attributes)
		assert.Equal(t, "log-b", attrs0["log.string"])
		assert.Equal(t, "24", attrs0["log.int"])
		assert.Equal(t, "2.71", attrs0["log.float"])
		assert.Equal(t, "false", attrs0["log.bool"])

		attrs2 := attrMap(entries[2].Attributes)
		assert.Equal(t, "log-a", attrs2["log.string"])
		assert.Equal(t, "42", attrs2["log.int"])
		assert.Equal(t, "3.14", attrs2["log.float"])
		assert.Equal(t, "true", attrs2["log.bool"])
	})

	t.Run("LogMetadata", func(t *testing.T) {
		entries := searchLogsAll(t, helper)
		assert.Equal(t, uint32(1), entries[0].DroppedAttributesCount)
		assert.Equal(t, uint32(1), entries[0].Flags)
		assert.Equal(t, "event.b", entries[0].EventName)
		assert.Equal(t, "event.c", entries[1].EventName)
		assert.Equal(t, "event.a", entries[2].EventName)
	})
}

// TestDeleteLogByID verifies that a single log can be deleted by its ID.
func TestDeleteLogByID(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	baseTime := time.Now().UnixNano()
	logs := createTestLogsPdata(baseTime)
	err := helper.Store.IngestLogs(helper.Ctx, logs)
	assert.NoError(t, err)

	entries := searchLogsAll(t, helper)
	assert.Len(t, entries, 3)

	targetID := entries[0].ID
	assert.NotEmpty(t, targetID)

	err = helper.Store.DeleteLogByID(helper.Ctx, targetID)
	assert.NoError(t, err)

	entries = searchLogsAll(t, helper)
	assert.Len(t, entries, 2)
	for _, e := range entries {
		assert.NotEqual(t, targetID, e.ID)
	}
}

// TestDeleteLogsByIDs verifies that multiple logs can be deleted by their IDs.
func TestDeleteLogsByIDs(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	baseTime := time.Now().UnixNano()
	logs := createTestLogsPdata(baseTime)
	err := helper.Store.IngestLogs(helper.Ctx, logs)
	assert.NoError(t, err)

	entries := searchLogsAll(t, helper)
	assert.Len(t, entries, 3)

	idsToDelete := []any{entries[0].ID, entries[1].ID}
	err = helper.Store.DeleteLogsByIDs(helper.Ctx, idsToDelete)
	assert.NoError(t, err)

	entries = searchLogsAll(t, helper)
	assert.Len(t, entries, 1)
}

// TestDeleteLogsByIDs_Empty verifies that deleting with an empty list is a no-op.
func TestDeleteLogsByIDs_Empty(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	err := helper.Store.DeleteLogsByIDs(helper.Ctx, []any{})
	assert.NoError(t, err)
}

// TestSearchLogs tests SearchLogs with various query types.
func TestSearchLogs(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	baseTime := time.Now().UnixNano()
	logs := createTestLogsPdata(baseTime)
	err := helper.Store.IngestLogs(helper.Ctx, logs)
	assert.NoError(t, err)

	startTime := baseTime - 24*int64(time.Hour)
	endTime := baseTime + 24*int64(time.Hour)

	parseEntries := func(raw json.RawMessage) []logEntryJSON {
		var e []logEntryJSON
		assert.NoError(t, json.Unmarshal(raw, &e))
		return e
	}

	t.Run("GlobalSearch_Body", func(t *testing.T) {
		query := &QueryNode{
			ID:   "q1",
			Type: "condition",
			Query: &Query{
				Field:         &FieldDefinition{SearchScope: "global"},
				FieldOperator: "CONTAINS",
				Value:         "Operation failed",
			},
		}
		raw, err := helper.Store.SearchLogs(helper.Ctx, startTime, endTime, query)
		assert.NoError(t, err)
		entries := parseEntries(raw)
		assert.Len(t, entries, 1)
		assert.Equal(t, "0000000000000002", entries[0].SpanID)
	})

	t.Run("GlobalSearch_EventName", func(t *testing.T) {
		query := &QueryNode{
			ID:   "q2",
			Type: "condition",
			Query: &Query{
				Field:         &FieldDefinition{SearchScope: "global"},
				FieldOperator: "CONTAINS",
				Value:         "event.a",
			},
		}
		raw, err := helper.Store.SearchLogs(helper.Ctx, startTime, endTime, query)
		assert.NoError(t, err)
		entries := parseEntries(raw)
		assert.NotEmpty(t, entries)
		assert.Equal(t, "event.a", entries[0].EventName)
	})

	t.Run("GlobalSearch_NoResults", func(t *testing.T) {
		query := &QueryNode{
			ID:   "q3",
			Type: "condition",
			Query: &Query{
				Field:         &FieldDefinition{SearchScope: "global"},
				FieldOperator: "CONTAINS",
				Value:         "nonexistent-log-text-xyz",
			},
		}
		raw, err := helper.Store.SearchLogs(helper.Ctx, startTime, endTime, query)
		assert.NoError(t, err)
		entries := parseEntries(raw)
		assert.Empty(t, entries)
	})

	t.Run("Field_SeverityText", func(t *testing.T) {
		query := &QueryNode{
			ID:   "q4",
			Type: "condition",
			Query: &Query{
				Field:         &FieldDefinition{Name: "severityText", SearchScope: "field"},
				FieldOperator: "=",
				Value:         "ERROR",
			},
		}
		raw, err := helper.Store.SearchLogs(helper.Ctx, startTime, endTime, query)
		assert.NoError(t, err)
		entries := parseEntries(raw)
		assert.Len(t, entries, 1)
		assert.Equal(t, "ERROR", entries[0].SeverityText)
	})

	t.Run("Field_SpanID", func(t *testing.T) {
		query := &QueryNode{
			ID:   "q5",
			Type: "condition",
			Query: &Query{
				Field:         &FieldDefinition{Name: "spanID", SearchScope: "field"},
				FieldOperator: "=",
				Value:         "0000000000000001",
			},
		}
		raw, err := helper.Store.SearchLogs(helper.Ctx, startTime, endTime, query)
		assert.NoError(t, err)
		entries := parseEntries(raw)
		assert.Len(t, entries, 1)
		assert.Equal(t, "0000000000000001", entries[0].SpanID)
	})

	t.Run("Attribute_LogString", func(t *testing.T) {
		query := &QueryNode{
			ID:   "q6",
			Type: "condition",
			Query: &Query{
				Field: &FieldDefinition{
					Name:           "log.string",
					SearchScope:    "attribute",
					AttributeScope: "log",
					Type:           "string",
				},
				FieldOperator: "=",
				Value:         "log-b",
			},
		}
		raw, err := helper.Store.SearchLogs(helper.Ctx, startTime, endTime, query)
		assert.NoError(t, err)
		entries := parseEntries(raw)
		assert.Len(t, entries, 1)
		assert.Equal(t, "log-b", attrMap(entries[0].Attributes)["log.string"])
	})

	t.Run("Attribute_Resource", func(t *testing.T) {
		query := &QueryNode{
			ID:   "q7",
			Type: "condition",
			Query: &Query{
				Field: &FieldDefinition{
					Name:           "service.name",
					SearchScope:    "attribute",
					AttributeScope: "resource",
					Type:           "string",
				},
				FieldOperator: "CONTAINS",
				Value:         "test-service",
			},
		}
		raw, err := helper.Store.SearchLogs(helper.Ctx, startTime, endTime, query)
		assert.NoError(t, err)
		entries := parseEntries(raw)
		assert.NotEmpty(t, entries)
		assert.Equal(t, "test-service", attrMap(entries[0].Resource.Attributes)["service.name"])
	})
}
