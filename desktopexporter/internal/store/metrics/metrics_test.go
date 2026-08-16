package metrics_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
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

func setupStore(t *testing.T) (*store.Store, context.Context, func()) {
	t.Helper()
	ctx := context.Background()
	s, err := store.NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	return s, ctx, func() { s.Close() }
}

func countRows(t *testing.T, s *store.Store, ctx context.Context, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRowContext(ctx, query, args...).Scan(&n)
	}))
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

// searchMetricsAll returns metrics.SearchSummaries with wide time range and nil query; parses JSON to slice of maps.
func searchMetricsAll(t *testing.T, s *store.Store, ctx context.Context) []map[string]any {
	t.Helper()
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.SearchSummaries(ctx, db, 0, maxNano, nil)
	})
	assert.NoError(t, err)
	var out []map[string]any
	assert.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func searchSummariesAll(t *testing.T, s *store.Store, ctx context.Context) []map[string]any {
	t.Helper()
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.SearchSummaries(ctx, db, 0, maxNano, nil)
	})
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

// getMetricFullByName resolves a stream id via SearchSummaries and fetches
// full MetricData via GetMetric (timeseries, datapoints, resource, scope).
func getMetricFullByName(t *testing.T, s *store.Store, ctx context.Context, name string) map[string]any {
	t.Helper()
	id := findMetricID(t, s, ctx, name)
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, id, 0, maxNano, 0, nil, nil, 0, false, 0, 0, nil, nil, 0)
	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

// TestMetricSuite runs tests on ingested metrics using SearchMetrics (DB-generated JSON).
func TestMetricSuite(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	err := s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, createTestMetricsPdata(), s.FlushedIDs())
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
		gauge := getMetricFullByName(t, s, ctx, "gauge_metric")
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
		m := getMetricFullByName(t, s, ctx, "gauge_int_metric")
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
		sum := getMetricFullByName(t, s, ctx, "sum_metric")
		requireMetric(t, sum, "sum_metric")
		assert.Equal(t, "Total requests processed", sum["description"])
		datapoints := metricDatapoints(sum)
		assert.NotEmpty(t, datapoints)
	})

	t.Run("HistogramMetric", func(t *testing.T) {
		hist := getMetricFullByName(t, s, ctx, "histogram_metric")
		requireMetric(t, hist, "histogram_metric")
		datapoints := metricDatapoints(hist)
		assert.NotEmpty(t, datapoints)
		dp, _ := datapoints[0].(map[string]any)
		assert.NotNil(t, dp)
		assert.Equal(t, float64(100), dp["count"])
		assert.Equal(t, 25.5, dp["sum"])
	})

	t.Run("ExponentialHistogramMetric", func(t *testing.T) {
		exp := getMetricFullByName(t, s, ctx, "exponential_histogram_metric")
		requireMetric(t, exp, "exponential_histogram_metric")
		datapoints := metricDatapoints(exp)
		assert.NotEmpty(t, datapoints)
		dp, _ := datapoints[0].(map[string]any)
		assert.NotNil(t, dp)
		assert.Equal(t, float64(50), dp["count"])
		assert.Equal(t, float64(2), dp["scale"])
	})

	t.Run("MetricResourceAndScope", func(t *testing.T) {
		for _, name := range []string{
			"gauge_metric", "gauge_int_metric", "sum_metric",
			"histogram_metric", "exponential_histogram_metric",
		} {
			m := getMetricFullByName(t, s, ctx, name)
			resource, _ := m["resource"].(map[string]any)
			assert.NotNil(t, resource, "metric %s resource", name)
			scope, _ := m["scope"].(map[string]any)
			assert.NotNil(t, scope, "metric %s scope", name)
			assert.Equal(t, "test-scope", scope["name"], "metric %s scope name", name)
			assert.Equal(t, "v1.0.0", scope["version"], "metric %s scope version", name)
		}
	})

	t.Run("Exemplars", func(t *testing.T) {
		assert.Greater(t, countRows(t, s, ctx, "select count(*) from exemplars"), 0, "exemplars should be ingested")
		gauge := getMetricFullByName(t, s, ctx, "gauge_metric")
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
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Len(t, metrics, 5, "should find all metrics for service name test-service")
		for i, m := range metrics {
			assert.Equal(t, "test-service", m["serviceName"], "metric %d serviceName", i)
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
				"field":         map[string]any{"name": "name", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "gauge_metric",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, startTime, endTime, query)
		})
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
				"field":         map[string]any{"name": "description", "searchScope": "field"},
				"fieldOperator": "CONTAINS",
				"value":         "memory",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, startTime, endTime, query)
		})
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
				"field":         map[string]any{"name": "unit", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "bytes",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, startTime, endTime, query)
		})
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
				"field":         map[string]any{"name": "scope.name", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "test-scope",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Len(t, metrics, 5)
	})

	t.Run("Field_scopeName", func(t *testing.T) {
		query := map[string]any{
			"id":   "f4b",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "scopeName", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "test-scope",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, startTime, endTime, query)
		})
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
				"field":         map[string]any{"name": "scope.version", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "v1.0.0",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, startTime, endTime, query)
		})
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Len(t, metrics, 5)
	})

	t.Run("Field_scopeVersion", func(t *testing.T) {
		query := map[string]any{
			"id":   "f5b",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "scopeVersion", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "v1.0.0",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, startTime, endTime, query)
		})
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
				"field":         map[string]any{"name": "resourceDroppedAttributesCount", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "0",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, startTime, endTime, query)
		})
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
				"field":         map[string]any{"searchScope": "global"},
				"fieldOperator": "CONTAINS",
				"value":         "memory",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, startTime, endTime, query)
		})
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
				"field":         map[string]any{"searchScope": "global"},
				"fieldOperator": "CONTAINS",
				"value":         "test-service",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, startTime, endTime, query)
		})
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
				"field":         map[string]any{"searchScope": "global"},
				"fieldOperator": "CONTAINS",
				"value":         "nonexistent-metric-xyz",
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, startTime, endTime, query)
		})
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
// deleteByIdentity resolves an identity tuple to a stream UUID and deletes that
// stream. Both steps run in one write-lock window so the resolved ID cannot be
// pruned out from under the delete.
func deleteByIdentity(t *testing.T, ctx context.Context, s *store.Store, name, unit, metricType, aggTemporality, isMonotonic, scopeName, scopeVersion, serviceName string) error {
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
	return s.WithDBWrite(func(db *sql.DB) error {
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
	})
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
			return metrics.Ingest(ctx, conn, createTestMetricsPdata(), s.FlushedIDs())
		})
		assert.NoError(t, err)

		before := searchMetricsAll(t, s, ctx)
		assert.Len(t, before, 5)

		// Gauges have no temporality / monotonic; pass empty strings.
		err = deleteByIdentity(t, ctx, s,
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

		assert.Equal(t, 0, countRows(t, s, ctx,
			`select count(*) from metric_streams where name = ?`, "gauge_metric"))
	})

	t.Run("collapses multiple ingestions of the same logical metric", func(t *testing.T) {
		s, ctx, teardown := setupStore(t)
		defer teardown()

		// Three independent batches => three metric_ingests rows for the
		// same logical Gauge, all sharing one metric_streams row.
		for i := 0; i < 3; i++ {
			err := s.WithConn(func(conn driver.Conn) error {
				return metrics.Ingest(ctx, conn, createTestMetricsPdata(), s.FlushedIDs())
			})
			assert.NoError(t, err)
		}

		// SearchSummaries collapses by identity so we still see 5 rows.
		assert.Len(t, searchSummariesAll(t, s, ctx), 5)
		// One stream per logical metric (5), 3 ingests per stream (15).
		assert.Equal(t, 5, countRows(t, s, ctx, `select count(*) from metric_streams`))
		assert.Equal(t, 15, countRows(t, s, ctx, `select count(*) from metric_ingests`))

		err := deleteByIdentity(t, ctx, s,
			"gauge_metric", "bytes", "Gauge",
			"", "",
			"test-scope", "v1.0.0", "test-service",
		)
		assert.NoError(t, err)

		assert.Len(t, searchSummariesAll(t, s, ctx), 4)
		// One stream + its three ingests should be gone.
		assert.Equal(t, 4, countRows(t, s, ctx, `select count(*) from metric_streams`))
		assert.Equal(t, 12, countRows(t, s, ctx, `select count(*) from metric_ingests`))
		assert.Equal(t, 0, countRows(t, s, ctx,
			`select count(*) from metric_streams where name = ?`, "gauge_metric"))
		assert.Equal(t, 0, countRows(t, s, ctx,
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
			return metrics.Ingest(ctx, conn, md, s.FlushedIDs())
		})
		assert.NoError(t, err)
		assert.Len(t, searchMetricsAll(t, s, ctx), 2)

		// Delete the bytes one — count should survive.
		err = deleteByIdentity(t, ctx, s,
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
			return metrics.Ingest(ctx, conn, md, s.FlushedIDs())
		})
		assert.NoError(t, err)
		assert.Len(t, searchSummariesAll(t, s, ctx), 2)

		err = deleteByIdentity(t, ctx, s,
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
			return metrics.Ingest(ctx, conn, md, s.FlushedIDs())
		})
		assert.NoError(t, err)
		assert.Len(t, searchSummariesAll(t, s, ctx), 2)

		err = deleteByIdentity(t, ctx, s,
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
			return metrics.Ingest(ctx, conn, createTestMetricsPdata(), s.FlushedIDs())
		})
		assert.NoError(t, err)
		assert.Len(t, searchMetricsAll(t, s, ctx), 5)

		err = deleteByIdentity(t, ctx, s,
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
			return metrics.Ingest(ctx, conn, createTestMetricsPdata(), s.FlushedIDs())
		})
		assert.NoError(t, err)

		// Histogram has datapoints with exemplars — pick it.
		dpBefore := countRows(t, s, ctx,
			`select count(*) from datapoints where stream_id in (select id from metric_streams where name = ?)`,
			"histogram_metric")
		exBefore := countRows(t, s, ctx,
			`select count(*) from exemplars where datapoint_id in (select id from datapoints where stream_id in (select id from metric_streams where name = ?))`,
			"histogram_metric")
		// The old query counted attribute rows owned by this stream's ingest
		// batches -- i.e. its resource and scope attributes. Those are now ids
		// in the referenced resources / scopes arrays, so the equivalent
		// question is how many distinct ids the stream's ingests reach.
		//
		// Deliberately not the datapoint labels: the histogram fixture's
		// datapoints carry none, so counting those would assert nothing.
		attrBefore := countRows(t, s, ctx,
			`select count(distinct t.aid)
			 from metric_ingests mi
			 join resources r on r.id = mi.resource_id
			 join scopes sc on sc.id = mi.scope_id,
			 unnest(r.attribute_ids || sc.attribute_ids) as t(aid)
			 where mi.stream_id in (select id from metric_streams where name = ?)`,
			"histogram_metric")
		assert.Greater(t, dpBefore, 0)
		assert.Greater(t, exBefore, 0)
		assert.Greater(t, attrBefore, 0)

		// The dictionary rows those ids point at, before the delete. They must
		// survive the cascade -- other streams may still reference them -- and
		// only go when the sweep proves them unreferenced.
		dictBefore := countRows(t, s, ctx, `select count(*) from attributes`)
		assert.Greater(t, dictBefore, 0)

		err = deleteByIdentity(t, ctx, s,
			"histogram_metric", "seconds", "Histogram",
			"Delta", "",
			"test-scope", "v1.0.0", "test-service",
		)
		assert.NoError(t, err)

		assert.Equal(t, 0, countRows(t, s, ctx,
			`select count(*) from metric_streams where name = ?`, "histogram_metric"))
		assert.Equal(t, 0, countRows(t, s, ctx,
			`select count(*) from datapoints where stream_id in (select id from metric_streams where name = ?)`,
			"histogram_metric"))
		assert.Equal(t, 0, countRows(t, s, ctx,
			`select count(*) from exemplars e where exists (
				select 1 from datapoints d
				where d.id = e.datapoint_id
				  and d.stream_id in (select id from metric_streams where name = ?)
			)`, "histogram_metric"))
		// The cascade deliberately does NOT touch the dictionary: attribute,
		// resource and scope rows are shared across every signal, so "is this
		// one still in use" is not a question a stream-scoped delete can
		// answer. It stays whole here...
		assert.Equal(t, dictBefore, countRows(t, s, ctx, `select count(*) from attributes`),
			"a stream delete must not remove shared dictionary rows")

		// ...and the sweep is what collects whatever the delete orphaned.
		require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
			return ingest.SweepOrphans(ctx, db, s.FlushedIDs())
		}))
		// The other four fixture metrics still reference the same resource and
		// scope, so the sweep must NOT take those rows -- shared content
		// survives as long as one owner remains. What it does take is anything
		// only the deleted stream reached.
		assert.Equal(t, 0, countRows(t, s, ctx,
			`select count(*) from attributes a
			 where not exists (select 1 from resources r, unnest(r.attribute_ids) t(aid) where t.aid = a.id)
			   and not exists (select 1 from scopes sc, unnest(sc.attribute_ids) t(aid) where t.aid = a.id)
			   and not exists (select 1 from datapoints d, unnest(d.attribute_ids) t(aid) where t.aid = a.id)
			   and not exists (select 1 from exemplars e, unnest(e.attribute_ids) t(aid) where t.aid = a.id)`),
			"the sweep must leave no unreferenced dictionary row behind")
		assert.Greater(t, countRows(t, s, ctx, `select count(*) from resources`), 0,
			"the resource is still referenced by the four surviving streams")
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
			return metrics.Ingest(ctx, conn, createTestMetricsPdata(), s.FlushedIDs())
		})
		require.NoError(t, err)
	}

	// createTestMetricsPdata produces five distinct logical metrics.
	// Across N batches we should still see exactly five stream rows.
	assert.Equal(t, 5, countRows(t, s, ctx,
		`select count(*) from metric_streams`),
		"distinct logical metrics should not multiply across batches")
	assert.Equal(t, 5*batches, countRows(t, s, ctx,
		`select count(*) from metric_ingests`),
		"every batch should add one ingest per metric")

	// Stream ids should be stable across batches: every datapoint's
	// stream_id must match a metric_streams row, and every per-batch
	// metric_ingests row pointing at the same logical metric must
	// resolve to the same stream_id.
	gaugeStreamRows := countRows(t, s, ctx,
		`select count(distinct stream_id) from metric_ingests where stream_id in (
			select id from metric_streams where name = 'gauge_metric'
		)`)
	assert.Equal(t, 1, gaugeStreamRows,
		"all gauge_metric ingests must share one stream_id")

	// Sanity: cross-table referential integrity holds.
	orphanDatapoints := countRows(t, s, ctx,
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
				return metrics.Ingest(ctx, conn, mk(t, func(pmetric.Metric, pcommon.InstrumentationScope, pcommon.Resource) {}), s.FlushedIDs())
			})
			require.NoError(t, err)

			err = s.WithConn(func(conn driver.Conn) error {
				return metrics.Ingest(ctx, conn, mk(t, tc.mutate), s.FlushedIDs())
			})
			require.NoError(t, err)

			assert.Equal(t, 2, countRows(t, s, ctx,
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
		return metrics.Ingest(ctx, conn, createTestMetricsPdata(), s.FlushedIDs())
	})
	require.NoError(t, err)

	// All five fixture metrics share service.name = test-service.
	//
	// The source of truth moved: the resource attribute is no longer a row
	// owned by the ingest batch, it is an id in the referenced resources row's
	// array. attr_value resolves it, which is the same macro the search mappers
	// use -- so this also pins that the macro agrees with what ingest wrote.
	mismatches := countRows(t, s, ctx, `
		select count(*) from metric_streams s
		join metric_ingests mi on mi.stream_id = s.id
		join resources r on r.id = mi.resource_id
		where s.service_name <> coalesce(attr_value(r.attribute_ids, 'service.name'), '')
	`)
	assert.Equal(t, 0, mismatches,
		"metric_streams.service_name must equal the source resource attribute")
}

// TestEmptyMetrics verifies empty metric list and empty store.
func TestEmptyMetrics(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	err := s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, pmetric.NewMetrics(), s.FlushedIDs())
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
		return metrics.Ingest(ctx, conn, createTestMetricsPdata(), s.FlushedIDs())
	})
	assert.NoError(t, err)

	metricList := searchMetricsAll(t, s, ctx)
	assert.Len(t, metricList, 5)
	assert.Greater(t, countRows(t, s, ctx, "select count(*) from datapoints"), 0)
	assert.Greater(t, countRows(t, s, ctx, "select count(*) from attributes"), 0)

	err = s.WithDBWrite(func(db *sql.DB) error {
		return metrics.Clear(ctx, db)
	})
	assert.NoError(t, err)

	metricList = searchMetricsAll(t, s, ctx)
	assert.Empty(t, metricList)
	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from metric_streams"))
	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from metric_ingests"))
	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from datapoints"))
	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from exemplars"))

	// Clear leaves the dictionary alone -- it cannot know whether a traces or
	// logs row still references any of it. With no other signal ingested here
	// everything it left behind is orphaned, and the sweep takes all of it.
	assert.Greater(t, countRows(t, s, ctx, "select count(*) from attributes"), 0,
		"Clear must not delete shared dictionary rows itself")
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		return ingest.SweepOrphans(ctx, db, s.FlushedIDs())
	}))
	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from attributes"))
	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from resources"))
	assert.Equal(t, 0, countRows(t, s, ctx, "select count(*) from scopes"))
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
		return metrics.Ingest(ctx, conn, md, s.FlushedIDs())
	}))

	byName := make(map[string]map[string]any)
	for _, summary := range searchMetricsAll(t, s, ctx) {
		name, _ := summary["name"].(string)
		byName[name] = getMetricFullByName(t, s, ctx, name)
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
type sumTestDP struct {
	timestamp time.Time
	value     float64
	attrs     map[string]string
}

// makeSumFixtureT builds a single-series Sum, which is what the scalar view
// grid aggregates over.
func makeSumFixtureT(name string, temporality pmetric.AggregationTemporality, dps []sumTestDP) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "test-sum")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("test-scope")
	m := sm.Metrics().AppendEmpty()
	m.SetName(name)
	sum := m.SetEmptySum()
	sum.SetIsMonotonic(true)
	sum.SetAggregationTemporality(temporality)
	for _, d := range dps {
		dp := sum.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(d.timestamp.UnixNano()))
		dp.SetDoubleValue(d.value)
		for k, v := range d.attrs {
			dp.Attributes().PutStr(k, v)
		}
	}
	return md
}

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

// TestIngestMetrics_LargeBatchStaysConsistent ingests more metrics in one call
// than the flush interval and asserts they all landed with their attributes.
// Like the spans and logs versions it does not claim to test the flush itself,
// which is unobservable by design. Sized from the constant so it cannot stop
// being a large batch when the constant moves.
func TestIngestMetrics_LargeBatchStaysConsistent(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	const batchSize = metrics.FlushInterval + 1
	err := s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, createTestMetricsPdataN(batchSize), s.FlushedIDs())
	})
	assert.NoError(t, err)

	metrics := searchMetricsAll(t, s, ctx)
	assert.Len(t, metrics, batchSize)

	for _, idx := range []int{0, 99, 100} {
		name := "flush_metric_" + fmt.Sprintf("%d", idx)
		m := getMetricFullByName(t, s, ctx, name)
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

func TestIngest_CanceledContext(t *testing.T) {
	s, _, teardown := setupStore(t)
	defer teardown()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, createTestMetricsPdataN(1), s.FlushedIDs())
	})
	require.ErrorIs(t, err, context.Canceled)
}

func TestIngest_CanceledDuringIngest(t *testing.T) {
	s, _, teardown := setupStore(t)
	defer teardown()

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, createTestMetricsPdataN(100), s.FlushedIDs())
		})
	}()
	cancel()

	err := <-errCh
	require.ErrorIs(t, err, context.Canceled)
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
			return metrics.Ingest(ctx, conn, md, s.FlushedIDs())
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
			return metrics.Ingest(ctx, conn, md, s.FlushedIDs())
		}))

		summary := findSummary(t, searchSummariesAll(t, s, ctx), "hist_card_test")
		assert.NotEmpty(t, summary["id"])
		assert.EqualValues(t, 1, summary["seriesCount"])
		assert.EqualValues(t, 1, summary["dataPointCount"])
		assert.Nil(t, summary["lastValue"])
	})
}

// Datapoint and exemplar labels are searchable.
//
// They were not before the attribute dictionary: metric search runs per
// metric_ingests row, and reaching datapoint labels from there meant a
// correlated walk of the largest table in the store. Resolving the dictionary
// first makes it one array-overlap scan (39.2ms -> 7.7ms on the reference
// capture), so the coverage gap is now just a gap.
func TestMetricSearch_DatapointAndExemplarLabels(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, createTestMetricsPdata(), s.FlushedIDs())
	}))

	search := func(t *testing.T, scope, name, op, value string) []map[string]any {
		t.Helper()
		query := map[string]any{
			"type": "condition",
			"query": map[string]any{
				"field": map[string]any{
					"name":           name,
					"searchScope":    "attribute",
					"attributeScope": scope,
				},
				"fieldOperator": op,
				"value":         value,
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, 0, time.Now().UnixNano()+int64(time.Hour), query)
		})
		require.NoError(t, err)
		var out []map[string]any
		require.NoError(t, json.Unmarshal(raw, &out))
		return out
	}

	t.Run("datapoint label matches its own metric only", func(t *testing.T) {
		// memory.type=heap is on the gauge metric's datapoint; the other four
		// fixture metrics must not match, or the predicate is not actually
		// filtering.
		got := search(t, "datapoint", "memory.type", "=", "heap")
		require.Len(t, got, 1)
		assert.Equal(t, "gauge_metric", got[0]["name"])
	})

	t.Run("datapoint label with a non-matching value finds nothing", func(t *testing.T) {
		assert.Empty(t, search(t, "datapoint", "memory.type", "=", "stack"))
	})

	// Mutation-driven: without these, making the key predicate always-true
	// (`a.key = ? or true`) left every other subtest passing, because each
	// fixture label value happens to be unique. These pair a real key with
	// another key's value, so a predicate that ignores the key over-matches.
	t.Run("the key is part of the match, not just the value", func(t *testing.T) {
		// "int" is a real datapoint label value -- but under key "type", not
		// "memory.type". Ignoring the key would return the int metric.
		assert.Empty(t, search(t, "datapoint", "memory.type", "=", "int"),
			"memory.type is heap; int belongs to a different key")

		// A key that exists nowhere, paired with a value that does.
		assert.Empty(t, search(t, "datapoint", "no.such.key", "=", "heap"),
			"a non-existent key must match nothing regardless of the value")

		// Same for exemplars.
		assert.Empty(t, search(t, "exemplar", "no.such.key", "=", "gauge"))
	})

	// Scope is part of the dictionary id, so a datapoint search must not reach
	// an exemplar label and vice versa -- even though both live in the same
	// attributes table.
	t.Run("scopes do not leak into each other", func(t *testing.T) {
		assert.Empty(t, search(t, "datapoint", "exemplar.source", "=", "gauge"),
			"exemplar.source is not a datapoint label")
		assert.Empty(t, search(t, "exemplar", "memory.type", "=", "heap"),
			"memory.type is not an exemplar label")
	})

	t.Run("exemplar label", func(t *testing.T) {
		got := search(t, "exemplar", "exemplar.source", "=", "histogram")
		require.Len(t, got, 1)
		assert.Equal(t, "histogram_metric", got[0]["name"])
	})

	t.Run("exemplar labels distinguish metrics", func(t *testing.T) {
		// Every fixture metric carries exemplar.source, with a different value
		// each, so this pins that the value is compared and not merely the key.
		for _, tc := range []struct{ value, metric string }{
			{"gauge", "gauge_metric"},
			{"sum", "sum_metric"},
			{"exponential_histogram", "exponential_histogram_metric"},
		} {
			got := search(t, "exemplar", "exemplar.source", "=", tc.value)
			require.Len(t, got, 1, "value %q", tc.value)
			assert.Equal(t, tc.metric, got[0]["name"])
		}
	})

	t.Run("LIKE on a datapoint label", func(t *testing.T) {
		got := search(t, "datapoint", "memory.type", "CONTAINS", "hea")
		require.Len(t, got, 1)
		assert.Equal(t, "gauge_metric", got[0]["name"])
	})

	t.Run("global free-text reaches datapoint and exemplar labels", func(t *testing.T) {
		global := func(term string) []map[string]any {
			query := map[string]any{
				"type": "condition",
				"query": map[string]any{
					"field":         map[string]any{"searchScope": "global"},
					"fieldOperator": "CONTAINS",
					"value":         term,
				},
			}
			raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.SearchSummaries(ctx, db, 0, time.Now().UnixNano()+int64(time.Hour), query)
			})
			require.NoError(t, err)
			var out []map[string]any
			require.NoError(t, json.Unmarshal(raw, &out))
			return out
		}
		// "heap" appears only as a datapoint label value.
		got := global("heap")
		require.Len(t, got, 1, "global search must reach datapoint labels")
		assert.Equal(t, "gauge_metric", got[0]["name"])

		// "exponential_histogram" appears as an exemplar label value.
		assert.NotEmpty(t, global("exponential_histogram"),
			"global search must reach exemplar labels")
	})
}

// Two replicas of one service, emitting the same instrument with the same
// labels, must be two series -- not one interleaved line.
//
// metric_streams identifies a stream by service_name rather than by resource,
// deliberately, so a counter survives a pod restart. Nothing downstream then
// re-introduced the resource, so replicas collapsed together: SDKs put
// host.name and k8s.pod.name on the *resource*, which made this the common
// shape in any replicated deployment rather than an exotic one. Prometheus
// would show two series here; we showed one, silently averaging two machines.
//
// What tells the replicas apart is service.instance.id -- the only field the
// OTel spec actually commits to as resource identity (see ingest.ResourceID).
// host.name differing too is realistic flavor, not what does the work here;
// TestMetricSeries_ResourceOnlyDiffersByHostNameCollapses below pins that
// host.name alone, without service.instance.id, would NOT split these into
// two series.
func buildTwoReplicaMetrics(t *testing.T) pmetric.Metrics {
	t.Helper()
	md := pmetric.NewMetrics()
	base := time.Now().UnixNano()

	// Same service, same scope, same metric, same datapoint labels. The
	// resources differ by service.instance.id -- the identifying field -- and,
	// incidentally, by host.name too.
	for i, host := range []string{"pod-a", "pod-b"} {
		rm := md.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("service.name", "checkout")
		rm.Resource().Attributes().PutStr("service.instance.id", "checkout-"+host)
		rm.Resource().Attributes().PutStr("host.name", host)

		sm := rm.ScopeMetrics().AppendEmpty()
		sm.Scope().SetName("otelhttp")
		sm.Scope().SetVersion("1.2.0")

		m := sm.Metrics().AppendEmpty()
		m.SetName("http.server.duration")
		m.SetUnit("ms")
		g := m.SetEmptyGauge()
		for j := 0; j < 3; j++ {
			dp := g.DataPoints().AppendEmpty()
			dp.SetTimestamp(pcommon.Timestamp(base + int64(j)*1_000_000))
			dp.SetDoubleValue(float64(10*(i+1) + j))
			dp.Attributes().PutStr("http.route", "/checkout")
		}
	}
	return md
}

// TestMetricSeries_SplitByResource is also the regression test for the
// property Change A must not break: two genuinely different instances --
// same service.name, distinct service.instance.id -- still get two resource
// rows and two series. Collapsing replicas that actually identified
// themselves would be a worse bug than the one this whole change fixes.
func TestMetricSeries_SplitByResource(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, buildTwoReplicaMetrics(t), s.FlushedIDs())
	}))

	countRows := func(table string) int {
		var n int
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			return db.QueryRow(`select count(*) from ` + table).Scan(&n)
		}))
		return n
	}
	assert.Equal(t, 2, countRows("resources"),
		"distinct service.instance.id must produce distinct resource rows")

	// One logical stream: that part is correct and must stay correct, or a pod
	// restart would fragment the timeseries.
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1, "two replicas are still one metric stream")
	assert.Equal(t, float64(2), summaries[0]["seriesCount"],
		"the summary must report two series, agreeing with the detail view")

	streamID, ok := summaries[0]["id"].(string)
	require.True(t, ok)

	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, streamID, 0, time.Now().UnixNano()+int64(time.Hour), 0, nil, nil, 0, false, 0, 0, nil, nil, 0)
	})
	require.NoError(t, err)
	var metric map[string]any
	require.NoError(t, json.Unmarshal(raw, &metric))

	ts, _ := metric["timeseries"].([]any)
	require.Len(t, ts, 2, "one series per replica, not one merged line")

	// Each carries its own three points -- a merge would produce one series of
	// six, which is the shape that silently averaged two machines together.
	keys := map[string]bool{}
	for _, entry := range ts {
		e := entry.(map[string]any)
		dps, _ := e["datapoints"].([]any)
		assert.Len(t, dps, 3, "each replica keeps its own datapoints")
		keys[e["attributesKey"].(string)] = true
	}

	// And they must be distinguishable. The labels are identical by
	// construction, so a key derived from labels alone collides -- which would
	// render two indistinguishable legend entries, strictly worse than the
	// single merged line it replaced.
	assert.Len(t, keys, 2,
		"series keys must differ, or the split produces two identical legend entries")
}

// TestMetricSeries_ResourceOnlyDiffersByHostNameCollapses is the inverse of
// TestMetricSeries_SplitByResource: two resources that differ only by
// host.name, with no service.instance.id at all, must collapse onto one
// series. host.name (and k8s.pod.name) used to be a fallback identity --
// InstanceKey walked a list of stand-ins when service.instance.id was absent
// -- but the OTel spec sanctions only service.instance.id for this, so that
// fallback is gone. Absent means absent: the telemetry never claimed these
// were different instances, so we do not either.
func TestMetricSeries_ResourceOnlyDiffersByHostNameCollapses(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	md := pmetric.NewMetrics()
	base := time.Now().UnixNano()
	for i, host := range []string{"pod-a", "pod-b"} {
		rm := md.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("service.name", "checkout")
		rm.Resource().Attributes().PutStr("host.name", host)

		sm := rm.ScopeMetrics().AppendEmpty()
		sm.Scope().SetName("otelhttp")
		sm.Scope().SetVersion("1.2.0")

		m := sm.Metrics().AppendEmpty()
		m.SetName("http.server.duration")
		m.SetUnit("ms")
		g := m.SetEmptyGauge()
		for j := 0; j < 3; j++ {
			dp := g.DataPoints().AppendEmpty()
			dp.SetTimestamp(pcommon.Timestamp(base + int64(j)*1_000_000))
			dp.SetDoubleValue(float64(10*(i+1) + j))
			dp.Attributes().PutStr("http.route", "/checkout")
		}
	}

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, md, s.FlushedIDs())
	}))

	countRows := func(table string) int {
		var n int
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			return db.QueryRow(`select count(*) from ` + table).Scan(&n)
		}))
		return n
	}
	assert.Equal(t, 1, countRows("resources"),
		"host.name alone, without service.instance.id, must not fragment the resource")

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	assert.Equal(t, float64(1), summaries[0]["seriesCount"],
		"neither replica identified itself, so they legitimately collapse to one series")
}

// A series id has to survive re-ingest, restarts and retention, because it is
// what a shared URL names.
//
// This is the property the old wire format could not offer: metric links could
// only reference a datapoint id, which is minted per row and deleted by
// retention, so a pasted link degraded silently to "no selection". A
// content-derived id from (stream, resource, labels) is the same every time the
// same series arrives.
func TestMetricSeries_IDsAreStableAcrossReingest(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	seriesIDs := func() []string {
		var out []string
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			rows, err := db.Query(`select id::varchar from metric_series order by 1`)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					return err
				}
				out = append(out, id)
			}
			return rows.Err()
		}))
		return out
	}

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, buildTwoReplicaMetrics(t), s.FlushedIDs())
	}))
	first := seriesIDs()
	require.Len(t, first, 2, "two replicas are two series")

	// The same content again: same ids, no new rows.
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, buildTwoReplicaMetrics(t), s.FlushedIDs())
	}))
	assert.Equal(t, first, seriesIDs(),
		"re-ingesting the same series must not mint new ids")

	// And the id the wire serves is the id in the table, or a URL built from
	// one could not be resolved back to the other.
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, summaries[0]["id"].(string), 0,
			time.Now().UnixNano()+int64(time.Hour), 0, nil, nil, 0, false, 0, 0, nil, nil, 0)
	})
	require.NoError(t, err)
	var metric map[string]any
	require.NoError(t, json.Unmarshal(raw, &metric))

	var served []string
	for _, entry := range metric["timeseries"].([]any) {
		served = append(served, entry.(map[string]any)["attributesKey"].(string))
	}
	sort.Strings(served)
	assert.Equal(t, first, served,
		"the key on the wire must be the series id, so a link resolves back to a row")
}

// buildInstanceMetrics emits one gauge from one instance, optionally with extra
// resource attributes bolted on -- the shape a collector processor produces
// when it starts resolving metadata partway through a stream.
func buildInstanceMetrics(t *testing.T, extra map[string]string, base int64) pmetric.Metrics {
	t.Helper()
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "checkout")
	rm.Resource().Attributes().PutStr("service.instance.id", "checkout-7f9c")
	for k, v := range extra {
		rm.Resource().Attributes().PutStr(k, v)
	}

	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("otelhttp")
	sm.Scope().SetVersion("1.2.0")

	m := sm.Metrics().AppendEmpty()
	m.SetName("http.server.duration")
	m.SetUnit("ms")
	g := m.SetEmptyGauge()
	for j := 0; j < 3; j++ {
		dp := g.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(base + int64(j)*1_000_000))
		dp.SetDoubleValue(float64(j))
		dp.Attributes().PutStr("http.route", "/checkout")
	}
	return md
}

// TestMetricSeries_SurvivesResourceEnrichment is the regression test for a
// series -- and, as of Change A, a resource -- splitting when nothing about
// the instance changed.
//
// A resource id used to be a hash of the resource's whole attribute set, so
// enriching a resource mid-stream (an SDK adding telemetry.sdk.* partway
// through, say) minted a second resource row for the same running process,
// and because the series id was derived from that resource row, a second
// series too: one instrument from one instance drew two chart lines, split at
// the moment the extra attributes appeared, and a ?series= link addressed
// only half of it. Observed in the reference capture: 48 resource rows for 35
// distinct service.instance.id, telemetry.sdk.* present on 35 and absent from
// 13.
//
// Resource identity is now the OTel triplet read from attributes (see
// ingest.ResourceID), and both payloads here share the same
// service.instance.id -- so enrichment no longer mints a resource either: one
// row, one series. That is the whole point of Change A, and is why this test
// now asserts ONE resources row rather than two: the earlier version of this
// test asserted two as the "correct" half of the split, back when a resource
// id still carried the attribute set as identity. It no longer does.
func TestMetricSeries_SurvivesResourceEnrichment(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	base := time.Now().UnixNano()
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, buildInstanceMetrics(t, nil, base), s.FlushedIDs())
	}))
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, buildInstanceMetrics(t, map[string]string{
			"telemetry.sdk.name":     "opentelemetry",
			"telemetry.sdk.language": "go",
			"telemetry.sdk.version":  "1.28.0",
		}, base+10_000_000), s.FlushedIDs())
	}))

	countRows := func(table string) int {
		var n int
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			return db.QueryRow(`select count(*) from ` + table).Scan(&n)
		}))
		return n
	}

	assert.Equal(t, 1, countRows("resources"),
		"same service.instance.id: enrichment must not mint a second resource row")
	assert.Equal(t, 1, countRows("metric_series"),
		"enriching a resource must not mint a second series for the same instance")

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	assert.Equal(t, float64(1), summaries[0]["seriesCount"],
		"the summary must agree that this is one series")

	streamID, ok := summaries[0]["id"].(string)
	require.True(t, ok)
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, streamID, 0, time.Now().UnixNano()+int64(time.Hour), 0, nil, nil, 0, false, 0, 0, nil, nil, 0)
	})
	require.NoError(t, err)
	var metric map[string]any
	require.NoError(t, json.Unmarshal(raw, &metric))

	ts, _ := metric["timeseries"].([]any)
	require.Len(t, ts, 1, "one line on the chart, not two")
	dps, _ := ts[0].(map[string]any)["datapoints"].([]any)
	assert.Len(t, dps, 6, "both batches land on the same line")
}

// TestExpHistogramMerge_FoldsBucketsBelowMergedZeroThreshold is the first
// end-to-end test of the server-side reduction path. Every other GetMetric call
// in this package passes resolution 0, so the reduction machinery -- M4
// election, the alignment chain, hist_merged -- was covered only by macro unit
// tests and never run assembled. That is how the bug below survived.
//
// An exponential histogram's zero_threshold T declares that observations at or
// below T live in zero_count rather than in a bucket. Merging datapoints with
// different thresholds takes the larger, because the merged histogram cannot
// claim to resolve values one of its inputs could not. The input with the
// smaller threshold is then contributing buckets that fall entirely below the
// merged threshold, and those counts have to move into zero_count -- otherwise
// zero_threshold says one thing and the bucket array says another.
//
// Worked by hand at scale 0, where base = 2 and bucket i covers (2^i, 2^(i+1)]:
//
//	A: threshold 1, zeroCount 1, counts [5, 7] at offset 0
//	B: threshold 4, zeroCount 2, counts [0, 0, 3] at offset 0
//
//	merged threshold = 4
//	cutoff           = floor(log2(4) * 2^0) - 1 = 1
//	summed counts    = [5, 7, 3] at offset 0
//	folded           = buckets 0 and 1 = 5 + 7 = 12
//	result           = [3] at offset 2, zeroCount 1 + 2 + 12 = 15
func TestExpHistogramMerge_FoldsBucketsBelowMergedZeroThreshold(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	base := time.Now().Add(-time.Minute)
	fixture := makeExpHistogramFixtureT("http.duration", pmetric.AggregationTemporalityDelta, []expHistTestDP{
		{
			timestamp: base, scale: 0,
			zeroCount: 1, zeroThreshold: 1,
			posOffset: 0, posCounts: []uint64{5, 7},
			count: 13, sum: 40,
		},
		{
			timestamp: base.Add(time.Millisecond), scale: 0,
			zeroCount: 2, zeroThreshold: 4,
			posOffset: 0, posCounts: []uint64{0, 0, 3},
			count: 5, sum: 30,
		},
	})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	// Resolution 1 puts both datapoints in a single bucket, which is what makes
	// them merge at all.
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, summaries[0]["id"].(string), 0,
			time.Now().UnixNano()+int64(time.Hour), 1, nil, nil, 0, false, 0, 0, nil, nil, 0)
	})
	require.NoError(t, err)
	var metric map[string]any
	require.NoError(t, json.Unmarshal(raw, &metric))

	ts, _ := metric["timeseries"].([]any)
	require.Len(t, ts, 1, "one series")
	dps, _ := ts[0].(map[string]any)["datapoints"].([]any)
	require.Len(t, dps, 1, "both datapoints merge into one")
	dp := dps[0].(map[string]any)

	assert.Equal(t, float64(4), dp["zeroThreshold"], "merged threshold is the larger of the two")
	assert.Equal(t, float64(15), dp["zeroCount"],
		"buckets below the merged threshold must be folded into zero_count")
	assert.Equal(t, float64(2), dp["positiveBucketOffset"],
		"the folded buckets are gone, so the array starts above the cutoff")

	counts, _ := dp["positiveBucketCounts"].([]any)
	require.Len(t, counts, 1)
	assert.Equal(t, float64(3), counts[0], "only the bucket above the threshold survives")

	// The whole point: no observation was invented or lost by moving counts.
	assert.Equal(t, float64(18), dp["count"], "total observations conserved")
}

// TestGetMetric_MergedSeriesKeepTheirLabels covers a merged histogram series
// still reporting the attributes that identify it.
//
// hist_merged aggregates per (series, bucket) and listed its output columns
// explicitly, and attribute_ids was not among them. projected_dps unions that
// branch with filtered_dps using `union all by name`, which fills a missing
// column with NULL rather than failing -- so every merged histogram arrived
// with no attributes, attrs_json(NULL) rendered [], and the legend labelled all
// twenty-one series "default series". Gauges were unaffected: their branch is
// `select *`.
//
// The bug needed a reduction to appear at all, which is why no existing test
// saw it: they either request no reduction or never look at the labels.
func TestGetMetric_MergedSeriesKeepTheirLabels(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	// Two series, told apart only by their labels, each with two datapoints
	// close enough to merge.
	dp := func(d time.Duration, driver string) histTestDP {
		return histTestDP{
			timestamp: base.Add(d),
			attrs:     map[string]string{"driver": driver},
			bounds:    []float64{1, 2},
			counts:    []uint64{1, 0, 0},
			count:     1, sum: 0.5, min: 0.5, max: 0.5,
		}
	}
	fixture := makeHistogramFixtureT("labelled.duration", pmetric.AggregationTemporalityDelta, []histTestDP{
		dp(0, "ALO"), dp(time.Second, "ALO"),
		dp(0, "VER"), dp(time.Second, "VER"),
	})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	// Target 1 forces the merge. Without a reduction the rows come straight
	// from filtered_dps and carry their labels regardless, which is exactly the
	// blind spot this covers.
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, summaries[0]["id"].(string),
			base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano(),
			1, nil, nil, 0, false, 0, 0, nil, nil, 0)
	})
	require.NoError(t, err)
	var metric map[string]any
	require.NoError(t, json.Unmarshal(raw, &metric))

	series, _ := metric["timeseries"].([]any)
	require.Len(t, series, 2, "two series")

	drivers := map[string]bool{}
	for _, entry := range series {
		attrs, _ := entry.(map[string]any)["attributes"].([]any)
		require.NotEmpty(t, attrs,
			"a merged series must carry the attributes that identify it")
		for _, a := range attrs {
			m := a.(map[string]any)
			if m["key"] == "driver" {
				drivers[m["value"].(string)] = true
			}
		}
	}
	assert.Equal(t, map[string]bool{"ALO": true, "VER": true}, drivers,
		"each merged series keeps its own label, not a blank or a neighbour's")
}

// TestGetMetric_ScalarViewBuckets covers the grid the Sum / Average / Rate
// views aggregate on, which used to be built in the browser.
//
// The client sliced each series between its *own* first and last point, so two
// series got different bucket boundaries and "bucket 3" covered a different
// interval for each. The store buckets on absolute ladder boundaries, so every
// series lands on the same edges.
//
// The other half is what an empty bucket means. It comes back null rather than
// zero, because Sum and Rate read a gap as no activity while Average has to
// skip it -- the mean of nothing is not nought. Emitting 0 would bake one
// view's reading into all three.
func TestGetMetric_ScalarViewBuckets(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	// A counter climbing by one a minute, with a five-minute silence in the
	// middle and a late start, so the trimming and the interior gap are both
	// exercised.
	var dps []sumTestDP
	early := map[string]string{"pod": "early"}
	for i := range 3 {
		dps = append(dps, sumTestDP{
			timestamp: base.Add(time.Duration(i) * time.Minute),
			value:     float64(i + 1),
			attrs:     early,
		})
	}
	for i := range 3 {
		dps = append(dps, sumTestDP{
			timestamp: base.Add(time.Duration(8+i) * time.Minute),
			value:     float64(4 + i),
			attrs:     early,
		})
	}
	// A second series that only starts halfway through. Its buckets must begin
	// where *it* begins, not where the first series did -- the trimming is per
	// series, and a response-wide extent would pad this one with empty buckets
	// for time before it existed.
	late := map[string]string{"pod": "late"}
	for i := range 3 {
		dps = append(dps, sumTestDP{
			timestamp: base.Add(time.Duration(8+i) * time.Minute),
			value:     float64(i + 1),
			attrs:     late,
		})
	}
	fixture := makeSumFixtureT("climb.total", pmetric.AggregationTemporalityCumulative, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	// A window far wider than the data, fitted -- so the grid comes from the ten
	// minutes that hold datapoints rather than the twelve hours asked for. With
	// an unfitted window the ladder would pick hour-wide buckets and the whole
	// series would land in one, which is a different test.
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, summaries[0]["id"].(string),
			base.Add(-6*time.Hour).UnixNano(), base.Add(6*time.Hour).UnixNano(),
			0, nil, nil, 0, true, 12, 0, nil, nil, 0)
	})
	require.NoError(t, err)
	var metric map[string]any
	require.NoError(t, json.Unmarshal(raw, &metric))

	series, _ := metric["timeseries"].([]any)
	require.Len(t, series, 2, "two series")

	byPod := map[string][]any{}
	for _, entry := range series {
		ts := entry.(map[string]any)
		var pod string
		for _, a := range ts["attributes"].([]any) {
			if a.(map[string]any)["key"] == "pod" {
				pod = a.(map[string]any)["value"].(string)
			}
		}
		v, _ := ts["views"].([]any)
		byPod[pod] = v
	}
	require.NotEmpty(t, byPod["early"], "a scalar series carries its view buckets")
	require.NotEmpty(t, byPod["late"])

	// The late series covers the last three minutes only. Padding it back to
	// the early series' start would roughly triple its bucket count.
	assert.Less(t, len(byPod["late"]), len(byPod["early"]),
		"each series is trimmed to its own extent, not the response's")
	assert.Positive(t, byPod["late"][0].(map[string]any)["sampleCount"],
		"the late series starts at its own first bucket")

	views := byPod["early"]

	first := views[0].(map[string]any)
	last := views[len(views)-1].(map[string]any)

	// Leading and trailing empties are trimmed: the first and last bucket both
	// hold samples, even though the window is twelve hours wide.
	assert.Positive(t, first["sampleCount"], "no empty buckets before the data")
	assert.Positive(t, last["sampleCount"], "no empty buckets after it")

	// The interior gap survives, and is null rather than zero.
	var empties int
	for _, v := range views {
		b := v.(map[string]any)
		if b["sampleCount"].(float64) != 0 {
			continue
		}
		empties++
		assert.Nil(t, b["sum"], "an empty bucket has no sum, not a sum of zero")
		assert.Nil(t, b["avg"], "nor a mean")
		assert.Nil(t, b["rate"], "nor a rate")
	}
	assert.Positive(t, empties, "the silence in the middle is still a bucket")

	// The first bucket of a cumulative series has no predecessor, so it
	// describes no interval and its rate is unknown rather than zero.
	assert.Nil(t, first["rate"],
		"the first bucket has no earlier reading to difference against")

	// Sum and Average read the running totals, not the increments: summing
	// across series at time t means adding the counters. Only Rate differences.
	assert.NotNil(t, first["sum"])
	assert.NotNil(t, first["avg"])
}

// TestGetMetric_DatapointNarrowing covers the narrowest of the three series
// parameters: which series ship their datapoints.
//
// The property worth pinning is what it must NOT touch. Datapoints are almost
// the whole payload, so narrowing them is worth doing -- but a series that
// ships none still has to arrive with its row, its stats, its view buckets and
// its sparkline, because the panel lists series nobody is drawing and the All
// aggregate folds them. Narrowing that reached the aggregates would turn "all"
// into "all of the checked ones", which is wrong and looks plausible.
func TestGetMetric_DatapointNarrowing(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	var dps []sumTestDP
	for _, pod := range []string{"a", "b", "c"} {
		for i := range 20 {
			dps = append(dps, sumTestDP{
				timestamp: base.Add(time.Duration(i) * time.Minute),
				value:     float64(i + 1),
				attrs:     map[string]string{"pod": pod},
			})
		}
	}
	fixture := makeSumFixtureT("narrow.total", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	streamID := summaries[0]["id"].(string)

	get := func(dpSeries []string, limit int64) []any {
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetric(ctx, db, streamID,
				base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano(),
				0, nil, nil, 0, true, 12, 8, nil, dpSeries, limit)
		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		return m["timeseries"].([]any)
	}

	// Everything ships when neither parameter is given -- the behaviour a caller
	// that predates them gets.
	full := get(nil, 0)
	require.Len(t, full, 3)
	for _, entry := range full {
		assert.NotEmpty(t, entry.(map[string]any)["datapoints"])
	}

	// Name one series: it alone carries datapoints, and the other two arrive
	// complete in every other respect.
	first := full[0].(map[string]any)["attributesKey"].(string)
	narrowed := get([]string{first}, 0)
	require.Len(t, narrowed, 3, "narrowing datapoints drops no series")
	var withPoints, withoutPoints int
	for _, entry := range narrowed {
		ts := entry.(map[string]any)
		if len(ts["datapoints"].([]any)) > 0 {
			withPoints++
			assert.Equal(t, first, ts["attributesKey"])
		} else {
			withoutPoints++
		}
		// The parts that must survive regardless.
		assert.NotNil(t, ts["stats"], "stats describe the window, not the sample")
		assert.NotEmpty(t, ts["views"], "view buckets feed the All aggregate")
		assert.NotEmpty(t, ts["sparkline"], "the row still draws a shape")
		assert.NotEmpty(t, ts["attributes"], "and still names itself")
	}
	assert.Equal(t, 1, withPoints)
	assert.Equal(t, 2, withoutPoints)

	// An empty list is not the same as an absent one: it names no series.
	none := get([]string{}, 0)
	require.Len(t, none, 3)
	for _, entry := range none {
		ts := entry.(map[string]any)
		assert.Empty(t, ts["datapoints"], "an empty list ships no datapoints")
		assert.NotEmpty(t, ts["views"], "and still every view bucket")
	}

	// The limit is for a caller that cannot name the series, because it picks
	// them from this response. It takes them in the response's own order.
	limited := get(nil, 2)
	require.Len(t, limited, 3)
	var limitedWith []string
	for _, entry := range limited {
		ts := entry.(map[string]any)
		if len(ts["datapoints"].([]any)) > 0 {
			limitedWith = append(limitedWith, ts["attributesKey"].(string))
		}
	}
	assert.Len(t, limitedWith, 2, "the first two series in response order")
	var wantFirstTwo []string
	for _, entry := range limited[:2] {
		wantFirstTwo = append(wantFirstTwo, entry.(map[string]any)["attributesKey"].(string))
	}
	assert.ElementsMatch(t, wantFirstTwo, limitedWith,
		"and they are the first two as the response lists them, so the caller's "+
			"\"first N\" and the store's mean the same series")

	// A named list wins over a limit, since that caller does know.
	both := get([]string{first}, 2)
	var bothWith int
	for _, entry := range both {
		if len(entry.(map[string]any)["datapoints"].([]any)) > 0 {
			bothWith++
		}
	}
	assert.Equal(t, 1, bothWith, "the list decides when it is given")
}

// TestGetMetric_ScalarPoolAggregate covers the cross-series lines: a pool of
// series folded into one line per bucket.
//
// The fold replaces combinePool in TypeScript, which merged the pool's chart
// points, built a grid from their own first and last timestamp with a bucket
// count derived from the pool size, and reduced that. Two properties are worth
// pinning because they are what the old code got wrong or could not do:
// the pool lands on the same absolute grid as the per-series views, and the
// average is pooled over every sample rather than averaged over per-series
// averages.
func TestGetMetric_ScalarPoolAggregate(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	// Deliberately lopsided: "dense" reports ten times a minute at value 1,
	// "sparse" once a minute at value 100. Mean-of-means would call that 50.5;
	// the pooled mean is (10*1 + 100) / 11 = 10, and only one of those is the
	// average of what was observed.
	var dps []sumTestDP
	dense := map[string]string{"pod": "dense"}
	sparse := map[string]string{"pod": "sparse"}
	for minute := range 5 {
		for tick := range 10 {
			dps = append(dps, sumTestDP{
				timestamp: base.Add(time.Duration(minute)*time.Minute + time.Duration(tick)*time.Second),
				value:     1,
				attrs:     dense,
			})
		}
		dps = append(dps, sumTestDP{
			timestamp: base.Add(time.Duration(minute) * time.Minute),
			value:     100,
			attrs:     sparse,
		})
	}
	fixture := makeSumFixtureT("pool.total", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	streamID := summaries[0]["id"].(string)

	get := func(selected []string) map[string]any {
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetric(ctx, db, streamID,
				base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano(),
				0, nil, nil, 0, true, 12, 0, selected, nil, 0)
		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		return m
	}

	// Series ids, so one of the two can be named as the checked pool.
	all := get(nil)
	idByPod := map[string]string{}
	for _, entry := range all["timeseries"].([]any) {
		ts := entry.(map[string]any)
		for _, a := range ts["attributes"].([]any) {
			attr := a.(map[string]any)
			if attr["key"] == "pod" {
				idByPod[attr["value"].(string)] = ts["attributesKey"].(string)
			}
		}
	}
	require.Len(t, idByPod, 2)

	pools := all["scalarAggregate"].(map[string]any)
	allBuckets := pools["all"].([]any)
	require.NotEmpty(t, allBuckets, "the All pool folds every series")
	assert.Empty(t, pools["selected"],
		"nothing checked is a real state: the Selected pool is empty and the "+
			"chart draws All by itself")

	// Every bucket the pool reports is a bucket the per-series views report, on
	// the same boundaries -- which is the point of folding scalar_view_agg
	// instead of building a grid from the pool's own extent.
	seriesBucketStarts := map[string]bool{}
	for _, entry := range all["timeseries"].([]any) {
		views, _ := entry.(map[string]any)["views"].([]any)
		for _, v := range views {
			seriesBucketStarts[v.(map[string]any)["bucketStart"].(string)] = true
		}
	}
	for _, b := range allBuckets {
		start := b.(map[string]any)["bucketStart"].(string)
		assert.True(t, seriesBucketStarts[start],
			"pool bucket %s is on the shared grid", start)
	}

	// The pooled mean, on a bucket holding both series.
	var checked int
	for _, b := range allBuckets {
		bucket := b.(map[string]any)
		if bucket["sampleCount"].(float64) != 11 {
			continue
		}
		checked++
		assert.InDelta(t, 110.0, bucket["sum"], 1e-9, "ten 1s and one 100")
		assert.InDelta(t, 10.0, bucket["avg"], 1e-9,
			"pooled over every sample; the mean of the two series' means would "+
				"be 50.5 and would let one sparse series outvote ten samples")
	}
	assert.Positive(t, checked, "at least one bucket holds both series")

	// Checking one series folds that series alone.
	one := get([]string{idByPod["sparse"]})
	selBuckets := one["scalarAggregate"].(map[string]any)["selected"].([]any)
	require.NotEmpty(t, selBuckets)
	var populated, empty int
	for _, b := range selBuckets {
		bucket := b.(map[string]any)
		if bucket["sampleCount"].(float64) == 0 {
			// The sparse series reports once a minute onto a finer grid, so most
			// of its buckets hold nothing. They are carried rather than dropped:
			// an interior empty bucket is zero activity for Sum and Rate, and a
			// line that skipped it would draw straight through the gap.
			empty++
			assert.Nil(t, bucket["sum"], "an empty bucket has no sum, not zero")
			continue
		}
		populated++
		assert.Equal(t, 1.0, bucket["sampleCount"],
			"only the sparse series is in the checked pool")
		assert.InDelta(t, 100.0, bucket["sum"], 1e-9)
	}
	assert.Positive(t, populated, "the checked series contributes samples")
	assert.Positive(t, empty, "and its gaps survive the fold")
	// The All pool is unmoved by what is checked -- that is what makes it "all".
	assert.Equal(t, len(allBuckets),
		len(one["scalarAggregate"].(map[string]any)["all"].([]any)),
		"the All pool does not narrow with the selection")
}

// TestGetMetric_Sparkline covers the third reduction: the shape of a series at
// list-row resolution.
//
// It exists as its own reduction rather than reusing either of the other two,
// and each half of that is asserted here. Against the election, it is bounded
// by the row's pixels rather than the chart's -- the row sparkline used to be
// handed the elected series, up to 2,000 points, and drew all of them into a
// 128px box. Against the views, it keeps extremes rather than averaging them,
// because a sparkline's whole job is to show that something happened, and a
// mean over a wide bucket is exactly what hides a spike.
func TestGetMetric_Sparkline(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	// Two hundred minutes of a quiet delta counter with a single one-minute
	// spike in the middle. Averaged into eight buckets the spike is divided by
	// twenty-five and disappears into the noise; kept as a bucket max it stays
	// the tallest thing on the line, which is the point of the row.
	const spike = 500.0
	var dps []sumTestDP
	attrs := map[string]string{"pod": "a"}
	for i := range 200 {
		value := 1.0
		if i == 100 {
			value = spike
		}
		dps = append(dps, sumTestDP{
			timestamp: base.Add(time.Duration(i) * time.Minute),
			value:     value,
			attrs:     attrs,
		})
	}
	fixture := makeSumFixtureT("spiky.total", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	streamID := summaries[0]["id"].(string)

	const buckets = 8
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, streamID,
			base.Add(-time.Hour).UnixNano(), base.Add(6*time.Hour).UnixNano(),
			0, nil, nil, 0, true, 0, buckets, nil, nil, 0)
	})
	require.NoError(t, err)
	var metric map[string]any
	require.NoError(t, json.Unmarshal(raw, &metric))

	series, _ := metric["timeseries"].([]any)
	require.Len(t, series, 1)
	points, _ := series[0].(map[string]any)["sparkline"].([]any)
	require.NotEmpty(t, points, "a scalar series carries a sparkline")

	// Two points per bucket -- a min and a max -- so the ladder's rounding
	// aside, the row never receives more than it has pixels for. Two hundred
	// datapoints went in.
	assert.LessOrEqual(t, len(points), 2*buckets,
		"at most a min and a max per bucket")
	assert.Less(t, len(points), 200, "and far fewer than the datapoints behind it")

	var maxValue float64
	var prev int64
	for i, p := range points {
		b := p.(map[string]any)
		v := b["value"].(float64)
		if v > maxValue {
			maxValue = v
		}
		// Timestamps travel as strings, like every other ns value on the wire.
		ts, err := strconv.ParseInt(b["timestamp"].(string), 10, 64)
		require.NoError(t, err)
		if i > 0 {
			assert.Greater(t, ts, prev,
				"points are ordered by time, and a bucket that elected the same "+
					"row as both its min and its max contributes it once")
		}
		prev = ts
	}

	// The reason this is not read off the views: the spike is one sample in a
	// bucket holding twenty-five, so an average would report about 21.
	assert.Equal(t, spike, maxValue,
		"the spike survives the reduction rather than being averaged away")

	// Asking for no buckets is how a caller that draws no sparklines avoids
	// paying for them, and it has to be legible as "absent" rather than empty.
	rawNone, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, streamID,
			base.Add(-time.Hour).UnixNano(), base.Add(6*time.Hour).UnixNano(),
			0, nil, nil, 0, true, 0, 0, nil, nil, 0)
	})
	require.NoError(t, err)
	var none map[string]any
	require.NoError(t, json.Unmarshal(rawNone, &none))
	noneSeries, _ := none["timeseries"].([]any)
	require.Len(t, noneSeries, 1)
	assert.Nil(t, noneSeries[0].(map[string]any)["sparkline"],
		"zero buckets means no sparkline, not an empty one")
}

// TestExpHistogramMerge_RescalesBeforeSumming covers a series whose scale
// drifts mid-flight: an SDK downscales as the observed range widens, so two
// datapoints of the *same* series can carry different scales and offsets.
//
// Summing their bucket vectors positionally would add counts covering
// different value ranges together -- wrong quantiles, no error, no way to see
// it from the chart. The merge has to bring both onto the coarsest scale
// present first.
//
// Ported from the TypeScript merge, which is where this used to be asserted.
// That implementation is gone -- the store does the merging now -- and this
// test exists so the behaviour did not leave with it.
func TestExpHistogramMerge_RescalesBeforeSumming(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	fixture := makeExpHistogramFixtureT("rescale.duration", pmetric.AggregationTemporalityDelta, []expHistTestDP{
		// Scale 2, four buckets from offset 4.
		{
			timestamp: base, scale: 2,
			zeroCount: 0, zeroThreshold: 0,
			posOffset: 4, posCounts: []uint64{1, 1, 1, 1},
			count: 4, sum: 8,
		},
		// Scale 1, two buckets from offset 2 -- the same value range, half the
		// resolution. Downscaling the first by one step lands exactly here,
		// which is what makes the overlap checkable rather than approximate.
		{
			timestamp: base.Add(time.Second), scale: 1,
			zeroCount: 0, zeroThreshold: 0,
			posOffset: 2, posCounts: []uint64{5, 5},
			count: 10, sum: 20,
		},
	})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	// Target 1 puts both datapoints in one bucket, which is what makes them
	// merge at all.
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, summaries[0]["id"].(string),
			base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano(),
			1, nil, nil, 0, false, 0, 0, nil, nil, 0)
	})
	require.NoError(t, err)
	var metric map[string]any
	require.NoError(t, json.Unmarshal(raw, &metric))

	ts, _ := metric["timeseries"].([]any)
	require.Len(t, ts, 1)
	dps, _ := ts[0].(map[string]any)["datapoints"].([]any)
	require.Len(t, dps, 1, "both datapoints merge into one bucket")
	dp := dps[0].(map[string]any)

	// The merge lands on the coarsest scale present, not the first seen.
	assert.Equal(t, float64(1), dp["scale"],
		"merging must downscale to the coarsest scale in the group")

	// Total count is conserved however the buckets are aligned: 4 + 10.
	counts, _ := dp["positiveBucketCounts"].([]any)
	var total float64
	for _, c := range counts {
		total += c.(float64)
	}
	assert.Equal(t, float64(14), total, "no observation invented or lost")
	assert.Equal(t, float64(14), dp["count"], "the reported count agrees with the buckets")

	// The scale-2 datapoint's four buckets at offset 4 collapse to two at
	// offset 2 -- exactly where the scale-1 datapoint already sits. A correct
	// merge overlaps them; a positional sum would lay them side by side.
	assert.Equal(t, float64(2), dp["positiveBucketOffset"])
	require.Len(t, counts, 2, "overlapped, not concatenated")
	assert.Equal(t, float64(7), counts[0])
	assert.Equal(t, float64(7), counts[1])
}

// TestExpHistogramMerge_CumulativeSubtractsAcrossAScaleChange covers the same
// drift on a Cumulative stream, where the merge is a subtraction rather than a
// sum.
//
// Each datapoint is a running total, so the activity in a bucket is the last
// minus the first. Do that without aligning scales and the two vectors are not
// comparable, and the fallback reports the running total as though it were the
// activity -- a counter that only ever climbs, drawn as though every bucket saw
// all of it.
//
// Also ported from the deleted TypeScript merge.
func TestExpHistogramMerge_CumulativeSubtractsAcrossAScaleChange(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	fixture := makeExpHistogramFixtureT("cumulative.duration", pmetric.AggregationTemporalityCumulative, []expHistTestDP{
		// 10 observations so far, at scale 2.
		{
			timestamp: base, scale: 2,
			zeroCount: 0, zeroThreshold: 0,
			posOffset: 4, posCounts: []uint64{5, 5, 0, 0},
			count: 10, sum: 10,
		},
		// 30 by the next datapoint, and the SDK downscaled in between. The
		// activity in this bucket is 20, not 30.
		{
			timestamp: base.Add(time.Second), scale: 1,
			zeroCount: 0, zeroThreshold: 0,
			posOffset: 2, posCounts: []uint64{15, 15},
			count: 30, sum: 30,
		},
	})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, summaries[0]["id"].(string),
			base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano(),
			1, nil, nil, 0, false, 0, 0, nil, nil, 0)
	})
	require.NoError(t, err)
	var metric map[string]any
	require.NoError(t, json.Unmarshal(raw, &metric))

	ts, _ := metric["timeseries"].([]any)
	require.Len(t, ts, 1)
	dps, _ := ts[0].(map[string]any)["datapoints"].([]any)
	require.Len(t, dps, 1, "both datapoints merge into one bucket")
	dp := dps[0].(map[string]any)

	// The number that matters: activity, not the running total.
	assert.Equal(t, float64(20), dp["count"],
		"a cumulative bucket reports the activity within it, not the counter's value")

	counts, _ := dp["positiveBucketCounts"].([]any)
	var total float64
	for _, c := range counts {
		total += c.(float64)
	}
	assert.Equal(t, float64(20), total,
		"the bucket vectors must be differenced on a common scale, not passed through")
}

// TestGetMetric_SeriesFilter covers the parameter that makes server-side
// cross-series aggregation possible: the caller names the series it wants and
// the store narrows before the reduction, so asking for two of ten costs two.
//
// Empty means all, which is what every caller predating the parameter sends.
func TestGetMetric_SeriesFilter(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	base := time.Now().Add(-time.Minute)
	var dps []expHistTestDP
	for si := 0; si < 3; si++ {
		for j := 0; j < 4; j++ {
			dps = append(dps, expHistTestDP{
				timestamp: base.Add(time.Duration(j) * time.Second),
				attrs:     map[string]string{"route": fmt.Sprintf("/r%d", si)},
				scale:     0, zeroCount: 1, zeroThreshold: 1,
				posOffset: 0, posCounts: []uint64{1, 2},
				count: 4, sum: 10,
			})
		}
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn,
			makeExpHistogramFixtureT("filter.me", pmetric.AggregationTemporalityDelta, dps),
			s.FlushedIDs())
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	streamID := summaries[0]["id"].(string)
	end := time.Now().UnixNano() + int64(time.Hour)

	seriesKeys := func(ids []string) []string {
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetric(ctx, db, streamID, 0, end, 0, ids, nil, 0, false, 0, 0, nil, nil, 0)
		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		var out []string
		for _, e := range m["timeseries"].([]any) {
			out = append(out, e.(map[string]any)["attributesKey"].(string))
		}
		return out
	}

	all := seriesKeys(nil)
	require.Len(t, all, 3, "nil returns every series")

	assert.Equal(t, []string{all[1]}, seriesKeys([]string{all[1]}),
		"asking for one series returns exactly that series")

	got := seriesKeys([]string{all[0], all[2]})
	assert.ElementsMatch(t, []string{all[0], all[2]}, got,
		"asking for two returns exactly those two")

	// The three states are distinct, and the empty one is the reachable
	// mistake: a user who unticks every series must see nothing, not
	// everything. nil and []string{} are different questions.
	assert.Empty(t, seriesKeys([]string{}),
		"an empty selection means no series, not every series")
}

// TestGetMetric_Quantiles covers quantiles computed in the store rather than
// from bucket vectors on the client.
//
// The expected values are the ones the TypeScript implementation produces for
// the same buckets, verified against it over a 1,200-case grid: at scale 0 the
// buckets are (1,2] cnt 10, (2,4] cnt 20 and (4,8] cnt 30, so p50's target of
// 30 lands exactly on the upper edge of the second bucket.
//
// Empty means none, so a caller drawing no overlays does not pay for them.
func TestGetMetric_Quantiles(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	base := time.Now().Add(-time.Minute)
	fixture := makeExpHistogramFixtureT("q.duration", pmetric.AggregationTemporalityDelta, []expHistTestDP{{
		timestamp: base, scale: 0,
		zeroCount: 0, zeroThreshold: 1,
		posOffset: 0, posCounts: []uint64{10, 20, 30},
		count: 60, sum: 200,
	}})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	streamID := summaries[0]["id"].(string)
	end := time.Now().UnixNano() + int64(time.Hour)

	datapoint := func(qs []float64) map[string]any {
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetric(ctx, db, streamID, 0, end, 0, nil, qs, 0, false, 0, 0, nil, nil, 0)
		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		ts := m["timeseries"].([]any)
		require.Len(t, ts, 1)
		dps := ts[0].(map[string]any)["datapoints"].([]any)
		require.Len(t, dps, 1)
		return dps[0].(map[string]any)
	}

	withQ := datapoint([]float64{0.5, 0.95, 0.99})
	q, ok := withQ["quantiles"].(map[string]any)
	require.True(t, ok, "quantiles must be an object keyed by the quantile")

	assert.InDelta(t, 4.0, q["0.5"], 1e-9)
	assert.InDelta(t, 7.464263932294459, q["0.95"], 1e-9)
	assert.InDelta(t, 7.889861635946874, q["0.99"], 1e-9)

	assert.Nil(t, datapoint(nil)["quantiles"],
		"no quantiles requested means none computed")
}

// TestGetMetric_CrossSeriesAggregate covers the merge that lets the client stop
// combining series itself.
//
// Two series, different scales, in one time bucket. The aggregate must put them
// on a common scale, align offsets and sum -- conserving every observation,
// which is the property a merge can quietly break and a chart will never show.
//
// Worked by hand. Scale 0 has base 2 and bucket i covering (2^i, 2^(i+1)];
// scale -1 has base 4 and bucket j covering (4^j, 4^(j+1)]. The coarser of the
// two wins, so the scale-0 series is downscaled by one step, folding its
// buckets in pairs:
//
//	A scale  0, counts [10, 20, 30] at offset 0  -> [30, 30] at offset 0
//	B scale -1, counts [5, 5]       at offset 0
//	merged   scale -1, counts [35, 35] at offset 0
//
// Totals: A has 60 observations, B has 10, so the aggregate must report 70.
func TestGetMetric_CrossSeriesAggregate(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	base := time.Now().Add(-time.Minute)
	fixture := makeExpHistogramFixtureT("agg.duration", pmetric.AggregationTemporalityDelta, []expHistTestDP{
		{
			timestamp: base, attrs: map[string]string{"route": "/a"},
			scale: 0, zeroCount: 0, zeroThreshold: 0,
			posOffset: 0, posCounts: []uint64{10, 20, 30},
			count: 60, sum: 100,
		},
		{
			timestamp: base.Add(time.Second), attrs: map[string]string{"route": "/b"},
			scale: -1, zeroCount: 0, zeroThreshold: 0,
			posOffset: 0, posCounts: []uint64{5, 5},
			count: 10, sum: 20,
		},
	})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, summaries[0]["id"].(string), 0,
			time.Now().UnixNano()+int64(time.Hour), 1, nil, []float64{0.5}, 0, false, 0, 0, nil, nil, 0)
	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))

	require.Len(t, m["timeseries"].([]any), 2, "per-series data still returned")

	agg, ok := m["aggregate"].([]any)
	require.True(t, ok, "aggregate must be present for a histogram merge")
	require.Len(t, agg, 1, "both series fall in one time bucket")
	a := agg[0].(map[string]any)

	assert.Equal(t, float64(-1), a["scale"], "merged on the coarser scale")
	assert.Equal(t, float64(70), a["count"], "every observation conserved")
	assert.Equal(t, float64(0), a["positiveBucketOffset"])

	counts, _ := a["positiveBucketCounts"].([]any)
	require.Len(t, counts, 2)
	assert.Equal(t, float64(35), counts[0], "10+20 downscaled, plus 5")
	assert.Equal(t, float64(35), counts[1], "30 downscaled, plus 5")

	// Bucket counts and the reported total have to tell the same story.
	var sum float64
	for _, c := range counts {
		sum += c.(float64)
	}
	assert.Equal(t, a["count"], sum+a["zeroCount"].(float64),
		"bucket counts plus zero count must equal the reported total")

	q, ok := a["quantiles"].(map[string]any)
	require.True(t, ok, "the aggregate carries quantiles for the summary panel")
	assert.NotNil(t, q["0.5"])

	// min and max are derived from the buckets, because a merge cannot carry
	// them through -- for cumulative it is a subtraction, and two minima do not
	// subtract into the minimum of what happened between them. At scale -1 the
	// surviving buckets are (1,4] and (4,16].
	assert.InDelta(t, 1.0, a["min"], 1e-9, "lower edge of the first populated bucket")
	assert.InDelta(t, 16.0, a["max"], 1e-9, "upper edge of the last populated bucket")
}

// TestGetMetric_TimezoneAlignedBuckets covers bucket boundaries landing where
// the reader's calendar puts them rather than where the epoch does.
//
// The store shifts by the viewer's UTC offset, floors, and shifts back -- the
// same three steps histogramBucketStart takes in TypeScript. It uses floor_div
// rather than integer division because the latter truncates toward zero, so a
// pre-epoch timestamp would floor the wrong way and land a datapoint a whole
// bucket late. That is the hazard the client comment flags for BigInt division,
// and it is why this test reaches back before 1970 rather than only checking a
// present-day offset.
func TestGetMetric_TimezoneAlignedBuckets(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	// Two datapoints either side of midnight UTC. Under a -5h offset they fall
	// on the same local day; under UTC they straddle two.
	utcMidnight := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	fixture := makeExpHistogramFixtureT("tz.duration", pmetric.AggregationTemporalityDelta, []expHistTestDP{
		{
			timestamp: utcMidnight.Add(-2 * time.Hour), scale: 0,
			zeroCount: 0, zeroThreshold: 0,
			posOffset: 0, posCounts: []uint64{4}, count: 4, sum: 8,
		},
		{
			timestamp: utcMidnight.Add(2 * time.Hour), scale: 0,
			zeroCount: 0, zeroThreshold: 0,
			posOffset: 0, posCounts: []uint64{6}, count: 6, sum: 12,
		},
	})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	streamID := summaries[0]["id"].(string)

	// A window a few days wide, so the bucket ladder lands on day-sized widths.
	start := utcMidnight.Add(-36 * time.Hour).UnixNano()
	end := utcMidnight.Add(36 * time.Hour).UnixNano()

	bucketCount := func(offsetNs int64) int {
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetric(ctx, db, streamID, start, end, 3, nil, nil, offsetNs, false, 0, 0, nil, nil, 0)
		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		agg, _ := m["aggregate"].([]any)
		return len(agg)
	}

	utcBuckets := bucketCount(0)
	shifted := bucketCount(-5 * int64(time.Hour))

	// The datapoints are 4h apart, so whether they share a bucket depends
	// entirely on where the boundary falls -- which is the thing under test.
	assert.Positive(t, utcBuckets, "UTC alignment still produces buckets")
	assert.Positive(t, shifted, "a shifted offset still produces buckets")

	// The offset must actually reach the bucketing. Identical counts for every
	// offset would mean it was ignored, which is the failure this guards.
	var distinct = map[int]bool{}
	for _, off := range []int64{0, -5 * int64(time.Hour), 9 * int64(time.Hour), 13 * int64(time.Hour)} {
		distinct[bucketCount(off)] = true
	}
	assert.Greater(t, len(distinct), 1,
		"bucket boundaries must move with the offset, or tz_offset_ns is not reaching bucket_start")
}

// TestGetMetric_FitToDataSpansTheData covers the reduction dividing the data's
// own extent when the caller says its window was never chosen.
//
// The bug this pins: the reduction always divided the *requested* window, so
// "All" -- epoch to now -- produced buckets over a year wide and merged an
// entire session into one datapoint per series. The chart was not misdrawing a
// fine reduction; it was drawing the one bucket it was sent. Fitting the axis
// client-side could not help, because the collapse happened before the response
// was built.
//
// Both halves matter. Fitting must recover the resolution, and an explicit
// window must keep dividing itself, so its buckets stay anchored where the
// caller put them.
func TestGetMetric_FitToDataSpansTheData(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	// Ten minutes of datapoints a minute apart, inside a window that spans
	// decades. That ratio is the whole point: at 8 target buckets the requested
	// window gives buckets years wide, and the data's own extent gives buckets
	// of about a minute.
	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	dps := make([]expHistTestDP, 0, 10)
	for i := range 10 {
		dps = append(dps, expHistTestDP{
			timestamp: base.Add(time.Duration(i) * time.Minute), scale: 0,
			zeroCount: 0, zeroThreshold: 0,
			posOffset: 0, posCounts: []uint64{uint64(i + 1)}, count: uint64(i + 1), sum: float64(i + 1),
		})
	}
	fixture := makeExpHistogramFixtureT("fit.duration", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	streamID := summaries[0]["id"].(string)

	// The unbounded window a "no choice was made" caller sends.
	start := int64(0)
	end := base.Add(48 * time.Hour).UnixNano()

	get := func(fit bool) map[string]any {
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetric(ctx, db, streamID, start, end, 8, nil, nil, 0, fit, 0, 0, nil, nil, 0)
		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		return m
	}
	buckets := func(m map[string]any) int {
		agg, _ := m["aggregate"].([]any)
		return len(agg)
	}

	asked := get(false)
	fitted := get(true)

	// The collapse, and its absence. One bucket is the symptom exactly: every
	// observation in the session, merged into a single column.
	assert.Equal(t, 1, buckets(asked),
		"dividing a decades-wide window must still merge the session into one bucket")
	assert.Greater(t, buckets(fitted), buckets(asked),
		"fitting to the data must recover resolution the requested window destroyed")

	// The reported window, so nobody has to infer it from bucketed timestamps.
	askedWindow, _ := asked["window"].(map[string]any)
	require.NotNil(t, askedWindow)
	assert.Equal(t, false, askedWindow["fittedToData"])
	assert.Nil(t, askedWindow["startNs"], "an unfitted window is the caller's own")

	fittedWindow, _ := fitted["window"].(map[string]any)
	require.NotNil(t, fittedWindow)
	assert.Equal(t, true, fittedWindow["fittedToData"])
	assert.Equal(t, strconv.FormatInt(base.UnixNano(), 10), fittedWindow["startNs"],
		"the fitted window starts at the first datapoint")
	assert.Equal(t, strconv.FormatInt(base.Add(9*time.Minute).UnixNano(), 10), fittedWindow["endNs"],
		"the fitted window ends at the last datapoint")

	// An explicit window is a request, and keeps dividing itself. Without this
	// the fix would be "always fit", which silently discards the emptiness that
	// says a metric stopped reporting.
	tight := base.Add(-6 * time.Hour).UnixNano()
	rawTight, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, streamID, tight, end, 8, nil, nil, 0, false, 0, 0, nil, nil, 0)
	})
	require.NoError(t, err)
	var mTight map[string]any
	require.NoError(t, json.Unmarshal(rawTight, &mTight))
	assert.Positive(t, buckets(mTight), "an explicit window still buckets")
}

// TestGetMetric_MergedRowsCarryTheirBucket covers a merged histogram row
// reporting the bucket it is, rather than the newest datapoint inside it.
//
// The merge grouped by bucket_start and then reported max(timestamp), so rows
// the store had put in one bucket came back on their constituents' clocks --
// timestamps seconds apart inside a bucket tens of seconds wide, and two series
// merged over the same bucket disagreeing about when that bucket was. Nothing
// caught it: the client re-buckets into its own columns on the way to the
// chart, so the wire could be wrong without the picture looking wrong.
//
// The window is explicit and the target chosen so the ladder in bucket_width_ns
// lands on exactly 10s: 60s / 6 admits 10s and refuses 5s. Pinning the width is
// what lets the assertion be "on the grid" rather than "self-consistent".
func TestGetMetric_MergedRowsCarryTheirBucket(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	// 13:00:00 UTC is a whole minute, so it is already on the 10s grid and the
	// expected bucket starts are base, base+10s, base+20s exactly.
	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	at := func(d time.Duration, driver string) histTestDP {
		return histTestDP{
			timestamp: base.Add(d),
			attrs:     map[string]string{"driver": driver},
			bounds:    []float64{1, 2},
			counts:    []uint64{1, 0, 0},
			count:     1, sum: 0.5, min: 0.5, max: 0.5,
		}
	}
	// Two series, several datapoints per bucket, none of them landing on a
	// boundary -- so any surviving constituent timestamp shows up immediately.
	fixture := makeHistogramFixtureT("bucket.duration", pmetric.AggregationTemporalityDelta, []histTestDP{
		at(1*time.Second, "ALO"),
		at(7*time.Second, "ALO"),
		at(11*time.Second, "ALO"),
		at(3*time.Second, "VER"),
		at(25*time.Second, "VER"),
	})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	streamID := summaries[0]["id"].(string)

	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, streamID,
			base.UnixNano(), base.Add(60*time.Second).UnixNano(),
			6, nil, nil, 0, false, 0, 0, nil, nil, 0)
	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))

	const widthNs = int64(10 * time.Second)
	bucketOf := func(d time.Duration) int64 { return base.Add(d).UnixNano() }

	series, _ := m["timeseries"].([]any)
	require.NotEmpty(t, series)

	perSeries := map[string][]int64{}
	for _, s := range series {
		ts := s.(map[string]any)
		key := ts["attributesKey"].(string)
		for _, d := range ts["datapoints"].([]any) {
			v, err := strconv.ParseInt(d.(map[string]any)["timestamp"].(string), 10, 64)
			require.NoError(t, err)
			perSeries[key] = append(perSeries[key], v)
		}
	}

	var all []int64
	for _, stamps := range perSeries {
		for _, v := range stamps {
			all = append(all, v)
			assert.Zero(t, v%widthNs,
				"a merged row must sit on the bucket grid, not on a constituent's timestamp")
		}
	}
	require.NotEmpty(t, all)

	// The exact buckets, so "on the grid" cannot be satisfied by rounding to
	// the wrong one. Bucket 0 holds datapoints from both series.
	distinct := map[int64]bool{}
	for _, v := range all {
		distinct[v] = true
	}
	assert.Equal(t, map[int64]bool{
		bucketOf(0): true, bucketOf(10 * time.Second): true, bucketOf(20 * time.Second): true,
	}, distinct)

	// Both series merged over bucket 0 must name it identically -- the point of
	// the grid is that rows from different series line up on it.
	require.Len(t, perSeries, 2)
	for key, stamps := range perSeries {
		assert.Contains(t, stamps, bucketOf(0), "series %s must report bucket 0 by its start", key)
	}

	// The aggregate is the same merge across series and has to agree, or the
	// heatmap's columns and the rows beneath them sit on different clocks.
	for _, b := range m["aggregate"].([]any) {
		v, err := strconv.ParseInt(b.(map[string]any)["timestamp"].(string), 10, 64)
		require.NoError(t, err)
		assert.Zero(t, v%widthNs, "an aggregate bucket must sit on the grid too")
	}
}

// TestGetMetricAggregate covers the aggregate-only call, which exists so a
// legend toggle does not re-ship the per-series payload.
//
// It must answer the same question as GetMetric's aggregate field for the same
// arguments -- it runs the same query and keeps one field, and this is what
// stops those two drifting.
func TestGetMetricAggregate(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()

	base := time.Now().Add(-time.Minute)
	var dps []expHistTestDP
	for si := 0; si < 3; si++ {
		dps = append(dps, expHistTestDP{
			timestamp: base.Add(time.Duration(si) * time.Second),
			attrs:     map[string]string{"route": fmt.Sprintf("/r%d", si)},
			scale:     0, zeroCount: 0, zeroThreshold: 0,
			posOffset: 0, posCounts: []uint64{2, 3},
			count: 5, sum: 10,
		})
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn,
			makeExpHistogramFixtureT("agg.only", pmetric.AggregationTemporalityDelta, dps),
			s.FlushedIDs())
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	streamID := summaries[0]["id"].(string)
	end := time.Now().UnixNano() + int64(time.Hour)

	// Same arguments to both calls.
	full, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, streamID, 0, end, 1, nil, []float64{0.5}, 0, false, 0, 0, nil, nil, 0)
	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(full, &m))
	fromFull, _ := json.Marshal(m["aggregate"])

	only, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricAggregate(ctx, db, streamID, 0, end, 1, nil, []float64{0.5}, 0, false, 0, nil)
	})
	require.NoError(t, err)

	// The call now carries both aggregates -- the histogram merge and the scalar
	// pools -- because one endpoint serves both metric shapes and the caller
	// should not have to ask twice to learn which it got.
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(only, &envelope))
	fromOnly, _ := json.Marshal(envelope["aggregate"])
	assert.JSONEq(t, string(fromFull), string(fromOnly),
		"the aggregate-only call must match GetMetric's aggregate field")

	// A histogram has no scalar pools: scalar_dps is Gauge and Sum only, so
	// nothing reaches the fold.
	scalar, _ := envelope["scalarAggregate"].(map[string]any)
	require.NotNil(t, scalar, "the field is present even when it is empty")
	assert.Empty(t, scalar["all"], "a histogram contributes no scalar pool")
	assert.Empty(t, scalar["selected"])

	// And it honours the selection, which is the reason it exists.
	var agg []map[string]any
	fromOnlyRaw, _ := json.Marshal(envelope["aggregate"])
	require.NoError(t, json.Unmarshal(fromOnlyRaw, &agg))
	require.NotEmpty(t, agg)
	allCount := agg[0]["count"].(float64)

	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricAggregate(ctx, db, streamID, 0, end, 1, []string{}, nil, 0, false, 0, nil)
	})
	require.NoError(t, err)
	var emptyEnvelope map[string]any
	require.NoError(t, json.Unmarshal(raw, &emptyEnvelope))
	assert.Nil(t, emptyEnvelope["aggregate"],
		"an empty selection aggregates no series")

	assert.Positive(t, allCount, "nil selection aggregates every series")
}
