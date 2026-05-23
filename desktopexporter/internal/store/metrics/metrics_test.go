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

func searchSummariesAll(t *testing.T, s *store.Store, ctx context.Context) []map[string]any {
	t.Helper()
	raw, err := metrics.SearchSummaries(ctx, s.DB(), 0, maxNano)
	require.NoError(t, err)
	var out []map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

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
