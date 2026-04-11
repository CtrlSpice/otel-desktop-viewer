package stats_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"database/sql/driver"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/stats"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func setupStore(t *testing.T) (*store.Store, context.Context, func()) {
	t.Helper()
	ctx := context.Background()
	s, err := store.NewStore(ctx, "")
	require.NoError(t, err)
	return s, ctx, func() { s.Close() }
}

func mustDecodeTraceID(s string) [16]byte {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 16 {
		panic("invalid trace ID hex: " + s)
	}
	var out [16]byte
	copy(out[:], b)
	return out
}

func mustDecodeSpanID(s string) [8]byte {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 8 {
		panic("invalid span ID hex: " + s)
	}
	var out [8]byte
	copy(out[:], b)
	return out
}

type statsJSON struct {
	Traces  traceStatsJSON  `json:"traces"`
	Logs    logStatsJSON    `json:"logs"`
	Metrics metricStatsJSON `json:"metrics"`
}

type traceStatsJSON struct {
	TraceCount   float64 `json:"traceCount"`
	SpanCount    float64 `json:"spanCount"`
	ServiceCount float64 `json:"serviceCount"`
	ErrorCount   float64 `json:"errorCount"`
	LastReceived *int64  `json:"lastReceived"`
}

type logStatsJSON struct {
	LogCount     float64 `json:"logCount"`
	ErrorCount   float64 `json:"errorCount"`
	LastReceived *int64  `json:"lastReceived"`
}

type metricStatsJSON struct {
	MetricCount    float64 `json:"metricCount"`
	DataPointCount float64 `json:"dataPointCount"`
	LastReceived   *int64  `json:"lastReceived"`
}

func getStats(t *testing.T, s *store.Store, ctx context.Context) statsJSON {
	t.Helper()
	raw, err := stats.GetStats(ctx, s.DB())
	require.NoError(t, err)
	var result statsJSON
	require.NoError(t, json.Unmarshal(raw, &result))
	return result
}

func TestGetStats_Empty(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	result := getStats(t, s, ctx)

	assert.Equal(t, float64(0), result.Traces.TraceCount)
	assert.Equal(t, float64(0), result.Traces.SpanCount)
	assert.Equal(t, float64(0), result.Traces.ServiceCount)
	assert.Equal(t, float64(0), result.Traces.ErrorCount)
	assert.Nil(t, result.Traces.LastReceived)

	assert.Equal(t, float64(0), result.Logs.LogCount)
	assert.Equal(t, float64(0), result.Logs.ErrorCount)
	assert.Nil(t, result.Logs.LastReceived)

	assert.Equal(t, float64(0), result.Metrics.MetricCount)
	assert.Equal(t, float64(0), result.Metrics.DataPointCount)
	assert.Nil(t, result.Metrics.LastReceived)
}

func TestGetStats_WithTraces(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	baseTime := time.Now().UnixNano()
	tr := buildTestTraces(baseTime)

	err := s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, tr)
	})
	require.NoError(t, err)

	result := getStats(t, s, ctx)

	// 2 traces, 3 spans total (2 in trace1, 1 in trace2)
	assert.Equal(t, float64(2), result.Traces.TraceCount)
	assert.Equal(t, float64(3), result.Traces.SpanCount)
	// 2 distinct services: "service-alpha" and "service-beta"
	assert.Equal(t, float64(2), result.Traces.ServiceCount)
	// 1 error span (child span has ERROR status)
	assert.Equal(t, float64(1), result.Traces.ErrorCount)
	assert.NotNil(t, result.Traces.LastReceived)
}

func TestGetStats_WithLogs(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	baseTime := time.Now().UnixNano()
	lg := buildTestLogs(baseTime)

	err := s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, lg)
	})
	require.NoError(t, err)

	result := getStats(t, s, ctx)

	// 3 logs total
	assert.Equal(t, float64(3), result.Logs.LogCount)
	// 1 ERROR log (severity_number 17)
	assert.Equal(t, float64(1), result.Logs.ErrorCount)
	assert.NotNil(t, result.Logs.LastReceived)
}

func TestGetStats_WithMetrics(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	m := buildTestMetrics()

	err := s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, m)
	})
	require.NoError(t, err)

	result := getStats(t, s, ctx)

	// 2 metrics (gauge + sum)
	assert.Equal(t, float64(2), result.Metrics.MetricCount)
	// 3 data points (2 gauge + 1 sum)
	assert.Equal(t, float64(3), result.Metrics.DataPointCount)
	assert.NotNil(t, result.Metrics.LastReceived)
}

func TestGetStats_AllSignals(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	baseTime := time.Now().UnixNano()

	err := s.WithConn(func(conn driver.Conn) error {
		if err := spans.Ingest(ctx, conn, buildTestTraces(baseTime)); err != nil {
			return err
		}
		if err := logs.Ingest(ctx, conn, buildTestLogs(baseTime)); err != nil {
			return err
		}
		return metrics.Ingest(ctx, conn, buildTestMetrics())
	})
	require.NoError(t, err)

	result := getStats(t, s, ctx)

	assert.Equal(t, float64(2), result.Traces.TraceCount)
	assert.Equal(t, float64(3), result.Traces.SpanCount)
	assert.Equal(t, float64(2), result.Traces.ServiceCount)
	assert.Equal(t, float64(1), result.Traces.ErrorCount)

	assert.Equal(t, float64(3), result.Logs.LogCount)
	assert.Equal(t, float64(1), result.Logs.ErrorCount)

	assert.Equal(t, float64(2), result.Metrics.MetricCount)
	assert.Equal(t, float64(3), result.Metrics.DataPointCount)
}

// buildTestTraces creates 2 traces across 2 services with 3 spans total,
// one of which has ERROR status.
func buildTestTraces(baseTime int64) ptrace.Traces {
	tr := ptrace.NewTraces()

	// Trace 1: service-alpha, root + error child
	rs1 := tr.ResourceSpans().AppendEmpty()
	rs1.Resource().Attributes().PutStr("service.name", "service-alpha")
	ss1 := rs1.ScopeSpans().AppendEmpty()

	root := ss1.Spans().AppendEmpty()
	root.SetTraceID(mustDecodeTraceID("00000000000000000000000000000001"))
	root.SetSpanID(mustDecodeSpanID("0000000000000001"))
	root.SetName("root-op")
	root.SetKind(ptrace.SpanKindServer)
	root.SetStartTimestamp(pcommon.Timestamp(baseTime))
	root.SetEndTimestamp(pcommon.Timestamp(baseTime + int64(time.Second)))

	child := ss1.Spans().AppendEmpty()
	child.SetTraceID(mustDecodeTraceID("00000000000000000000000000000001"))
	child.SetSpanID(mustDecodeSpanID("0000000000000002"))
	child.SetParentSpanID(mustDecodeSpanID("0000000000000001"))
	child.SetName("child-op")
	child.SetKind(ptrace.SpanKindInternal)
	child.SetStartTimestamp(pcommon.Timestamp(baseTime + int64(100*time.Millisecond)))
	child.SetEndTimestamp(pcommon.Timestamp(baseTime + int64(500*time.Millisecond)))
	child.Status().SetCode(ptrace.StatusCodeError)
	child.Status().SetMessage("something failed")

	// Trace 2: service-beta, single span
	rs2 := tr.ResourceSpans().AppendEmpty()
	rs2.Resource().Attributes().PutStr("service.name", "service-beta")
	ss2 := rs2.ScopeSpans().AppendEmpty()

	s2 := ss2.Spans().AppendEmpty()
	s2.SetTraceID(mustDecodeTraceID("00000000000000000000000000000002"))
	s2.SetSpanID(mustDecodeSpanID("0000000000000003"))
	s2.SetName("beta-op")
	s2.SetKind(ptrace.SpanKindClient)
	s2.SetStartTimestamp(pcommon.Timestamp(baseTime + int64(2*time.Second)))
	s2.SetEndTimestamp(pcommon.Timestamp(baseTime + int64(3*time.Second)))

	return tr
}

// buildTestLogs creates 3 log records: INFO, ERROR, WARN.
func buildTestLogs(baseTime int64) plog.Logs {
	lg := plog.NewLogs()
	rl := lg.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", "test-service")
	sl := rl.ScopeLogs().AppendEmpty()
	sl.Scope().SetName("test-scope")

	rec0 := sl.LogRecords().AppendEmpty()
	rec0.SetTimestamp(pcommon.Timestamp(baseTime))
	rec0.SetObservedTimestamp(pcommon.Timestamp(baseTime))
	rec0.SetSeverityText("INFO")
	rec0.SetSeverityNumber(plog.SeverityNumberInfo)
	rec0.Body().SetStr("info message")

	rec1 := sl.LogRecords().AppendEmpty()
	rec1.SetTimestamp(pcommon.Timestamp(baseTime + int64(time.Second)))
	rec1.SetObservedTimestamp(pcommon.Timestamp(baseTime + int64(time.Second)))
	rec1.SetSeverityText("ERROR")
	rec1.SetSeverityNumber(plog.SeverityNumberError)
	rec1.Body().SetStr("error message")

	rec2 := sl.LogRecords().AppendEmpty()
	rec2.SetTimestamp(pcommon.Timestamp(baseTime + int64(2*time.Second)))
	rec2.SetObservedTimestamp(pcommon.Timestamp(baseTime + int64(2*time.Second)))
	rec2.SetSeverityText("WARN")
	rec2.SetSeverityNumber(plog.SeverityNumberWarn)
	rec2.Body().SetStr("warn message")

	return lg
}

// buildTestMetrics creates 2 metrics: a gauge with 2 data points and a sum with 1 data point.
func buildTestMetrics() pmetric.Metrics {
	base := time.Now().UnixNano()
	m := pmetric.NewMetrics()
	rm := m.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "test-service")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("test-scope")

	// Gauge with 2 data points
	g := sm.Metrics().AppendEmpty()
	g.SetName("cpu.usage")
	g.SetUnit("percent")
	gauge := g.SetEmptyGauge()
	dp0 := gauge.DataPoints().AppendEmpty()
	dp0.SetTimestamp(pcommon.Timestamp(base))
	dp0.SetStartTimestamp(pcommon.Timestamp(base))
	dp0.SetDoubleValue(45.2)
	dp1 := gauge.DataPoints().AppendEmpty()
	dp1.SetTimestamp(pcommon.Timestamp(base + int64(time.Minute)))
	dp1.SetStartTimestamp(pcommon.Timestamp(base))
	dp1.SetDoubleValue(52.8)

	// Sum with 1 data point
	su := sm.Metrics().AppendEmpty()
	su.SetName("requests.total")
	su.SetUnit("count")
	sum := su.SetEmptySum()
	sum.SetIsMonotonic(true)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	dp2 := sum.DataPoints().AppendEmpty()
	dp2.SetTimestamp(pcommon.Timestamp(base + int64(2*time.Minute)))
	dp2.SetDoubleValue(1500.0)

	return m
}
