package server

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap/zaptest"
)

func TestArrowDatasetReads(t *testing.T) {
	h := setupHandlerWithData(t)
	data := buildTestMetrics()
	batch := data.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()
	hist := batch.AppendEmpty()
	hist.SetName("arrow.histogram")
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
	trace := read("getTraceView", `{"traceID":"`+testTraceIDHex+`"}`)
	require.Equal(t, int64(1), trace.NumRows())
	require.Positive(t, trace.Column(5).(*array.List).ListValues().Len())
	metricRows := read("searchMetricSummaries", `{"startTime":null,"endTime":null}`)
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
	}
	page := read("searchLogPage", `{"startTime":null,"endTime":null,"pageSize":1,"offset":0,"columns":[{"source":"log","kind":"field","key":"flags"},{"source":"resource","kind":"attribute","key":"service.name"},{"source":"log","kind":"attribute","key":"missing"}]}`)
	require.Equal(t, int64(1), page.Column(0).(*array.Int64).Value(0))
	rows := page.Column(1).(*array.List).ListValues().(*array.Struct)
	require.Equal(t, 1, rows.Len())
	id := rows.Field(0).(*array.String).Value(0)
	detail := read("getLog", `{"logRef":"`+id+`"}`)
	require.Equal(t, id, detail.Column(0).(*array.String).Value(0))
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
