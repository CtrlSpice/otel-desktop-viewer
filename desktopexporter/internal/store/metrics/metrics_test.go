package metrics_test

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
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

const maxNano = 1<<63 - 1

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
		datapoints, _ := gauge["datapoints"].([]any)
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
		datapoints, _ := m["datapoints"].([]any)
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
		datapoints, _ := sum["datapoints"].([]any)
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
		datapoints, _ := hist["datapoints"].([]any)
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
		datapoints, _ := exp["datapoints"].([]any)
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
		datapoints, _ := gauge["datapoints"].([]any)
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

// TestDeleteMetricByID verifies that a single metric can be deleted by its ID, including child rows.
func TestDeleteMetricByID(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	err := s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, createTestMetricsPdata())
	})
	assert.NoError(t, err)

	metricList := searchMetricsAll(t, s, ctx)
	assert.Len(t, metricList, 5)

	targetID, ok := metricList[0]["id"].(string)
	assert.True(t, ok, "metric ID should be a string")
	assert.NotEmpty(t, targetID)

	dpBefore := countRows(t, s.DB(), ctx, "select count(*) from datapoints where metric_id = ?", targetID)
	attrsBefore := countRows(t, s.DB(), ctx, "select count(*) from attributes where metric_id = ?", targetID)
	assert.Greater(t, dpBefore+attrsBefore, 0, "target metric should have child rows")

	err = metrics.DeleteMetricByID(ctx, s.DB(), targetID)
	assert.NoError(t, err)

	metricList = searchMetricsAll(t, s, ctx)
	assert.Len(t, metricList, 4)
	for _, m := range metricList {
		assert.NotEqual(t, targetID, m["id"])
	}

	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from datapoints where metric_id = ?", targetID))
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from attributes where metric_id = ?", targetID))
}

// TestDeleteMetricsByIDs verifies that multiple metrics can be deleted by their IDs, including child rows.
func TestDeleteMetricsByIDs(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	err := s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, createTestMetricsPdata())
	})
	assert.NoError(t, err)

	metricList := searchMetricsAll(t, s, ctx)
	assert.Len(t, metricList, 5)

	idsToDelete := []any{metricList[0]["id"], metricList[1]["id"]}
	dpBefore := countRows(t, s.DB(), ctx, "select count(*) from datapoints where metric_id in (?, ?)", idsToDelete...)
	assert.Greater(t, dpBefore, 0, "deleted metrics should have datapoints")

	err = metrics.DeleteMetricsByIDs(ctx, s.DB(), idsToDelete)
	assert.NoError(t, err)

	metricList = searchMetricsAll(t, s, ctx)
	assert.Len(t, metricList, 3)

	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from datapoints where metric_id in (?, ?)", idsToDelete...))
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from attributes where metric_id in (?, ?)", idsToDelete...))
}

// TestDeleteMetricsByIDs_Empty verifies that deleting with an empty list is a no-op.
func TestDeleteMetricsByIDs_Empty(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	err := metrics.DeleteMetricsByIDs(ctx, s.DB(), []any{})
	assert.NoError(t, err)
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
	assert.Greater(t, countRows(t, s.DB(), ctx, "select count(*) from attributes where metric_id is not null"), 0)

	err = metrics.Clear(ctx, s.DB())
	assert.NoError(t, err)

	metricList = searchMetricsAll(t, s, ctx)
	assert.Empty(t, metricList)
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from datapoints"))
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from exemplars"))
	assert.Equal(t, 0, countRows(t, s.DB(), ctx, "select count(*) from attributes where metric_id is not null"))
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
