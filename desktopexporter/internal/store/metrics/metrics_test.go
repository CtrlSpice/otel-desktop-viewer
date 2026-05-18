package metrics_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

const maxNano = 1<<63 - 1

// Wide window + many "pixels" used by quantile-series tests when the test
// itself doesn't care about bucket boundaries -- it just wants every fixture
// timestamp to land in a distinct bucket. With these values bucket_ns is
// roughly 4 seconds, comfortably finer than any spacing our fixtures use
// (createTestMetricsPdata spaces by minutes, the merged tests use 60s
// gaps), so the existing per-row test expectations hold.
const (
	testQuantileWindowStartTs int64 = 0
	testQuantileWindowEndTs   int64 = 4_000_000_000_000_000_000 // ~year 2096 in nanoseconds
	testQuantileWindowPoints  int   = 1_000_000_000
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

func mustDecodeTraceIDMetrics(s string) [16]byte {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 16 {
		panic("invalid trace ID hex: " + s)
	}
	var out [16]byte
	copy(out[:], b)
	return out
}

func mustDecodeSpanIDMetrics(s string) [8]byte {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 8 {
		panic("invalid span ID hex: " + s)
	}
	var out [8]byte
	copy(out[:], b)
	return out
}

// createTestMetricsPdataN builds pmetric.Metrics with n gauge metrics (one resource/scope).
// Each metric has resource and scope attributes. Used to exercise flushIntervalMetrics by ingesting >= 100 metrics.
func createTestMetricsPdataN(n int) pmetric.Metrics {
	base := time.Now().UnixNano()
	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "test-service")
	rm.Resource().Attributes().PutStr("resource.key", "resource.val")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("test-scope")
	sm.Scope().SetVersion("v1.0.0")
	sm.Scope().Attributes().PutStr("scope.key", "scope.val")
	for i := 0; i < n; i++ {
		m := sm.Metrics().AppendEmpty()
		m.SetName("flush_metric_" + fmt.Sprintf("%d", i))
		m.SetDescription("Batch metric")
		m.SetUnit("count")
		g := m.SetEmptyGauge()
		dp := g.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(base + int64(i)))
		dp.SetStartTimestamp(pcommon.Timestamp(base))
		dp.SetDoubleValue(float64(i))
	}
	return metrics
}

// createTestMetricsPdata returns pmetric.Metrics with four metrics: gauge, sum, histogram, exponential histogram.
func createTestMetricsPdata() pmetric.Metrics {
	base := time.Now().UnixNano()
	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "test-service")
	rm.Resource().Attributes().PutStr("service.version", "1.0.0")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("test-scope")
	sm.Scope().SetVersion("v1.0.0")

	// Gauge
	m0 := sm.Metrics().AppendEmpty()
	m0.SetName("gauge_metric")
	m0.SetDescription("Current memory usage")
	m0.SetUnit("bytes")
	gauge := m0.SetEmptyGauge()
	dp0 := gauge.DataPoints().AppendEmpty()
	dp0.SetTimestamp(pcommon.Timestamp(base))
	dp0.SetStartTimestamp(pcommon.Timestamp(base - int64(time.Hour)))
	dp0.SetDoubleValue(1024.5)
	dp0.Attributes().PutStr("memory.type", "heap")
	ex0 := dp0.Exemplars().AppendEmpty()
	ex0.SetTimestamp(pcommon.Timestamp(base - int64(time.Minute)))
	ex0.SetDoubleValue(1000.0)
	ex0.SetTraceID(mustDecodeTraceIDMetrics("00000000000000000000000000000099"))
	ex0.SetSpanID(mustDecodeSpanIDMetrics("0000000000000001"))
	ex0.FilteredAttributes().PutStr("exemplar.source", "gauge")

	// Gauge with Int value (covers numberDataPointValue Int branch: return nil, dp.IntValue(), typeStr)
	m0int := sm.Metrics().AppendEmpty()
	m0int.SetName("gauge_int_metric")
	m0int.SetDescription("Integer gauge")
	m0int.SetUnit("count")
	gaugeInt := m0int.SetEmptyGauge()
	dp0int := gaugeInt.DataPoints().AppendEmpty()
	dp0int.SetTimestamp(pcommon.Timestamp(base + int64(time.Minute)))
	dp0int.SetStartTimestamp(pcommon.Timestamp(base))
	dp0int.SetIntValue(42)
	dp0int.Attributes().PutStr("type", "int")

	// Sum
	m1 := sm.Metrics().AppendEmpty()
	m1.SetName("sum_metric")
	m1.SetDescription("Total requests processed")
	m1.SetUnit("requests")
	sum := m1.SetEmptySum()
	sum.SetIsMonotonic(true)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	dp1 := sum.DataPoints().AppendEmpty()
	dp1.SetTimestamp(pcommon.Timestamp(base + int64(2*time.Minute)))
	dp1.SetDoubleValue(1500.0)
	ex1 := dp1.Exemplars().AppendEmpty()
	ex1.SetTimestamp(pcommon.Timestamp(base + int64(2*time.Minute)))
	ex1.SetDoubleValue(1400.0)
	ex1.SetTraceID(mustDecodeTraceIDMetrics("00000000000000000000000000000099"))
	ex1.SetSpanID(mustDecodeSpanIDMetrics("0000000000000002"))
	ex1.FilteredAttributes().PutStr("exemplar.source", "sum")

	// Histogram
	m2 := sm.Metrics().AppendEmpty()
	m2.SetName("histogram_metric")
	m2.SetDescription("Request duration distribution")
	m2.SetUnit("seconds")
	hist := m2.SetEmptyHistogram()
	hist.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dp2 := hist.DataPoints().AppendEmpty()
	dp2.SetTimestamp(pcommon.Timestamp(base + int64(4*time.Minute)))
	dp2.SetCount(100)
	dp2.SetSum(25.5)
	dp2.SetMin(0.1)
	dp2.SetMax(2.5)
	dp2.BucketCounts().FromRaw([]uint64{10, 20, 30, 25, 15})
	dp2.ExplicitBounds().FromRaw([]float64{0.5, 1.0, 1.5, 2.0})
	ex2 := dp2.Exemplars().AppendEmpty()
	ex2.SetTimestamp(pcommon.Timestamp(base + int64(4*time.Minute)))
	ex2.SetDoubleValue(1.25)
	ex2.SetTraceID(mustDecodeTraceIDMetrics("00000000000000000000000000000099"))
	ex2.SetSpanID(mustDecodeSpanIDMetrics("0000000000000007"))
	ex2.FilteredAttributes().PutStr("exemplar.source", "histogram")

	// Exponential histogram
	m3 := sm.Metrics().AppendEmpty()
	m3.SetName("exponential_histogram_metric")
	m3.SetDescription("Response size distribution")
	m3.SetUnit("bytes")
	exp := m3.SetEmptyExponentialHistogram()
	exp.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dp3 := exp.DataPoints().AppendEmpty()
	dp3.SetTimestamp(pcommon.Timestamp(base + int64(6*time.Minute)))
	dp3.SetCount(50)
	dp3.SetSum(10240.0)
	dp3.SetMin(100.0)
	dp3.SetMax(2048.0)
	dp3.SetScale(2)
	dp3.SetZeroCount(5)
	dp3.Positive().SetOffset(1)
	dp3.Positive().BucketCounts().FromRaw([]uint64{5, 10, 15, 10, 5})
	dp3.Negative().SetOffset(0)
	dp3.Negative().BucketCounts().FromRaw([]uint64{2, 3})
	ex3 := dp3.Exemplars().AppendEmpty()
	ex3.SetTimestamp(pcommon.Timestamp(base + int64(6*time.Minute)))
	ex3.SetDoubleValue(512.0)
	ex3.SetTraceID(mustDecodeTraceIDMetrics("00000000000000000000000000000099"))
	ex3.SetSpanID(mustDecodeSpanIDMetrics("000000000000000a"))
	ex3.FilteredAttributes().PutStr("exemplar.source", "exponential_histogram")

	return metrics
}

// searchMetricsAll returns metrics.Search with wide time range and nil query; parses JSON to slice of maps.
func searchMetricsAll(t *testing.T, s *store.Store, ctx context.Context) []map[string]any {
	t.Helper()
	raw, err := metrics.Search(ctx, s.DB(), 0, maxNano, nil)
	assert.NoError(t, err)
	var out []map[string]any
	assert.NoError(t, json.Unmarshal(raw, &out))
	return out
}

// TestMetricSuite runs tests on ingested metrics using SearchMetrics (DB-generated JSON).
func TestMetricSuite(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	err := s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, createTestMetricsPdata())
	})
	assert.NoError(t, err, "ingest test metrics")

	t.Run("MetricRetrieval", func(t *testing.T) {
		metrics := searchMetricsAll(t, s, ctx)
		assert.Len(t, metrics, 5, "should have five metrics")
		names := make([]string, len(metrics))
		for i, m := range metrics {
			if n, ok := m["name"].(string); ok {
				names[i] = n
			}
		}
		assert.Contains(t, names, "gauge_metric")
		assert.Contains(t, names, "gauge_int_metric")
		assert.Contains(t, names, "sum_metric")
		assert.Contains(t, names, "histogram_metric")
		assert.Contains(t, names, "exponential_histogram_metric")
	})

	t.Run("GaugeMetric", func(t *testing.T) {
		metrics := searchMetricsAll(t, s, ctx)
		var gauge map[string]any
		for _, m := range metrics {
			if m["name"] == "gauge_metric" {
				gauge = m
				break
			}
		}
		requireMetric(t, gauge, "gauge_metric")
		assert.Equal(t, "Current memory usage", gauge["description"])
		assert.Equal(t, "bytes", gauge["unit"])
		datapoints := metricDatapoints(gauge)
		assert.NotEmpty(t, datapoints)
		dp0, _ := datapoints[0].(map[string]any)
		assert.NotNil(t, dp0)
		// DB returns doubleValue; value type may vary
		if v, ok := dp0["doubleValue"].(float64); ok {
			assert.Equal(t, 1024.5, v)
		}
	})

	t.Run("GaugeIntMetric", func(t *testing.T) {
		// Covers numberDataPointValue Int branch: return nil, dp.IntValue(), typeStr
		metrics := searchMetricsAll(t, s, ctx)
		var m map[string]any
		for _, metric := range metrics {
			if metric["name"] == "gauge_int_metric" {
				m = metric
				break
			}
		}
		requireMetric(t, m, "gauge_int_metric")
		datapoints := metricDatapoints(m)
		assert.Len(t, datapoints, 1)
		dp, _ := datapoints[0].(map[string]any)
		assert.NotNil(t, dp)
		assert.Equal(t, "Int", dp["valueType"], "valueType for integer datapoint")
		// intValue is written when ValueType is Int; DB returns as number
		switch v := dp["intValue"].(type) {
		case float64:
			assert.Equal(t, 42.0, v)
		case int64:
			assert.Equal(t, int64(42), v)
		default:
			t.Errorf("intValue expected number, got %T", dp["intValue"])
		}
	})

	t.Run("SumMetric", func(t *testing.T) {
		metrics := searchMetricsAll(t, s, ctx)
		var sum map[string]any
		for _, m := range metrics {
			if m["name"] == "sum_metric" {
				sum = m
				break
			}
		}
		requireMetric(t, sum, "sum_metric")
		assert.Equal(t, "Total requests processed", sum["description"])
		datapoints := metricDatapoints(sum)
		assert.NotEmpty(t, datapoints)
	})

	t.Run("HistogramMetric", func(t *testing.T) {
		metrics := searchMetricsAll(t, s, ctx)
		var hist map[string]any
		for _, m := range metrics {
			if m["name"] == "histogram_metric" {
				hist = m
				break
			}
		}
		requireMetric(t, hist, "histogram_metric")
		datapoints := metricDatapoints(hist)
		assert.NotEmpty(t, datapoints)
		dp, _ := datapoints[0].(map[string]any)
		assert.NotNil(t, dp)
		assert.Equal(t, float64(100), dp["count"])
		assert.Equal(t, 25.5, dp["sum"])
	})

	t.Run("ExponentialHistogramMetric", func(t *testing.T) {
		metrics := searchMetricsAll(t, s, ctx)
		var exp map[string]any
		for _, m := range metrics {
			if m["name"] == "exponential_histogram_metric" {
				exp = m
				break
			}
		}
		requireMetric(t, exp, "exponential_histogram_metric")
		datapoints := metricDatapoints(exp)
		assert.NotEmpty(t, datapoints)
		dp, _ := datapoints[0].(map[string]any)
		assert.NotNil(t, dp)
		assert.Equal(t, float64(50), dp["count"])
		assert.Equal(t, float64(2), dp["scale"])
	})

	t.Run("MetricResourceAndScope", func(t *testing.T) {
		metrics := searchMetricsAll(t, s, ctx)
		for i, m := range metrics {
			resource, _ := m["resource"].(map[string]any)
			assert.NotNil(t, resource, "metric %d resource", i)
			scope, _ := m["scope"].(map[string]any)
			assert.NotNil(t, scope, "metric %d scope", i)
			assert.Equal(t, "test-scope", scope["name"], "metric %d scope name", i)
			assert.Equal(t, "v1.0.0", scope["version"], "metric %d scope version", i)
		}
	})

	t.Run("Exemplars", func(t *testing.T) {
		assert.Greater(t, countRows(t, s.DB(), ctx, "select count(*) from exemplars"), 0, "exemplars should be ingested")
		metrics := searchMetricsAll(t, s, ctx)
		var gauge map[string]any
		for _, m := range metrics {
			if m["name"] == "gauge_metric" {
				gauge = m
				break
			}
		}
		requireMetric(t, gauge, "gauge_metric")
		datapoints := metricDatapoints(gauge)
		assert.NotEmpty(t, datapoints)
		dp0, _ := datapoints[0].(map[string]any)
		exemplars, _ := dp0["exemplars"].([]any)
		assert.Len(t, exemplars, 1, "gauge datapoint should have one exemplar")
		ex, _ := exemplars[0].(map[string]any)
		assert.Equal(t, 1000.0, ex["value"], "exemplar value")
		assert.NotEmpty(t, ex["traceID"], "exemplar traceID")
		assert.NotEmpty(t, ex["spanID"], "exemplar spanID")
	})

	t.Run("QueryByServiceName", func(t *testing.T) {
		// Exercise ParseQueryTree(query) and BuildMetricSQL with a resource attribute condition.
		base := time.Now().UnixNano()
		startTime := base - int64(2*time.Hour)
		endTime := base + int64(2*time.Hour)
		query := map[string]any{
			"id":   "q1",
			"type": "condition",
			"query": map[string]any{
				"field": map[string]any{
					"name":           "service.name",
					"searchScope":    "attribute",
					"attributeScope": "resource",
				},
				"fieldOperator": "=",
				"value":         "test-service",
			},
		}
		raw, err := metrics.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Len(t, metrics, 5, "should find all metrics for service name test-service")
		for i, m := range metrics {
			resource, _ := m["resource"].(map[string]any)
			assert.NotNil(t, resource, "metric %d resource", i)
			attrs, _ := resource["attributes"].([]any)
			var serviceName string
			for _, a := range attrs {
				kv, _ := a.(map[string]any)
				if k, _ := kv["key"].(string); k == "service.name" {
					serviceName, _ = kv["value"].(string)
					break
				}
			}
			assert.Equal(t, "test-service", serviceName, "metric %d resource service.name", i)
		}
	})

	// Field expression tests (mapMetricFieldExpression cases)
	base := time.Now().UnixNano()
	startTime := base - int64(2*time.Hour)
	endTime := base + int64(2*time.Hour)

	t.Run("Field_name", func(t *testing.T) {
		query := map[string]any{
			"id":   "f1",
			"type": "condition",
			"query": map[string]any{
				"field":          map[string]any{"name": "name", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "gauge_metric",
			},
		}
		raw, err := metrics.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Len(t, metrics, 1)
		assert.Equal(t, "gauge_metric", metrics[0]["name"])
	})

	t.Run("Field_description", func(t *testing.T) {
		query := map[string]any{
			"id":   "f2",
			"type": "condition",
			"query": map[string]any{
				"field":          map[string]any{"name": "description", "searchScope": "field"},
				"fieldOperator": "CONTAINS",
				"value":         "memory",
			},
		}
		raw, err := metrics.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Len(t, metrics, 1)
		assert.Equal(t, "gauge_metric", metrics[0]["name"])
	})

	t.Run("Field_unit", func(t *testing.T) {
		query := map[string]any{
			"id":   "f3",
			"type": "condition",
			"query": map[string]any{
				"field":          map[string]any{"name": "unit", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "bytes",
			},
		}
		raw, err := metrics.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Len(t, metrics, 2) // gauge_metric, exponential_histogram_metric
		names := make([]string, len(metrics))
		for i, m := range metrics {
			names[i] = m["name"].(string)
		}
		assert.Contains(t, names, "gauge_metric")
		assert.Contains(t, names, "exponential_histogram_metric")
	})

	t.Run("Field_scope.name", func(t *testing.T) {
		query := map[string]any{
			"id":   "f4",
			"type": "condition",
			"query": map[string]any{
				"field":          map[string]any{"name": "scope.name", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "test-scope",
			},
		}
		raw, err := metrics.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Len(t, metrics, 5)
		for _, m := range metrics {
			assert.Equal(t, "test-scope", m["scopeName"])
		}
	})

	t.Run("Field_scopeName", func(t *testing.T) {
		query := map[string]any{
			"id":   "f4b",
			"type": "condition",
			"query": map[string]any{
				"field":          map[string]any{"name": "scopeName", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "test-scope",
			},
		}
		raw, err := metrics.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Len(t, metrics, 5)
	})

	t.Run("Field_scope.version", func(t *testing.T) {
		query := map[string]any{
			"id":   "f5",
			"type": "condition",
			"query": map[string]any{
				"field":          map[string]any{"name": "scope.version", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "v1.0.0",
			},
		}
		raw, err := metrics.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Len(t, metrics, 5)
		for _, m := range metrics {
			assert.Equal(t, "v1.0.0", m["scopeVersion"])
		}
	})

	t.Run("Field_scopeVersion", func(t *testing.T) {
		query := map[string]any{
			"id":   "f5b",
			"type": "condition",
			"query": map[string]any{
				"field":          map[string]any{"name": "scopeVersion", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "v1.0.0",
			},
		}
		raw, err := metrics.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Len(t, metrics, 5)
	})

	t.Run("Field_default", func(t *testing.T) {
		// default branch: cap first letter -> m.ResourceDroppedAttributesCount
		query := map[string]any{
			"id":   "f6",
			"type": "condition",
			"query": map[string]any{
				"field":          map[string]any{"name": "resourceDroppedAttributesCount", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "0",
			},
		}
		raw, err := metrics.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Len(t, metrics, 5)
	})

	// Global search (mapMetricGlobalExpressions: explicit fields + attributes)
	t.Run("GlobalSearch_Description", func(t *testing.T) {
		query := map[string]any{
			"id":   "g1",
			"type": "condition",
			"query": map[string]any{
				"field":          map[string]any{"searchScope": "global"},
				"fieldOperator": "CONTAINS",
				"value":         "memory",
			},
		}
		raw, err := metrics.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Len(t, metrics, 1, "description contains 'memory' (gauge description)")
		assert.Equal(t, "gauge_metric", metrics[0]["name"])
	})

	t.Run("GlobalSearch_Attribute", func(t *testing.T) {
		query := map[string]any{
			"id":   "g2",
			"type": "condition",
			"query": map[string]any{
				"field":          map[string]any{"searchScope": "global"},
				"fieldOperator": "CONTAINS",
				"value":         "test-service",
			},
		}
		raw, err := metrics.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Len(t, metrics, 5, "resource attribute service.name = test-service")
	})

	t.Run("GlobalSearch_NoResults", func(t *testing.T) {
		query := map[string]any{
			"id":   "g3",
			"type": "condition",
			"query": map[string]any{
				"field":          map[string]any{"searchScope": "global"},
				"fieldOperator": "CONTAINS",
				"value":         "nonexistent-metric-xyz",
			},
		}
		raw, err := metrics.Search(ctx, s.DB(), startTime, endTime, query)
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Empty(t, metrics)
	})
}

func requireMetric(t *testing.T, m map[string]any, name string) {
	t.Helper()
	assert.NotNil(t, m, "metric %q not found", name)
	if m != nil {
		assert.Equal(t, name, m["name"], "metric name")
	}
}

// metricDatapoints flattens m["timeseries"][*]["datapoints"][*] into a
// single []any in the order the SQL emits them: timeseries sorted by
// latest dp timestamp desc, datapoints within each timeseries sorted
// by timestamp desc. Tests that don't care about per-timeseries
// grouping use this to keep their assertions terse; tests that DO care
// about grouping should walk m["timeseries"] directly.
func metricDatapoints(m map[string]any) []any {
	if m == nil {
		return nil
	}
	timeseries, _ := m["timeseries"].([]any)
	out := make([]any, 0)
	for _, ts := range timeseries {
		ts, _ := ts.(map[string]any)
		if ts == nil {
			continue
		}
		dps, _ := ts["datapoints"].([]any)
		out = append(out, dps...)
	}
	return out
}

// deleteByIdentity is a thin test helper that resolves the 8-field OTel
// identity to a stream UUID via metric_streams and then calls
// DeleteMetricStream. The production JSON-RPC layer does the same
// resolve-then-delete pattern; we replicate it here so the existing
// test cases stay readable without needing to spell out streamIDs.
func deleteByIdentity(t *testing.T, ctx context.Context, db *sql.DB, name, unit, metricType, aggTemporality, isMonotonic, scopeName, scopeVersion, serviceName string) error {
	t.Helper()
	const q = `
		select id::varchar from metric_streams
		where name = ?
		  and unit = ?
		  and metric_type = ?
		  and aggregation_temporality = ?
		  and is_monotonic = ?
		  and scope_name = ?
		  and scope_version = ?
		  and service_name = ?
		limit 1
	`
	var streamID string
	err := db.QueryRowContext(ctx, q,
		name, unit, metricType, aggTemporality, isMonotonic == "true",
		scopeName, scopeVersion, serviceName,
	).Scan(&streamID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return metrics.DeleteMetricStream(ctx, db, streamID)
}

// TestDeleteMetricStream covers the per-stream cascade. Each subtest
// ingests a fixture, resolves an identity tuple to a stream UUID, calls
// DeleteMetricStream, and checks that (a) every row backing that stream
// is gone and (b) nothing else was touched.
func TestDeleteMetricStream(t *testing.T) {
	t.Run("removes a single Gauge by name+unit+scope+service", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		err := s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, createTestMetricsPdata())
		})
		assert.NoError(t, err)

		before := searchMetricsAll(t, s, ctx)
		assert.Len(t, before, 5)

		// Gauges have no temporality / monotonic; pass empty strings.
		err = deleteByIdentity(t, ctx, s.DB(),
			"gauge_metric", "bytes", "Gauge",
			"", "",
			"test-scope", "v1.0.0", "test-service",
		)
		assert.NoError(t, err)

		after := searchMetricsAll(t, s, ctx)
		assert.Len(t, after, 4)
		for _, m := range after {
			assert.NotEqual(t, "gauge_metric", m["name"])
		}

		assert.Equal(t, 0, countRows(t, s.DB(), ctx,
			`select count(*) from metric_streams where name = ?`, "gauge_metric"))
	})

	t.Run("collapses multiple ingestions of the same logical metric", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		// Three independent batches => three metric_ingests rows for the
		// same logical Gauge, all sharing one metric_streams row.
		for i := 0; i < 3; i++ {
			err := s.WithConn(func(conn driver.Conn) error {
				return metrics.Ingest(ctx, conn, createTestMetricsPdata())
			})
			assert.NoError(t, err)
		}

		// SearchSummaries collapses by identity so we still see 5 rows.
		assert.Len(t, searchSummariesAll(t, s, ctx), 5)
		// One stream per logical metric (5), 3 ingests per stream (15).
		assert.Equal(t, 5, countRows(t, s.DB(), ctx, `select count(*) from metric_streams`))
		assert.Equal(t, 15, countRows(t, s.DB(), ctx, `select count(*) from metric_ingests`))

		err := deleteByIdentity(t, ctx, s.DB(),
			"gauge_metric", "bytes", "Gauge",
			"", "",
			"test-scope", "v1.0.0", "test-service",
		)
		assert.NoError(t, err)

		assert.Len(t, searchSummariesAll(t, s, ctx), 4)
		// One stream + its three ingests should be gone.
		assert.Equal(t, 4, countRows(t, s.DB(), ctx, `select count(*) from metric_streams`))
		assert.Equal(t, 12, countRows(t, s.DB(), ctx, `select count(*) from metric_ingests`))
		assert.Equal(t, 0, countRows(t, s.DB(), ctx,
			`select count(*) from metric_streams where name = ?`, "gauge_metric"))
		assert.Equal(t, 0, countRows(t, s.DB(), ctx,
			`select count(*) from datapoints d join metric_streams s on s.id = d.stream_id where s.name = ?`,
			"gauge_metric"))
	})

	t.Run("unit discriminates same-name metrics", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		md := pmetric.NewMetrics()
		rm := md.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("service.name", "svc")
		sm := rm.ScopeMetrics().AppendEmpty()
		sm.Scope().SetName("scope")
		sm.Scope().SetVersion("v1")
		// Same name, different units.
		for _, unit := range []string{"bytes", "count"} {
			m := sm.Metrics().AppendEmpty()
			m.SetName("requests")
			m.SetUnit(unit)
			g := m.SetEmptyGauge()
			dp := g.DataPoints().AppendEmpty()
			dp.SetTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
			dp.SetIntValue(1)
		}
		err := s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		})
		assert.NoError(t, err)
		assert.Len(t, searchMetricsAll(t, s, ctx), 2)

		// Delete the bytes one — count should survive.
		err = deleteByIdentity(t, ctx, s.DB(),
			"requests", "bytes", "Gauge", "", "",
			"scope", "v1", "svc",
		)
		assert.NoError(t, err)

		after := searchMetricsAll(t, s, ctx)
		assert.Len(t, after, 1)
		assert.Equal(t, "count", after[0]["unit"])
	})

	t.Run("service.name discriminates same-name metrics from different services", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		md := pmetric.NewMetrics()
		for _, svc := range []string{"svc-a", "svc-b"} {
			rm := md.ResourceMetrics().AppendEmpty()
			rm.Resource().Attributes().PutStr("service.name", svc)
			sm := rm.ScopeMetrics().AppendEmpty()
			sm.Scope().SetName("scope")
			sm.Scope().SetVersion("v1")
			m := sm.Metrics().AppendEmpty()
			m.SetName("requests")
			m.SetUnit("count")
			g := m.SetEmptyGauge()
			dp := g.DataPoints().AppendEmpty()
			dp.SetTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
			dp.SetIntValue(1)
		}
		err := s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		})
		assert.NoError(t, err)
		assert.Len(t, searchSummariesAll(t, s, ctx), 2)

		err = deleteByIdentity(t, ctx, s.DB(),
			"requests", "count", "Gauge", "", "",
			"scope", "v1", "svc-a",
		)
		assert.NoError(t, err)

		// Summaries expose serviceName at the top level.
		after := searchSummariesAll(t, s, ctx)
		assert.Len(t, after, 1)
		assert.Equal(t, "svc-b", after[0]["serviceName"])
	})

	t.Run("is_monotonic discriminates Sum metrics", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		md := pmetric.NewMetrics()
		rm := md.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("service.name", "svc")
		sm := rm.ScopeMetrics().AppendEmpty()
		sm.Scope().SetName("scope")
		sm.Scope().SetVersion("v1")
		// Same name + unit + temporality, different monotonic flags.
		for _, monotonic := range []bool{true, false} {
			m := sm.Metrics().AppendEmpty()
			m.SetName("requests")
			m.SetUnit("count")
			sum := m.SetEmptySum()
			sum.SetIsMonotonic(monotonic)
			sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
			dp := sum.DataPoints().AppendEmpty()
			dp.SetTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
			dp.SetIntValue(1)
		}
		err := s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		})
		assert.NoError(t, err)
		assert.Len(t, searchSummariesAll(t, s, ctx), 2)

		err = deleteByIdentity(t, ctx, s.DB(),
			"requests", "count", "Sum", "Cumulative", "true",
			"scope", "v1", "svc",
		)
		assert.NoError(t, err)

		// isMonotonic is reported by SearchSummaries (not by the Search
		// detail endpoint, which nests it inside per-datapoint payloads).
		after := searchSummariesAll(t, s, ctx)
		assert.Len(t, after, 1)
		assert.Equal(t, false, after[0]["isMonotonic"])
	})

	t.Run("no-match identity is a no-op", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		err := s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, createTestMetricsPdata())
		})
		assert.NoError(t, err)
		assert.Len(t, searchMetricsAll(t, s, ctx), 5)

		err = deleteByIdentity(t, ctx, s.DB(),
			"nonexistent", "bytes", "Gauge", "", "",
			"test-scope", "v1.0.0", "test-service",
		)
		assert.NoError(t, err)
		assert.Len(t, searchMetricsAll(t, s, ctx), 5)
	})

	t.Run("cascade removes attributes, exemplars, datapoints", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		err := s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, createTestMetricsPdata())
		})
		assert.NoError(t, err)

		// Histogram has datapoints with exemplars — pick it.
		dpBefore := countRows(t, s.DB(), ctx,
			`select count(*) from datapoints where stream_id in (select id from metric_streams where name = ?)`,
			"histogram_metric")
		exBefore := countRows(t, s.DB(), ctx,
			`select count(*) from exemplars where datapoint_id in (select id from datapoints where stream_id in (select id from metric_streams where name = ?))`,
			"histogram_metric")
		attrBefore := countRows(t, s.DB(), ctx,
			`select count(*) from attributes where metric_ingest_id in (select id from metric_ingests where stream_id in (select id from metric_streams where name = ?))`,
			"histogram_metric")
		assert.Greater(t, dpBefore, 0)
		assert.Greater(t, exBefore, 0)
		assert.Greater(t, attrBefore, 0)

		err = deleteByIdentity(t, ctx, s.DB(),
			"histogram_metric", "seconds", "Histogram",
			"Delta", "",
			"test-scope", "v1.0.0", "test-service",
		)
		assert.NoError(t, err)

		assert.Equal(t, 0, countRows(t, s.DB(), ctx,
			`select count(*) from metric_streams where name = ?`, "histogram_metric"))
		assert.Equal(t, 0, countRows(t, s.DB(), ctx,
			`select count(*) from datapoints where stream_id in (select id from metric_streams where name = ?)`,
			"histogram_metric"))
		assert.Equal(t, 0, countRows(t, s.DB(), ctx,
			`select count(*) from exemplars e where exists (
				select 1 from datapoints d
				where d.id = e.datapoint_id
				  and d.stream_id in (select id from metric_streams where name = ?)
			)`, "histogram_metric"))
		assert.Equal(t, 0, countRows(t, s.DB(), ctx,
			`select count(*) from attributes where metric_ingest_id in (select id from metric_ingests where stream_id in (select id from metric_streams where name = ?))`,
			"histogram_metric"))
	})
}

// TestMetricStreams_FindOrInsertIdempotent verifies the contract that
// matters most for the normalized identity layer: ingesting the same
// 8-field stream identity across N independent OTLP batches collapses
// to exactly one metric_streams row. Per-batch context (description,
// dropped counts) lives on metric_ingests, so we expect N ingest rows
// but only one stream row, and every datapoint / attribute / exemplar
// should point at the same stream_id.
//
// This test is the find-or-insert mirror of the cascade-delete test:
// together they pin down the two halves of "identity is canonical."
func TestMetricStreams_FindOrInsertIdempotent(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	const batches = 5
	for i := 0; i < batches; i++ {
		err := s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, createTestMetricsPdata())
		})
		require.NoError(t, err)
	}

	// createTestMetricsPdata produces five distinct logical metrics.
	// Across N batches we should still see exactly five stream rows.
	assert.Equal(t, 5, countRows(t, s.DB(), ctx,
		`select count(*) from metric_streams`),
		"distinct logical metrics should not multiply across batches")
	assert.Equal(t, 5*batches, countRows(t, s.DB(), ctx,
		`select count(*) from metric_ingests`),
		"every batch should add one ingest per metric")

	// Stream ids should be stable across batches: every datapoint's
	// stream_id must match a metric_streams row, and every per-batch
	// metric_ingests row pointing at the same logical metric must
	// resolve to the same stream_id.
	gaugeStreamRows := countRows(t, s.DB(), ctx,
		`select count(distinct stream_id) from metric_ingests where stream_id in (
			select id from metric_streams where name = 'gauge_metric'
		)`)
	assert.Equal(t, 1, gaugeStreamRows,
		"all gauge_metric ingests must share one stream_id")

	// Sanity: cross-table referential integrity holds.
	orphanDatapoints := countRows(t, s.DB(), ctx,
		`select count(*) from datapoints d
		 left join metric_streams s on s.id = d.stream_id
		 where s.id is null`)
	assert.Equal(t, 0, orphanDatapoints, "no datapoint may dangle after dedup")
}

// TestMetricStreams_DistinctIdentitiesStayDistinct guards the inverse
// of the dedup contract: two metrics that differ in any one of the
// eight identity fields must produce two metric_streams rows, even when
// the rest of the tuple matches. We change one field at a time and
// assert each change yields a fresh stream so a future "be permissive"
// regression won't silently merge two semantically distinct streams.
func TestMetricStreams_DistinctIdentitiesStayDistinct(t *testing.T) {
	mk := func(t *testing.T, mutate func(m pmetric.Metric, scope pcommon.InstrumentationScope, res pcommon.Resource)) pmetric.Metrics {
		t.Helper()
		md := pmetric.NewMetrics()
		rm := md.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("service.name", "svc-a")
		sm := rm.ScopeMetrics().AppendEmpty()
		sm.Scope().SetName("scope")
		sm.Scope().SetVersion("v1")
		m := sm.Metrics().AppendEmpty()
		m.SetName("requests")
		m.SetUnit("count")
		m.SetEmptySum().SetIsMonotonic(true)
		m.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		dp := m.Sum().DataPoints().AppendEmpty()
		dp.SetIntValue(1)
		mutate(m, sm.Scope(), rm.Resource())
		return md
	}

	cases := []struct {
		name   string
		mutate func(m pmetric.Metric, scope pcommon.InstrumentationScope, res pcommon.Resource)
	}{
		{"name", func(m pmetric.Metric, _ pcommon.InstrumentationScope, _ pcommon.Resource) { m.SetName("requests_v2") }},
		{"unit", func(m pmetric.Metric, _ pcommon.InstrumentationScope, _ pcommon.Resource) { m.SetUnit("ms") }},
		{"temporality", func(m pmetric.Metric, _ pcommon.InstrumentationScope, _ pcommon.Resource) {
			m.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
		}},
		{"is_monotonic", func(m pmetric.Metric, _ pcommon.InstrumentationScope, _ pcommon.Resource) {
			m.Sum().SetIsMonotonic(false)
		}},
		{"scope_name", func(_ pmetric.Metric, sc pcommon.InstrumentationScope, _ pcommon.Resource) { sc.SetName("scope-b") }},
		{"scope_version", func(_ pmetric.Metric, sc pcommon.InstrumentationScope, _ pcommon.Resource) { sc.SetVersion("v2") }},
		{"service_name", func(_ pmetric.Metric, _ pcommon.InstrumentationScope, res pcommon.Resource) {
			res.Attributes().PutStr("service.name", "svc-b")
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, ctx, teardown := setupStore(t)
			defer teardown()

			err := s.WithConn(func(conn driver.Conn) error {
				return metrics.Ingest(ctx, conn, mk(t, func(pmetric.Metric, pcommon.InstrumentationScope, pcommon.Resource) {}))
			})
			require.NoError(t, err)

			err = s.WithConn(func(conn driver.Conn) error {
				return metrics.Ingest(ctx, conn, mk(t, tc.mutate))
			})
			require.NoError(t, err)

			assert.Equal(t, 2, countRows(t, s.DB(), ctx,
				`select count(*) from metric_streams`),
				"changing %s should produce a distinct stream", tc.name)
		})
	}
}

// TestMetricStreams_ServiceNameDenormStaysConsistent verifies the
// invariant that justifies denormalizing service.name as a column
// alongside its source-of-truth attribute row: for every metric_streams
// row, the column value must equal the resource attribute value that
// produced it. If we ever break this (e.g. by writing only the column
// and dropping the attribute, or by ingesting two batches with
// inconsistent service names for the same identity), this test will
// catch it.
func TestMetricStreams_ServiceNameDenormStaysConsistent(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	err := s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, createTestMetricsPdata())
	})
	require.NoError(t, err)

	// All five fixture metrics share service.name = test-service.
	// Match the column against the resource attribute by joining
	// metric_streams -> metric_ingests -> attributes(scope=resource,
	// key=service.name).
	mismatches := countRows(t, s.DB(), ctx, `
		select count(*) from metric_streams s
		join metric_ingests mi on mi.stream_id = s.id
		join attributes a
		     on a.metric_ingest_id = mi.id
		    and a.scope = 'resource'
		    and a.key = 'service.name'
		where s.service_name <> a.value
	`)
	assert.Equal(t, 0, mismatches,
		"metric_streams.service_name must equal the source resource attribute")
}

// TestEmptyMetrics verifies empty metric list and empty store.
func TestEmptyMetrics(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	err := s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, pmetric.NewMetrics())
	})
	assert.NoError(t, err)

	metricList := searchMetricsAll(t, s, ctx)
	assert.Empty(t, metricList)
}

// TestClearMetrics verifies that all metrics can be cleared, including child rows.
func TestClearMetrics(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	err := s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, createTestMetricsPdata())
	})
	assert.NoError(t, err)

	metricList := searchMetricsAll(t, s, ctx)
	assert.Len(t, metricList, 5)
	assert.Greater(t, countRows(t, s.DB(), ctx, "select count(*) from datapoints"), 0)
	assert.Greater(t, countRows(t, s.DB(), ctx, "select count(*) from attributes where metric_ingest_id is not null"), 0)

	err = metrics.Clear(ctx, s.DB())
	assert.NoError(t, err)

	metricList = searchMetricsAll(t, s, ctx)
	assert.Empty(t, metricList)
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from metric_streams"))
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from metric_ingests"))
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from datapoints"))
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from exemplars"))
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from attributes where metric_ingest_id is not null"))
}

// TestGetDatapointQuantiles exercises the metric_type dispatch and JSON shape
// of GetDatapointQuantiles. Macro correctness (linear vs log-linear
// interpolation, edge cases, etc.) is covered in schema_test.go; this test
// focuses on the Go helper stitching the right macro call together for each
// datapoint kind and surfacing the typed errors.
func TestGetDatapointQuantiles(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	err := s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, createTestMetricsPdata())
	})
	require.NoError(t, err, "ingest test metrics")

	// Index the first datapoint ID for each metric so subtests can ask for the
	// kind they need by name.
	metricList := searchMetricsAll(t, s, ctx)
	dpIDByMetric := make(map[string]string)
	for _, m := range metricList {
		name, _ := m["name"].(string)
		dps := metricDatapoints(m)
		if len(dps) == 0 {
			continue
		}
		dp, _ := dps[0].(map[string]any)
		if id, ok := dp["id"].(string); ok {
			dpIDByMetric[name] = id
		}
	}

	t.Run("Histogram", func(t *testing.T) {
		id := dpIDByMetric["histogram_metric"]
		require.NotEmpty(t, id)
		raw, err := metrics.GetDatapointQuantiles(ctx, s.DB(), id, []float64{0.5, 0.95, 0.99})
		require.NoError(t, err)

		var out map[string]any
		require.NoError(t, json.Unmarshal(raw, &out))
		assert.Contains(t, out, "0.5")
		assert.Contains(t, out, "0.95")
		assert.Contains(t, out, "0.99")

		// bounds=[0.5,1.0,1.5,2.0], counts=[10,20,30,25,15], total=100.
		// p50 target=50, cumulative=10,30,60,85,100. First acc>=50 is bucket 3
		// (1.0,1.5]: interp_linear = 1.0 + (1.5-1.0)*(50-30)/30 = 4/3.
		p50, ok := out["0.5"].(float64)
		require.True(t, ok, "p50 should be a number, got %T", out["0.5"])
		assert.InDelta(t, 4.0/3.0, p50, 1e-9)

		// p95 target=95: first acc>=95 is the clamped last bucket (2.0,2.0],
		// which interpolates to exactly 2.0.
		p95, ok := out["0.95"].(float64)
		require.True(t, ok)
		assert.InDelta(t, 2.0, p95, 1e-9)
	})

	t.Run("ExponentialHistogram", func(t *testing.T) {
		id := dpIDByMetric["exponential_histogram_metric"]
		require.NotEmpty(t, id)
		raw, err := metrics.GetDatapointQuantiles(ctx, s.DB(), id, []float64{0.5, 0.99})
		require.NoError(t, err)

		var out map[string]any
		require.NoError(t, json.Unmarshal(raw, &out))
		assert.Contains(t, out, "0.5")
		assert.Contains(t, out, "0.99")
		// Don't pin exact values here -- exp_hist_quantile correctness is in
		// schema_test.go. Just confirm both came back as finite numbers.
		_, ok := out["0.5"].(float64)
		assert.True(t, ok, "p50 should be a number, got %T", out["0.5"])
		_, ok = out["0.99"].(float64)
		assert.True(t, ok, "p99 should be a number, got %T", out["0.99"])
	})

	t.Run("UnsupportedType_Gauge", func(t *testing.T) {
		id := dpIDByMetric["gauge_metric"]
		require.NotEmpty(t, id)
		_, err := metrics.GetDatapointQuantiles(ctx, s.DB(), id, []float64{0.5})
		assert.ErrorIs(t, err, metrics.ErrQuantilesNotSupportedForType)
	})

	t.Run("UnsupportedType_Sum", func(t *testing.T) {
		id := dpIDByMetric["sum_metric"]
		require.NotEmpty(t, id)
		_, err := metrics.GetDatapointQuantiles(ctx, s.DB(), id, []float64{0.5})
		assert.ErrorIs(t, err, metrics.ErrQuantilesNotSupportedForType)
	})

	t.Run("DatapointNotFound", func(t *testing.T) {
		_, err := metrics.GetDatapointQuantiles(ctx, s.DB(), "00000000-0000-0000-0000-000000000000", []float64{0.5})
		assert.ErrorIs(t, err, metrics.ErrDatapointIDNotFound)
	})

	t.Run("EmptyQuantiles", func(t *testing.T) {
		id := dpIDByMetric["histogram_metric"]
		require.NotEmpty(t, id)
		raw, err := metrics.GetDatapointQuantiles(ctx, s.DB(), id, nil)
		assert.NoError(t, err)
		assert.JSONEq(t, "{}", string(raw))
	})
}

// TestGetMetricQuantileSeries_PerAttribute validates the per-attribute JSON shape,
// quantile dispatch, ordering, attribute key construction, and error paths
// of GetMetricQuantileSeries in "per-attribute" mode. The fixture only has one
// datapoint per histogram metric, so we don't exercise multi-row ordering
// here -- that lives in a richer fixture once 3b/3c land. Macro correctness
// (interpolation math) is covered in schema_test.go; we lean on
// GetDatapointQuantiles' p50=4/3 calc as a sanity-check anchor.
func TestGetMetricQuantileSeries_PerAttribute(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	err := s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, createTestMetricsPdata())
	})
	require.NoError(t, err, "ingest test metrics")

	metricList := searchMetricsAll(t, s, ctx)
	metricIDByName := make(map[string]string)
	for _, m := range metricList {
		name, _ := m["name"].(string)
		id, _ := m["id"].(string)
		if name != "" && id != "" {
			metricIDByName[name] = id
		}
	}

	t.Run("Histogram", func(t *testing.T) {
		id := metricIDByName["histogram_metric"]
		require.NotEmpty(t, id)

		raw, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5, 0.95, 0.99}, "per-attribute", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		require.Len(t, points, 1, "histogram_metric fixture has one datapoint")

		pt := points[0]
		// Required fields all present.
		assert.Contains(t, pt, "timestamp")
		assert.Contains(t, pt, "attributesKey")
		assert.Contains(t, pt, "attributes")
		assert.Contains(t, pt, "quantiles")
		assert.Contains(t, pt, "count")
		assert.Contains(t, pt, "sum")
		assert.Contains(t, pt, "min")
		assert.Contains(t, pt, "max")

		// Totals match the fixture (count=100, sum=25.5, min=0.1, max=2.5).
		assert.EqualValues(t, 100, pt["count"])
		assert.InDelta(t, 25.5, pt["sum"], 1e-9)
		assert.InDelta(t, 0.1, pt["min"], 1e-9)
		assert.InDelta(t, 2.5, pt["max"], 1e-9)

		// Attributes array is empty for this datapoint (fixture sets none on
		// the histogram dp). attributesKey should reflect that with "".
		assert.Equal(t, "", pt["attributesKey"])
		attrs, ok := pt["attributes"].([]any)
		require.True(t, ok)
		assert.Empty(t, attrs)

		// Quantile values match the same calc as GetDatapointQuantiles:
		// bounds=[0.5,1.0,1.5,2.0], counts=[10,20,30,25,15], total=100.
		// p50: bucket (1.0,1.5] -> 1.0 + 0.5 * (50-30)/30 = 4/3.
		quantiles, ok := pt["quantiles"].(map[string]any)
		require.True(t, ok)
		p50, ok := quantiles["0.5"].(float64)
		require.True(t, ok, "p50 should be a number, got %T", quantiles["0.5"])
		assert.InDelta(t, 4.0/3.0, p50, 1e-9)

		p95, ok := quantiles["0.95"].(float64)
		require.True(t, ok)
		assert.InDelta(t, 2.0, p95, 1e-9)

		_, ok = quantiles["0.99"].(float64)
		assert.True(t, ok, "p99 should be a number")
	})

	t.Run("ExponentialHistogram", func(t *testing.T) {
		id := metricIDByName["exponential_histogram_metric"]
		require.NotEmpty(t, id)

		raw, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5, 0.99}, "per-attribute", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		require.Len(t, points, 1)

		pt := points[0]
		assert.EqualValues(t, 50, pt["count"])
		assert.InDelta(t, 10240.0, pt["sum"], 1e-9)

		quantiles, ok := pt["quantiles"].(map[string]any)
		require.True(t, ok)
		// Don't pin exact values here -- exp_hist_quantile correctness is in
		// schema_test.go. Just confirm both came back as finite numbers.
		_, ok = quantiles["0.5"].(float64)
		assert.True(t, ok, "p50 should be a number, got %T", quantiles["0.5"])
		_, ok = quantiles["0.99"].(float64)
		assert.True(t, ok, "p99 should be a number")
	})

	t.Run("UnsupportedType_Gauge", func(t *testing.T) {
		id := metricIDByName["gauge_metric"]
		require.NotEmpty(t, id)
		_, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "per-attribute", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
		assert.ErrorIs(t, err, metrics.ErrQuantilesNotSupportedForType)
	})

	t.Run("UnsupportedType_Sum", func(t *testing.T) {
		id := metricIDByName["sum_metric"]
		require.NotEmpty(t, id)
		// gauge/sum fixtures are Cumulative -- the temporality check sits
		// after the type check, so we still see ErrQuantilesNotSupportedForType.
		_, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "per-attribute", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
		assert.ErrorIs(t, err, metrics.ErrQuantilesNotSupportedForType)
	})

	t.Run("MetricNotFound", func(t *testing.T) {
		_, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), "00000000-0000-0000-0000-000000000000", []float64{0.5}, "per-attribute", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
		assert.ErrorIs(t, err, metrics.ErrMetricIDNotFound)
	})

	t.Run("EmptyQuantiles", func(t *testing.T) {
		id := metricIDByName["histogram_metric"]
		require.NotEmpty(t, id)
		// Short-circuits before the type pre-check, so it doesn't even need a
		// real metric -- but use a real one anyway for clarity.
		raw, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, nil, "per-attribute", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
		assert.NoError(t, err)
		assert.JSONEq(t, "[]", string(raw))
	})

	t.Run("InvalidMode", func(t *testing.T) {
		id := metricIDByName["histogram_metric"]
		require.NotEmpty(t, id)
		_, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "bogus", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
		assert.ErrorIs(t, err, metrics.ErrInvalidQuantileSeriesMode)
	})

	t.Run("InvalidTimeRange", func(t *testing.T) {
		id := metricIDByName["histogram_metric"]
		require.NotEmpty(t, id)
		_, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "per-attribute", 100, 100, testQuantileWindowPoints)
		assert.ErrorIs(t, err, metrics.ErrInvalidTimeRange, "endTs == startTs is invalid")
		_, err = metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "per-attribute", 200, 100, testQuantileWindowPoints)
		assert.ErrorIs(t, err, metrics.ErrInvalidTimeRange, "endTs < startTs is invalid")
	})

	t.Run("InvalidMaxPoints", func(t *testing.T) {
		id := metricIDByName["histogram_metric"]
		require.NotEmpty(t, id)
		_, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "per-attribute", testQuantileWindowStartTs, testQuantileWindowEndTs, 0)
		assert.ErrorIs(t, err, metrics.ErrInvalidMaxPoints, "maxPoints=0 is invalid")
		_, err = metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "per-attribute", testQuantileWindowStartTs, testQuantileWindowEndTs, -1)
		assert.ErrorIs(t, err, metrics.ErrInvalidMaxPoints, "negative maxPoints is invalid")
	})

	t.Run("MergedExpHistogramSingleTimeseries", func(t *testing.T) {
		// The shared fixture only has one ExpHistogram datapoint, so the
		// merged path collapses trivially: no downscaling needed (one
		// scale), no offset alignment (one offset), no fold (default
		// zero_threshold = 0). The dispatch should still wire through
		// successfully and produce one row with finite quantiles.
		id := metricIDByName["exponential_histogram_metric"]
		require.NotEmpty(t, id)
		raw, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5, 0.99}, "merged", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		require.Len(t, points, 1)
		pt := points[0]
		assert.EqualValues(t, 50, pt["count"])
		assert.InDelta(t, 10240.0, pt["sum"], 1e-9)
		// Merged mode strips per-attribute identity.
		assert.Equal(t, "", pt["attributesKey"])
		quantiles, _ := pt["quantiles"].(map[string]any)
		_, ok := quantiles["0.5"].(float64)
		assert.True(t, ok, "p50 should be a finite number")
	})
}

// TestGetMetricQuantileSeries_UnspecifiedTemporality verifies the helper
// rejects metrics with Unspecified aggregation_temporality up front, so
// neither mode silently mis-aggregates. We can't get there via the normal
// pdata helpers (they default to one of Delta/Cumulative), so this test
// constructs an Unspecified-temporality histogram metric directly via
// pmetric and confirms ingest preserves it well enough for the pre-check
// to fire.
func TestGetMetricQuantileSeries_UnspecifiedTemporality(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	ts := time.Unix(1700000000, 0)
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "unspec-temp")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("test-scope")

	m := sm.Metrics().AppendEmpty()
	m.SetName("hist_unspec_temp")
	hist := m.SetEmptyHistogram()
	// Default temporality is Unspecified; leave it alone.
	hist.SetAggregationTemporality(pmetric.AggregationTemporalityUnspecified)
	dp := hist.DataPoints().AppendEmpty()
	dp.SetTimestamp(pcommon.Timestamp(ts.UnixNano()))
	dp.SetCount(10)
	dp.SetSum(5.0)
	dp.SetMin(0.1)
	dp.SetMax(2.0)
	dp.BucketCounts().FromRaw([]uint64{5, 5})
	dp.ExplicitBounds().FromRaw([]float64{1.0})

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, md)
	}))

	id := findMetricID(t, s, ctx, "hist_unspec_temp")

	_, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "per-attribute", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
	assert.ErrorIs(t, err, metrics.ErrUnspecifiedTemporality, "per-attribute should reject Unspecified")

	_, err = metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "merged", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
	assert.ErrorIs(t, err, metrics.ErrUnspecifiedTemporality, "merged should also reject Unspecified")
}

// TestGetMetricQuantileSeries_PerAttributeBucketing exercises the bucketing +
// temporality dispatch behavior for the per-attribute path. It covers both
// merge strategies (Delta sums, Cumulative picks latest), the half-open
// time window, and the empty-window case. Each subtest sets startTs/endTs
// tightly around a known timestamp range so we can also assert bucket
// boundary inclusivity.
func TestGetMetricQuantileSeries_PerAttributeBucketing(t *testing.T) {
	// Bucket size is min(1ms, (endTs-startTs)/maxPoints). With a 60s window
	// and maxPoints=1, bucket_ns = 60s -- so all our same-stream samples
	// land in one bucket and we can directly observe the merge behavior.
	const wideWindow = int64(60 * time.Second)
	const onePointMaxPoints = 1

	t.Run("DeltaSumsBucketCounts", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		baseSec := int64(1_700_000_000)
		startTs := baseSec * int64(time.Second)
		endTs := startTs + wideWindow

		// Two delta samples in the same bucket from the same stream:
		//   t=base+1s : counts [1, 2, 3]  (n=6)
		//   t=base+2s : counts [4, 5, 6]  (n=15)
		// Delta merge -> counts [5, 7, 9] (n=21).
		bounds := []float64{1.0, 2.0}
		md := makeHistogramFixtureT("hist_delta_bucket", pmetric.AggregationTemporalityDelta, []histTestDP{
			{timestamp: time.Unix(baseSec+1, 0), attrs: map[string]string{"host": "a"},
				bounds: bounds, counts: []uint64{1, 2, 3},
				count: 6, sum: 7.0, min: 0.5, max: 2.5},
			{timestamp: time.Unix(baseSec+2, 0), attrs: map[string]string{"host": "a"},
				bounds: bounds, counts: []uint64{4, 5, 6},
				count: 15, sum: 22.0, min: 0.1, max: 3.0},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "hist_delta_bucket")

		raw, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "per-attribute", startTs, endTs, onePointMaxPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		require.Len(t, points, 1, "two samples in one bucket -> one merged row")

		pt := points[0]
		// Merged totals: 6+15, 7.0+22.0, min(0.5,0.1), max(2.5,3.0).
		assert.EqualValues(t, 21, pt["count"])
		assert.InDelta(t, 29.0, pt["sum"], 1e-9)
		assert.InDelta(t, 0.1, pt["min"], 1e-9)
		assert.InDelta(t, 3.0, pt["max"], 1e-9)

		// Stream identity preserved. attributesKey is the canonical
		// "key=value|key=value|..." form of the datapoint's attribute set
		// (sorted ascending by key) -- the same string the frontend
		// computes locally to group raw datapoints, so the two paths
		// share one identity encoding. All we need to assert here is
		// that the per-attribute merge produced a single non-empty key.
		key, _ := pt["attributesKey"].(string)
		assert.NotEmpty(t, key, "attributesKey should be the canonical k=v|... form")
		assert.Contains(t, key, "=", "attributesKey should look like key=value|...")

		// p50 over merged buckets [5,7,9] (total 21, target 10.5):
		//   acc: 5, 12, 21. First acc>=10.5 is bucket 2 (1.0,2.0].
		//   linear interp = 1.0 + 1.0 * (10.5 - 5) / 7 = 1.0 + 5.5/7.
		quantiles, _ := pt["quantiles"].(map[string]any)
		require.Contains(t, quantiles, "0.5")
		p50, _ := quantiles["0.5"].(float64)
		assert.InDelta(t, 1.0+5.5/7.0, p50, 1e-9)
	})

	t.Run("CumulativeTakesLatestPerBucket", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		baseSec := int64(1_700_000_000)
		startTs := baseSec * int64(time.Second)
		endTs := startTs + wideWindow

		// Two cumulative samples in one bucket from the same stream.
		// Cumulative semantics: each row is a running total, summing would
		// double-count. We should only see the latest sample's values.
		bounds := []float64{1.0, 2.0}
		md := makeHistogramFixtureT("hist_cumul_bucket", pmetric.AggregationTemporalityCumulative, []histTestDP{
			{timestamp: time.Unix(baseSec+1, 0), attrs: map[string]string{"host": "a"},
				bounds: bounds, counts: []uint64{1, 2, 3},
				count: 6, sum: 7.0, min: 0.5, max: 2.5},
			{timestamp: time.Unix(baseSec+2, 0), attrs: map[string]string{"host": "a"},
				bounds: bounds, counts: []uint64{2, 4, 6},
				count: 12, sum: 15.0, min: 0.5, max: 3.0},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "hist_cumul_bucket")

		raw, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "per-attribute", startTs, endTs, onePointMaxPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		require.Len(t, points, 1)

		pt := points[0]
		// Should be the LATEST sample's values, not the sum.
		assert.EqualValues(t, 12, pt["count"], "cumulative path takes latest, not sum")
		assert.InDelta(t, 15.0, pt["sum"], 1e-9)
		assert.InDelta(t, 3.0, pt["max"], 1e-9)
	})

	t.Run("BucketBoundariesAreHalfOpen", func(t *testing.T) {
		// Window is [startTs, endTs). Sample at startTs included, sample at
		// endTs excluded. Both samples are from the same stream so the only
		// difference between the two cases is whether they're in the window.
		s, ctx, teardown := setupStore(t)
		defer teardown()

		baseSec := int64(1_700_000_000)
		startTs := baseSec * int64(time.Second)
		endTs := startTs + int64(10*time.Second)

		bounds := []float64{1.0}
		md := makeHistogramFixtureT("hist_boundary", pmetric.AggregationTemporalityDelta, []histTestDP{
			// At exactly startTs -- included.
			{timestamp: time.Unix(baseSec, 0), attrs: map[string]string{"host": "a"},
				bounds: bounds, counts: []uint64{1, 0},
				count: 1, sum: 0.5, min: 0.5, max: 0.5},
			// Inside the window -- included.
			{timestamp: time.Unix(baseSec+5, 0), attrs: map[string]string{"host": "a"},
				bounds: bounds, counts: []uint64{0, 2},
				count: 2, sum: 3.0, min: 1.5, max: 1.5},
			// At exactly endTs -- excluded.
			{timestamp: time.Unix(baseSec+10, 0), attrs: map[string]string{"host": "a"},
				bounds: bounds, counts: []uint64{0, 9},
				count: 9, sum: 13.5, min: 1.5, max: 1.5},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "hist_boundary")

		raw, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "per-attribute", startTs, endTs, onePointMaxPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		require.Len(t, points, 1, "samples at startTs + interior land in same bucket; sample at endTs excluded")

		// Merged of the two included samples (delta sum):
		// counts [1,0]+[0,2]=[1,2] -> total=3, sum=3.5.
		// If endTs sample leaked in, count would be 12.
		assert.EqualValues(t, 3, points[0]["count"], "endTs sample must be excluded by the half-open window")
		assert.InDelta(t, 3.5, points[0]["sum"], 1e-9)
	})

	t.Run("EmptyWindowReturnsEmptyArray", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		// Ingest a datapoint, then query a window that doesn't contain it.
		baseSec := int64(1_700_000_000)
		md := makeHistogramFixtureT("hist_empty_window", pmetric.AggregationTemporalityDelta, []histTestDP{
			{timestamp: time.Unix(baseSec, 0), attrs: map[string]string{"host": "a"},
				bounds: []float64{1.0}, counts: []uint64{1, 1},
				count: 2, sum: 1.5, min: 0.5, max: 1.5},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "hist_empty_window")

		// Query a future window where no datapoints exist.
		futureStart := (baseSec + 1_000_000) * int64(time.Second)
		futureEnd := futureStart + wideWindow
		raw, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "per-attribute", futureStart, futureEnd, onePointMaxPoints)
		require.NoError(t, err)
		assert.JSONEq(t, "[]", string(raw))
	})

	t.Run("ExpHistogramDeltaSumsBucketCounts", func(t *testing.T) {
		// Two ExpHist samples in the same bucket from the same stream;
		// scale + offsets stable (the realistic delta case). Within-stream
		// alignment is assumed: any_value(scale)/any_value(offset) +
		// sum_bucket_vectors. Asserts the count/sum totals plus that the
		// quantile macro returns a finite number from the merged buckets.
		s, ctx, teardown := setupStore(t)
		defer teardown()

		baseSec := int64(1_700_000_000)
		startTs := baseSec * int64(time.Second)
		endTs := startTs + wideWindow

		md := makeExpHistogramFixtureT("exp_delta_bucket", pmetric.AggregationTemporalityDelta, []expHistTestDP{
			{timestamp: time.Unix(baseSec+1, 0), attrs: map[string]string{"host": "a"},
				scale: 2, zeroCount: 0, posOffset: 0, posCounts: []uint64{1, 2, 3},
				count: 6, sum: 6.0, min: 0.1, max: 5.0},
			{timestamp: time.Unix(baseSec+2, 0), attrs: map[string]string{"host": "a"},
				scale: 2, zeroCount: 1, posOffset: 0, posCounts: []uint64{2, 2, 2},
				count: 7, sum: 8.0, min: 0.05, max: 5.0},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "exp_delta_bucket")

		raw, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "per-attribute", startTs, endTs, onePointMaxPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		require.Len(t, points, 1)
		pt := points[0]
		assert.EqualValues(t, 13, pt["count"], "delta sums totals")
		assert.InDelta(t, 14.0, pt["sum"], 1e-9)
		assert.InDelta(t, 0.05, pt["min"], 1e-9)

		quantiles, _ := pt["quantiles"].(map[string]any)
		_, ok := quantiles["0.5"].(float64)
		assert.True(t, ok, "p50 should be a finite number from the merged buckets")
	})

	t.Run("ExpHistogramCumulativeTakesLatest", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		baseSec := int64(1_700_000_000)
		startTs := baseSec * int64(time.Second)
		endTs := startTs + wideWindow

		md := makeExpHistogramFixtureT("exp_cumul_bucket", pmetric.AggregationTemporalityCumulative, []expHistTestDP{
			{timestamp: time.Unix(baseSec+1, 0), attrs: map[string]string{"host": "a"},
				scale: 2, zeroCount: 1, posOffset: 0, posCounts: []uint64{1, 2, 3},
				count: 7, sum: 6.0, min: 0.1, max: 5.0},
			{timestamp: time.Unix(baseSec+2, 0), attrs: map[string]string{"host": "a"},
				scale: 2, zeroCount: 2, posOffset: 0, posCounts: []uint64{3, 4, 5},
				count: 14, sum: 20.0, min: 0.05, max: 8.0},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "exp_cumul_bucket")

		raw, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "per-attribute", startTs, endTs, onePointMaxPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		require.Len(t, points, 1)
		// Latest sample only.
		assert.EqualValues(t, 14, points[0]["count"])
		assert.InDelta(t, 20.0, points[0]["sum"], 1e-9)
	})
}

// TestExpHistogramZeroThresholdRoundTrip pins the round-trip of the
// ExpHistogram zero_threshold column. We added the column ahead of the 3c
// merged-ExpHistogram path because the spec's merge rule is:
//
//	"taking the largest zero_threshold of all involved Histograms and merge
//	the lower buckets of Histograms with a smaller zero_threshold into the
//	common wider zero bucket"
//
// — which only works if we actually carry the per-attribute threshold. This
// test verifies that (a) the default zero_threshold of 0 from pdata makes
// it through ingest and surfaces in the JSON, and (b) a non-zero value
// also round-trips intact.
func TestExpHistogramZeroThresholdRoundTrip(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	ts := time.Unix(1700000000, 0)
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "zt-test")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("test-scope")

	// Metric 1: defaults (zero_threshold should land as 0).
	m1 := sm.Metrics().AppendEmpty()
	m1.SetName("exphist_zt_default")
	exp1 := m1.SetEmptyExponentialHistogram()
	exp1.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dp1 := exp1.DataPoints().AppendEmpty()
	dp1.SetTimestamp(pcommon.Timestamp(ts.UnixNano()))
	dp1.SetCount(10)
	dp1.SetSum(5.0)
	dp1.SetScale(2)
	dp1.SetZeroCount(1)
	dp1.Positive().SetOffset(0)
	dp1.Positive().BucketCounts().FromRaw([]uint64{4, 5})
	dp1.Negative().SetOffset(0)
	dp1.Negative().BucketCounts().FromRaw([]uint64{})

	// Metric 2: explicit non-zero threshold; the merge story in 3c needs
	// this value preserved exactly.
	m2 := sm.Metrics().AppendEmpty()
	m2.SetName("exphist_zt_set")
	exp2 := m2.SetEmptyExponentialHistogram()
	exp2.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dp2 := exp2.DataPoints().AppendEmpty()
	dp2.SetTimestamp(pcommon.Timestamp(ts.UnixNano()))
	dp2.SetCount(10)
	dp2.SetSum(5.0)
	dp2.SetScale(2)
	dp2.SetZeroCount(1)
	dp2.SetZeroThreshold(0.001)
	dp2.Positive().SetOffset(0)
	dp2.Positive().BucketCounts().FromRaw([]uint64{4, 5})
	dp2.Negative().SetOffset(0)
	dp2.Negative().BucketCounts().FromRaw([]uint64{})

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, md)
	}))

	byName := make(map[string]map[string]any)
	for _, m := range searchMetricsAll(t, s, ctx) {
		name, _ := m["name"].(string)
		byName[name] = m
	}

	// Helper: pull the first datapoint from a metric and return its
	// zeroThreshold value.
	zeroThreshold := func(metricName string) any {
		m, ok := byName[metricName]
		require.True(t, ok, "metric %s not in search results", metricName)
		dps := metricDatapoints(m)
		require.Len(t, dps, 1)
		dp, _ := dps[0].(map[string]any)
		require.Contains(t, dp, "zeroThreshold", "zeroThreshold missing from output JSON")
		return dp["zeroThreshold"]
	}

	// Default case: pdata's default is 0, so ingest writes 0.
	got := zeroThreshold("exphist_zt_default")
	gotF, ok := got.(float64)
	require.True(t, ok, "expected number, got %T", got)
	assert.InDelta(t, 0.0, gotF, 1e-12)

	// Explicit case: 0.001 should round-trip exactly.
	got = zeroThreshold("exphist_zt_set")
	gotF, ok = got.(float64)
	require.True(t, ok, "expected number, got %T", got)
	assert.InDelta(t, 0.001, gotF, 1e-12)
}

// makeMergedHistogramFixture builds a pmetric.Metrics with one
// Histogram metric (Delta temporality) containing the given datapoints. Used
// by the merged quantile series tests so each subtest can compose its
// own scenario (multi-stream, multi-timestamp, bounds mismatch) without
// perturbing the shared createTestMetricsPdata fixture.
func makeMergedHistogramFixture(name string, dps []histTestDP) pmetric.Metrics {
	return makeHistogramFixtureT(name, pmetric.AggregationTemporalityDelta, dps)
}

// makeHistogramFixtureT is the temporality-parameterized variant of
// makeMergedHistogramFixture. Bucketing tests use this to exercise both
// Delta (within-bucket sum) and Cumulative (within-bucket arg_max-latest)
// dispatch paths.
func makeHistogramFixtureT(name string, temporality pmetric.AggregationTemporality, dps []histTestDP) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "test-merged")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("test-scope")
	m := sm.Metrics().AppendEmpty()
	m.SetName(name)
	hist := m.SetEmptyHistogram()
	hist.SetAggregationTemporality(temporality)
	for _, dp := range dps {
		h := hist.DataPoints().AppendEmpty()
		h.SetTimestamp(pcommon.Timestamp(dp.timestamp.UnixNano()))
		h.SetCount(dp.count)
		h.SetSum(dp.sum)
		h.SetMin(dp.min)
		h.SetMax(dp.max)
		h.BucketCounts().FromRaw(dp.counts)
		h.ExplicitBounds().FromRaw(dp.bounds)
		for k, v := range dp.attrs {
			h.Attributes().PutStr(k, v)
		}
	}
	return md
}

// expHistTestDP is the ExpHistogram analogue of histTestDP. Tests build
// these for the bucketing + alignment paths.
type expHistTestDP struct {
	timestamp     time.Time
	attrs         map[string]string
	scale         int32
	zeroCount     uint64
	zeroThreshold float64
	posOffset     int32
	posCounts     []uint64
	negOffset     int32
	negCounts     []uint64
	count         uint64
	sum           float64
	min           float64
	max           float64
}

// makeExpHistogramFixtureT builds a single-metric ExpHistogram fixture with
// the given datapoints and temporality.
func makeExpHistogramFixtureT(name string, temporality pmetric.AggregationTemporality, dps []expHistTestDP) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "test-exphist")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("test-scope")
	m := sm.Metrics().AppendEmpty()
	m.SetName(name)
	exp := m.SetEmptyExponentialHistogram()
	exp.SetAggregationTemporality(temporality)
	for _, dp := range dps {
		e := exp.DataPoints().AppendEmpty()
		e.SetTimestamp(pcommon.Timestamp(dp.timestamp.UnixNano()))
		e.SetCount(dp.count)
		e.SetSum(dp.sum)
		e.SetMin(dp.min)
		e.SetMax(dp.max)
		e.SetScale(dp.scale)
		e.SetZeroCount(dp.zeroCount)
		e.SetZeroThreshold(dp.zeroThreshold)
		e.Positive().SetOffset(dp.posOffset)
		e.Positive().BucketCounts().FromRaw(dp.posCounts)
		e.Negative().SetOffset(dp.negOffset)
		e.Negative().BucketCounts().FromRaw(dp.negCounts)
		for k, v := range dp.attrs {
			e.Attributes().PutStr(k, v)
		}
	}
	return md
}

// histTestDP is a compact builder for one Histogram datapoint, used only
// in tests. Mirrors the shape of pmetric.HistogramDataPoint with the
// fields we actually exercise.
type histTestDP struct {
	timestamp time.Time
	attrs     map[string]string
	bounds    []float64
	counts    []uint64
	count     uint64
	sum       float64
	min       float64
	max       float64
}

// findMetricID looks up the ingested metric's UUID by name via Search. The
// id is generated at ingest time so we can't predict it.
func findMetricID(t *testing.T, s *store.Store, ctx context.Context, name string) string {
	t.Helper()
	for _, m := range searchMetricsAll(t, s, ctx) {
		if m["name"] == name {
			id, _ := m["id"].(string)
			return id
		}
	}
	t.Fatalf("metric %q not found", name)
	return ""
}

// TestGetMetricQuantileSeries_MergedHistogram covers the merge math,
// timestamp ordering, and bounds-mismatch error path of merged mode for
// regular Histograms. ExpHistogram aggregation is in 3c.
func TestGetMetricQuantileSeries_MergedHistogram(t *testing.T) {
	t.Run("MergesStreamsAtSameTimestamp", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		ts := time.Unix(1700000000, 0)
		bounds := []float64{0.5, 1.0, 1.5, 2.0}
		// Two streams at the same timestamp, same bounds. Merged buckets
		// should be element-wise sums:
		//   A: [10, 20, 30, 25, 15] (n=100)
		//   B: [ 5, 10, 15, 10, 10] (n=50)
		//   merged: [15, 30, 45, 35, 25] (n=150)
		md := makeMergedHistogramFixture("agg_hist", []histTestDP{
			{timestamp: ts, attrs: map[string]string{"host": "a"},
				bounds: bounds, counts: []uint64{10, 20, 30, 25, 15},
				count: 100, sum: 150.0, min: 0.1, max: 2.5},
			{timestamp: ts, attrs: map[string]string{"host": "b"},
				bounds: bounds, counts: []uint64{5, 10, 15, 10, 10},
				count: 50, sum: 75.0, min: 0.05, max: 2.0},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "agg_hist")

		raw, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5, 0.95}, "merged", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		require.Len(t, points, 1, "two streams at one timestamp -> one merged row")

		pt := points[0]
		// Merged mode strips per-attribute identity.
		assert.Equal(t, "", pt["attributesKey"])
		attrs, _ := pt["attributes"].([]any)
		assert.Empty(t, attrs)
		// Merged totals: 100+50, 150+75, min(0.1,0.05), max(2.5,2.0).
		assert.EqualValues(t, 150, pt["count"])
		assert.InDelta(t, 225.0, pt["sum"], 1e-9)
		assert.InDelta(t, 0.05, pt["min"], 1e-9)
		assert.InDelta(t, 2.5, pt["max"], 1e-9)

		// p50 over merged buckets [15,30,45,35,25] (total 150, target 75):
		//   acc: 15, 45, 90, 125, 150. First acc>=75 is bucket 3 (1.0,1.5].
		//   linear interp = 1.0 + 0.5 * (75 - 45) / 45 = 1.0 + 1/3.
		quantiles, _ := pt["quantiles"].(map[string]any)
		require.Contains(t, quantiles, "0.5")
		p50, _ := quantiles["0.5"].(float64)
		assert.InDelta(t, 1.0+1.0/3.0, p50, 1e-9)
	})

	t.Run("MultipleTimestampsOrderedAscending", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		bounds := []float64{1.0}
		t1 := time.Unix(1700000000, 0)
		t2 := time.Unix(1700000060, 0)
		t3 := time.Unix(1700000120, 0)
		md := makeMergedHistogramFixture("agg_hist_multi", []histTestDP{
			// Insert out of order to verify ORDER BY in the SQL.
			{timestamp: t3, attrs: map[string]string{"host": "a"}, bounds: bounds, counts: []uint64{1, 1}, count: 2, sum: 1.5, min: 0.1, max: 1.5},
			{timestamp: t1, attrs: map[string]string{"host": "a"}, bounds: bounds, counts: []uint64{2, 0}, count: 2, sum: 0.5, min: 0.1, max: 0.9},
			{timestamp: t2, attrs: map[string]string{"host": "a"}, bounds: bounds, counts: []uint64{1, 2}, count: 3, sum: 4.0, min: 0.5, max: 2.0},
			{timestamp: t2, attrs: map[string]string{"host": "b"}, bounds: bounds, counts: []uint64{0, 1}, count: 1, sum: 1.5, min: 1.5, max: 1.5},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "agg_hist_multi")

		raw, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "merged", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		require.Len(t, points, 3, "one row per distinct timestamp")

		// Verify ascending order by timestamp.
		ts0, _ := points[0]["timestamp"].(string)
		ts1, _ := points[1]["timestamp"].(string)
		ts2, _ := points[2]["timestamp"].(string)
		assert.Less(t, ts0, ts1)
		assert.Less(t, ts1, ts2)

		// t2 row merges streams a+b: counts [1,2]+[0,1] = [1,3], total 4,
		// sum 4.0+1.5=5.5, min(0.5,1.5)=0.5, max(2.0,1.5)=2.0.
		t2pt := points[1]
		assert.EqualValues(t, 4, t2pt["count"])
		assert.InDelta(t, 5.5, t2pt["sum"], 1e-9)
		assert.InDelta(t, 0.5, t2pt["min"], 1e-9)
		assert.InDelta(t, 2.0, t2pt["max"], 1e-9)
	})

	t.Run("BoundsMismatchRaisesTypedError", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		ts := time.Unix(1700000000, 0)
		// Two datapoints at the same timestamp with different bounds: this
		// is mathematically not mergeable and should surface as
		// ErrHistogramBoundsMismatch.
		md := makeMergedHistogramFixture("agg_hist_mismatch", []histTestDP{
			{timestamp: ts, attrs: map[string]string{"host": "a"},
				bounds: []float64{1.0}, counts: []uint64{5, 5},
				count: 10, sum: 5.0, min: 0.1, max: 2.0},
			{timestamp: ts, attrs: map[string]string{"host": "b"},
				bounds: []float64{0.5, 1.5}, counts: []uint64{3, 4, 3},
				count: 10, sum: 5.0, min: 0.1, max: 2.0},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "agg_hist_mismatch")

		_, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "merged", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
		assert.ErrorIs(t, err, metrics.ErrHistogramBoundsMismatch)
	})

	t.Run("BoundsMismatchAtDifferentTimestampsIsFine", func(t *testing.T) {
		// Bounds may legitimately change *across* timestamps (e.g., the
		// instrument was reconfigured). Each row's merge is independent, so
		// only intra-group mismatches should error.
		s, ctx, teardown := setupStore(t)
		defer teardown()

		t1 := time.Unix(1700000000, 0)
		t2 := time.Unix(1700000060, 0)
		md := makeMergedHistogramFixture("agg_hist_drift", []histTestDP{
			{timestamp: t1, attrs: map[string]string{"host": "a"},
				bounds: []float64{1.0}, counts: []uint64{5, 5},
				count: 10, sum: 5.0, min: 0.1, max: 2.0},
			{timestamp: t2, attrs: map[string]string{"host": "a"},
				bounds: []float64{0.5, 1.5}, counts: []uint64{3, 4, 3},
				count: 10, sum: 5.0, min: 0.1, max: 2.0},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "agg_hist_drift")

		raw, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "merged", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		assert.Len(t, points, 2, "different bounds at different timestamps -> two independent rows")
	})
}

// TestGetMetricQuantileSeries_MergedHistogramBucketing exercises the
// merged-Histogram path under the bucketing layer: multiple samples
// from multiple streams within a single bucket must merge first across time
// (per stream, per the temporality dispatch) and then across streams. Also
// verifies the bounds-mismatch detection still fires when streams disagree
// inside a bucket, and that within-stream cumulative samples are not
// double-counted across the cross-stream sum.
func TestGetMetricQuantileSeries_MergedHistogramBucketing(t *testing.T) {
	const wideWindow = int64(60 * time.Second)
	const onePointMaxPoints = 1

	t.Run("DeltaMultiStreamMultiSamplePerBucket", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		baseSec := int64(1_700_000_000)
		startTs := baseSec * int64(time.Second)
		endTs := startTs + wideWindow

		// Two streams, each contributing two delta samples in the same
		// bucket. Time merge per stream first, then cross-stream merge:
		//   stream A: [1,2,3]+[4,5,6] = [5,7,9]   (n=21)
		//   stream B: [0,1,2]+[3,4,5] = [3,5,7]   (n=15)
		//   final  : [5,7,9]+[3,5,7]  = [8,12,16] (n=36)
		bounds := []float64{1.0, 2.0}
		md := makeHistogramFixtureT("agg_hist_delta_bucket", pmetric.AggregationTemporalityDelta, []histTestDP{
			{timestamp: time.Unix(baseSec+1, 0), attrs: map[string]string{"host": "a"},
				bounds: bounds, counts: []uint64{1, 2, 3}, count: 6, sum: 7.0, min: 0.5, max: 2.5},
			{timestamp: time.Unix(baseSec+2, 0), attrs: map[string]string{"host": "a"},
				bounds: bounds, counts: []uint64{4, 5, 6}, count: 15, sum: 22.0, min: 0.1, max: 3.0},
			{timestamp: time.Unix(baseSec+1, 0), attrs: map[string]string{"host": "b"},
				bounds: bounds, counts: []uint64{0, 1, 2}, count: 3, sum: 4.0, min: 1.5, max: 2.5},
			{timestamp: time.Unix(baseSec+2, 0), attrs: map[string]string{"host": "b"},
				bounds: bounds, counts: []uint64{3, 4, 5}, count: 12, sum: 17.0, min: 0.5, max: 3.0},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "agg_hist_delta_bucket")

		raw, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "merged", startTs, endTs, onePointMaxPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		require.Len(t, points, 1)

		pt := points[0]
		// Merged mode strips per-attribute identity.
		assert.Equal(t, "", pt["attributesKey"])
		// Merged totals across all 4 samples.
		assert.EqualValues(t, 36, pt["count"])
		assert.InDelta(t, 50.0, pt["sum"], 1e-9)
		assert.InDelta(t, 0.1, pt["min"], 1e-9)
		assert.InDelta(t, 3.0, pt["max"], 1e-9)

		// p50 over [8,12,16] (total 36, target 18):
		//   acc: 8, 20, 36. First acc>=18 is bucket 2 (1.0,2.0].
		//   linear interp = 1.0 + 1.0 * (18 - 8) / 12 = 1.0 + 10/12.
		quantiles, _ := pt["quantiles"].(map[string]any)
		p50, _ := quantiles["0.5"].(float64)
		assert.InDelta(t, 1.0+10.0/12.0, p50, 1e-9)
	})

	t.Run("CumulativeMultiStreamLatestPerBucketThenSum", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		baseSec := int64(1_700_000_000)
		startTs := baseSec * int64(time.Second)
		endTs := startTs + wideWindow

		// Two streams, each with two cumulative samples in the same bucket.
		// Within-stream: take latest. Cross-stream: sum.
		//   stream A latest: [2, 4, 6]  (n=12)
		//   stream B latest: [3, 5, 7]  (n=15)
		//   final         : [5, 9, 13]  (n=27)
		// If we mistakenly summed across time, A would be [3,6,9] etc.
		bounds := []float64{1.0, 2.0}
		md := makeHistogramFixtureT("agg_hist_cumul_bucket", pmetric.AggregationTemporalityCumulative, []histTestDP{
			{timestamp: time.Unix(baseSec+1, 0), attrs: map[string]string{"host": "a"},
				bounds: bounds, counts: []uint64{1, 2, 3}, count: 6, sum: 7.0, min: 0.5, max: 2.5},
			{timestamp: time.Unix(baseSec+2, 0), attrs: map[string]string{"host": "a"},
				bounds: bounds, counts: []uint64{2, 4, 6}, count: 12, sum: 15.0, min: 0.5, max: 3.0},
			{timestamp: time.Unix(baseSec+1, 0), attrs: map[string]string{"host": "b"},
				bounds: bounds, counts: []uint64{1, 2, 3}, count: 6, sum: 7.0, min: 0.5, max: 2.5},
			{timestamp: time.Unix(baseSec+2, 0), attrs: map[string]string{"host": "b"},
				bounds: bounds, counts: []uint64{3, 5, 7}, count: 15, sum: 18.0, min: 0.5, max: 3.0},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "agg_hist_cumul_bucket")

		raw, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "merged", startTs, endTs, onePointMaxPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		require.Len(t, points, 1)

		pt := points[0]
		assert.EqualValues(t, 27, pt["count"], "should sum streams' latest, not double-count by time-summing first")
		assert.InDelta(t, 33.0, pt["sum"], 1e-9)
	})

	t.Run("BoundsMismatchAcrossStreamsWithinBucket", func(t *testing.T) {
		// Two streams in the same bucket with different bounds is still an
		// error, exactly like before bucketing.
		s, ctx, teardown := setupStore(t)
		defer teardown()

		baseSec := int64(1_700_000_000)
		startTs := baseSec * int64(time.Second)
		endTs := startTs + wideWindow

		md := makeHistogramFixtureT("agg_hist_bucket_mismatch", pmetric.AggregationTemporalityDelta, []histTestDP{
			{timestamp: time.Unix(baseSec+1, 0), attrs: map[string]string{"host": "a"},
				bounds: []float64{1.0}, counts: []uint64{5, 5}, count: 10, sum: 5.0, min: 0.1, max: 2.0},
			{timestamp: time.Unix(baseSec+2, 0), attrs: map[string]string{"host": "b"},
				bounds: []float64{0.5, 1.5}, counts: []uint64{3, 4, 3}, count: 10, sum: 5.0, min: 0.1, max: 2.0},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "agg_hist_bucket_mismatch")

		_, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "merged", startTs, endTs, onePointMaxPoints)
		assert.ErrorIs(t, err, metrics.ErrHistogramBoundsMismatch)
	})
}

// TestGetMetricQuantileSeries_MergedExpHistogramBucketing exercises the
// merged-ExpHistogram path under the bucketing layer: it feeds multiple
// streams (with varying scale, offset, and zero_threshold) through the full
// alignment pipeline (downscale -> pad_left -> sum -> fold) and verifies
// that totals roll up correctly and that quantiles come back as finite
// numbers. Quantile *values* depend on internal bucket math we don't want
// to over-pin (the layout of merged buckets after downscale + fold is
// already covered by macro-level tests in schema_test.go); here we focus
// on the contract: ordering, counts/sum/min/max conservation across the
// merge, and dispatch on temporality.
func TestGetMetricQuantileSeries_MergedExpHistogramBucketing(t *testing.T) {
	const wideWindow = int64(60 * time.Second)
	const onePointMaxPoints = 1

	t.Run("DeltaSameScaleSameOffset", func(t *testing.T) {
		// Two streams with identical scale and offset land in one bucket.
		// No downscale or padding needed; the alignment pipeline collapses
		// to a plain element-wise sum across streams.
		s, ctx, teardown := setupStore(t)
		defer teardown()

		baseSec := int64(1_700_000_000)
		startTs := baseSec * int64(time.Second)
		endTs := startTs + wideWindow

		md := makeExpHistogramFixtureT("agg_exp_same_align", pmetric.AggregationTemporalityDelta, []expHistTestDP{
			{timestamp: time.Unix(baseSec+1, 0), attrs: map[string]string{"host": "a"},
				scale: 2, zeroCount: 0, posOffset: 0, posCounts: []uint64{1, 2, 3},
				count: 6, sum: 6.0, min: 0.5, max: 5.0},
			{timestamp: time.Unix(baseSec+1, 0), attrs: map[string]string{"host": "b"},
				scale: 2, zeroCount: 1, posOffset: 0, posCounts: []uint64{2, 3, 4},
				count: 10, sum: 10.0, min: 0.1, max: 6.0},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "agg_exp_same_align")

		raw, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5, 0.95}, "merged", startTs, endTs, onePointMaxPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		require.Len(t, points, 1)
		pt := points[0]

		assert.Equal(t, "", pt["attributesKey"], "merged mode strips per-attribute identity")
		// Cross-stream totals are independent of the alignment pipeline.
		assert.EqualValues(t, 16, pt["count"])
		assert.InDelta(t, 16.0, pt["sum"], 1e-9)
		assert.InDelta(t, 0.1, pt["min"], 1e-9)
		assert.InDelta(t, 6.0, pt["max"], 1e-9)

		quantiles, _ := pt["quantiles"].(map[string]any)
		_, ok := quantiles["0.5"].(float64)
		assert.True(t, ok, "p50 should be a finite number")
		_, ok = quantiles["0.95"].(float64)
		assert.True(t, ok, "p95 should be a finite number")
	})

	t.Run("DeltaMixedScales", func(t *testing.T) {
		// Two streams with different scales force the downscale step:
		// stream A is downscaled to stream B's coarser scale, then both
		// align at the common scale and sum. Totals must still match.
		s, ctx, teardown := setupStore(t)
		defer teardown()

		baseSec := int64(1_700_000_000)
		startTs := baseSec * int64(time.Second)
		endTs := startTs + wideWindow

		md := makeExpHistogramFixtureT("agg_exp_mixed_scales", pmetric.AggregationTemporalityDelta, []expHistTestDP{
			{timestamp: time.Unix(baseSec+1, 0), attrs: map[string]string{"host": "a"},
				scale: 2, zeroCount: 0, posOffset: 0, posCounts: []uint64{1, 2, 3, 4},
				count: 10, sum: 12.0, min: 0.2, max: 4.0},
			{timestamp: time.Unix(baseSec+1, 0), attrs: map[string]string{"host": "b"},
				scale: 1, zeroCount: 0, posOffset: 0, posCounts: []uint64{5, 7},
				count: 12, sum: 18.0, min: 0.3, max: 5.0},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "agg_exp_mixed_scales")

		raw, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "merged", startTs, endTs, onePointMaxPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		require.Len(t, points, 1)
		pt := points[0]

		assert.EqualValues(t, 22, pt["count"], "downscale must not change total count")
		assert.InDelta(t, 30.0, pt["sum"], 1e-9, "downscale must not change total sum")
		assert.InDelta(t, 0.2, pt["min"], 1e-9)
		assert.InDelta(t, 5.0, pt["max"], 1e-9)

		quantiles, _ := pt["quantiles"].(map[string]any)
		_, ok := quantiles["0.5"].(float64)
		assert.True(t, ok, "p50 should be finite after mixed-scale alignment")
	})

	t.Run("DeltaMixedOffsets", func(t *testing.T) {
		// Same scale, different offsets: pad_left_to_offset shifts both
		// streams to the bucket's min offset, then sum_bucket_vectors
		// merges. Totals are again independent of the bucket layout.
		s, ctx, teardown := setupStore(t)
		defer teardown()

		baseSec := int64(1_700_000_000)
		startTs := baseSec * int64(time.Second)
		endTs := startTs + wideWindow

		md := makeExpHistogramFixtureT("agg_exp_mixed_offsets", pmetric.AggregationTemporalityDelta, []expHistTestDP{
			{timestamp: time.Unix(baseSec+1, 0), attrs: map[string]string{"host": "a"},
				scale: 2, zeroCount: 0, posOffset: 0, posCounts: []uint64{2, 3, 4},
				count: 9, sum: 9.0, min: 0.5, max: 3.0},
			{timestamp: time.Unix(baseSec+1, 0), attrs: map[string]string{"host": "b"},
				scale: 2, zeroCount: 0, posOffset: 2, posCounts: []uint64{5, 6},
				count: 11, sum: 22.0, min: 1.0, max: 4.0},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "agg_exp_mixed_offsets")

		raw, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "merged", startTs, endTs, onePointMaxPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		require.Len(t, points, 1)
		pt := points[0]

		assert.EqualValues(t, 20, pt["count"])
		assert.InDelta(t, 31.0, pt["sum"], 1e-9)

		quantiles, _ := pt["quantiles"].(map[string]any)
		_, ok := quantiles["0.5"].(float64)
		assert.True(t, ok, "p50 should be finite after offset alignment")
	})

	t.Run("DeltaMixedZeroThresholdsFolds", func(t *testing.T) {
		// Two streams with different zero_thresholds: max wins, and
		// buckets at or below the corresponding cutoff fold back into
		// zero_count. We check totals (which are conserved by the fold)
		// and that the quantile pipeline still produces a finite p95.
		//
		// At target_scale=2, target_zero_threshold=1.5:
		//   cutoff = floor(log2(1.5) * 2^2) - 1 = floor(2.34) - 1 = 1
		// So merged positive buckets 0 and 1 fold into zero_count.
		s, ctx, teardown := setupStore(t)
		defer teardown()

		baseSec := int64(1_700_000_000)
		startTs := baseSec * int64(time.Second)
		endTs := startTs + wideWindow

		md := makeExpHistogramFixtureT("agg_exp_fold", pmetric.AggregationTemporalityDelta, []expHistTestDP{
			{timestamp: time.Unix(baseSec+1, 0), attrs: map[string]string{"host": "a"},
				scale: 2, zeroCount: 0, zeroThreshold: 0, posOffset: 0, posCounts: []uint64{5, 5, 5, 5},
				count: 20, sum: 25.0, min: 0.5, max: 5.0},
			{timestamp: time.Unix(baseSec+1, 0), attrs: map[string]string{"host": "b"},
				scale: 2, zeroCount: 2, zeroThreshold: 1.5, posOffset: 0, posCounts: []uint64{1, 1, 1, 1},
				count: 6, sum: 6.0, min: 0.0, max: 5.0},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "agg_exp_fold")

		raw, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.95}, "merged", startTs, endTs, onePointMaxPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		require.Len(t, points, 1)
		pt := points[0]

		// Folding moves counts between buckets and zero_count, but never
		// loses any. Cross-stream count/sum/min/max are direct aggregates
		// over time_merged, so they don't change shape under fold.
		assert.EqualValues(t, 26, pt["count"])
		assert.InDelta(t, 31.0, pt["sum"], 1e-9)

		quantiles, _ := pt["quantiles"].(map[string]any)
		p95, ok := quantiles["0.95"].(float64)
		require.True(t, ok, "p95 should be finite after fold")
		// p95 falls in the upper buckets, well above the fold cutoff,
		// so it must be strictly positive.
		assert.Greater(t, p95, 0.0)
	})

	t.Run("CumulativeMultiStreamLatestPerBucketThenSum", func(t *testing.T) {
		// Cumulative variant: per-attribute within-bucket merge takes the
		// latest sample (arg_max), then cross-stream merge sums. If the
		// dispatch is wrong (e.g. delta-summing across time first), the
		// totals would double-count; the assertion catches that.
		s, ctx, teardown := setupStore(t)
		defer teardown()

		baseSec := int64(1_700_000_000)
		startTs := baseSec * int64(time.Second)
		endTs := startTs + wideWindow

		md := makeExpHistogramFixtureT("agg_exp_cumul", pmetric.AggregationTemporalityCumulative, []expHistTestDP{
			{timestamp: time.Unix(baseSec+1, 0), attrs: map[string]string{"host": "a"},
				scale: 2, zeroCount: 0, posOffset: 0, posCounts: []uint64{1, 2, 3},
				count: 6, sum: 6.0, min: 0.5, max: 5.0},
			{timestamp: time.Unix(baseSec+2, 0), attrs: map[string]string{"host": "a"},
				scale: 2, zeroCount: 1, posOffset: 0, posCounts: []uint64{2, 4, 6},
				count: 13, sum: 15.0, min: 0.5, max: 6.0},
			{timestamp: time.Unix(baseSec+1, 0), attrs: map[string]string{"host": "b"},
				scale: 2, zeroCount: 0, posOffset: 0, posCounts: []uint64{1, 1, 1},
				count: 3, sum: 3.0, min: 0.5, max: 5.0},
			{timestamp: time.Unix(baseSec+2, 0), attrs: map[string]string{"host": "b"},
				scale: 2, zeroCount: 2, posOffset: 0, posCounts: []uint64{3, 5, 7},
				count: 17, sum: 20.0, min: 0.4, max: 7.0},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "agg_exp_cumul")

		raw, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "merged", startTs, endTs, onePointMaxPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		require.Len(t, points, 1)
		pt := points[0]

		// Latest per stream: A=13/15.0, B=17/20.0. Cross-stream sum: 30/35.0.
		assert.EqualValues(t, 30, pt["count"], "should sum streams' latest, not double-count by time-summing first")
		assert.InDelta(t, 35.0, pt["sum"], 1e-9)
	})

	t.Run("MultiBucketOrderedAscending", func(t *testing.T) {
		// Three buckets across the window. With wideWindow=60s and
		// maxPoints=60, bucket_ns = 1s, so each timestamp lands in its
		// own bucket. Inserts are deliberately out of order to verify
		// the ORDER BY bucket_start in the final select.
		s, ctx, teardown := setupStore(t)
		defer teardown()

		baseSec := int64(1_700_000_000)
		startTs := baseSec * int64(time.Second)
		endTs := startTs + wideWindow
		const maxPoints = 60

		md := makeExpHistogramFixtureT("agg_exp_multi_bucket", pmetric.AggregationTemporalityDelta, []expHistTestDP{
			{timestamp: time.Unix(baseSec+30, 0), attrs: map[string]string{"host": "a"},
				scale: 2, zeroCount: 0, posOffset: 0, posCounts: []uint64{2, 2, 2},
				count: 6, sum: 6.0, min: 0.5, max: 5.0},
			{timestamp: time.Unix(baseSec+1, 0), attrs: map[string]string{"host": "a"},
				scale: 2, zeroCount: 0, posOffset: 0, posCounts: []uint64{1, 1, 1},
				count: 3, sum: 3.0, min: 0.5, max: 5.0},
			{timestamp: time.Unix(baseSec+50, 0), attrs: map[string]string{"host": "a"},
				scale: 2, zeroCount: 0, posOffset: 0, posCounts: []uint64{3, 3, 3},
				count: 9, sum: 9.0, min: 0.5, max: 5.0},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "agg_exp_multi_bucket")

		raw, err := metrics.GetMetricQuantileSeries(ctx, s.DB(), id, []float64{0.5}, "merged", startTs, endTs, maxPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		require.Len(t, points, 3, "one row per bucket")

		ts0, _ := points[0]["timestamp"].(string)
		ts1, _ := points[1]["timestamp"].(string)
		ts2, _ := points[2]["timestamp"].(string)
		assert.Less(t, ts0, ts1, "buckets should be ordered ascending by timestamp")
		assert.Less(t, ts1, ts2)

		// Each bucket has exactly one sample, so totals match the input.
		assert.EqualValues(t, 3, points[0]["count"])
		assert.EqualValues(t, 6, points[1]["count"])
		assert.EqualValues(t, 9, points[2]["count"])
	})
}

// TestIngestMetrics_FlushInterval exercises the flushIntervalMetrics codepath by ingesting
// more than 100 metrics in one call (flush runs when metricCount % 100 == 0). All metrics
// have resource and scope attributes; we assert they were flushed correctly.
func TestIngestMetrics_FlushInterval(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	const batchSize = 101 // > flushIntervalMetrics (100)
	err := s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, createTestMetricsPdataN(batchSize))
	})
	assert.NoError(t, err)

	metrics := searchMetricsAll(t, s, ctx)
	assert.Len(t, metrics, batchSize)

	// Find metrics by name so we can assert attributes on first, 99th, 100th, 101st
	byName := make(map[string]map[string]any)
	for i := range metrics {
		m := metrics[i]
		if name, ok := m["name"].(string); ok {
			byName[name] = m
		}
	}
	for _, idx := range []int{0, 99, 100} {
		name := "flush_metric_" + fmt.Sprintf("%d", idx)
		m, ok := byName[name]
		assert.True(t, ok, "metric %s", name)
		resource, _ := m["resource"].(map[string]any)
		assert.NotNil(t, resource)
		attrs, _ := resource["attributes"].([]any)
		var resourceKey string
		for _, a := range attrs {
			kv, _ := a.(map[string]any)
			if k, _ := kv["key"].(string); k == "resource.key" {
				resourceKey, _ = kv["value"].(string)
				break
			}
		}
		assert.Equal(t, "resource.val", resourceKey, "metric %s resource.key", name)
		scope, _ := m["scope"].(map[string]any)
		assert.NotNil(t, scope)
		scopeAttrs, _ := scope["attributes"].([]any)
		var scopeKey string
		for _, a := range scopeAttrs {
			kv, _ := a.(map[string]any)
			if k, _ := kv["key"].(string); k == "scope.key" {
				scopeKey, _ = kv["value"].(string)
				break
			}
		}
		assert.Equal(t, "scope.val", scopeKey, "metric %s scope.key", name)
	}
}

// ---------------------------------------------------------------------------
// GetMetricBucketSeries
// ---------------------------------------------------------------------------

func TestGetMetricBucketSeries_PerAttribute(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	err := s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, createTestMetricsPdata())
	})
	require.NoError(t, err, "ingest test metrics")

	metricList := searchMetricsAll(t, s, ctx)
	metricIDByName := make(map[string]string)
	for _, m := range metricList {
		name, _ := m["name"].(string)
		id, _ := m["id"].(string)
		if name != "" && id != "" {
			metricIDByName[name] = id
		}
	}

	t.Run("Histogram", func(t *testing.T) {
		id := metricIDByName["histogram_metric"]
		require.NotEmpty(t, id)

		raw, err := metrics.GetMetricBucketSeries(ctx, s.DB(), id, "per-attribute", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		require.Len(t, points, 1, "histogram_metric fixture has one datapoint")

		pt := points[0]
		assert.Equal(t, "histogram", pt["kind"])
		assert.Contains(t, pt, "timestamp")
		assert.Contains(t, pt, "attributesKey")
		assert.Contains(t, pt, "attributes")
		assert.Contains(t, pt, "bounds")
		assert.Contains(t, pt, "counts")
		assert.Contains(t, pt, "totals")

		assert.Equal(t, "", pt["attributesKey"])

		totals, ok := pt["totals"].(map[string]any)
		require.True(t, ok)
		assert.EqualValues(t, 100, totals["count"])
		assert.InDelta(t, 25.5, totals["sum"], 1e-9)
		assert.InDelta(t, 0.1, totals["min"], 1e-9)
		assert.InDelta(t, 2.5, totals["max"], 1e-9)

		bounds, ok := pt["bounds"].([]any)
		require.True(t, ok)
		assert.Len(t, bounds, 4, "fixture has 4 explicit bounds")

		counts, ok := pt["counts"].([]any)
		require.True(t, ok)
		assert.Len(t, counts, 5, "fixture has 5 bucket counts")
	})

	t.Run("ExponentialHistogram", func(t *testing.T) {
		id := metricIDByName["exponential_histogram_metric"]
		require.NotEmpty(t, id)

		raw, err := metrics.GetMetricBucketSeries(ctx, s.DB(), id, "per-attribute", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		require.Len(t, points, 1)

		pt := points[0]
		assert.Equal(t, "expHistogram", pt["kind"])
		assert.Contains(t, pt, "scale")
		assert.Contains(t, pt, "zeroThreshold")
		assert.Contains(t, pt, "zeroCount")
		assert.Contains(t, pt, "positiveOffset")
		assert.Contains(t, pt, "positiveCounts")
		assert.Contains(t, pt, "negativeOffset")
		assert.Contains(t, pt, "negativeCounts")

		totals, ok := pt["totals"].(map[string]any)
		require.True(t, ok)
		assert.EqualValues(t, 50, totals["count"])
		assert.InDelta(t, 10240.0, totals["sum"], 1e-9)
	})

	t.Run("UnsupportedType_Gauge", func(t *testing.T) {
		id := metricIDByName["gauge_metric"]
		require.NotEmpty(t, id)
		_, err := metrics.GetMetricBucketSeries(ctx, s.DB(), id, "per-attribute", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
		assert.ErrorIs(t, err, metrics.ErrBucketSeriesNotSupportedForType)
	})

	t.Run("UnsupportedType_Sum", func(t *testing.T) {
		id := metricIDByName["sum_metric"]
		require.NotEmpty(t, id)
		_, err := metrics.GetMetricBucketSeries(ctx, s.DB(), id, "per-attribute", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
		assert.ErrorIs(t, err, metrics.ErrBucketSeriesNotSupportedForType)
	})

	t.Run("MetricNotFound", func(t *testing.T) {
		_, err := metrics.GetMetricBucketSeries(ctx, s.DB(), "00000000-0000-0000-0000-000000000000", "per-attribute", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
		assert.ErrorIs(t, err, metrics.ErrMetricIDNotFound)
	})

	t.Run("InvalidMode", func(t *testing.T) {
		id := metricIDByName["histogram_metric"]
		require.NotEmpty(t, id)
		_, err := metrics.GetMetricBucketSeries(ctx, s.DB(), id, "bogus", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
		assert.ErrorIs(t, err, metrics.ErrInvalidQuantileSeriesMode)
	})

	t.Run("InvalidTimeRange", func(t *testing.T) {
		id := metricIDByName["histogram_metric"]
		require.NotEmpty(t, id)
		_, err := metrics.GetMetricBucketSeries(ctx, s.DB(), id, "per-attribute", 100, 100, testQuantileWindowPoints)
		assert.ErrorIs(t, err, metrics.ErrInvalidTimeRange, "endTs == startTs is invalid")
		_, err = metrics.GetMetricBucketSeries(ctx, s.DB(), id, "per-attribute", 200, 100, testQuantileWindowPoints)
		assert.ErrorIs(t, err, metrics.ErrInvalidTimeRange, "endTs < startTs is invalid")
	})

	t.Run("InvalidMaxPoints", func(t *testing.T) {
		id := metricIDByName["histogram_metric"]
		require.NotEmpty(t, id)
		_, err := metrics.GetMetricBucketSeries(ctx, s.DB(), id, "per-attribute", testQuantileWindowStartTs, testQuantileWindowEndTs, 0)
		assert.ErrorIs(t, err, metrics.ErrInvalidMaxPoints, "maxPoints=0 is invalid")
		_, err = metrics.GetMetricBucketSeries(ctx, s.DB(), id, "per-attribute", testQuantileWindowStartTs, testQuantileWindowEndTs, -1)
		assert.ErrorIs(t, err, metrics.ErrInvalidMaxPoints, "negative maxPoints is invalid")
	})

	t.Run("MergedExpHistogramSingleTimeseries", func(t *testing.T) {
		id := metricIDByName["exponential_histogram_metric"]
		require.NotEmpty(t, id)
		raw, err := metrics.GetMetricBucketSeries(ctx, s.DB(), id, "merged", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		require.Len(t, points, 1)
		pt := points[0]
		assert.Equal(t, "expHistogram", pt["kind"])
		assert.Equal(t, "", pt["attributesKey"])

		totals, ok := pt["totals"].(map[string]any)
		require.True(t, ok)
		assert.EqualValues(t, 50, totals["count"])
		assert.InDelta(t, 10240.0, totals["sum"], 1e-9)
	})
}

func TestGetMetricBucketSeries_MergedHistogram(t *testing.T) {
	t.Run("MergesStreamsAtSameTimestamp", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		ts := time.Unix(1700000000, 0)
		bounds := []float64{0.5, 1.0, 1.5, 2.0}
		md := makeMergedHistogramFixture("agg_hist_bs", []histTestDP{
			{timestamp: ts, attrs: map[string]string{"host": "a"},
				bounds: bounds, counts: []uint64{10, 20, 30, 25, 15},
				count: 100, sum: 150.0, min: 0.1, max: 2.5},
			{timestamp: ts, attrs: map[string]string{"host": "b"},
				bounds: bounds, counts: []uint64{5, 10, 15, 10, 10},
				count: 50, sum: 75.0, min: 0.05, max: 2.0},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "agg_hist_bs")

		raw, err := metrics.GetMetricBucketSeries(ctx, s.DB(), id, "merged", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		require.Len(t, points, 1, "two streams at one timestamp -> one merged row")

		pt := points[0]
		assert.Equal(t, "histogram", pt["kind"])
		assert.Equal(t, "", pt["attributesKey"])

		totals, ok := pt["totals"].(map[string]any)
		require.True(t, ok)
		assert.EqualValues(t, 150, totals["count"])
		assert.InDelta(t, 225.0, totals["sum"], 1e-9)
		assert.InDelta(t, 0.05, totals["min"], 1e-9)
		assert.InDelta(t, 2.5, totals["max"], 1e-9)

		// Merged bucket counts: [15, 30, 45, 35, 25].
		counts, ok := pt["counts"].([]any)
		require.True(t, ok)
		require.Len(t, counts, 5)
		assert.EqualValues(t, 15, counts[0])
		assert.EqualValues(t, 30, counts[1])
		assert.EqualValues(t, 45, counts[2])
		assert.EqualValues(t, 35, counts[3])
		assert.EqualValues(t, 25, counts[4])

		// Bounds preserved from the uniform inputs.
		bds, ok := pt["bounds"].([]any)
		require.True(t, ok)
		require.Len(t, bds, 4)
	})

	t.Run("BoundsMismatchRaisesTypedError", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		ts := time.Unix(1700000000, 0)
		md := makeMergedHistogramFixture("agg_hist_bs_mismatch", []histTestDP{
			{timestamp: ts, attrs: map[string]string{"host": "a"},
				bounds: []float64{1.0}, counts: []uint64{5, 5},
				count: 10, sum: 5.0, min: 0.1, max: 2.0},
			{timestamp: ts, attrs: map[string]string{"host": "b"},
				bounds: []float64{0.5, 1.5}, counts: []uint64{3, 4, 3},
				count: 10, sum: 5.0, min: 0.1, max: 2.0},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "agg_hist_bs_mismatch")

		_, err := metrics.GetMetricBucketSeries(ctx, s.DB(), id, "merged", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
		assert.ErrorIs(t, err, metrics.ErrHistogramBoundsMismatch)
	})

	t.Run("MultipleTimestampsOrderedAscending", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		bounds := []float64{1.0}
		t1 := time.Unix(1700000000, 0)
		t2 := time.Unix(1700000060, 0)
		t3 := time.Unix(1700000120, 0)
		md := makeMergedHistogramFixture("agg_hist_bs_multi", []histTestDP{
			{timestamp: t3, attrs: map[string]string{"host": "a"}, bounds: bounds, counts: []uint64{1, 1}, count: 2, sum: 1.5, min: 0.1, max: 1.5},
			{timestamp: t1, attrs: map[string]string{"host": "a"}, bounds: bounds, counts: []uint64{2, 0}, count: 2, sum: 0.5, min: 0.1, max: 0.9},
			{timestamp: t2, attrs: map[string]string{"host": "a"}, bounds: bounds, counts: []uint64{1, 2}, count: 3, sum: 4.0, min: 0.5, max: 2.0},
			{timestamp: t2, attrs: map[string]string{"host": "b"}, bounds: bounds, counts: []uint64{0, 1}, count: 1, sum: 1.5, min: 1.5, max: 1.5},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "agg_hist_bs_multi")

		raw, err := metrics.GetMetricBucketSeries(ctx, s.DB(), id, "merged", testQuantileWindowStartTs, testQuantileWindowEndTs, testQuantileWindowPoints)
		require.NoError(t, err)

		var points []map[string]any
		require.NoError(t, json.Unmarshal(raw, &points))
		require.Len(t, points, 3, "one row per distinct timestamp")

		ts0, _ := points[0]["timestamp"].(string)
		ts1, _ := points[1]["timestamp"].(string)
		ts2, _ := points[2]["timestamp"].(string)
		assert.Less(t, ts0, ts1)
		assert.Less(t, ts1, ts2)

		// t2 row merges streams a+b: counts [1,2]+[0,1] = [1,3].
		t2pt := points[1]
		totals, ok := t2pt["totals"].(map[string]any)
		require.True(t, ok)
		assert.EqualValues(t, 4, totals["count"])
		assert.InDelta(t, 5.5, totals["sum"], 1e-9)
	})
}

// searchSummariesAll runs SearchSummaries over a wide window and decodes
// the JSON into a slice of maps. Mirrors searchMetricsAll's shape so the
// tests below stay consistent with the rest of the suite.
func searchSummariesAll(t *testing.T, s *store.Store, ctx context.Context) []map[string]any {
	t.Helper()
	raw, err := metrics.SearchSummaries(ctx, s.DB(), 0, maxNano)
	require.NoError(t, err)
	var out []map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

// findSummary looks up the SearchSummaries row for a metric by name. Used
// by the temporality tests which know exactly which metric they ingested.
func findSummary(t *testing.T, summaries []map[string]any, name string) map[string]any {
	t.Helper()
	for _, s := range summaries {
		if s["name"] == name {
			return s
		}
	}
	t.Fatalf("summary %q not found", name)
	return nil
}

// TestSearchSummaries_CardFields verifies the slim summary projection used
// by drawer cards: stream id, series count, scalar last value, last seen.
func TestSearchSummaries_CardFields(t *testing.T) {
	t.Run("Gauge", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		ts := time.Unix(1700000000, 0)
		md := pmetric.NewMetrics()
		rm := md.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("service.name", "gauge-svc")
		sm := rm.ScopeMetrics().AppendEmpty()
		sm.Scope().SetName("test-scope")
		m := sm.Metrics().AppendEmpty()
		m.SetName("gauge_card_test")
		m.SetDescription("Memory used by the process")
		g := m.SetEmptyGauge()
		dp1 := g.DataPoints().AppendEmpty()
		dp1.SetTimestamp(pcommon.Timestamp(ts.UnixNano()))
		dp1.SetDoubleValue(1.0)
		dp2 := g.DataPoints().AppendEmpty()
		dp2.SetTimestamp(pcommon.Timestamp(ts.Add(time.Second).UnixNano()))
		dp2.SetDoubleValue(2.0)
		dp2.Attributes().PutStr("host", "a")

		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))

		summary := findSummary(t, searchSummariesAll(t, s, ctx), "gauge_card_test")
		assert.NotEmpty(t, summary["id"])
		assert.Equal(t, "Memory used by the process", summary["description"])
		assert.EqualValues(t, 2, summary["seriesCount"])
		assert.EqualValues(t, 2, summary["dataPointCount"])
		assert.InDelta(t, 2.0, summary["lastValue"], 1e-9)
		assert.NotEmpty(t, summary["lastSeen"])
	})

	t.Run("HistogramOmitsLastValue", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		bounds := []float64{1.0, 2.0}
		ts := time.Unix(1700000000, 0)
		md := makeHistogramFixtureT("hist_card_test", pmetric.AggregationTemporalityDelta, []histTestDP{
			{timestamp: ts, attrs: map[string]string{"host": "a"},
				bounds: bounds, counts: []uint64{1, 2, 3},
				count: 6, sum: 7.0, min: 0.5, max: 2.5},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))

		summary := findSummary(t, searchSummariesAll(t, s, ctx), "hist_card_test")
		assert.NotEmpty(t, summary["id"])
		assert.EqualValues(t, 1, summary["seriesCount"])
		assert.EqualValues(t, 1, summary["dataPointCount"])
		assert.Nil(t, summary["lastValue"])
	})
}


// TestGetMetricMergedQuantiles covers the new "single quantile set
// across the whole window" entry point. The function delegates to
// GetMetricQuantileSeries with maxPoints=1, so all the per-attribute merge,
// cross-time merge, temporality, and bounds-mismatch logic is already
// exercised elsewhere; these tests focus on:
//
//   - the unwrap step (extracting the single quantile object from the
//     1-element series) returns the right shape,
//   - the math from a known fixture matches what we'd expect from
//     summing all bucket vectors first then computing the quantile,
//   - error paths (no datapoints / unsupported type / bounds mismatch /
//     unspecified temporality) propagate with the same sentinels as the
//     series query, just wrapped under the new function name.
func TestGetMetricMergedQuantiles(t *testing.T) {
	// Window-wide constants. Use the same generous window the series tests
	// use so we don't accidentally exclude any test-fixture timestamps.
	const startTs = testQuantileWindowStartTs
	const endTs = testQuantileWindowEndTs

	t.Run("HistogramSingleStreamSingleTimestamp", func(t *testing.T) {
		// Sanity: one dp, one bucket vector. The merged quantile
		// should match what GetDatapointQuantiles would compute on that dp
		// directly. Anchors against the same p50=4/3 calc the
		// per-datapoint test uses.
		s, ctx, teardown := setupStore(t)
		defer teardown()

		ts := time.Unix(1700000000, 0)
		md := makeMergedHistogramFixture("agg_q_single", []histTestDP{
			{timestamp: ts, attrs: map[string]string{"host": "a"},
				bounds: []float64{0.5, 1.0, 1.5, 2.0},
				counts: []uint64{10, 20, 30, 25, 15},
				count:  100, sum: 150.0, min: 0.1, max: 2.5},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "agg_q_single")

		raw, err := metrics.GetMetricMergedQuantiles(ctx, s.DB(), id, []float64{0.5, 0.95}, startTs, endTs)
		require.NoError(t, err)

		var out map[string]any
		require.NoError(t, json.Unmarshal(raw, &out))
		require.Contains(t, out, "0.5")
		require.Contains(t, out, "0.95")

		// Same buckets as TestGetDatapointQuantiles -> p50=4/3, p95=2.0.
		p50, ok := out["0.5"].(float64)
		require.True(t, ok, "p50 should be a number, got %T", out["0.5"])
		assert.InDelta(t, 4.0/3.0, p50, 1e-9)
		p95, ok := out["0.95"].(float64)
		require.True(t, ok)
		assert.InDelta(t, 2.0, p95, 1e-9)
	})

	t.Run("HistogramMultiStreamMergesAcrossWindow", func(t *testing.T) {
		// Two streams, two timestamps. Aggregating across the whole
		// window should produce the same answer as summing all four
		// vectors element-wise, then computing the quantile.
		//
		// All four use bounds=[1.0]. counts:
		//   t1, host=a: [4, 0]
		//   t1, host=b: [0, 4]
		//   t2, host=a: [2, 2]
		//   t2, host=b: [0, 4]
		// Element-wise sum: [6, 10] (total=16, p50 target=8). Cumulative
		// 6, 16. First acc>=8 is bucket 2 (clamped to (1.0, 1.0]) which
		// interpolates to 1.0 exactly per hist_buckets clamping rules.
		s, ctx, teardown := setupStore(t)
		defer teardown()

		bounds := []float64{1.0}
		t1 := time.Unix(1700000000, 0)
		t2 := time.Unix(1700000060, 0)
		md := makeMergedHistogramFixture("agg_q_multi", []histTestDP{
			{timestamp: t1, attrs: map[string]string{"host": "a"}, bounds: bounds, counts: []uint64{4, 0}, count: 4, sum: 1.0, min: 0.1, max: 0.9},
			{timestamp: t1, attrs: map[string]string{"host": "b"}, bounds: bounds, counts: []uint64{0, 4}, count: 4, sum: 6.0, min: 1.5, max: 1.5},
			{timestamp: t2, attrs: map[string]string{"host": "a"}, bounds: bounds, counts: []uint64{2, 2}, count: 4, sum: 4.0, min: 0.5, max: 2.0},
			{timestamp: t2, attrs: map[string]string{"host": "b"}, bounds: bounds, counts: []uint64{0, 4}, count: 4, sum: 6.0, min: 1.5, max: 1.5},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "agg_q_multi")

		raw, err := metrics.GetMetricMergedQuantiles(ctx, s.DB(), id, []float64{0.5}, startTs, endTs)
		require.NoError(t, err)

		var out map[string]any
		require.NoError(t, json.Unmarshal(raw, &out))
		p50, ok := out["0.5"].(float64)
		require.True(t, ok)
		assert.InDelta(t, 1.0, p50, 1e-9)
	})

	t.Run("ExpHistogramSingleStream", func(t *testing.T) {
		// Don't pin exact values; exp_hist_quantile correctness lives in
		// schema_test.go. Just confirm the shape and finite numeric
		// outputs come back so we know the unwrap step works end-to-end.
		s, ctx, teardown := setupStore(t)
		defer teardown()

		ts := time.Unix(1700000000, 0)
		md := makeExpHistogramFixtureT("agg_q_exp", pmetric.AggregationTemporalityDelta, []expHistTestDP{
			{timestamp: ts, attrs: map[string]string{"host": "a"},
				scale: 0, zeroCount: 0, posOffset: 0,
				posCounts: []uint64{10, 20, 30, 40},
				count:     100, sum: 50.0, min: 0.1, max: 16.0},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "agg_q_exp")

		raw, err := metrics.GetMetricMergedQuantiles(ctx, s.DB(), id, []float64{0.5, 0.99}, startTs, endTs)
		require.NoError(t, err)

		var out map[string]any
		require.NoError(t, json.Unmarshal(raw, &out))
		_, ok := out["0.5"].(float64)
		assert.True(t, ok, "p50 should be a finite number")
		_, ok = out["0.99"].(float64)
		assert.True(t, ok, "p99 should be a finite number")
	})

	t.Run("EmptyQuantilesShortCircuits", func(t *testing.T) {
		// No quantiles requested -> empty object, no DB hit. We don't
		// need a real metric; the function should return before touching
		// the store.
		s, ctx, teardown := setupStore(t)
		defer teardown()

		raw, err := metrics.GetMetricMergedQuantiles(ctx, s.DB(), "00000000-0000-0000-0000-000000000000", nil, startTs, endTs)
		require.NoError(t, err)
		assert.JSONEq(t, "{}", string(raw))
	})

	t.Run("MetricNotFound", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		_, err := metrics.GetMetricMergedQuantiles(ctx, s.DB(), "00000000-0000-0000-0000-000000000000", []float64{0.5}, startTs, endTs)
		assert.ErrorIs(t, err, metrics.ErrMetricIDNotFound)
	})

	t.Run("UnsupportedType_Gauge", func(t *testing.T) {
		// Reuses the standard test-metrics fixture so we don't have to
		// hand-build a gauge here.
		s, ctx, teardown := setupStore(t)
		defer teardown()

		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, createTestMetricsPdata())
		}))
		id := findMetricID(t, s, ctx, "gauge_metric")
		_, err := metrics.GetMetricMergedQuantiles(ctx, s.DB(), id, []float64{0.5}, startTs, endTs)
		assert.ErrorIs(t, err, metrics.ErrQuantilesNotSupportedForType)
	})

	t.Run("BoundsMismatchPropagates", func(t *testing.T) {
		// Two streams, same timestamp, different bounds. Mathematically
		// not mergeable; should bubble ErrHistogramBoundsMismatch from
		// the underlying series query. We're verifying the wrap
		// preserves the sentinel.
		s, ctx, teardown := setupStore(t)
		defer teardown()

		ts := time.Unix(1700000000, 0)
		md := makeMergedHistogramFixture("agg_q_mismatch", []histTestDP{
			{timestamp: ts, attrs: map[string]string{"host": "a"},
				bounds: []float64{1.0}, counts: []uint64{5, 5},
				count: 10, sum: 5.0, min: 0.1, max: 2.0},
			{timestamp: ts, attrs: map[string]string{"host": "b"},
				bounds: []float64{0.5, 1.5}, counts: []uint64{3, 4, 3},
				count: 10, sum: 5.0, min: 0.1, max: 2.0},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "agg_q_mismatch")

		_, err := metrics.GetMetricMergedQuantiles(ctx, s.DB(), id, []float64{0.5}, startTs, endTs)
		assert.ErrorIs(t, err, metrics.ErrHistogramBoundsMismatch)
	})

	t.Run("UnspecifiedTemporalityRefuses", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		ts := time.Unix(1700000000, 0)
		md := makeExpHistogramFixtureT("agg_q_unspec", pmetric.AggregationTemporalityUnspecified, []expHistTestDP{
			{timestamp: ts, attrs: map[string]string{"host": "a"},
				scale: 0, zeroCount: 0, posOffset: 0,
				posCounts: []uint64{1, 2, 3}, count: 6, sum: 6.0},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))
		id := findMetricID(t, s, ctx, "agg_q_unspec")

		_, err := metrics.GetMetricMergedQuantiles(ctx, s.DB(), id, []float64{0.5}, startTs, endTs)
		assert.ErrorIs(t, err, metrics.ErrUnspecifiedTemporality)
	})

	t.Run("InvalidTimeRange", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()
		_, err := metrics.GetMetricMergedQuantiles(ctx, s.DB(), "00000000-0000-0000-0000-000000000000", []float64{0.5}, 100, 100)
		assert.ErrorIs(t, err, metrics.ErrInvalidTimeRange)
	})
}
