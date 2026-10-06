package query_test

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	storemetrics "github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/query"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

type queryResponse struct {
	Columns   []query.Column      `json:"columns"`
	Rows      [][]json.RawMessage `json:"rows"`
	Truncated bool                `json:"truncated"`
}

func decodeResponse(t *testing.T, raw json.RawMessage) queryResponse {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var response queryResponse
	require.NoError(t, decoder.Decode(&response))
	return response
}

func withConnection(t *testing.T, databasePath string, fn func(*sql.Conn)) {
	t.Helper()
	viewerStore, err := store.NewStore(context.Background(), databasePath, zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, viewerStore.Close()) })
	require.NoError(t, viewerStore.WithDBRead(func(db *sql.DB) error {
		conn, err := db.Conn(context.Background())
		require.NoError(t, err)
		defer func() { require.NoError(t, conn.Close()) }()
		fn(conn)
		return nil
	}))
}

func TestExecuteReturnsNativeScalarsAndExactTopLevelJSON(t *testing.T) {
	withConnection(t, "", func(conn *sql.Conn) {
		raw, err := query.Execute(context.Background(), conn, `select
          9007199254740993::bigint as signed,
          true as flag,
          1.25::double as ratio,
          'text'::varchar as label,
          null::integer as absent,
          json('{"n":9007199254740993,"k":1,"k":2}') as payload`, 25)
		require.NoError(t, err)
		result := decodeResponse(t, raw)
		require.Equal(t, []query.Column{
			{Name: "signed", Type: "BIGINT"},
			{Name: "flag", Type: "BOOLEAN"},
			{Name: "ratio", Type: "DOUBLE"},
			{Name: "label", Type: "VARCHAR"},
			{Name: "absent", Type: "INTEGER"},
			{Name: "payload", Type: "JSON"},
		}, result.Columns)
		assert.JSONEq(t, `9007199254740993`, string(result.Rows[0][0]))
		assert.JSONEq(t, `true`, string(result.Rows[0][1]))
		assert.JSONEq(t, `1.25`, string(result.Rows[0][2]))
		assert.JSONEq(t, `"text"`, string(result.Rows[0][3]))
		assert.JSONEq(t, `null`, string(result.Rows[0][4]))
		assert.Equal(t, json.RawMessage(`{"n":9007199254740993,"k":1,"k":2}`), result.Rows[0][5])
		assert.Contains(t, string(raw), `9007199254740993`)
		assert.Contains(t, string(raw), `{"n":9007199254740993,"k":1,"k":2}`)
	})
}

func TestExecuteLimitLookaheadAndIntrospection(t *testing.T) {
	withConnection(t, "", func(conn *sql.Conn) {
		for _, tc := range []struct {
			name      string
			statement string
			limit     uint64
			rowCount  int
			truncated bool
		}{
			{name: "zero", statement: "select * from range(1)", limit: 0, truncated: true},
			{name: "exact", statement: "select * from range(25)", limit: 25, rowCount: 25},
			{name: "lookahead", statement: "select * from range(26)", limit: 25, rowCount: 25, truncated: true},
			{name: "submitted limit remains authoritative", statement: "select * from range(26) limit 1", limit: 25, rowCount: 1},
			{name: "maximum avoids overflow", statement: "select 1", limit: math.MaxUint64, rowCount: 1},
		} {
			t.Run(tc.name, func(t *testing.T) {
				raw, err := query.Execute(context.Background(), conn, tc.statement, tc.limit)
				require.NoError(t, err)
				result := decodeResponse(t, raw)
				assert.Len(t, result.Rows, tc.rowCount)
				assert.Equal(t, tc.truncated, result.Truncated)
			})
		}

		for _, statement := range []string{"SHOW TABLES", "DESCRIBE spans"} {
			raw, err := query.Execute(context.Background(), conn, statement, 25)
			require.NoError(t, err, statement)
			result := decodeResponse(t, raw)
			assert.NotEmpty(t, result.Columns)
		}
	})
}

func TestExecuteRejectsWritesAndLeavesConnectionReusable(t *testing.T) {
	withConnection(t, "", func(conn *sql.Conn) {
		for _, statement := range []string{
			"create table nope(i integer)",
			"delete from spans",
			"select 1; select 2",
			"copy spans to '/tmp/query-test.csv'",
		} {
			_, err := query.Execute(context.Background(), conn, statement, 25)
			assert.ErrorIs(t, err, query.ErrReadOnly, statement)
		}

		raw, err := query.Execute(context.Background(), conn, "select 42::integer as answer", 25)
		require.NoError(t, err)
		result := decodeResponse(t, raw)
		assert.JSONEq(t, `42`, string(result.Rows[0][0]))
	})
}

func TestExecuteCancellationLeavesConnectionReusable(t *testing.T) {
	withConnection(t, "", func(conn *sql.Conn) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		_, err := query.Execute(ctx, conn, "select sum(i) from range(1000000000) t(i)", 25)
		require.Error(t, err)
		assert.True(t, errors.Is(err, context.DeadlineExceeded), "%v", err)

		raw, err := query.Execute(context.Background(), conn, "select 1::integer", 25)
		require.NoError(t, err)
		result := decodeResponse(t, raw)
		assert.JSONEq(t, `1`, string(result.Rows[0][0]))
	})
}

func TestExecuteReadsBothStoreModes(t *testing.T) {
	for _, tc := range []struct {
		name string
		path func(*testing.T) string
	}{
		{name: "in-memory", path: func(*testing.T) string { return "" }},
		{name: "persistent", path: func(t *testing.T) string { return filepath.Join(t.TempDir(), "viewer.db") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withConnection(t, tc.path(t), func(conn *sql.Conn) {
				raw, err := query.Execute(context.Background(), conn,
					"select count(*)::bigint as span_count from spans", 25)
				require.NoError(t, err)
				result := decodeResponse(t, raw)
				assert.JSONEq(t, `0`, string(result.Rows[0][0]))
			})
		})
	}
}

func TestExecuteUsesDuckDBToPreserveSQLCreatedNestedJSON(t *testing.T) {
	withConnection(t, "", func(conn *sql.Conn) {
		raw, err := query.Execute(context.Background(), conn,
			`select {'payload': json('{"n":9007199254740993}')} as composite`, 25)
		require.NoError(t, err)
		result := decodeResponse(t, raw)
		assert.Equal(t, `STRUCT(payload JSON)`, result.Columns[0].Type)
		assert.JSONEq(t, `{"payload":{"n":9007199254740993}}`, string(result.Rows[0][0]))
	})
}

func TestExecutePreservesDuplicateColumnsAndEmptyRows(t *testing.T) {
	withConnection(t, "", func(conn *sql.Conn) {
		raw, err := query.Execute(context.Background(), conn,
			"select 1 as duplicate, 2 as duplicate where false", 25)
		require.NoError(t, err)
		result := decodeResponse(t, raw)
		assert.Equal(t, []query.Column{
			{Name: "duplicate", Type: "INTEGER"},
			{Name: "duplicate", Type: "INTEGER"},
		}, result.Columns)
		assert.Empty(t, result.Rows)
		assert.False(t, result.Truncated)
	})
}

func TestExecuteBuildsReadableDoubleJSON(t *testing.T) {
	withConnection(t, "", func(conn *sql.Conn) {
		raw, err := query.Execute(context.Background(), conn, `select
			1.5::double as finite,
			'-0.0'::double as negative_zero,
			'nan'::double as nan,
			'infinity'::double as positive_infinity,
			'-infinity'::double as negative_infinity`, 25)
		require.NoError(t, err)
		result := decodeResponse(t, raw)
		assert.Equal(t, "1.5", string(result.Rows[0][0]))
		assert.Equal(t, "-0.0", string(result.Rows[0][1]))
		assert.Equal(t, `"NaN"`, string(result.Rows[0][2]))
		assert.Equal(t, `"Infinity"`, string(result.Rows[0][3]))
		assert.Equal(t, `"-Infinity"`, string(result.Rows[0][4]))
	})
}

func TestExecuteReturnsIngestedNestedTypedValues(t *testing.T) {
	viewerStore, err := store.NewStore(context.Background(), "", zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, viewerStore.Close()) })

	telemetry := plog.NewLogs()
	record := telemetry.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
	record.SetTimestamp(pcommon.Timestamp(1))
	body := record.Body().SetEmptyMap()
	body.PutInt("large", 9_007_199_254_740_993)
	body.PutEmptySlice("nested").AppendEmpty().SetInt(9_007_199_254_740_993)
	record.Attributes().PutInt("direct", 9_007_199_254_740_993)
	record.Attributes().PutEmptyMap("map").PutInt("large", 9_007_199_254_740_993)
	require.NoError(t, viewerStore.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(context.Background(), conn, telemetry, viewerStore.FlushedIDs())
	}))

	require.NoError(t, viewerStore.WithDBRead(func(db *sql.DB) error {
		conn, err := db.Conn(context.Background())
		require.NoError(t, err)
		defer conn.Close()
		raw, err := query.Execute(context.Background(), conn, `
			select l.body, a.value
			from logs l, unnest(l.attribute_ids) ids(attribute_id)
			join attributes a on a.id = ids.attribute_id
			where a.key = 'direct'`, 25)
		require.NoError(t, err)
		result := decodeResponse(t, raw)
		require.Len(t, result.Rows, 1)
		assert.Contains(t, string(result.Rows[0][0]), `{"kind":"int64","value":"9007199254740993"}`)
		assert.JSONEq(t, `{"kind":"int64","value":"9007199254740993"}`, string(result.Rows[0][1]))
		return nil
	}))
}

func TestExecuteReturnsCurrentMetricDatapointFields(t *testing.T) {
	viewerStore, err := store.NewStore(context.Background(), "", zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, viewerStore.Close()) })

	now := time.Now().Add(-time.Minute)
	telemetry := pmetric.NewMetrics()
	rm := telemetry.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "checkout")
	metric := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	metric.SetName("checkout.queue.depth")
	point := metric.SetEmptyGauge().DataPoints().AppendEmpty()
	point.SetTimestamp(pcommon.Timestamp(now.UnixNano()))
	point.SetStartTimestamp(pcommon.Timestamp(now.Add(-time.Hour).UnixNano()))
	point.SetIntValue(9_007_199_254_740_993)
	point.Attributes().PutStr("queue", "payments")
	require.NoError(t, viewerStore.WithConn(func(conn driver.Conn) error {
		return storemetrics.Ingest(context.Background(), conn, telemetry, viewerStore.FlushedIDs())
	}))

	require.NoError(t, viewerStore.WithDBRead(func(db *sql.DB) error {
		conn, err := db.Conn(context.Background())
		require.NoError(t, err)
		defer conn.Close()
		raw, err := query.Execute(context.Background(), conn, `
			select
				m.id::varchar as metric_ref,
				ms.id::varchar as series_ref,
				m.name,
				m.metric_type,
				m.aggregation_temporality,
				m.is_monotonic,
				d.timestamp,
				d.start_time,
				d.value_type,
				d.int_value,
				d.double_value,
				d.count,
				d.sum,
				d.min,
				d.max
			from metrics as m
			join metric_series as ms on ms.metric_id = m.id
			join metric_datapoints as d
				on d.metric_id = m.id and d.series_id = ms.id
			where d.timestamp >= epoch_ns(current_timestamp - interval '1 hour')
				and d.timestamp <= epoch_ns(current_timestamp)
			order by d.timestamp desc, m.name, series_ref`, 25)
		require.NoError(t, err)
		result := decodeResponse(t, raw)
		require.Len(t, result.Rows, 1)
		assert.Equal(t, "UBIGINT", result.Columns[6].Type)
		assert.Equal(t, "UBIGINT", result.Columns[7].Type)
		assert.NotEqual(t, `""`, string(result.Rows[0][0]))
		assert.NotEqual(t, `""`, string(result.Rows[0][1]))
		assert.JSONEq(t, `"checkout.queue.depth"`, string(result.Rows[0][2]))
		assert.JSONEq(t, `"Gauge"`, string(result.Rows[0][3]))
		assert.JSONEq(t, `0`, string(result.Rows[0][4]))
		assert.JSONEq(t, `false`, string(result.Rows[0][5]))
		assert.JSONEq(t, strconv.FormatInt(now.UnixNano(), 10), string(result.Rows[0][6]))
		assert.JSONEq(t, strconv.FormatInt(now.Add(-time.Hour).UnixNano(), 10), string(result.Rows[0][7]))
		assert.JSONEq(t, `"Int"`, string(result.Rows[0][8]))
		assert.JSONEq(t, `9007199254740993`, string(result.Rows[0][9]))
		for _, index := range []int{10, 11, 12, 13, 14} {
			assert.JSONEq(t, `null`, string(result.Rows[0][index]))
		}
		assert.False(t, result.Truncated)
		return nil
	}))
}

func TestExecuteCountsCommonTypedSpanAttributeValuesByOwningSpan(t *testing.T) {
	viewerStore, err := store.NewStore(context.Background(), "", zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, viewerStore.Close()) })

	telemetry := ptrace.NewTraces()
	ss := telemetry.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans()
	now := time.Now()
	for i := 0; i < 5; i++ {
		span := ss.AppendEmpty()
		traceByte := []byte{1, 2, 1, 3, 4}[i]
		spanByte := []byte{1, 1, 2, 3, 4}[i]
		span.SetTraceID(pcommon.TraceID{15: traceByte})
		span.SetSpanID(pcommon.SpanID{7: spanByte})
		span.SetStartTimestamp(pcommon.Timestamp(now.UnixNano()))
		switch i {
		case 0, 1:
			span.Attributes().PutStr("http.request.method", "GET")
		case 2:
			span.Attributes().PutStr("http.request.method", "POST")
		case 3:
			span.Attributes().PutInt("http.request.method", 7)
		case 4:
			span.SetStartTimestamp(pcommon.Timestamp(now.Add(time.Hour).UnixNano()))
			span.Attributes().PutStr("http.request.method", "GET")
		}
	}
	require.NoError(t, viewerStore.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(context.Background(), conn, telemetry, viewerStore.FlushedIDs())
	}))

	require.NoError(t, viewerStore.WithDBRead(func(db *sql.DB) error {
		conn, err := db.Conn(context.Background())
		require.NoError(t, err)
		defer conn.Close()
		raw, err := query.Execute(context.Background(), conn, `
			with owned_values as (
				select s.trace_id, s.span_id,
					json_extract_string(a.value, '$.kind') as value_kind,
					a.value as tagged_value
				from spans s
				cross join unnest(s.attribute_ids) owned(attribute_id)
				join attributes a on a.id = owned.attribute_id
				where s.start_time >= epoch_ns(current_timestamp - interval '1 hour')
				  and s.start_time <= epoch_ns(current_timestamp)
				  and a.key = 'http.request.method'
			), value_counts as (
				select value_kind, tagged_value,
					count(distinct struct_pack(trace_id := trace_id, span_id := span_id)) as owning_span_count
				from owned_values group by value_kind, tagged_value
			), denominator as (
				select count(distinct struct_pack(trace_id := trace_id, span_id := span_id)) as owning_span_count
				from owned_values
			)
			select value_kind, tagged_value, value_counts.owning_span_count,
				value_counts.owning_span_count::double /
					nullif(denominator.owning_span_count, 0)::double as relative_frequency
			from value_counts cross join denominator
			order by value_counts.owning_span_count desc, value_kind, tagged_value::varchar
			limit 10`, 10)
		require.NoError(t, err)
		result := decodeResponse(t, raw)
		require.Len(t, result.Rows, 3)
		assert.JSONEq(t, `2`, string(result.Rows[0][2]))
		assert.JSONEq(t, `0.5`, string(result.Rows[0][3]))
		assert.Contains(t, string(result.Rows[0][1]), `"kind":"string"`)
		assert.Contains(t, string(raw), `"kind":"int64","value":"7"`)
		assert.False(t, result.Truncated)
		return nil
	}))
}
