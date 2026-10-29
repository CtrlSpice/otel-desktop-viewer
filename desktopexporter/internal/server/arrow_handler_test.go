package server

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap/zaptest"
)

func readArrowDataset(t *testing.T, s *Server, method, params string) arrow.RecordBatch {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/data/"+method, strings.NewReader(params))
	s.server.Handler.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, arrowStreamContentType, w.Header().Get("Content-Type"))
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	data := w.Body.Bytes()
	require.True(t, bytes.HasSuffix(data, []byte{255, 255, 255, 255, 0, 0, 0, 0}))
	reader, err := ipc.NewReader(bytes.NewReader(data))
	require.NoError(t, err)
	defer reader.Release()
	require.True(t, reader.Next())
	record := reader.RecordBatch()
	record.Retain()
	t.Cleanup(record.Release)
	return record
}

func TestArrowAttributeDiscovery(t *testing.T) {
	h := setupHandlerWithData(t)
	data := buildTestTraces()
	span := data.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	span.SetSpanID(pcommon.SpanID{7: 0x42})
	require.NoError(t, span.Attributes().PutEmpty("nested").FromRaw([]any{map[string]any{"0": map[string]any{"": int64(9007199254740993)}}}))
	span.Attributes().PutDouble("received.nan", math.Float64frombits(0x7ff8000000000042))
	span.Events().AppendEmpty().Attributes().PutInt("native.sample", 9007199254740993)
	span.Links().AppendEmpty().Attributes().PutInt("native.sample", 9007199254740993)
	data.ResourceSpans().At(0).Resource().Attributes().PutInt("native.sample", 9007199254740993)
	data.ResourceSpans().At(0).ScopeSpans().At(0).Scope().Attributes().PutInt("native.sample", 9007199254740993)
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return spans.Ingest(t.Context(), conn, data) }))
	metricData := buildTestMetrics()
	metric := metricData.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().AppendEmpty()
	metric.SetName("native.sample")
	metric.Metadata().PutInt("native.sample", 9007199254740993)
	point := metric.SetEmptyGauge().DataPoints().AppendEmpty()
	point.SetTimestamp(100)
	point.SetIntValue(1)
	point.Attributes().PutInt("native.sample", 9007199254740993)
	point.Exemplars().AppendEmpty().FilteredAttributes().PutInt("native.sample", 9007199254740993)
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return metrics.Ingest(t.Context(), conn, metricData) }))
	s, err := NewServer("localhost:8000", 0, h.store, zaptest.NewLogger(t), nil)
	require.NoError(t, err)
	for _, method := range []string{"getTraceAttributeDefinitions", "getLogAttributeDefinitions", "getMetricAttributeDefinitions", "getTraceAttributeDefinitionsByTraceID"} {
		params := map[string]any{}
		if method == "getTraceAttributeDefinitionsByTraceID" {
			params["traceID"] = testTraceIDHex
		}
		raw, err := h.Handle(t.Context(), createRequest(method, params))
		require.NoError(t, err)
		var expected []struct {
			Name  string `json:"name"`
			Owner string `json:"owner"`
			Scope string `json:"attributeScope"`
			Kind  string `json:"type"`
			Path  []any  `json:"path"`
		}
		require.NoError(t, json.Unmarshal(raw.(json.RawMessage), &expected))
		wire, err := json.Marshal(params)
		require.NoError(t, err)
		if len(expected) == 0 {
			w := httptest.NewRecorder()
			s.server.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/data/"+method, bytes.NewReader(wire)))
			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, "0", w.Header().Get("X-Arrow-Rows"))
			reader, err := ipc.NewReader(bytes.NewReader(w.Body.Bytes()))
			require.NoError(t, err)
			require.Len(t, reader.Schema().Fields(), 5)
			require.False(t, reader.Next())
			require.NoError(t, reader.Err())
			reader.Release()
			continue
		}
		record := readArrowDataset(t, s, method, string(wire))
		require.Equal(t, len(expected), int(record.NumRows()), method)
		for row, want := range expected {
			for column, text := range map[int]string{0: want.Name, 1: want.Scope, 2: want.Owner, 4: want.Kind} {
				require.Equal(t, text, record.Column(column).(*array.String).Value(row), method)
			}
			paths := record.Column(3).(*array.List)
			start, end := paths.ValueOffsets(row)
			parts := paths.ListValues().(*array.Struct)
			require.Equal(t, len(want.Path), int(end-start))
			for i, part := range want.Path {
				index := int(start) + i
				if key, ok := part.(string); ok {
					require.Equal(t, key, parts.Field(0).(*array.String).Value(index))
					require.True(t, parts.Field(1).IsNull(index))
				} else {
					require.True(t, parts.Field(0).IsNull(index))
					require.Equal(t, uint64(part.(float64)), parts.Field(1).(*array.Uint64).Value(index))
				}
			}
		}
	}
	for _, tc := range []struct {
		signal string
		owner  string
		path   []any
		kind   string
		bits   uint64
	}{
		{"traces", "span", []any{"attributes", "nested", 0, "0", ""}, "int64", 9007199254740993},
		{"traces", "span", []any{"attributes", "received.nan"}, "double", 0x7ff8000000000042},
		{"traces", "event", []any{"attributes", "native.sample"}, "int64", 9007199254740993},
		{"traces", "link", []any{"attributes", "native.sample"}, "int64", 9007199254740993},
		{"traces", "resource", []any{"attributes", "native.sample"}, "int64", 9007199254740993},
		{"traces", "scope", []any{"attributes", "native.sample"}, "int64", 9007199254740993},
		{"metrics", "metric", []any{"metadata", "native.sample"}, "int64", 9007199254740993},
		{"metrics", "datapoint", []any{"attributes", "native.sample"}, "int64", 9007199254740993},
		{"metrics", "exemplar", []any{"attributes", "native.sample"}, "int64", 9007199254740993},
	} {
		t.Run(tc.owner+"/"+tc.kind, func(t *testing.T) {
			wire, err := json.Marshal(map[string]any{"term": "", "signal": tc.signal, "owner": tc.owner, "path": tc.path, "type": tc.kind})
			require.NoError(t, err)
			legacy, err := h.Handle(t.Context(), createRequest("searchAttributeMatches", json.RawMessage(wire)))
			require.NoError(t, err)
			var matches []struct {
				MatchCount int64             `json:"matchCount"`
				Samples    []json.RawMessage `json:"sampleValues"`
			}
			require.NoError(t, json.Unmarshal(legacy.(json.RawMessage), &matches))
			require.Len(t, matches, 1)
			require.Len(t, matches[0].Samples, 1)
			record := readArrowDataset(t, s, "searchAttributeMatches", string(wire))
			require.Equal(t, int64(1), record.NumRows())
			require.Equal(t, matches[0].MatchCount, record.Column(5).(*array.Int64).Value(0))
			samples := record.Column(6).(*array.List).ListValues()
			require.Equal(t, 1, samples.Len())
			if tc.kind == "int64" {
				require.Equal(t, int64(tc.bits), samples.(*array.Int64).Value(0))
			} else {
				// NaN is preserved semantically; storage may canonicalize its payload.
				require.True(t, math.IsNaN(samples.(*array.Float64).Value(0)))
			}
		})
	}
	keys := readArrowDataset(t, s, "searchAttributeMatches", `{"term":"nested","signal":"traces"}`)
	require.Positive(t, keys.NumRows())
	require.Zero(t, keys.Column(6).(*array.List).ListValues().Len())
	for _, index := range []any{-1, 0.5, uint64(4294967295), true, nil} {
		params := map[string]any{"term": "", "signal": "traces", "owner": "span", "path": []any{"attributes", "nested", index}}
		_, err := h.Handle(t.Context(), createRequest("searchAttributeMatches", params))
		require.Error(t, err)
		wire, err := json.Marshal(params)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/data/searchAttributeMatches", bytes.NewReader(wire)))
		require.Equal(t, http.StatusBadRequest, w.Code)
	}
}

func TestArrowDatasetReads(t *testing.T) {
	h := setupHandlerWithData(t)
	data := buildTestMetrics()
	batch := data.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()
	hist := batch.AppendEmpty()
	hist.SetName("arrow.histogram")
	hist.SetUnit("ms")
	hist.SetEmptyHistogram().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dp := hist.Histogram().DataPoints().AppendEmpty()
	dp.SetTimestamp(100)
	dp.SetCount(3)
	dp.SetSum(5)
	dp.ExplicitBounds().FromRaw([]float64{1, 4})
	dp.BucketCounts().FromRaw([]uint64{1, 2, 0})
	exemplar := dp.Exemplars().AppendEmpty()
	exemplar.SetTimestamp(100)
	exemplar.SetIntValue(9007199254740993)
	exemplar.SetTraceID(pcommon.TraceID{15: 1})
	exemplar.SetSpanID(pcommon.SpanID{7: 1})
	exemplar.FilteredAttributes().PutInt("__proto__", 9007199254740993)
	exp := batch.AppendEmpty()
	exp.SetName("arrow.exponential")
	exp.SetEmptyExponentialHistogram().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	edp := exp.ExponentialHistogram().DataPoints().AppendEmpty()
	edp.SetTimestamp(100)
	edp.SetCount(3)
	edp.SetSum(5)
	edp.Positive().BucketCounts().FromRaw([]uint64{3})
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return metrics.Ingest(t.Context(), conn, data) }))
	s, err := NewServer("localhost:8000", 0, h.store, zaptest.NewLogger(t), nil)
	require.NoError(t, err)
	read := func(method, params string) arrow.RecordBatch {
		t.Helper()
		return readArrowDataset(t, s, method, params)
	}
	trace := read("getTraceView", `{"traceID":"`+testTraceIDHex+`"}`)
	for _, completion := range []struct{ signal, field string }{{"traces", "name"}, {"logs", "eventName"}, {"metrics", "name"}, {"metrics", "unit"}} {
		params := map[string]any{"signal": completion.signal, "field": completion.field, "term": "", "limit": 10}
		legacy, err := h.Handle(t.Context(), createRequest("getFieldValueCompletions", params))
		require.NoError(t, err)
		var want []string
		require.NoError(t, json.Unmarshal(legacy.(json.RawMessage), &want))
		require.NotEmpty(t, want)
		wire, err := json.Marshal(params)
		require.NoError(t, err)
		got := read("getFieldValueCompletions", string(wire))
		require.Equal(t, len(want), int(got.NumRows()))
		for i, value := range want {
			require.Equal(t, value, got.Column(0).(*array.String).Value(i))
		}
	}
	services := read("discoverTelemetry", `{"options":{"kind":"services","queries":{"traces":null,"logs":null,"metrics":null},"limit":1}}`)
	serviceRows := services.Column(0).(*array.List).ListValues().(*array.Struct)
	require.Equal(t, 1, serviceRows.Len())
	require.False(t, services.Column(2).IsNull(0))
	serviceType := serviceRows.DataType().(*arrow.StructType)
	identityIndex, ok := serviceType.FieldIdx("serviceIdentity")
	require.True(t, ok)
	serviceIdentity := serviceRows.Field(identityIndex).(*array.String).Value(0)
	for _, kind := range []string{"connections", "connection-spans"} {
		params, err := json.Marshal(map[string]any{"options": map[string]any{"kind": kind, "serviceIdentity": serviceIdentity, "targetIdentity": serviceIdentity}})
		require.NoError(t, err)
		read("discoverTelemetry", string(params))
	}
	read("discoverTelemetry", `{"options":{"kind":"services","offset":1000}}`)
	keys := read("discoverTelemetry", `{"options":{"kind":"keys","signal":"logs","ownerType":"resource"}}`)
	require.Positive(t, keys.Column(0).(*array.List).ListValues().Len())
	values := read("discoverTelemetry", `{"options":{"kind":"values","signal":"logs","ownerType":"resource","key":"service.name","sortBy":"name","sortDirection":"asc"}}`)
	valueRows := values.Column(0).(*array.List).ListValues().(*array.Struct)
	require.Positive(t, valueRows.Len())
	valueType := valueRows.DataType().(*arrow.StructType)
	valueIndex, ok := valueType.FieldIdx("value")
	require.True(t, ok)
	require.Equal(t, arrow.STRING, valueRows.Field(valueIndex).DataType().ID())
	require.Equal(t, int64(1), trace.NumRows())
	require.Positive(t, trace.Column(trace.Schema().FieldIndices("spans")[0]).(*array.List).ListValues().Len())
	metricRows := read("searchMetricSummaries", `{"startTime":null,"endTime":null}`)
	require.Empty(t, metricRows.Schema().FieldIndices("description"))
	for i := 0; i < int(metricRows.NumRows()); i++ {
		id := metricRows.Column(0).(*array.String).Value(i)
		view := read("getMetricView", `{"metricRef":"`+id+`","startTime":null,"endTime":null,"targetBuckets":10,"viewBuckets":5,"sparklineBuckets":5,"quantiles":[0.5]}`)
		timeseries := view.Column(view.Schema().FieldIndices("timeseries")[0]).(*array.List).ListValues().(*array.Struct)
		require.Positive(t, timeseries.Len())
		read("getMetricAggregateView", `{"metricRef":"`+id+`","startTime":null,"endTime":null,"targetBuckets":10,"viewBuckets":5,"quantiles":[0.5]}`)
	}
	metricID := metricRows.Column(0).(*array.String).Value(0)
	metric := read("getMetric", `{"metricRef":"`+metricID+`"}`)
	read("getMetricView", `{"metricRef":"`+metricID+`","startTime":null,"endTime":null,"targetBuckets":10,"viewBuckets":5,"sparklineBuckets":5}`)
	read("getMetricAggregateView", `{"metricRef":"`+metricID+`","startTime":null,"endTime":null,"targetBuckets":10,"viewBuckets":5}`)
	series := metric.Column(metric.Schema().FieldIndices("series")[0]).(*array.List).ListValues().(*array.Struct)
	seriesID := series.Field(0).(*array.String).Value(0)
	points := read("getMetricSeries", `{"metricRef":"`+metricID+`","seriesRef":"`+seriesID+`","startTime":null,"endTime":null}`)
	require.Positive(t, points.Column(points.Schema().FieldIndices("datapoints")[0]).(*array.List).ListValues().Len())
	for _, method := range []string{"searchTraceSummaries", "searchMetricSummaries", "searchLogSummaries"} {
		record := read(method, `{"startTime":null,"endTime":null}`)
		require.Positive(t, record.NumRows())
		for _, f := range record.Schema().Fields() {
			name, _ := f.Metadata.GetValue("ARROW:extension:name")
			require.NotEqual(t, "arrow.json", name)
			require.NotEqual(t, "arrow.parquet.variant", name)
		}
		if method == "searchLogSummaries" {
			require.Empty(t, record.Schema().FieldIndices("body"))
			require.Len(t, record.Schema().FieldIndices("bodyPreview"), 1)
			require.Len(t, record.Schema().FieldIndices("bodyType"), 1)
		}
	}
	traceLogs := read("getTraceLogSummaries", `{"traceID":"`+testTraceIDHex+`"}`)
	require.Empty(t, traceLogs.Schema().FieldIndices("body"))
	require.Len(t, traceLogs.Schema().FieldIndices("bodyPreview"), 1)
	page := read("searchLogPage", `{"startTime":null,"endTime":null,"pageSize":1,"offset":0,"columns":[{"source":"log","kind":"field","key":"flags"},{"source":"resource","kind":"attribute","key":"service.name"},{"source":"log","kind":"attribute","key":"missing"}]}`)
	require.Equal(t, int64(1), page.Column(0).(*array.Int64).Value(0))
	rows := page.Column(1).(*array.List).ListValues().(*array.Struct)
	require.Equal(t, 1, rows.Len())
	rowType := rows.DataType().(*arrow.StructType)
	_, hasBody := rowType.FieldIdx("body")
	require.False(t, hasBody)
	previewIndex, hasPreview := rowType.FieldIdx("bodyPreview")
	require.True(t, hasPreview)
	require.Equal(t, arrow.STRING, rows.Field(previewIndex).DataType().ID())
	id := rows.Field(0).(*array.String).Value(0)
	detail := read("getLog", `{"logRef":"`+id+`"}`)
	require.Equal(t, id, detail.Column(0).(*array.String).Value(0))
	require.Len(t, detail.Schema().FieldIndices("body"), 1)
	empty := read("searchLogPage", `{"startTime":null,"endTime":null,"pageSize":1,"offset":1}`)
	require.Equal(t, int64(1), empty.Column(0).(*array.Int64).Value(0))
	require.Zero(t, empty.Column(1).(*array.List).ListValues().Len())
	for _, tc := range []struct {
		method, params string
		status         int
	}{
		{"getLog", `{"logRef":"00000000-0000-0000-0000-000000000001"}`, http.StatusNotFound},
		{"getLog", `{"logRef":"bad"}`, http.StatusBadRequest},
		{"clearLogs", `{}`, http.StatusNotFound},
		{"searchLogSummaries", `{`, http.StatusBadRequest},
		{"getFieldValueCompletions", `{"signal":"traces","field":"statusMessage","term":"","limit":10}`, http.StatusBadRequest},
	} {
		w := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/data/"+tc.method, strings.NewReader(tc.params)))
		require.Equal(t, tc.status, w.Code, w.Body.String())
		require.Equal(t, "application/json", w.Header().Get("Content-Type"))
		var failure struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		require.NoError(t, json.NewDecoder(w.Body).Decode(&failure))
		require.Negative(t, failure.Code)
		require.NotEmpty(t, failure.Message)
		_, err := io.Copy(io.Discard, w.Body)
		require.NoError(t, err)
	}
}
