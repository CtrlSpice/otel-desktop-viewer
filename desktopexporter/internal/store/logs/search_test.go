package logs_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestSearchLogPageSelectedColumns(t *testing.T) {
	s, ctx := storetest.New(t)
	data := plog.NewLogs()
	rl := data.ResourceLogs().AppendEmpty()
	rl.SetSchemaUrl("https://example.test/resource")
	rl.Resource().Attributes().PutStr("same", "resource")
	sl := rl.ScopeLogs().AppendEmpty()
	sl.Scope().SetName("instrumentation")
	sl.Scope().Attributes().PutBool("same", false)
	l := sl.LogRecords().AppendEmpty()
	l.SetTimestamp(pcommon.Timestamp(math.MaxUint64))
	l.SetFlags(128)
	l.Attributes().PutInt("same", math.MaxInt64)
	l.Attributes().PutInt("flags", 0)
	l.Attributes().PutStr("x' OR 1=1 --", "literal key")
	l.Attributes().PutEmpty("")
	l.Attributes().PutDouble("negative-zero", math.Copysign(0, -1))
	l.Body().SetStr("body")
	other := sl.LogRecords().AppendEmpty()
	other.SetTimestamp(1)
	other.Body().SetStr("outside page")
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return logs.Ingest(ctx, conn, data) }))
	columns := []logs.Column{
		{Source: "log", Kind: "field", Key: "timeUnixNano"},
		{Source: "log", Kind: "field", Key: "flags"},
		{Source: "log", Kind: "attribute", Key: "flags"},
		{Source: "log", Kind: "attribute", Key: "same"},
		{Source: "resource", Kind: "attribute", Key: "same"},
		{Source: "scope", Kind: "attribute", Key: "same"},
		{Source: "log", Kind: "attribute", Key: "missing"},
		{Source: "log", Kind: "attribute", Key: ""},
		{Source: "log", Kind: "attribute", Key: "x' OR 1=1 --"},
		{Source: "log", Kind: "attribute", Key: "negative-zero"},
		{Source: "log", Kind: "field", Key: "traceId"},
		{Source: "resource", Kind: "field", Key: "schemaUrl"},
		{Source: "scope", Kind: "field", Key: "name"},
	}
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return logs.SearchPage(ctx, db, store.BoundedTimeRange(uint64(0), uint64(math.MaxUint64)), nil, search.ResultOptions{}, logs.PageOptions{Size: 1, Columns: columns})
	})
	require.NoError(t, err)
	var page struct {
		Total int `json:"total"`
		Logs  []struct {
			ColumnValues []struct {
				logs.Column
				Present   bool   `json:"present"`
				ValueType string `json:"valueType"`
				Preview   string `json:"preview"`
			} `json:"columnValues"`
		} `json:"logs"`
	}
	require.NoError(t, json.Unmarshal(raw, &page))
	require.Equal(t, 2, page.Total)
	require.Len(t, page.Logs, 1)
	want := []struct {
		present bool
		kind    string
		preview string
	}{
		{true, "string", "18446744073709551615"},
		{true, "int64", "128"},
		{true, "int64", "0"},
		{true, "int64", "9223372036854775807"},
		{true, "string", "resource"},
		{true, "bool", "false"},
		{false, "empty", "null"}, {true, "empty", "null"},
		{true, "string", "literal key"},
		{true, "double", "-0"},
		{false, "empty", "null"}, {true, "string", "https://example.test/resource"},
		{true, "string", "instrumentation"},
	}
	require.Len(t, page.Logs[0].ColumnValues, len(columns))
	for i, cell := range page.Logs[0].ColumnValues {
		require.Equal(t, columns[i], cell.Column)
		require.Equal(t, want[i].present, cell.Present, "%+v", columns[i])
		require.Equal(t, want[i].kind, cell.ValueType, "%+v", columns[i])
		require.Equal(t, want[i].preview, cell.Preview, "%+v", columns[i])
	}
}

func TestSearchLogPage(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	data := createTestLogsPdata(1700000000123456789)
	// Tie all effective timestamps, including the observed-time fallback.
	records := data.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords()
	for i := 0; i < records.Len(); i++ {
		record := records.At(i)
		record.SetTimestamp(0)
		record.SetObservedTimestamp(pcommon.Timestamp(18446744073709551614))
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return logs.Ingest(ctx, conn, data) }))
	type result struct {
		Logs  []logSummaryJSON `json:"logs"`
		Total int              `json:"total"`
	}
	fetch := func(offset, size int64, options search.ResultOptions, query any, window store.TimeRange) result {
		t.Helper()
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.SearchPage(ctx, db, window, query, options, logs.PageOptions{Size: size, Offset: offset})
		})
		require.NoError(t, err)
		var page result
		require.NoError(t, json.Unmarshal(raw, &page))
		require.NotNil(t, page.Logs)
		return page
	}
	window := store.BoundedTimeRange(0, ^uint64(0))
	all := fetch(0, 100, search.ResultOptions{}, nil, window)
	require.Equal(t, 3, all.Total)
	for i, log := range all.Logs {
		page := fetch(int64(i), 1, search.ResultOptions{}, nil, window)
		require.Equal(t, 3, page.Total)
		require.Equal(t, []logSummaryJSON{log}, page.Logs)
		require.Equal(t, "18446744073709551614", log.Timestamp)
		if i > 0 {
			require.Less(t, all.Logs[i-1].LogRef, log.LogRef)
		}
	}
	empty := fetch(3, 100, search.ResultOptions{}, nil, window)
	require.Empty(t, empty.Logs)
	require.Equal(t, 3, empty.Total)
	limit := int64(2)
	options := search.ResultOptions{Limit: &limit, Sort: &search.Sort{Field: "severity", Direction: "asc"}}
	limited := fetch(1, 100, options, nil, window)
	require.Equal(t, 2, limited.Total)
	require.Len(t, limited.Logs, 1)
	require.Equal(t, "WARN", limited.Logs[0].SeverityText)
	require.Empty(t, fetch(2, 100, options, nil, window).Logs)

	query := map[string]any{"id": "severity", "type": "condition", "query": map[string]any{"field": map[string]any{"name": "severityNumber", "searchScope": "field", "type": "int64"}, "fieldOperator": ">=", "value": "13"}}
	filtered := fetch(0, 1, search.ResultOptions{}, query, window)
	require.Equal(t, 2, filtered.Total)
	require.Len(t, filtered.Logs, 1)
	require.GreaterOrEqual(t, filtered.Logs[0].SeverityNumber, int32(13))
	outside := fetch(0, 100, search.ResultOptions{}, query, store.BoundedTimeRange(0, 1700000000000000000))
	require.Zero(t, outside.Total)
	require.Empty(t, outside.Logs)
}

func TestSearchLogPageSortsSeverityNumerically(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	data := createTestLogsPdata(1700000000123456789)
	records := data.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords()
	// Labels deliberately disagree with numeric and lexicographic-number order.
	for i, severity := range []plog.SeverityNumber{2, 10, 17} {
		records.At(i).SetSeverityNumber(severity)
		records.At(i).SetSeverityText([]string{"ZZZ", "AAA", "MMM"}[i])
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return logs.Ingest(ctx, conn, data) }))
	for _, tc := range []struct {
		direction string
		want      []int32
	}{{"asc", []int32{2, 10, 17}}, {"desc", []int32{17, 10, 2}}} {
		t.Run(tc.direction, func(t *testing.T) {
			for offset, want := range tc.want {
				raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
					return logs.SearchPage(ctx, db, store.BoundedTimeRange(0, ^uint64(0)), nil,
						search.ResultOptions{Sort: &search.Sort{Field: "severity", Direction: tc.direction}},
						logs.PageOptions{Size: 1, Offset: int64(offset)})
				})
				require.NoError(t, err)
				var page struct {
					Total int              `json:"total"`
					Logs  []logSummaryJSON `json:"logs"`
				}
				require.NoError(t, json.Unmarshal(raw, &page))
				require.Equal(t, 3, page.Total)
				require.Len(t, page.Logs, 1)
				require.Equal(t, want, page.Logs[0].SeverityNumber)
			}
		})
	}
}

type traceLogSummaryJSON struct {
	LogRef         string  `json:"logRef"`
	Timestamp      string  `json:"timestamp"`
	SpanID         *string `json:"spanID"`
	SeverityText   string  `json:"severityText"`
	SeverityNumber int32   `json:"severityNumber"`
	ServiceName    string  `json:"serviceName"`
	EventName      string  `json:"eventName"`
	BodyPreview    string  `json:"bodyPreview"`
}

func TestGetTraceLogSummariesPreservesTraceScopedRowsAndOrder(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	traceID := storetest.MustDecodeTraceID("00000000000000000000000000000099")
	otherTraceID := storetest.MustDecodeTraceID("00000000000000000000000000000098")
	directSpanID := storetest.MustDecodeSpanID("0000000000000001")
	missingSpanID := storetest.MustDecodeSpanID("0000000000000007")

	traces := ptrace.NewTraces()
	resourceSpans := traces.ResourceSpans().AppendEmpty()
	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()
	span := scopeSpans.Spans().AppendEmpty()
	span.SetTraceID(traceID)
	span.SetSpanID(directSpanID)
	span.SetName("direct-owner")
	span.SetStartTimestamp(1)
	span.SetEndTimestamp(2)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, traces)
	}))

	data := plog.NewLogs()
	resourceLogs := data.ResourceLogs().AppendEmpty()
	resourceLogs.Resource().Attributes().PutStr("service.name", "trace-service")
	scopeLogs := resourceLogs.ScopeLogs().AppendEmpty()

	traceOnly := scopeLogs.LogRecords().AppendEmpty()
	traceOnly.SetTraceID(traceID)
	traceOnly.SetTimestamp(0)
	traceOnly.SetObservedTimestamp(100)
	traceOnly.Body().SetStr("trace-only")

	missingSpan := scopeLogs.LogRecords().AppendEmpty()
	missingSpan.SetTraceID(traceID)
	missingSpan.SetSpanID(missingSpanID)
	missingSpan.SetTimestamp(200)
	missingSpan.SetObservedTimestamp(999)
	missingSpan.SetEventName("cart.updated")
	missingSpan.Body().SetStr("missing-span")

	for _, body := range []string{"tie-a", "tie-b"} {
		record := scopeLogs.LogRecords().AppendEmpty()
		record.SetTraceID(traceID)
		record.SetSpanID(directSpanID)
		record.SetTimestamp(300)
		record.SetObservedTimestamp(999)
		record.SetSeverityText("INFO")
		record.SetSeverityNumber(plog.SeverityNumberInfo)
		record.Body().SetStr(body)
	}

	otherTrace := scopeLogs.LogRecords().AppendEmpty()
	otherTrace.SetTraceID(otherTraceID)
	otherTrace.SetSpanID(directSpanID)
	otherTrace.SetTimestamp(50)
	otherTrace.Body().SetStr("same-span-other-trace")

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, data)
	}))

	// Fix the equal-time IDs so the expected ID tie-break is explicit rather
	// than dependent on random ingest UUIDs.
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		if _, err := db.ExecContext(ctx,
			`update logs set id = ?::uuid where body_preview(otel_value_json(body)) = ?`,
			"00000000-0000-0000-0000-0000000000ff", "tie-a"); err != nil {
			return err
		}
		_, err := db.ExecContext(ctx,
			`update logs set id = ?::uuid where body_preview(otel_value_json(body)) = ?`,
			"00000000-0000-0000-0000-000000000010", "tie-b")
		return err
	}))

	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return logs.GetTraceLogSummaries(ctx, db, "00000000000000000000000000000099")
	})
	require.NoError(t, err)

	var got []traceLogSummaryJSON
	require.NoError(t, json.Unmarshal(raw, &got))
	require.Len(t, got, 4)
	require.Equal(t, []string{"100", "200", "300", "300"}, []string{
		got[0].Timestamp, got[1].Timestamp, got[2].Timestamp, got[3].Timestamp,
	})
	require.Nil(t, got[0].SpanID)
	require.Equal(t, "trace-only", got[0].BodyPreview)
	require.Equal(t, "0000000000000007", *got[1].SpanID)
	require.Equal(t, "cart.updated", got[1].EventName)
	require.Equal(t, "missing-span", got[1].BodyPreview)
	require.Empty(t, got[0].EventName)
	require.Equal(t, "00000000-0000-0000-0000-000000000010", got[2].LogRef)
	require.Equal(t, "tie-b", got[2].BodyPreview)
	require.Equal(t, "00000000-0000-0000-0000-0000000000ff", got[3].LogRef)
	require.Equal(t, "tie-a", got[3].BodyPreview)
	for _, entry := range got {
		require.NotEqual(t, "same-span-other-trace", entry.BodyPreview)
	}

	var shape []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &shape))
	for _, entry := range shape {
		require.Len(t, entry, 8)
		for _, key := range []string{"logRef", "timestamp", "spanID", "severityText", "severityNumber", "serviceName", "eventName", "bodyPreview"} {
			require.Contains(t, entry, key)
		}
		require.NotContains(t, entry, "body")
		require.NotContains(t, entry, "attributes")
		require.NotContains(t, entry, "traceID")
	}

	empty, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return logs.GetTraceLogSummaries(ctx, db, "00000000000000000000000000000097")
	})
	require.NoError(t, err)
	require.JSONEq(t, `[]`, string(empty))
}

func TestGetTraceLogSummariesIgnoresSearchTimeRangeAndHasNoDefaultLimit(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	traceID := storetest.MustDecodeTraceID("00000000000000000000000000000096")

	data := plog.NewLogs()
	resourceLogs := data.ResourceLogs().AppendEmpty()
	scopeLogs := resourceLogs.ScopeLogs().AppendEmpty()
	for i := 0; i < 101; i++ {
		record := scopeLogs.LogRecords().AppendEmpty()
		record.SetTraceID(traceID)
		record.SetTimestamp(pcommon.Timestamp(10_000 + i))
		record.Body().SetStr("complete")
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, data)
	}))

	clipped, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return logs.Search(ctx, db, store.BoundedTimeRange(10_010, 10_011), nil)
	})
	require.NoError(t, err)
	var searchResult []logSummaryJSON
	require.NoError(t, json.Unmarshal(clipped, &searchResult))
	require.Len(t, searchResult, 2)

	complete, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return logs.GetTraceLogSummaries(ctx, db, "00000000000000000000000000000096")
	})
	require.NoError(t, err)
	var traceResult []traceLogSummaryJSON
	require.NoError(t, json.Unmarshal(complete, &traceResult))
	require.Len(t, traceResult, 101)
	require.Equal(t, "10000", traceResult[0].Timestamp)
	require.Equal(t, "10100", traceResult[100].Timestamp)
}

func TestGetTraceLogSummariesHonorsCanceledContext(t *testing.T) {
	t.Parallel()
	s, _ := storetest.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return logs.GetTraceLogSummaries(ctx, db, "00000000000000000000000000000099")
	})
	require.ErrorIs(t, err, context.Canceled)
}

func TestLogScalarMembershipExecution(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	baseTime := time.Now().UnixNano()
	data := createTestLogsPdata(baseTime)
	records := data.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords()
	records.At(0).Attributes().PutStr("same.key", "42")
	records.At(1).Attributes().PutInt("same.key", 42)
	records.At(0).Attributes().PutEmptySlice("received.array").AppendEmpty().SetStr("value")
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, data)
	}))

	searchCount := func(field *search.FieldDefinition, operator, value string) (int, error) {
		t.Helper()
		query := &search.QueryNode{Type: "condition", Query: &search.Query{
			Field: field, FieldOperator: operator, Value: value,
		}}
		var raw json.RawMessage
		err := s.WithDBRead(func(db *sql.DB) error {
			var err error
			raw, err = logs.Search(ctx, db, store.BoundedTimeRange(baseTime-int64(time.Hour), baseTime+int64(time.Hour)), query)
			return err
		})
		if err != nil {
			return 0, err
		}
		var summaries []logSummaryJSON
		if err := json.Unmarshal(raw, &summaries); err != nil {
			return 0, err
		}
		return len(summaries), nil
	}

	for _, tc := range []struct {
		name     string
		field    *search.FieldDefinition
		operator string
		value    string
		want     int
	}{
		{"text IN", &search.FieldDefinition{Name: "severityText", SearchScope: "field"}, "IN", `["ERROR"]`, 1},
		{"native integer NOT IN", &search.FieldDefinition{Name: "severityNumber", SearchScope: "field"}, "NOT IN", `["17"]`, 2},
		{"wire ID IN", &search.FieldDefinition{Name: "traceID", SearchScope: "field"}, "IN", `["00000000-0000-0000-0000-000000000099"]`, 3},
		{"malformed wire ID", &search.FieldDefinition{Name: "traceID", SearchScope: "field"}, "IN", `["not-a-trace"]`, 0},
		{"hostile text remains data", &search.FieldDefinition{Name: "severityText", SearchScope: "field"}, "IN", `["ERROR') OR TRUE --"]`, 0},
		{"string kind", &search.FieldDefinition{Name: "same.key", SearchScope: "attribute", AttributeScope: "log", Type: "string"}, "IN", `["42"]`, 1},
		{"integer kind remains text-bound", &search.FieldDefinition{Name: "same.key", SearchScope: "attribute", AttributeScope: "log", Type: "int64"}, "IN", `["42"]`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := searchCount(tc.field, tc.operator, tc.value)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	_, err := searchCount(&search.FieldDefinition{
		Name: "received.array", SearchScope: "attribute", AttributeScope: "log", Type: "array",
	}, "IN", `["value"]`)
	assert.ErrorIs(t, err, search.ErrInvalidQuery)
}

func TestNativeArrayAndBodyPredicates(t *testing.T) {
	s, ctx := storetest.New(t)
	data := plog.NewLogs()
	records := data.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords()
	for _, value := range []any{int64(math.MaxInt64), "9223372036854775807", int64(math.MaxInt64 - 1), true, "true", 42.5} {
		r := records.AppendEmpty()
		require.NoError(t, r.Attributes().PutEmptySlice("values").AppendEmpty().FromRaw(value))
	}
	records.AppendEmpty().Attributes().PutEmptySlice("values")
	records.AppendEmpty().Body().SetEmptyMap().PutStr("status", "ready")
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return logs.Ingest(ctx, conn, data) }))
	for _, tc := range []struct {
		kind, operator, value string
		want                  int
	}{
		{"int64[]", "CONTAINS", "9223372036854775807", 1},
		{"int64[]", "CONTAINS", "9223372036854775806", 1},
		{"string[]", "CONTAINS", "9223372036854775807", 1},
		{"boolean[]", "CONTAINS", "true", 1},
		{"string[]", "CONTAINS", "true", 1},
		{"float64[]", "CONTAINS", "42.5", 1},
		{"array", "CONTAINS", "9223372036854775807", 2},
		{"array", "NOT CONTAINS", "9223372036854775807", 5},
		{"body", "CONTAINS", "ready", 1},
		{"body", "CONTAINS", "kind", 0},
		{"body", "CONTAINS", "value", 0},
	} {
		t.Run(tc.kind+"/"+tc.operator+"/"+tc.value, func(t *testing.T) {
			field := &search.FieldDefinition{Owner: "log", Path: []any{"attributes", "values"}, Type: tc.kind}
			if tc.kind == "body" {
				field = &search.FieldDefinition{Name: "body", SearchScope: "field"}
			}
			query, args, err := logs.MatchingRowsSQL(&search.QueryNode{Type: "condition", Query: &search.Query{Field: field, FieldOperator: tc.operator, Value: tc.value}}, timerange.TimeRange{})
			require.NoError(t, err)
			require.NotContains(t, query, "otel_value_json")
			require.NotContains(t, query, "json_each")
			require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
				var count int
				if err := db.QueryRowContext(ctx, "select count(*) from ("+query+")", args...).Scan(&count); err != nil {
					return err
				}
				require.Equal(t, tc.want, count)
				return nil
			}))
		})
	}
}

// Regex and negated-regex searches must execute against DuckDB's ~ and !~
// operators.
func TestRegexpSearchExecutes(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", "svc")
	sl := rl.ScopeLogs().AppendEmpty()
	for _, body := range []string{"checkout failed hard", "payment ok"} {
		lr := sl.LogRecords().AppendEmpty()
		lr.SetTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
		lr.Body().SetStr(body)
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, ld)
	}))

	run := func(operator, pattern string) []any {
		tree := &search.QueryNode{
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{Name: "body", SearchScope: "field", Type: "string"},
				FieldOperator: operator,
				Value:         pattern,
			},
		}
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.Search(ctx, db, store.BoundedTimeRange(0, time.Now().UnixNano()+int64(time.Hour)), tree)
		})
		require.NoError(t, err, "%s must execute", operator)
		var out []any
		require.NoError(t, json.Unmarshal(raw, &out))
		return out
	}

	// ~ is a full match, so the pattern spans the whole body.
	assert.Len(t, run("REGEXP", "checkout.*"), 1)
	assert.Len(t, run("NOT REGEXP", "checkout.*"), 1)
	assert.Len(t, run("REGEXP", ".*"), 2)
	assert.Len(t, run("LIKE", "checkout%"), 1)
	assert.Len(t, run("NOT LIKE", "checkout%"), 1)
	assert.Len(t, run("LIKE", "payment o_"), 1)
	assert.Len(t, run("LIKE", "checkout"), 0)
}

func TestLogErrorNavigation(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	data := createTestLogsPdata(1700000000123456789)
	records := data.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords()
	records.At(2).SetSeverityNumber(21) // Fatal is also an error-navigation target.
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return logs.Ingest(ctx, conn, data) }))
	type target struct {
		LogRef string `json:"logRef"`
		Offset int    `json:"offset"`
	}
	type navigation struct {
		Count    int     `json:"count"`
		Index    *int    `json:"index"`
		Previous *target `json:"previous"`
		Next     *target `json:"next"`
	}
	options := search.ResultOptions{Sort: &search.Sort{Field: "severity", Direction: "asc"}}
	fetch := func(selected string, opts search.ResultOptions, query any) navigation {
		t.Helper()
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.GetErrorNavigation(ctx, db, store.BoundedTimeRange(0, ^uint64(0)), query, opts, selected)
		})
		require.NoError(t, err)
		var result navigation
		require.NoError(t, json.Unmarshal(raw, &result))
		return result
	}
	start := fetch("", options, nil)
	require.Equal(t, 2, start.Count)
	require.Nil(t, start.Index)
	require.Nil(t, start.Previous)
	require.NotNil(t, start.Next)
	require.Equal(t, 1, start.Next.Offset)
	first := fetch(start.Next.LogRef, options, nil)
	require.Equal(t, 1, *first.Index)
	require.Nil(t, first.Previous)
	require.NotNil(t, first.Next)
	require.Equal(t, 2, first.Next.Offset)
	last := fetch(first.Next.LogRef, options, nil)
	require.Equal(t, 2, *last.Index)
	require.Nil(t, last.Next)
	require.Equal(t, start.Next, last.Previous)
	limit := int64(1)
	options.Limit = &limit
	require.Equal(t, 0, fetch("", options, nil).Count)
	options.Limit = nil
	query := map[string]any{"id": "severity", "type": "condition", "query": map[string]any{"field": map[string]any{"name": "severityNumber", "searchScope": "field", "type": "int64"}, "fieldOperator": ">=", "value": "21"}}
	filtered := fetch("", options, query)
	require.Equal(t, 1, filtered.Count)
	require.Equal(t, 0, filtered.Next.Offset)
}

func TestBodyPreview(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	cases := []struct{ name, body, want string }{
		{"string", `{"kind":"string","value":"hello\n  world"}`, "hello world"},
		{"json string stays string", `{"kind":"string","value":"{\"message\":\"hello\"}"}`, `{"message":"hello"}`},
		{"map", `{"kind":"map","value":[{"key":"message","value":{"kind":"string","value":"failed"}},{"key":"count","value":{"kind":"int64","value":"9223372036854775807"}},{"key":"details","value":{"kind":"map","value":[]}}]}`, `{message: "failed", count: 9223372036854775807, details: object (0 fields)}`},
		{"empty map", `{"kind":"map","value":[]}`, "{}"},
		{"empty array", `{"kind":"array","value":[]}`, "[]"},
		{"array", `{"kind":"array","value":[{"kind":"int64","value":"9007199254740993"},{"kind":"bool","value":false},{"kind":"double","value":-0.0},{"kind":"string","value":"tail"}]}`, `[9007199254740993, false, -0.0, … (+1 items)]`},
		{"empty", `{"kind":"empty","value":null}`, "empty"},
		{"bytes", `{"kind":"bytes","value":"AAEC"}`, "bytes (base64): AAEC"},
		{"long", `{"kind":"string","value":"` + strings.Repeat("x", 301) + `"}`, strings.Repeat("x", 299) + "…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := storetest.Read(s, func(db *sql.DB) (string, error) {
				var result string
				err := db.QueryRowContext(ctx, `select body_preview(?::json)`, tc.body).Scan(&result)
				return result, err
			})
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestNativeBodyPreviewMatchesJSON(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	cases := []struct{ name, expression string }{
		{"string", `'hello\n  world'::variant`},
		{"JSON string", `'{"message":"hello"}'::variant`},
		{"Unicode limit", `repeat('雪😀', 151)::variant`},
		{"long string child", `[repeat('雪😀', 30)]::variant`},
		{"JSON quoting", `['<>&"\\雪' || chr(8) || chr(11) || chr(31) || chr(8232) || chr(8233)]::variant`},
		{"boolean", `[true, false]::variant`},
		{"long key", `map([repeat('雪😀', 20)], ['value'])::variant`},
		{"map order and count", `{'z': 'last', 'a': 9007199254740993::bigint, 'c': {'nested': [1,2,3]}, 'b': 'text'}::variant`},
		{"empty map", `map([]::varchar[], []::varchar[])::variant`},
		{"empty array", `[]::variant`},
		{"empty", `null::variant`},
		{"empty child", `[null::variant, 1::variant]::variant`},
		{"integer", `9223372036854775807::bigint::variant`},
		{"double", `1.5::double::variant`},
		{"negative zero", `'-0'::double::variant`},
		{"nonfinite", `['NaN'::double, 'Infinity'::double, '-Infinity'::double]::variant`},
		{"bytes", `'\x00\x01\x02'::blob::variant`},
		{"long bytes", `repeat('a', 40)::blob::variant`},
		{"array count", `[1,2,3,4,5]::variant`},
		{"nested array", `[[[1,2,3]]]::variant`},
		{"nested map", `{'context': {'messages': [repeat('nested',10000)]}}::variant`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
				var native, legacy string
				err := db.QueryRowContext(ctx, `select native_body_preview(body), body_preview(otel_value_json(body)) from (select `+tc.expression+` as body)`).Scan(&native, &legacy)
				require.NoError(t, err)
				require.Equal(t, legacy, native)
				return nil
			}))
		})
	}
}
