package metrics_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
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
		return metrics.GetMetric(ctx, db, id, 0, maxNano, 0, nil, nil, 0, false, 0, 0, nil, "", nil, 0)
	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

// TestMetricSuite runs tests on ingested metrics using SearchMetrics (DB-generated JSON).
func TestMetricSuite(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

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
	t.Parallel()
	t.Run("removes a single Gauge by name+unit+scope+service", func(t *testing.T) {
		s, ctx := storetest.New(t)

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
		s, ctx := storetest.New(t)

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
		s, ctx := storetest.New(t)

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
		s, ctx := storetest.New(t)

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
		s, ctx := storetest.New(t)

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
		s, ctx := storetest.New(t)

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
		s, ctx := storetest.New(t)

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
	t.Parallel()
	s, ctx := storetest.New(t)

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
	t.Parallel()
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
			s, ctx := storetest.New(t)

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
	t.Parallel()
	s, ctx := storetest.New(t)

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
	t.Parallel()
	s, ctx := storetest.New(t)

	err := s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, pmetric.NewMetrics(), s.FlushedIDs())
	})
	assert.NoError(t, err)

	metricList := searchMetricsAll(t, s, ctx)
	assert.Empty(t, metricList)
}

// TestClearMetrics verifies that all metrics can be cleared, including child rows.
func TestClearMetrics(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

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
	t.Parallel()
	s, ctx := storetest.New(t)

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
	t.Parallel()
	s, ctx := storetest.New(t)

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
	t.Parallel()
	s, _ := storetest.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, createTestMetricsPdataN(1), s.FlushedIDs())
	})
	require.ErrorIs(t, err, context.Canceled)
}

func TestIngest_CanceledDuringIngest(t *testing.T) {
	t.Parallel()
	s, _ := storetest.New(t)

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
	t.Parallel()
	t.Run("Gauge", func(t *testing.T) {
		s, ctx := storetest.New(t)

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
		s, ctx := storetest.New(t)

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
	t.Parallel()
	s, ctx := storetest.New(t)

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
	t.Parallel()
	s, ctx := storetest.New(t)

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
		return metrics.GetMetric(ctx, db, streamID, 0, time.Now().UnixNano()+int64(time.Hour), 0, nil, nil, 0, false, 0, 0, nil, "", nil, 0)
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
	t.Parallel()
	s, ctx := storetest.New(t)

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
	t.Parallel()
	s, ctx := storetest.New(t)

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
			time.Now().UnixNano()+int64(time.Hour), 0, nil, nil, 0, false, 0, 0, nil, "", nil, 0)
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
	t.Parallel()
	s, ctx := storetest.New(t)

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
		return metrics.GetMetric(ctx, db, streamID, 0, time.Now().UnixNano()+int64(time.Hour), 0, nil, nil, 0, false, 0, 0, nil, "", nil, 0)
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
	t.Parallel()
	s, ctx := storetest.New(t)

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
			time.Now().UnixNano()+int64(time.Hour), 1, nil, nil, 0, false, 0, 0, nil, "", nil, 0)
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
	t.Parallel()
	s, ctx := storetest.New(t)

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
			1, nil, nil, 0, false, 0, 0, nil, "", nil, 0)
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
	t.Parallel()
	s, ctx := storetest.New(t)

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
			0, nil, nil, 0, true, 12, 0, nil, "", nil, 0)
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

// TestGetMetric_RateSlopeAndStats covers the two numbers derived from the
// drawn rate line: the slope arriving at each drawn point, and the line's
// extremes for the rate view's badges.
//
// Both used to be client arithmetic over the drawn points. The store now
// states the drawn sequence once -- an empty bucket draws a zero, a bucket
// with samples but no rate draws nothing -- and derives both from it, so the
// overlay, the badges and the line cannot disagree.
func TestGetMetric_RateSlopeAndStats(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	// A counter climbing a minute at a time with a five-minute silence: the
	// gap draws zeros, and the slope into and out of the gap spans them.
	var dps []sumTestDP
	attrs := map[string]string{"pod": "a"}
	for i := range 3 {
		dps = append(dps, sumTestDP{
			timestamp: base.Add(time.Duration(i) * time.Minute),
			value:     float64(i + 1),
			attrs:     attrs,
		})
	}
	for i := range 3 {
		dps = append(dps, sumTestDP{
			timestamp: base.Add(time.Duration(8+i) * time.Minute),
			value:     float64(10 * (i + 1)),
			attrs:     attrs,
		})
	}
	fixture := makeSumFixtureT("slope.total", pmetric.AggregationTemporalityCumulative, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, summaries[0]["id"].(string),
			base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano(),
			0, nil, nil, 0, true, 12, 0, nil, "", nil, 0)
	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	series := m["timeseries"].([]any)
	require.Len(t, series, 1)
	ts := series[0].(map[string]any)
	views := ts["views"].([]any)
	require.NotEmpty(t, views)

	// Recompute the drawn sequence from the response's own buckets and check
	// every slope against it. The test may do arithmetic; the client may not.
	type drawnPoint struct {
		startNs float64
		value   float64
	}
	var drawn []drawnPoint
	var emptyBuckets, firstRateNull int
	for _, v := range views {
		b := v.(map[string]any)
		start, err := strconv.ParseFloat(b["bucketStart"].(string), 64)
		require.NoError(t, err)
		sampleCount := b["sampleCount"].(float64)
		rate, hasRate := b["rate"].(float64)
		slope, hasSlope := b["slope"].(float64)

		if sampleCount > 0 && !hasRate {
			// A series' first bucket: no predecessor, nothing drawn, no slope.
			firstRateNull++
			assert.False(t, hasSlope, "an undrawn bucket has no slope")
			continue
		}
		value := 0.0
		if sampleCount > 0 {
			value = rate
		} else {
			emptyBuckets++
		}
		if len(drawn) == 0 {
			assert.False(t, hasSlope, "the first drawn point has no arriving segment")
		} else {
			prev := drawn[len(drawn)-1]
			want := (value - prev.value) / ((start - prev.startNs) / 1e9)
			require.True(t, hasSlope, "every drawn point after the first carries a slope")
			assert.InDelta(t, want, slope, 1e-9,
				"slope is Δrate over the seconds since the previous drawn point")
		}
		drawn = append(drawn, drawnPoint{start, value})
	}
	require.Positive(t, emptyBuckets, "the silence draws zeros, and they are in the sequence")
	require.Positive(t, firstRateNull, "the cumulative first bucket is undrawn")

	// The badges' numbers are the drawn line's, gap zeros included.
	rs := ts["rateStats"].(map[string]any)
	minWant, maxWant, sum := drawn[0].value, drawn[0].value, 0.0
	for _, d := range drawn {
		minWant, maxWant, sum = min(minWant, d.value), max(maxWant, d.value), sum+d.value
	}
	assert.InDelta(t, minWant, rs["min"], 1e-9, "gap zeros pull the minimum to the floor the chart shows")
	assert.InDelta(t, maxWant, rs["max"], 1e-9)
	assert.InDelta(t, sum/float64(len(drawn)), rs["avg"], 1e-9)

	// The pools carry slope by the same rule; one bucket proves the field.
	pools := m["scalarAggregate"].(map[string]any)
	all := pools["all"].([]any)
	require.NotEmpty(t, all)
	var poolSlopes int
	for _, v := range all {
		if _, ok := v.(map[string]any)["slope"].(float64); ok {
			poolSlopes++
		}
	}
	assert.Positive(t, poolSlopes, "the pooled line's segments carry slopes too")
}

// TestCumulativeHistogramMerge_DifferencesAcrossBuckets covers the reduction of
// a cumulative histogram whose buckets hold one datapoint each.
//
// A cumulative reading is a running total, so a bucket's activity is measured
// against the reading *before* it -- which is in the previous bucket whenever
// the requested width is at or below the reporting cadence. Differencing within
// the bucket instead has nothing to subtract and reports zero activity for a
// series that is plainly counting, and it does so for every ordinary request:
// the caller asks for a bucket count, not a width, so any cadence at or below
// the resulting width lands here.
//
// The scalar path already answers this correctly (scalar_lagged differences
// each datapoint against its predecessor in the series, then buckets); this
// pins the histogram merge to the same rule.
func TestCumulativeHistogramMerge_DifferencesAcrossBuckets(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	bounds := []float64{10, 20, 30}
	attrs := map[string]string{"pod": "a"}

	// Running totals: two more observations every minute, all in the first
	// bucket of the vector. Activity per minute is 2, forever.
	var dps []histTestDP
	for i := range 6 {
		n := uint64(2 * (i + 1))
		dps = append(dps, histTestDP{
			timestamp: base.Add(time.Duration(i) * time.Minute),
			attrs:     attrs,
			bounds:    bounds,
			counts:    []uint64{n, 0, 0, 0},
			count:     n,
			sum:       float64(n) * 5,
			min:       5, max: 5,
		})
	}
	fixture := makeHistogramFixtureT("climb.hist", pmetric.AggregationTemporalityCumulative, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	streamID := summaries[0]["id"].(string)

	get := func(targetBuckets int64) []map[string]any {
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetric(ctx, db, streamID,
				base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano(),
				targetBuckets, nil, nil, 0, true, 0, 0, nil, "", nil, 0)
		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		ts := m["timeseries"].([]any)
		require.Len(t, ts, 1)
		var out []map[string]any
		for _, d := range ts[0].(map[string]any)["datapoints"].([]any) {
			out = append(out, d.(map[string]any))
		}
		return out
	}

	// Unreduced: the running totals themselves, untouched.
	unreduced := get(0)
	require.Len(t, unreduced, 6)
	assert.Equal(t, float64(12), unreduced[0]["count"],
		"without a reduction a cumulative datapoint keeps its running total")

	// Reduced onto minute buckets, which is the cadence: every bucket holds one
	// datapoint and must be differenced against the previous bucket's.
	for _, targetBuckets := range []int64{6, 60, 600} {
		merged := get(targetBuckets)
		require.NotEmpty(t, merged, "targetBuckets=%d", targetBuckets)

		// The first reading establishes the baseline and measures no interval,
		// exactly as the scalar path drops a series' first datapoint.
		assert.Len(t, merged, 5,
			"targetBuckets=%d: six readings describe five intervals", targetBuckets)

		var total float64
		for _, dp := range merged {
			assert.Equal(t, float64(2), dp["count"],
				"targetBuckets=%d: each minute adds two observations", targetBuckets)
			assert.Equal(t, float64(10), dp["sum"],
				"targetBuckets=%d: sum is differenced with count", targetBuckets)
			buckets := dp["bucketCounts"].([]any)
			var vec float64
			for _, c := range buckets {
				vec += c.(float64)
			}
			assert.Equal(t, dp["count"], vec,
				"targetBuckets=%d: the vector agrees with the count on the same row",
				targetBuckets)
			total += dp["count"].(float64)
		}
		assert.Equal(t, float64(10), total,
			"targetBuckets=%d: the intervals sum to the counter's climb (12-2)",
			targetBuckets)
	}
}

// TestCumulativeHistogramMerge_ResetIsConsistentAcrossFields covers a counter
// restart inside a merged bucket.
//
// The reset rule -- a fall means the counter restarted, so the later reading is
// the activity since the restart -- has to reach every field of the row from
// one decision. Applied per field it split: the scalars clamped with
// greatest(max-min, 0) while the vectors detected the negative difference and
// fell back to the later slice, so one datapoint could claim more observations
// than its own buckets held. Nothing downstream can reconcile that, because the
// count badge and the quantiles read different fields of the same row.
func TestCumulativeHistogramMerge_ResetIsConsistentAcrossFields(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	bounds := []float64{10, 20, 30}
	attrs := map[string]string{"pod": "a"}
	// Climb to 105, restart, climb to 8 -- all inside one merged bucket.
	readings := []uint64{100, 105, 3, 8}
	var dps []histTestDP
	for i, n := range readings {
		dps = append(dps, histTestDP{
			timestamp: base.Add(time.Duration(i) * time.Second),
			attrs:     attrs,
			bounds:    bounds,
			counts:    []uint64{n, 0, 0, 0},
			count:     n,
			sum:       float64(n),
			min:       1, max: 1,
		})
	}
	fixture := makeHistogramFixtureT("reset.hist", pmetric.AggregationTemporalityCumulative, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, summaries[0]["id"].(string),
			base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano(),
			1, nil, nil, 0, false, 0, 0, nil, "", nil, 0)
	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	dpl := m["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any)
	require.Len(t, dpl, 1, "all four readings merge into one bucket")
	dp := dpl[0].(map[string]any)

	// 5 before the restart, then 3 (the reading itself), then 5 after.
	assert.Equal(t, float64(13), dp["count"],
		"activity is summed per reading, with the restart's own value counted once")

	var vec float64
	for _, c := range dp["bucketCounts"].([]any) {
		vec += c.(float64)
	}
	assert.Equal(t, dp["count"], vec,
		"the bucket vector and the count describe the same observations")
	assert.Equal(t, dp["count"], dp["sum"],
		"and so does the sum, since every observation here has value 1")
}

// TestGetMetric_WindowSummaryIsOneBucket covers the request that asks a single
// question about a whole span: one bucket, not "about one bucket".
//
// The ladder cannot express it. bucket_width_ns snaps to a nameable width and
// bucketed_dps floors to absolute boundaries -- deliberately, so a chart's
// columns stay put while the reader pans -- so a span that starts mid-rung
// straddles two or three of them. The caller then reads the first and reports
// it as the window: measured on the reference stream, 61% of the observations
// under a p50 that belonged to the earlier fragment.
//
// Absolute boundaries are right for a chart and wrong for a summary, which is
// why one bucket is a different request rather than a smaller number of them.
func TestGetMetric_WindowSummaryIsOneBucket(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	// 10:20 to 12:00 -- a 100 minute span, which the ladder serves with the
	// 1 hour rung, so absolute flooring splits it across 10:00, 11:00 and 12:00.
	base := time.Date(2026, 5, 24, 10, 20, 0, 0, time.UTC)
	bounds := []float64{10, 20, 30}
	attrs := map[string]string{"pod": "a"}
	var dps []histTestDP
	for i := range 21 {
		dps = append(dps, histTestDP{
			timestamp: base.Add(time.Duration(i*5) * time.Minute),
			attrs:     attrs,
			bounds:    bounds,
			counts:    []uint64{1, 1, 0, 0},
			count:     2,
			sum:       25,
			min:       5, max: 15,
		})
	}
	fixture := makeHistogramFixtureT("window.hist", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricAggregate(ctx, db, summaries[0]["id"].(string),
			base.Add(-time.Hour).UnixNano(), base.Add(3*time.Hour).UnixNano(),
			1, nil, []float64{0.5}, 0, true, 0, nil, "")
	})
	require.NoError(t, err)
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(raw, &envelope))
	agg, _ := envelope["aggregate"].([]any)

	require.Len(t, agg, 1,
		"one bucket means one bucket: the caller reads [0] and calls it the window")
	bucket := agg[0].(map[string]any)

	// Every observation, not the share that fell in the first ladder rung.
	assert.Equal(t, float64(42), bucket["count"],
		"21 datapoints of 2 observations each")
	assert.Equal(t, float64(525), bucket["sum"])
	var vec float64
	for _, c := range bucket["bucketCounts"].([]any) {
		vec += c.(float64)
	}
	assert.Equal(t, bucket["count"], vec,
		"the vector describes the same observations as the count")

	// The quantile is the window's, which is the reason a summary cannot be
	// repaired by adding buckets up client-side: quantiles do not sum.
	q, _ := bucket["quantiles"].(map[string]any)
	require.NotNil(t, q)
	assert.NotNil(t, q["0.5"], "a window p50 over every observation in the span")
}

// TestGetMetric_DatapointLimitMatchesResponseOrder pins the agreement between
// the two sides of "the first N series".
//
// The client cannot name the series it wants on a first visit -- it picks them
// from the response -- so it sends a limit and checks the first N of what comes
// back. That only works if the store's rank and the response's order name the
// same series. They did not on the merge path: the rank read raw datapoint
// timestamps while the projection replaces a merged row's timestamp with its
// bucket start, so a reduced histogram shipped one set and the client drew
// another. Measured on a 21-series histogram: three checked series arrived with
// no datapoints, and three that were shipped theirs were never drawn.
func TestGetMetric_DatapointLimitMatchesResponseOrder(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	bounds := []float64{10, 20, 30}
	// Staggered start times, so raw recency and bucket recency disagree: every
	// series' last datapoint lands in the same merged bucket, but their raw
	// timestamps differ by seconds.
	var dps []histTestDP
	for series := range 8 {
		attrs := map[string]string{"pod": fmt.Sprintf("pod-%d", series)}
		for i := range 4 {
			dps = append(dps, histTestDP{
				timestamp: base.
					Add(time.Duration(i) * time.Minute).
					Add(time.Duration(series) * time.Second),
				attrs:  attrs,
				bounds: bounds,
				counts: []uint64{1, 1, 0, 0},
				count:  2, sum: 25, min: 5, max: 15,
			})
		}
	}
	fixture := makeHistogramFixtureT("order.hist", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	const limit = 3
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, summaries[0]["id"].(string),
			base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano(),
			// Reduced, which is what puts merged rows on bucket timestamps.
			20, nil, nil, 0, true, 0, 0, nil, "", nil, limit)
	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	series := m["timeseries"].([]any)
	require.Len(t, series, 8)

	firstN := map[string]bool{}
	for _, entry := range series[:limit] {
		firstN[entry.(map[string]any)["attributesKey"].(string)] = true
	}
	shipped := map[string]bool{}
	for _, entry := range series {
		ts := entry.(map[string]any)
		if len(ts["datapoints"].([]any)) > 0 {
			shipped[ts["attributesKey"].(string)] = true
		}
	}
	require.Len(t, shipped, limit, "the limit ships exactly N series")
	assert.Equal(t, firstN, shipped,
		"the series the store ships datapoints for are the first N the response "+
			"lists, so the client's \"check the first N\" checks series that have data")
}

// TestGetMetric_BoundsMismatchIsReported covers the one histogram disagreement
// that cannot be reconciled: two datapoints in a bucket carrying different
// explicit bounds.
//
// Scales can be reconciled -- downscale_exp_buckets moves both onto the coarser
// one -- but boundaries cannot: there is no transformation that turns [10,20,30]
// into [5,50,500] without inventing observations. So the merge is refused.
//
// Refusing is right; refusing silently is not. The bucket simply vanished, and
// a missing bucket is indistinguishable from a stretch with no data -- so an
// exporter that changed its histogram configuration mid-window, which is a real
// finding for anyone debugging one, looked like an idle period.
func TestGetMetric_BoundsMismatchIsReported(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	attrs := map[string]string{"pod": "a"}
	// Same bucket, two boundary sets: the SDK's view was reconfigured between
	// these two readings.
	dps := []histTestDP{
		{
			timestamp: base, attrs: attrs,
			bounds: []float64{10, 20, 30}, counts: []uint64{1, 1, 0, 0},
			count: 2, sum: 25, min: 5, max: 15,
		},
		{
			timestamp: base.Add(time.Second), attrs: attrs,
			bounds: []float64{5, 50, 500}, counts: []uint64{2, 0, 0, 0},
			count: 2, sum: 4, min: 2, max: 2,
		},
	}
	fixture := makeHistogramFixtureT("drift.hist", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	streamID := summaries[0]["id"].(string)

	get := func(targetBuckets int64) map[string]any {
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetric(ctx, db, streamID,
				base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano(),
				targetBuckets, nil, nil, 0, false, 0, 0, nil, "", nil, 0)
		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		return m
	}

	// Reduced: the merge is refused, the bucket is gone, and the response says
	// so instead of leaving the reader to notice a hole.
	reduced := get(1)
	mismatch, _ := reduced["boundsMismatch"].(map[string]any)
	require.NotNil(t, mismatch,
		"a refused merge is reported, not merely omitted")
	assert.Equal(t, float64(1), mismatch["seriesBuckets"],
		"one (series, bucket) merge refused")
	assert.Equal(t, float64(1), mismatch["aggregateBuckets"],
		"and the cross-series merge over the same bucket, for the same reason")

	// The rows really are absent -- this is a report of a refusal, not a
	// warning attached to a merged-anyway row. With every bucket refused the
	// series has nothing left to carry, so it drops out of the response
	// entirely; the report is the only thing that says why.
	assert.Empty(t, reduced["timeseries"],
		"a series whose every bucket was refused has nothing to show")
	assert.Nil(t, reduced["aggregate"],
		"and the cross-series merge over those buckets is refused with it, "+
			"rather than summing vectors measured against different boundaries")

	// Unreduced: nothing is merged, so nothing is refused, and the datapoints
	// arrive exactly as they were recorded -- differing bounds and all.
	unreduced := get(0)
	assert.Nil(t, unreduced["boundsMismatch"],
		"no merge, no refusal to report")
	uts := unreduced["timeseries"].([]any)
	require.Len(t, uts, 1)
	assert.Len(t, uts[0].(map[string]any)["datapoints"], 2,
		"both readings survive when nothing is being merged")
}

// TestGetMetric_ViewGridRespectsCadence covers the grid the Sum, Average and
// Rate views are drawn on: it may not divide finer than the data arrives.
//
// The ladder picks a width from the span and a bucket count and knows nothing
// about cadence, so asking for 120 buckets of a series that reported 20 times
// gives buckets narrower than the gaps between readings. scalar_view_spine then
// emits every one of them, and Sum and Rate draw an empty bucket as the zero it
// honestly is -- producing a sawtooth that is a property of the grid, not of the
// data, and is indistinguishable from a series that really did stop and start.
//
// The cap is on the bucket count rather than the width, so the answer stays a
// ladder rung: a reader can name a 1-minute boundary and cannot name a
// 30.5-second one.
func TestGetMetric_ViewGridRespectsCadence(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	// Once a minute for twenty minutes, with no gaps: every bucket the grid
	// produces should hold a reading.
	var dps []sumTestDP
	attrs := map[string]string{"pod": "a"}
	for i := range 20 {
		dps = append(dps, sumTestDP{
			timestamp: base.Add(time.Duration(i) * time.Minute),
			value:     float64(i + 1),
			attrs:     attrs,
		})
	}
	fixture := makeSumFixtureT("sparse.total", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, summaries[0]["id"].(string),
			base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano(),
			// 120 view buckets over 20 readings: six times more buckets than
			// the series has intervals.
			0, nil, nil, 0, true, 120, 0, nil, "", nil, 0)
	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	views := m["timeseries"].([]any)[0].(map[string]any)["views"].([]any)
	require.NotEmpty(t, views)

	var empty int
	var starts []int64
	for _, v := range views {
		b := v.(map[string]any)
		if b["sampleCount"].(float64) == 0 {
			empty++
		}
		start, err := strconv.ParseInt(b["bucketStart"].(string), 10, 64)
		require.NoError(t, err)
		starts = append(starts, start)
	}

	assert.Zero(t, empty,
		"a series reporting on a steady cadence with no gaps should produce no "+
			"empty buckets: every empty one here would draw as a zero the data "+
			"does not contain")
	assert.LessOrEqual(t, len(views), 20,
		"no more buckets than the series has reporting intervals")

	// The width is still a ladder rung, which is what makes a boundary nameable.
	require.Greater(t, len(starts), 1)
	width := starts[1] - starts[0]
	assert.Equal(t, int64(time.Minute), width,
		"one minute, the rung at the reporting cadence -- not the raw median gap")

	// A dense series is not coarsened by the cap: it asks for fewer buckets than
	// its cadence allows, so the ladder answers unchanged.
	dense := []sumTestDP{}
	for i := range 600 {
		dense = append(dense, sumTestDP{
			timestamp: base.Add(time.Duration(i) * time.Second),
			value:     1,
			attrs:     map[string]string{"pod": "dense"},
		})
	}
	denseFixture := makeSumFixtureT("dense.total", pmetric.AggregationTemporalityDelta, dense)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, denseFixture, s.FlushedIDs())
	}))
	all := searchMetricsAll(t, s, ctx)
	var denseID string
	for _, sum := range all {
		if sum["name"] == "dense.total" {
			denseID = sum["id"].(string)
		}
	}
	require.NotEmpty(t, denseID)
	denseRaw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, denseID,
			base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano(),
			0, nil, nil, 0, true, 120, 0, nil, "", nil, 0)
	})
	require.NoError(t, err)
	var dm map[string]any
	require.NoError(t, json.Unmarshal(denseRaw, &dm))
	denseViews := dm["timeseries"].([]any)[0].(map[string]any)["views"].([]any)
	assert.Greater(t, len(denseViews), 20,
		"a series reporting every second keeps the resolution it can support")
}

// TestGetMetric_CountsDescribeTheWindow pins the distinction between what the
// window holds and what the response carries.
//
// Datapoints are narrowed to the series being drawn and reduced besides, so
// counting the array that arrives answers "how much did I receive" while the
// reader is asking "how much is there". On a 22-series Gauge the header read
// 5,908 of 19,319, and twelve series showed a count of zero beside sparklines
// visibly full of data -- reading as series that had stopped reporting rather
// than ones this response did not carry.
func TestGetMetric_CountsDescribeTheWindow(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	var dps []sumTestDP
	for _, pod := range []string{"a", "b", "c", "d"} {
		for i := range 25 {
			dps = append(dps, sumTestDP{
				timestamp: base.Add(time.Duration(i) * time.Minute),
				value:     float64(i),
				attrs:     map[string]string{"pod": pod},
			})
		}
	}
	fixture := makeSumFixtureT("counts.total", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	// Narrowed hard: datapoints for one series of four.
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, summaries[0]["id"].(string),
			base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano(),
			0, nil, nil, 0, true, 0, 0, nil, "", nil, 1)
	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	series := m["timeseries"].([]any)
	require.Len(t, series, 4)

	var shipped, counted int
	for _, entry := range series {
		ts := entry.(map[string]any)
		shipped += len(ts["datapoints"].([]any))
		counted += int(ts["datapointCount"].(float64))

		// Every series knows its own total and when it last reported, whether
		// or not this response carried a single one of its datapoints.
		assert.Equal(t, float64(25), ts["datapointCount"],
			"each series holds 25 datapoints in the window")
		assert.NotNil(t, ts["lastSeenNs"],
			"including the ones whose datapoints were narrowed away")
	}

	assert.Equal(t, 100, counted, "the per-series counts describe the window")
	assert.Equal(t, 25, shipped, "while the response carried one series' worth")
	assert.Equal(t, float64(100), m["datapointCount"],
		"and the metric's own total agrees with the sum of its series")

	// The metric's last-seen does not depend on which series shipped.
	lastSeen, ok := m["lastSeenNs"].(string)
	require.True(t, ok, "last seen is present even under narrowing")
	ns, err := strconv.ParseInt(lastSeen, 10, 64)
	require.NoError(t, err)
	assert.Equal(t, base.Add(24*time.Minute).UnixNano(), ns,
		"the most recent datapoint in the window, across every series")

	// A reduced histogram is where "the window" and "the response" diverge
	// most: the merge replaces a bucket's datapoints with one merged row, so
	// counting the rows the projection carries reports buckets and calls them
	// datapoints. The count has to come from before the merge.
	bounds := []float64{10, 20, 30}
	var hdps []histTestDP
	for i := range 30 {
		hdps = append(hdps, histTestDP{
			timestamp: base.Add(time.Duration(i) * time.Minute),
			attrs:     map[string]string{"pod": "h"},
			bounds:    bounds,
			counts:    []uint64{1, 1, 0, 0},
			count:     2, sum: 25, min: 5, max: 15,
		})
	}
	hfixture := makeHistogramFixtureT("counts.hist", pmetric.AggregationTemporalityDelta, hdps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, hfixture, s.FlushedIDs())
	}))
	var histID string
	for _, sum := range searchMetricsAll(t, s, ctx) {
		if sum["name"] == "counts.hist" {
			histID = sum["id"].(string)
		}
	}
	require.NotEmpty(t, histID)

	hraw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		// Reduced to a handful of buckets, so the merge collapses 30 datapoints
		// into far fewer rows.
		return metrics.GetMetric(ctx, db, histID,
			base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano(),
			5, nil, nil, 0, true, 0, 0, nil, "", nil, 0)
	})
	require.NoError(t, err)
	var hm map[string]any
	require.NoError(t, json.Unmarshal(hraw, &hm))
	hts := hm["timeseries"].([]any)[0].(map[string]any)
	merged := len(hts["datapoints"].([]any))

	assert.Equal(t, float64(30), hts["datapointCount"],
		"the datapoints the window holds, not the merged rows standing in for them")
	assert.Less(t, merged, 30,
		"the merge really did collapse them, or this proves nothing")
	assert.Equal(t, float64(30), hm["datapointCount"],
		"and the metric's total counts datapoints too")
}

// TestGetMetric_Deterministic pins the answer to a question the store must
// only have one answer to: the same request against the same data returns the
// same bytes.
//
// The fixture is built from ties, because ties are where determinism goes to
// die: a flat series whose datapoints share one timestamp and one value gives
// every election -- first, last, min, max -- nothing to distinguish rows by
// except the tiebreak, and a series with duplicate timestamps exercises the
// delta join and the list ordering the same way. Found live: the same request
// returned six different responses in six tries, differing in which datapoints
// the M4 election kept.
func TestGetMetric_Deterministic(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	var dps []sumTestDP
	// Twelve rows on one instant at one value: the election's worst case.
	flat := map[string]string{"pod": "flat"}
	for range 12 {
		dps = append(dps, sumTestDP{timestamp: base, value: 5, attrs: flat})
	}
	// A climb with a duplicated timestamp, which OTLP permits: two readings at
	// t1 with different values. The lag and the delta join must not care.
	dup := map[string]string{"pod": "dup"}
	for _, v := range []struct {
		min   int
		value float64
	}{{0, 1}, {1, 2}, {1, 3}, {2, 4}, {3, 5}} {
		dps = append(dps, sumTestDP{
			timestamp: base.Add(time.Duration(v.min) * time.Minute),
			value:     v.value,
			attrs:     dup,
		})
	}
	fixture := makeSumFixtureT("ties.total", pmetric.AggregationTemporalityCumulative, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	streamID := summaries[0]["id"].(string)

	get := func(targetBuckets int64) json.RawMessage {
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetric(ctx, db, streamID,
				base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano(),
				targetBuckets, nil, nil, 0, true, 4, 2, nil, "", nil, 0)
		})
		require.NoError(t, err)
		return raw
	}

	// The headline property: byte-identical across runs, reduced and not.
	for _, tb := range []int64{0, 1} {
		first := get(tb)
		for range 3 {
			require.Equal(t, string(first), string(get(tb)),
				"the same request must return the same bytes (targetBuckets=%d)", tb)
		}
	}

	seriesByPod := func(raw json.RawMessage) map[string][]map[string]any {
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		out := map[string][]map[string]any{}
		for _, entry := range m["timeseries"].([]any) {
			ts := entry.(map[string]any)
			var pod string
			for _, a := range ts["attributes"].([]any) {
				if a.(map[string]any)["key"] == "pod" {
					pod = a.(map[string]any)["value"].(string)
				}
			}
			for _, dp := range ts["datapoints"].([]any) {
				out[pod] = append(out[pod], dp.(map[string]any))
			}
		}
		return out
	}

	full := seriesByPod(get(0))
	require.Len(t, full["flat"], 12)
	require.Len(t, full["dup"], 5)

	// No row appears twice. The delta join used to match on (series, timestamp),
	// so each duplicated instant fanned out and the same datapoint shipped once
	// per delta at that instant.
	for pod, list := range full {
		seen := map[string]bool{}
		for _, dp := range list {
			id := dp["id"].(string)
			assert.False(t, seen[id], "%s: datapoint %s shipped twice", pod, id)
			seen[id] = true
		}
	}

	// Rows an ORDER BY cannot separate by timestamp arrive in id order --
	// verified on the flat series, where every row shares the instant.
	for i := 1; i < len(full["flat"]); i++ {
		assert.Less(t, full["flat"][i-1]["id"].(string), full["flat"][i]["id"].(string),
			"tied timestamps must order by id")
	}

	// The election's tie contract: among rows sharing a value, min and max
	// elect by id, and on the flat series first/last collapse onto the same
	// rows -- so exactly the smallest and largest id survive. DuckDB orders
	// UUIDs as their text sorts (verified against the CLI), so the expectation
	// is computable here.
	ids := make([]string, 0, 12)
	for _, dp := range full["flat"] {
		ids = append(ids, dp["id"].(string))
	}
	slices.Sort(ids)
	reduced := seriesByPod(get(1))
	var got []string
	for _, dp := range reduced["flat"] {
		got = append(got, dp["id"].(string))
	}
	slices.Sort(got)
	assert.Equal(t, []string{ids[0], ids[len(ids)-1]}, got,
		"a fully tied bucket elects the smallest and largest id, nothing else")
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
	t.Parallel()
	s, ctx := storetest.New(t)

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
				0, nil, nil, 0, true, 12, 8, nil, "", dpSeries, limit)
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
	t.Parallel()
	s, ctx := storetest.New(t)

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
				0, nil, nil, 0, true, 12, 0, selected, "", nil, 0)
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
	t.Parallel()
	s, ctx := storetest.New(t)

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
			0, nil, nil, 0, true, 0, buckets, nil, "", nil, 0)
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
			0, nil, nil, 0, true, 0, 0, nil, "", nil, 0)
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
	t.Parallel()
	s, ctx := storetest.New(t)

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
			1, nil, nil, 0, false, 0, 0, nil, "", nil, 0)
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
	t.Parallel()
	s, ctx := storetest.New(t)

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
			1, nil, nil, 0, false, 0, 0, nil, "", nil, 0)
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
	t.Parallel()
	s, ctx := storetest.New(t)

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
			return metrics.GetMetric(ctx, db, streamID, 0, end, 0, ids, nil, 0, false, 0, 0, nil, "", nil, 0)
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
	t.Parallel()
	s, ctx := storetest.New(t)

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
			return metrics.GetMetric(ctx, db, streamID, 0, end, 0, nil, qs, 0, false, 0, 0, nil, "", nil, 0)
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

// TestGetMetric_QuantileHandWorked pins the quantile arithmetic to values
// worked out by hand, through the real query rather than against macro
// literals.
//
// These cases lived in schema_test as literal calls to hist_quantile and
// exp_hist_quantile. Those macros are gone -- each computed its walk in a
// per-row sub-plan, and the walk now happens once, set-based, inside
// get_metric.sql's quantile CTEs -- so the arithmetic they pinned is pinned
// here instead, where it exercises the code that actually ships. The
// hand-derivations in the comments are unchanged from the originals.
func TestGetMetric_QuantileHandWorked(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	base := time.Now().Add(-time.Minute)

	histCases := []struct {
		name   string
		counts []uint64
		bounds []float64
		q      float64
		want   *float64
	}{
		// counts cumulative: 0, 50, 100, 100, 100. Total=100, p50 target=50.
		// First bucket with acc >= 50 is bucket 2 (1,2]. Linear interp at
		// fraction 1.0 gives 2.0 (the upper bound).
		{"p50 lands cleanly on a bucket boundary",
			[]uint64{0, 50, 50, 0, 0}, []float64{1, 2, 5, 10}, 0.5, ptrF(2.0)},
		// counts cumulative: 0, 10, 30, 60, 100. p95 target=95. Lands in the
		// unbounded tail (bucket 5), where lo=hi=10.0, so we clamp to the
		// last known bound.
		{"p95 in unbounded tail clamps to last known bound",
			[]uint64{0, 10, 20, 30, 40}, []float64{1, 2, 5, 10}, 0.95, ptrF(10.0)},
		// No bounds at all: nothing to interpolate against, so the requested
		// key is present and null rather than absent or an error.
		{"no bounds yields a null quantile",
			[]uint64{10}, nil, 0.5, nil},
		// A populated layout with nothing observed: same null, via the
		// all-zero guard rather than the missing-bounds one.
		{"all-zero counts yield a null quantile",
			[]uint64{0, 0, 0, 0, 0}, []float64{1, 2, 5, 10}, 0.5, nil},
	}
	expCases := []struct {
		name      string
		scale     int32
		zeroCount uint64
		posOffset int32
		posCounts []uint64
		negOffset int32
		negCounts []uint64
		q         float64
		want      *float64
		delta     float64
	}{
		// scale=0 -> base=2. pos_counts=[50,50] at offset=0: bucket1 (1,2]
		// cnt=50, bucket2 (2,4] cnt=50. Total=100. p50 target=50 -> first
		// bucket at acc>=50 is bucket1. loglin: 1 * (2/1)^(50/50) = 2.0.
		{"positive-only p50 (scale=0, two equal buckets)",
			0, 0, 0, []uint64{50, 50}, 0, nil, 0.5, ptrF(2.0), 1e-9},
		// All weight in zero bucket. p50 -> zero bucket -> 0.
		{"zero-only p50 returns 0",
			0, 100, 0, nil, 0, nil, 0.5, ptrF(0.0), 1e-9},
		// neg=[10,10], zero=20, pos=[10,10] at scale=0. Total=60. CDF acc:
		// 10, 20, 40, 50, 60. p50 target=30 -> first acc>=30 is the zero
		// bucket (acc=40). Loglin over [0,0] falls back to linear -> 0.
		{"symmetric three-region p50 lands in zero bucket",
			0, 20, 0, []uint64{10, 10}, 0, []uint64{10, 10}, 0.5, ptrF(0.0), 1e-9},
		// Reference dataset hand-computed in the planning notes. scale=2 ->
		// base = 2^(2^-2) = 2^0.25 ~= 1.189. Hand calc predicted p95 ~= 6.35.
		{"reference dataset p95 matches hand calc",
			2, 0, 6, []uint64{1200, 3800, 4200, 2100, 720, 280, 70, 22, 6, 2}, 0, nil,
			0.95, ptrF(6.349604207872798), 1e-6},
		// scale=0 -> buckets (1,2], (2,4], (4,8]. No zero region, so the
		// smallest observation is above 1: q=0 returns the first populated
		// bucket's lower edge, not 0.
		{"q=0 returns the first populated bucket's lower edge",
			0, 0, 0, []uint64{10, 20, 30}, 0, nil, 0.0, ptrF(1.0), 1e-9},
		// A genuine zero region: the zero bucket is populated, so it is
		// selected on its merits and 0 is the right answer.
		{"q=0 with a real zero region still returns 0",
			0, 5, 0, []uint64{10, 20, 30}, 0, nil, 0.0, ptrF(0.0), 1e-9},
		// An empty bucket in the middle must not absorb the target.
		{"empty interior bucket is skipped",
			0, 0, 0, []uint64{10, 0, 30}, 0, nil, 0.25, ptrF(2.0), 1e-9},
		// Unchanged by the filter -- guards the "no-op for q > 0" claim: the
		// cnt > 0 pick rule must skip only *empty* buckets, never shift a
		// quantile that lands past one.
		{"p50 unaffected by the empty-bucket skip",
			0, 0, 0, []uint64{10, 20, 30}, 0, nil, 0.5, ptrF(4.0), 1e-9},
		// Nothing observed anywhere: the key is present and null.
		{"empty exponential histogram yields a null quantile",
			0, 0, 0, nil, 0, nil, 0.5, nil, 0},
	}

	// One metric per case; each reads its single datapoint's quantile object
	// off the wire.
	for i, tc := range histCases {
		name := fmt.Sprintf("hw.hist.%d", i)
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			var total uint64
			for _, c := range tc.counts {
				total += c
			}
			return metrics.Ingest(ctx, conn,
				makeHistogramFixtureT(name, pmetric.AggregationTemporalityDelta, []histTestDP{{
					timestamp: base, bounds: tc.bounds, counts: tc.counts,
					count: total, sum: float64(total),
				}}), s.FlushedIDs())
		}))
	}
	for i, tc := range expCases {
		name := fmt.Sprintf("hw.exp.%d", i)
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn,
				makeExpHistogramFixtureT(name, pmetric.AggregationTemporalityDelta, []expHistTestDP{{
					timestamp: base, scale: tc.scale,
					zeroCount: tc.zeroCount, zeroThreshold: 0,
					posOffset: tc.posOffset, posCounts: tc.posCounts,
					negOffset: tc.negOffset, negCounts: tc.negCounts,
				}}), s.FlushedIDs())
		}))
	}

	end := time.Now().UnixNano() + int64(time.Hour)
	// Returns the quantile's value and whether its key survived to the wire.
	//
	// A quantile that cannot be computed does not arrive as a null-valued
	// key: datapoint_json assembles its output with json_merge_patch, and
	// RFC 7386 deletes null members recursively, so the key is stripped and
	// the client reads absence. That was already true before the quantile
	// walk moved into the CTEs -- the old macros returned null and the merge
	// stripped it the same way -- it was just never pinned at wire level:
	// the deleted schema_test cases asserted on the macros directly, one
	// layer below where the stripping happens.
	quantileOf := func(name string, q float64) (any, bool) {
		id := findMetricID(t, s, ctx, name)
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetric(ctx, db, id, 0, end, 0, nil, []float64{q}, 0, false, 0, 0, nil, "", nil, 0)
		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		dps := m["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any)
		require.Len(t, dps, 1)
		qobj, ok := dps[0].(map[string]any)["quantiles"].(map[string]any)
		require.True(t, ok, "quantiles must be an object even when empty")
		// Each call requests exactly one quantile, so read the object's only
		// entry rather than reconstruct its key -- DuckDB spells the key from
		// the double (0.0 -> "0.0") where Go's FormatFloat would say "0".
		require.LessOrEqual(t, len(qobj), 1)
		for _, v := range qobj {
			return v, true
		}
		return nil, false
	}

	for i, tc := range histCases {
		t.Run(tc.name, func(t *testing.T) {
			v, present := quantileOf(fmt.Sprintf("hw.hist.%d", i), tc.q)
			if tc.want == nil {
				assert.False(t, present, "an uncomputable quantile is stripped, not null")
			} else {
				require.True(t, present)
				assert.InDelta(t, *tc.want, v, 1e-9)
			}
		})
	}
	for i, tc := range expCases {
		t.Run(tc.name, func(t *testing.T) {
			v, present := quantileOf(fmt.Sprintf("hw.exp.%d", i), tc.q)
			if tc.want == nil {
				assert.False(t, present, "an uncomputable quantile is stripped, not null")
			} else {
				require.True(t, present)
				assert.InDelta(t, *tc.want, v, tc.delta)
			}
		})
	}

	// End-to-end merge, from schema_test's sum_bucket_vectors suite: two
	// series over bounds [1, 2, 5, 10]:
	//   A counts = [0, 50,  50,  0, 0]
	//   B counts = [0, 30,  50, 20, 0]
	//   sum     = [0, 80, 100, 20, 0]  total = 200
	// p50 target = 100. CDF acc = 0, 80, 180, 200, 200. First acc >= 100 is
	// bucket 3 (lo=2, hi=5, cnt=100, acc_prev=80). Linear interp:
	//   2 + (5 - 2) * (100 - 80) / 100 = 2.6
	// A duplicated quantile must be tolerated, not fatal. The wire object is
	// built with map(), which raises on a duplicate key, so getMetric dedupes
	// the request first -- json_group_object used to absorb this silently,
	// and a caller that could send [0.5, 0.5] yesterday still can.
	t.Run("duplicate quantiles are tolerated", func(t *testing.T) {
		id := findMetricID(t, s, ctx, "hw.exp.0")
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetric(ctx, db, id, 0, end, 0, nil,
				[]float64{0.5, 0.5, 0.95, 0.5}, 0, false, 0, 0, nil, "", nil, 0)
		})
		require.NoError(t, err, "a duplicated quantile must not fail the request")
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		dps := m["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any)
		qobj := dps[0].(map[string]any)["quantiles"].(map[string]any)
		assert.Len(t, qobj, 2, "duplicates collapse; distinct quantiles survive")
		assert.InDelta(t, 2.0, qobj["0.5"], 1e-9)
	})

	t.Run("aggregate p50 over merged series", func(t *testing.T) {
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn,
				makeHistogramFixtureT("hw.merge", pmetric.AggregationTemporalityDelta, []histTestDP{
					{timestamp: base, attrs: map[string]string{"route": "/a"},
						bounds: []float64{1, 2, 5, 10}, counts: []uint64{0, 50, 50, 0, 0}, count: 100, sum: 100},
					{timestamp: base, attrs: map[string]string{"route": "/b"},
						bounds: []float64{1, 2, 5, 10}, counts: []uint64{0, 30, 50, 20, 0}, count: 100, sum: 100},
				}), s.FlushedIDs())
		}))
		id := findMetricID(t, s, ctx, "hw.merge")
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricAggregate(ctx, db, id, 0, end, 1, nil, []float64{0.5}, 0, false, 1, nil, "")
		})
		require.NoError(t, err)
		var env map[string]any
		require.NoError(t, json.Unmarshal(raw, &env))
		agg := env["aggregate"].([]any)
		require.Len(t, agg, 1)
		q := agg[0].(map[string]any)["quantiles"].(map[string]any)
		assert.InDelta(t, 2.6, q["0.5"], 1e-9)
	})
}

func ptrF(v float64) *float64 { return &v }

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
	t.Parallel()
	s, ctx := storetest.New(t)

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
			time.Now().UnixNano()+int64(time.Hour), 1, nil, []float64{0.5}, 0, false, 0, 0, nil, "", nil, 0)
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
	t.Parallel()
	s, ctx := storetest.New(t)

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
			return metrics.GetMetric(ctx, db, streamID, start, end, 3, nil, nil, offsetNs, false, 0, 0, nil, "", nil, 0)
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
	t.Parallel()
	s, ctx := storetest.New(t)

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
			return metrics.GetMetric(ctx, db, streamID, start, end, 8, nil, nil, 0, fit, 0, 0, nil, "", nil, 0)
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
		return metrics.GetMetric(ctx, db, streamID, tight, end, 8, nil, nil, 0, false, 0, 0, nil, "", nil, 0)
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
	t.Parallel()
	s, ctx := storetest.New(t)

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
			6, nil, nil, 0, false, 0, 0, nil, "", nil, 0)
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
	t.Parallel()
	s, ctx := storetest.New(t)

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
		return metrics.GetMetric(ctx, db, streamID, 0, end, 1, nil, []float64{0.5}, 0, false, 0, 0, nil, "", nil, 0)
	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(full, &m))
	fromFull, _ := json.Marshal(m["aggregate"])

	only, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricAggregate(ctx, db, streamID, 0, end, 1, nil, []float64{0.5}, 0, false, 0, nil, "")
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
		return metrics.GetMetricAggregate(ctx, db, streamID, 0, end, 1, []string{}, nil, 0, false, 0, nil, "")
	})
	require.NoError(t, err)
	var emptyEnvelope map[string]any
	require.NoError(t, json.Unmarshal(raw, &emptyEnvelope))
	assert.Nil(t, emptyEnvelope["aggregate"],
		"an empty selection aggregates no series")

	assert.Positive(t, allCount, "nil selection aggregates every series")

	// The envelope carries these two fields and nothing else.
	//
	// It is built by SQL now rather than assembled in Go, so a stray field in
	// the projection would travel silently: the caller reads the two it knows
	// about and never notices the rest. That matters more than the bytes --
	// the projection is what the planner prunes from, so a field nobody reads
	// still drags its whole CTE chain into the plan.
	assert.ElementsMatch(t, []string{"aggregate", "scalarAggregate"},
		mapKeys(envelope),
		"the aggregate call must project exactly its envelope")

	// A scalar is the mirror image, and the reason the shape is chosen per
	// metric: no histogram merge to report, but real pools.
	t.Run("scalar metric reports pools and no merge", func(t *testing.T) {
		var sums []sumTestDP
		for i := 0; i < 4; i++ {
			sums = append(sums, sumTestDP{
				timestamp: base.Add(time.Duration(i) * time.Second),
				value:     float64(10 * (i + 1)),
				attrs:     map[string]string{"route": "/scalar"},
			})
		}
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn,
				makeSumFixtureT("agg.scalar", pmetric.AggregationTemporalityCumulative, sums),
				s.FlushedIDs())
		}))
		scalarID := findMetricID(t, s, ctx, "agg.scalar")

		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricAggregate(ctx, db, scalarID, 0, end, 1, nil, nil, 0, false, 4, nil, "")
		})
		require.NoError(t, err)
		var env map[string]any
		require.NoError(t, json.Unmarshal(raw, &env))

		assert.ElementsMatch(t, []string{"aggregate", "scalarAggregate"}, mapKeys(env))
		// Null, not empty: a Sum has no bucket vectors, so there is no merge to
		// report, and the client tells that apart from "merged to nothing".
		assert.Nil(t, env["aggregate"], "a scalar has no histogram merge")
		pools, _ := env["scalarAggregate"].(map[string]any)
		require.NotNil(t, pools)
		assert.NotEmpty(t, pools["all"], "a scalar contributes an All pool")
	})

	// The full projection keeps every field, which the shared template makes
	// worth asserting: one stray conditional would truncate the real response
	// and every test above would still pass, because none of them read it.
	t.Run("the full projection is unaffected", func(t *testing.T) {
		m := getMetricFullByName(t, s, ctx, "agg.only")
		for _, k := range []string{
			"id", "name", "description", "unit", "metricType", "resource",
			"scope", "timeseries", "aggregate", "scalarAggregate",
			"lastSeenNs", "datapointCount", "window",
		} {
			assert.Containsf(t, m, k, "getMetric must still emit %q", k)
		}
	})
}

// mapKeys is the sorted key set of a decoded JSON object.
func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestGetMetric_BucketsFollowTheZoneAcrossDST pins bucketing to the viewer's
// timezone rather than to one offset sampled from it.
//
// The caller used to send a single tzOffsetNs, captured in the browser at the
// instant of the request. That is the zone's offset *now*, and a window can
// span a moment where it changes: London is UTC+1 through 2026-10-25T01:00Z and
// UTC+0 after, which makes that local day 25 hours long. Applying either offset
// to the whole window puts the two readings below on different local days, so a
// day bucket splits a day that did not end.
func TestGetMetric_BucketsFollowTheZoneAcrossDST(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	// Both readings fall on local Sunday 25 October 2026 in London: the first
	// half an hour into it while BST is still in force, the second half an hour
	// before it ends, by which point the clocks have gone back.
	morning := time.Date(2026, 10, 24, 23, 30, 0, 0, time.UTC)
	evening := time.Date(2026, 10, 25, 23, 30, 0, 0, time.UTC)
	// And one on the Monday after, by which point the offset has changed for
	// good. Its bucket has to start at local midnight under the *new* offset.
	monday := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	attrs := map[string]string{"pod": "a"}
	fixture := makeSumFixtureT("dst.total", pmetric.AggregationTemporalityDelta, []sumTestDP{
		{timestamp: morning, value: 1, attrs: attrs},
		{timestamp: evening, value: 2, attrs: attrs},
		{timestamp: monday, value: 3, attrs: attrs},
	})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	// One view bucket over a ~25h window selects the one-day rung, so the grid
	// asks exactly the question this test is about: which local day is each
	// reading on? Read the views rather than the datapoints -- the datapoint
	// reduction elects a bucket's first *and* last point, so a bucket holding
	// both readings still emits two, and the count would say nothing.
	dayBuckets := func(tzName string, tzOffsetNs int64) []any {
		t.Helper()
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetric(ctx, db, summaries[0]["id"].(string),
				morning.Add(-time.Minute).UnixNano(), monday.Add(time.Minute).UnixNano(),
				0, nil, nil, tzOffsetNs, true, 2, 0, nil, tzName, nil, 0)
		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		return m["timeseries"].([]any)[0].(map[string]any)["views"].([]any)
	}

	// The offset a London browser reports while BST is in force. Sent alone it
	// is right for the first reading and an hour wrong for the second.
	const bstOffsetNs = int64(time.Hour)

	// Each bucket as (start, samples). The count alone cannot tell the two
	// answers apart -- both produce two buckets -- because the offset does not
	// lose a day, it puts the boundary in the wrong place and moves one reading
	// across it.
	shape := func(views []any) []string {
		t.Helper()
		var out []string
		for _, v := range views {
			b := v.(map[string]any)
			out = append(out, fmt.Sprintf("%s/%d",
				b["bucketStart"].(string), int(b["sampleCount"].(float64))))
		}
		return out
	}
	utc := func(day, hour int) string {
		return strconv.FormatInt(
			time.Date(2026, 10, day, hour, 0, 0, 0, time.UTC).UnixNano(), 10)
	}

	assert.Equal(t,
		// Local midnight on each day, 25 hours apart: that Sunday is 25 hours
		// long, and its boundaries are BST at the start and GMT at the end.
		[]string{utc(24, 23) + "/2", utc(26, 0) + "/1"},
		shape(dayBuckets("Europe/London", bstOffsetNs)),
		"both Sunday readings belong to the Sunday, and each bucket has to begin "+
			"at local midnight under the offset in force there")

	assert.Equal(t,
		[]string{utc(24, 23) + "/1", utc(25, 23) + "/2"},
		shape(dayBuckets("", bstOffsetNs)),
		"with no zone named the single offset still applies to the whole window, "+
			"so the Sunday ends an hour early and takes its last reading into "+
			"the Monday -- the behaviour a zone-less caller keeps, and the bug")
}

// TestGetMetric_HistogramMergeFollowsTheZoneAcrossDST is the histogram half of
// the DST question. The scalar test above reads the view grid, which never
// exercises the datapoint election -- histograms have no views, and their
// merged rows land *on* bucket timestamps, so the election's bucket cutting is
// directly visible here and nowhere else. A mutant reverting the election's
// zone conversion survives the scalar test; this one is built to kill it.
func TestGetMetric_HistogramMergeFollowsTheZoneAcrossDST(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	// The same three moments as the scalar test: two on London's 25-hour local
	// Sunday -- one while BST holds, one after the clocks went back -- and one
	// the following Monday.
	morning := time.Date(2026, 10, 24, 23, 30, 0, 0, time.UTC)
	evening := time.Date(2026, 10, 25, 23, 30, 0, 0, time.UTC)
	monday := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	dp := func(ts time.Time) histTestDP {
		return histTestDP{
			timestamp: ts,
			attrs:     map[string]string{"pod": "a"},
			bounds:    []float64{1, 2},
			counts:    []uint64{1, 0, 0},
			count:     1, sum: 0.5, min: 0.5, max: 0.5,
		}
	}
	fixture := makeHistogramFixtureT("dst.hist", pmetric.AggregationTemporalityDelta,
		[]histTestDP{dp(morning), dp(evening), dp(monday)})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	// Two target buckets over ~36 hours of data selects the one-day rung, and
	// a histogram reduction merges rather than elects: each local day's rows
	// fold into one row stamped with the bucket's start.
	shape := func(tzName string, tzOffsetNs int64) []string {
		t.Helper()
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetric(ctx, db, summaries[0]["id"].(string),
				morning.Add(-time.Minute).UnixNano(), monday.Add(time.Minute).UnixNano(),
				2, nil, nil, tzOffsetNs, true, 0, 0, nil, tzName, nil, 0)
		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		dps := m["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any)
		var out []string
		for _, v := range dps {
			b := v.(map[string]any)
			out = append(out, fmt.Sprintf("%s/%d",
				b["timestamp"].(string), int(b["count"].(float64))))
		}
		sort.Strings(out)
		return out
	}
	utc := func(day, hour int) string {
		return strconv.FormatInt(
			time.Date(2026, 10, day, hour, 0, 0, 0, time.UTC).UnixNano(), 10)
	}
	const bstOffsetNs = int64(time.Hour)

	assert.Equal(t,
		[]string{utc(24, 23) + "/2", utc(26, 0) + "/1"},
		shape("Europe/London", bstOffsetNs),
		"the two Sunday observations merge into the Sunday bucket and the "+
			"Monday one stands alone, each stamped with its local midnight")

	assert.Equal(t,
		[]string{utc(24, 23) + "/1", utc(25, 23) + "/2"},
		shape("", bstOffsetNs),
		"the single offset ends the Sunday an hour early, carrying its second "+
			"observation into the Monday -- the behaviour zone-less callers keep")
}

// TestGetMetric_ExemplarsAreCappedPerBucket pins the ceiling on how much an
// exemplar-dense stream can grow a reduced response.
//
// Exemplar-bearing datapoints are retained on top of the four M4 elects, so
// trace links survive reduction. That retention used to be uncapped on the
// reasoning that exemplars are sparse -- an assumption about other people's SDK
// settings rather than anything this query controls. A stream sampling every
// datapoint defeated the reduction outright: every row came back.
func TestGetMetric_ExemplarsAreCappedPerBucket(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	// One bucket's worth of readings, every one of them carrying an exemplar.
	const n = 60
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "exemplar-cap")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("cap-scope")
	m := sm.Metrics().AppendEmpty()
	m.SetName("capped.total")
	sum := m.SetEmptySum()
	sum.SetIsMonotonic(true)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	for i := range n {
		dp := sum.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(base.Add(time.Duration(i) * time.Second).UnixNano()))
		dp.SetDoubleValue(float64(i % 7))
		dp.Attributes().PutStr("pod", "a")
		ex := dp.Exemplars().AppendEmpty()
		ex.SetTimestamp(dp.Timestamp())
		ex.SetDoubleValue(float64(i))
		ex.SetTraceID(pcommon.TraceID([16]byte{byte(i), 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}))
		ex.SetSpanID(pcommon.SpanID([8]byte{byte(i), 2, 3, 4, 5, 6, 7, 8}))
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, md, s.FlushedIDs())
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	// One target bucket, so the whole run reduces to a single bucket and the
	// per-bucket ceiling is the whole response's ceiling.
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, summaries[0]["id"].(string),
			base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano(),
			1, nil, nil, 0, true, 0, 0, nil, "", nil, 0)
	})
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	dps := got["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any)

	// Four elected plus at most two exemplar carriers, and the two sets
	// overlap, so six is the ceiling rather than the expected count.
	assert.LessOrEqual(t, len(dps), 6,
		"a bucket yields at most the four M4 elects plus two exemplar carriers; "+
			"without the cap all %d readings come back and the reduction does nothing", n)
	assert.Greater(t, len(dps), 0, "the bucket should still yield its elected points")

	// Trace correlation survives: the point of retaining carriers at all.
	var withExemplars int
	for _, d := range dps {
		if ex, _ := d.(map[string]any)["exemplars"].([]any); len(ex) > 0 {
			withExemplars++
		}
	}
	assert.Greater(t, withExemplars, 0,
		"capping must not become dropping -- a reader still needs a way into a trace")
}

// TestGetMetric_ExemplarListIsCappedAndCounted covers the other direction the
// response grew in: how many exemplars one datapoint carries, which OTel does
// not limit. The list is capped and the true count travels beside it, so a
// client can say "5 of 60" rather than silently showing five.
func TestGetMetric_ExemplarListIsCappedAndCounted(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	const perDatapoint = 60
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "exemplar-fanout")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("fanout-scope")
	m := sm.Metrics().AppendEmpty()
	m.SetName("fanout.total")
	g := m.SetEmptyGauge()
	dp := g.DataPoints().AppendEmpty()
	dp.SetTimestamp(pcommon.Timestamp(base.UnixNano()))
	dp.SetDoubleValue(1)
	dp.Attributes().PutStr("pod", "a")
	for i := range perDatapoint {
		ex := dp.Exemplars().AppendEmpty()
		// Staggered, so the (timestamp, id) order the cap keeps a prefix of is
		// a real order rather than an accident of insertion.
		ex.SetTimestamp(pcommon.Timestamp(base.Add(time.Duration(i) * time.Millisecond).UnixNano()))
		ex.SetDoubleValue(float64(i))
		ex.SetTraceID(pcommon.TraceID([16]byte{byte(i), 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}))
		ex.SetSpanID(pcommon.SpanID([8]byte{byte(i), 2, 3, 4, 5, 6, 7, 8}))
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, md, s.FlushedIDs())
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, summaries[0]["id"].(string),
			base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano(),
			0, nil, nil, 0, true, 0, 0, nil, "", nil, 0)
	})
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	dps := got["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any)
	require.Len(t, dps, 1)
	only := dps[0].(map[string]any)

	assert.Len(t, only["exemplars"].([]any), 5,
		"one datapoint ships at most five exemplars, whatever the SDK attached")
	assert.Equal(t, float64(perDatapoint), only["exemplarCount"],
		"and reports how many it actually holds, so the cap is visible rather "+
			"than a silent loss of trace links")
}

// TestGetMetric_ExemplarSelectionKeepsBothExtremes pins which exemplars survive
// the cap, not merely how many.
//
// The cap first kept a prefix in time order, which is reproducible and useless:
// a reader following an exemplar is chasing the slow request or the one that
// returned nothing, and time order hands them whichever happened first. Both
// caps now rank from either extreme, so what survives spans the range.
func TestGetMetric_ExemplarSelectionKeepsBothExtremes(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "exemplar-extremes")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("extremes-scope")
	m := sm.Metrics().AppendEmpty()
	m.SetName("extremes.gauge")
	g := m.SetEmptyGauge()
	dp := g.DataPoints().AppendEmpty()
	dp.SetTimestamp(pcommon.Timestamp(base.UnixNano()))
	dp.SetDoubleValue(1)
	dp.Attributes().PutStr("pod", "a")
	// Values 100, 101, ... 119 in time order. Time order would keep 100-104;
	// ranking from both ends keeps the extremes of the range instead. The
	// slowest request is the one worth a trace link, and so is the fastest.
	const n = 20
	for i := range n {
		ex := dp.Exemplars().AppendEmpty()
		ex.SetTimestamp(pcommon.Timestamp(base.Add(time.Duration(i) * time.Millisecond).UnixNano()))
		ex.SetDoubleValue(float64(100 + i))
		ex.SetTraceID(pcommon.TraceID([16]byte{byte(i), 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}))
		ex.SetSpanID(pcommon.SpanID([8]byte{byte(i), 2, 3, 4, 5, 6, 7, 8}))
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, md, s.FlushedIDs())
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, summaries[0]["id"].(string),
			base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano(),
			0, nil, nil, 0, true, 0, 0, nil, "", nil, 0)
	})
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	dps := got["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any)
	require.Len(t, dps, 1)

	var values []float64
	for _, e := range dps[0].(map[string]any)["exemplars"].([]any) {
		values = append(values, e.(map[string]any)["value"].(float64))
	}
	sort.Float64s(values)

	// Five kept, working inward from both ends: 100, 101, 102 from the bottom
	// and 118, 119 from the top -- or the mirror of that, which is the same
	// spread. What matters is that both extremes are present and the middle is
	// not.
	require.Len(t, values, 5)
	assert.Equal(t, 100.0, values[0],
		"the lowest exemplar is one a reader would want and must survive the cap")
	assert.Equal(t, 119.0, values[len(values)-1],
		"so is the highest -- time-order selection would have dropped it")
	assert.NotContains(t, values, 110.0,
		"and the middle of the range is what the cap should be spending its "+
			"budget on last")
}

// TestGetMetric_ExemplarCarriersAreTheExtremeOnes is the per-bucket half of the
// same question: of the datapoints a bucket could retain for their exemplars,
// which two does it keep? The ones whose exemplars reach lowest and highest,
// not the two that happened first.
func TestGetMetric_ExemplarCarriersAreTheExtremeOnes(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "carrier-extremes")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("carrier-scope")
	m := sm.Metrics().AppendEmpty()
	m.SetName("carriers.total")
	sum := m.SetEmptySum()
	sum.SetIsMonotonic(true)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)

	// Twelve readings in one bucket, each carrying one exemplar.
	//
	// The exemplar extremes sit in the middle of the run, at i=5 and i=6, while
	// the datapoint values rise monotonically. That separation is the whole
	// fixture: M4 elects the first, last, smallest and largest *readings* --
	// here i=0 and i=11 -- and those carry unremarkable exemplars. So the only
	// thing that can put exemplar 10 or 99 in the response is the carrier cap
	// choosing them, and a cap that kept the earliest two would ship three
	// identical 55s and reach neither.
	//
	// An earlier version of this test ramped the exemplar values with time,
	// which made the extremes coincide with the first and last readings -- the
	// elects supplied them, and the test passed no matter what the carrier cap
	// did.
	const n = 12
	for i := range n {
		dp := sum.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(base.Add(time.Duration(i) * time.Second).UnixNano()))
		dp.SetDoubleValue(float64(i))
		dp.Attributes().PutStr("pod", "a")
		value := 55.0
		switch i {
		case 5:
			value = 10
		case 6:
			value = 99
		}
		ex := dp.Exemplars().AppendEmpty()
		ex.SetTimestamp(dp.Timestamp())
		ex.SetDoubleValue(value)
		ex.SetTraceID(pcommon.TraceID([16]byte{byte(i), 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}))
		ex.SetSpanID(pcommon.SpanID([8]byte{byte(i), 2, 3, 4, 5, 6, 7, 8}))
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, md, s.FlushedIDs())
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	// One bucket for the whole run, so the per-bucket carrier cap is what
	// decides the answer.
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, summaries[0]["id"].(string),
			base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano(),
			1, nil, nil, 0, true, 0, 0, nil, "", nil, 0)
	})
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	dps := got["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any)

	var shipped []float64
	for _, d := range dps {
		for _, e := range d.(map[string]any)["exemplars"].([]any) {
			shipped = append(shipped, e.(map[string]any)["value"].(float64))
		}
	}
	sort.Float64s(shipped)
	require.NotEmpty(t, shipped)

	assert.Contains(t, shipped, 99.0,
		"the bucket's most extreme exemplar is the one a reader is hunting, and "+
			"no elected reading carries it -- only the carrier cap can")
	assert.Contains(t, shipped, 10.0,
		"and the other end matters too -- a request that returned instantly is "+
			"as diagnostic as one that hung")
}

// TestGetMetric_ColumnWindowMergesTheWholeColumn pins the contract a heatmap
// click depends on: asked for one bucket over one column's time range, the store
// returns each series merged across that whole range.
//
// The click used to read a per-series datapoint stamped at the column's start
// instead. That datapoint exists and lines up exactly -- both grids snap to the
// same ladder and its rungs divide evenly, so a column start is always a
// per-series bucket start -- but a column holds several per-series buckets, and
// the first one is not the column. Measured on the grids the UI actually asks
// for, a 30s column holds three 10s buckets, so the click described a third of
// what was clicked, silently.
//
// The fixture makes that difference as large as it gets: fast readings at the
// start of each column, slow ones after, which is the shape of a latency spike
// and the reason someone clicks a heatmap in the first place.
func TestGetMetric_ColumnWindowMergesTheWholeColumn(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	// Enough readings that the heatmap's target is what sets its width. Fewer and
	// the cadence floor clamps both grids to the 10s reporting interval, they come
	// out identical, and there is no mismatch left to test: the finer grid can
	// never be finer than the data reports.
	const readings = 240
	var dps []histTestDP
	for i := range readings {
		// One in three readings is fast; the rest are slow. With 10s readings and
		// a 30s column, that puts the fast one exactly at each column start.
		fast := i%3 == 0
		counts := []uint64{100, 0, 0, 0}
		sum, mn, mx := 50.0, 0.4, 0.9
		if !fast {
			counts = []uint64{0, 0, 0, 100}
			sum, mn, mx = 5000.0, 40.0, 90.0
		}
		dps = append(dps, histTestDP{
			timestamp: base.Add(time.Duration(i) * 10 * time.Second),
			attrs:     map[string]string{"pod": "a"},
			bounds:    []float64{1, 5, 10},
			counts:    counts,
			count:     100,
			sum:       sum, min: mn, max: mx,
		})
	}
	fixture := makeHistogramFixtureT("column.hist", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	id := summaries[0]["id"].(string)

	// fitToData is the caller's, because the two reads disagree about it and the
	// test has to exercise what the app sends: the wide read treats an unbounded
	// window as the absence of a choice, while the column read *is* the request.
	p95 := func(target, from, to int64, fitToData bool, atTimestamp *int64) float64 {
		t.Helper()
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetric(ctx, db, id, from, to,
				target, nil, []float64{0.95}, 0, fitToData, 0, 0, nil, "", nil, 0)
		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		for _, d := range m["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any) {
			dp := d.(map[string]any)
			if atTimestamp != nil {
				v, err := strconv.ParseInt(dp["timestamp"].(string), 10, 64)
				require.NoError(t, err)
				if v != *atTimestamp {
					continue
				}
			}
			q, ok := dp["quantiles"].(map[string]any)
			require.True(t, ok, "quantiles must be present")
			return q["0.95"].(float64)
		}
		t.Fatal("no datapoint matched")
		return 0
	}

	windowStart := base.Add(-time.Minute).UnixNano()
	windowEnd := base.Add(time.Duration(readings)*10*time.Second + time.Minute).UnixNano()

	// Find a column boundary the way the UI does: the heatmap's own grid.
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, id, windowStart, windowEnd,
			100, nil, nil, 0, true, 0, 0, nil, "", nil, 0)
	})
	require.NoError(t, err)
	var heat map[string]any
	require.NoError(t, json.Unmarshal(raw, &heat))
	columns := heat["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any)
	require.Greater(t, len(columns), 2, "need several columns to measure a width")
	first, err := strconv.ParseInt(columns[0].(map[string]any)["timestamp"].(string), 10, 64)
	require.NoError(t, err)
	second, err := strconv.ParseInt(columns[1].(map[string]any)["timestamp"].(string), 10, 64)
	require.NoError(t, err)
	columnWidth := first - second // datapoints arrive newest first
	if columnWidth < 0 {
		columnWidth = -columnWidth
	}
	require.Positive(t, columnWidth)

	// A column in the middle, away from the window edges.
	columnStart, err := strconv.ParseInt(
		columns[len(columns)/2].(map[string]any)["timestamp"].(string), 10, 64)
	require.NoError(t, err)

	// The column's range, half-open. The store's window filter is inclusive at
	// both ends (`timestamp >= start and timestamp <= end`) while floor_div cuts
	// buckets half-open, so asking for [start, start+width] pulls in the next
	// column's first reading and counts it twice -- once in each column. One
	// nanosecond short of the next boundary is the range the column actually
	// covers.
	columnEnd := columnStart + columnWidth - 1

	// What the old click read: the per-series bucket stamped at the column start.
	atStart := p95(500, windowStart, windowEnd, true, &columnStart)
	// What the column holds: one bucket over exactly the column's range, fetched
	// the way the page fetches it.
	whole := p95(1, columnStart, columnEnd, false, nil)

	assert.NotEqual(t, atStart, whole,
		"a per-series bucket at the column start is not the column: if these agree "+
			"the fixture no longer distinguishes the two reads and the test proves nothing")
	assert.Greater(t, whole, atStart,
		"the column contains the slow readings its first bucket misses, so its p95 "+
			"must be the higher of the two -- this is the number a click has to show")

	// And the window must be exactly one column. Without this the test passes for
	// any window at least a column wide, so a fix that fetched too much would
	// look correct: the p95 would still be the high one, just drawn from readings
	// outside the column the user clicked.
	perColumn := columnWidth / (10 * int64(time.Second)) // readings inside one column
	require.Positive(t, perColumn)
	raw, err = readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, id, columnStart, columnEnd,
			1, nil, nil, 0, false, 0, 0, nil, "", nil, 0)
	})
	require.NoError(t, err)
	var merged map[string]any
	require.NoError(t, json.Unmarshal(raw, &merged))
	mergedDps := merged["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any)
	require.Len(t, mergedDps, 1, "one bucket over one column is one merged datapoint")
	assert.Equal(t, float64(perColumn*100), mergedDps[0].(map[string]any)["count"],
		"the merged column must total exactly the readings inside it -- %d readings "+
			"of 100 observations each; more means the fetched window is wider than "+
			"the column", perColumn)
}

// TestHistogramBoundsAreStoredOnce pins the bounds dictionary: the vector is a
// property of the instrument, so a series reporting all session writes it one
// time, however many datapoints arrive -- and a stream whose bounds genuinely
// change mid-flight gets a second row, not a collision, because OTel only
// makes fixed bounds a practice, never a promise.
func TestHistogramBoundsAreStoredOnce(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	boundsA := []float64{1, 5, 10}
	boundsB := []float64{2, 4, 8, 16}
	var dps []histTestDP
	for i := range 40 {
		b := boundsA
		counts := []uint64{1, 2, 3, 4}
		if i >= 30 {
			// The pathological case: the SDK was reconfigured mid-stream.
			b = boundsB
			counts = []uint64{1, 2, 3, 4, 5}
		}
		dps = append(dps, histTestDP{
			timestamp: base.Add(time.Duration(i) * 10 * time.Second),
			attrs:     map[string]string{"pod": "a"},
			bounds:    b,
			counts:    counts,
			count:     10, sum: 30, min: 0.5, max: 9.5,
		})
	}
	fixture := makeHistogramFixtureT("bounds.hist", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))

	assert.Equal(t, 2, countRows(t, s, ctx, `select count(*) from histogram_bounds`),
		"40 datapoints carry 2 distinct bounds vectors, so the dictionary holds 2 rows")
	assert.Zero(t, countRows(t, s, ctx,
		`select count(*) from datapoints
		 where bounds_id is not null
		   and not exists (select 1 from histogram_bounds hb where hb.id = bounds_id)`),
		"every reference resolves; nothing may point at a row that is not there")

	// The wire is unchanged: each datapoint comes back with the exact vector
	// it arrived with, resolved through the dictionary.
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, summaries[0]["id"].(string),
			base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano(),
			0, nil, nil, 0, true, 0, 0, nil, "", nil, 0)
	})
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	seen := map[int]int{}
	for _, ts := range got["timeseries"].([]any) {
		for _, d := range ts.(map[string]any)["datapoints"].([]any) {
			eb := d.(map[string]any)["explicitBounds"].([]any)
			seen[len(eb)]++
		}
	}
	assert.Equal(t, map[int]int{3: 30, 4: 10}, seen,
		"each datapoint resolves its own vector -- 30 with the original bounds, "+
			"10 with the reconfigured ones")

	// Sweep: clearing the metrics orphans both rows, and the sweep takes them.
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		return metrics.Clear(ctx, db)
	}))
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		return ingest.SweepOrphans(ctx, db, s.FlushedIDs())
	}))
	assert.Zero(t, countRows(t, s, ctx, `select count(*) from histogram_bounds`),
		"bounds nothing references are garbage, and retention measures garbage "+
			"as if it were data")

	// And re-ingest after the sweep still works: the FlushedIDs entries for the
	// swept rows must have been forgotten, or the dictionary skips the insert
	// and the datapoints' foreign key has nothing to point at.
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, makeHistogramFixtureT("bounds.hist",
			pmetric.AggregationTemporalityDelta, dps), s.FlushedIDs())
	}))
	assert.Equal(t, 2, countRows(t, s, ctx, `select count(*) from histogram_bounds`))
}

// TestSeriesCountsAreWindowAndLifetime pins the two numbers apart.
//
// The card used to show one count whose meaning changed with the window and
// said so only in a tooltip: narrow the range and "21 series" became "3
// series", which reads as data going missing rather than as a different
// question being answered. Both are now reported, and this fails if either
// starts answering the other's question -- in particular if seriesCount is
// ever "optimised" into a count over metric_series, which agrees on an
// unbounded window and diverges exactly where it matters.
func TestSeriesCountsAreWindowAndLifetime(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	// Twelve series on the stream; only three of them report in the first
	// minute, which is the window the narrow search below asks about.
	var dps []sumTestDP
	for i := range 12 {
		pod := map[string]string{"pod": string(rune('a' + i))}
		at := base.Add(time.Duration(i) * 10 * time.Minute)
		if i < 3 {
			at = base.Add(time.Duration(i) * 10 * time.Second)
		}
		dps = append(dps, sumTestDP{timestamp: at, value: float64(i), attrs: pod})
	}
	fixture := makeSumFixtureT("cardinality.total", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}))

	summarise := func(from, to time.Time) map[string]any {
		t.Helper()
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, from.UnixNano(), to.UnixNano(), nil)
		})
		require.NoError(t, err)
		var rows []map[string]any
		require.NoError(t, json.Unmarshal(raw, &rows))
		require.Len(t, rows, 1)
		return rows[0]
	}

	whole := summarise(base.Add(-time.Hour), base.Add(4*time.Hour))
	assert.Equal(t, float64(12), whole["seriesCount"],
		"every series reported somewhere in an unbounded window")
	assert.Equal(t, float64(12), whole["seriesCardinality"],
		"and the lifetime total agrees there, which is why one number hid this")

	narrow := summarise(base.Add(-time.Second), base.Add(time.Minute))
	assert.Equal(t, float64(3), narrow["seriesCount"],
		"three series reported in this minute, so that is what the window count says")
	assert.Equal(t, float64(12), narrow["seriesCardinality"],
		"the stream still has twelve series; narrowing the window did not delete nine")
}

// TestIngest_SingleBucketHistogram covers a histogram whose bucket_counts has
// one element and whose explicit_bounds therefore has none -- the "everything
// falls in one bucket" shape, which OTLP explicitly permits and the
// OpenTelemetry demo emits.
//
// pcommon renders an empty Float64Slice as a *nil* []float64, not an empty one
// (AsRaw appends onto a nil destination, and appending nothing to nil yields
// nil). That nil travelled into the dictionary's bounds map and out again as a
// nil inner slice of the [][]float64 handed to the driver, which dereferenced
// it and took the process down -- a panic during ingest, not an error, so the
// batch could not even be rejected cleanly.
//
// Every existing histogram fixture supplies at least one bound, which is why
// the whole suite passed while the crash was reachable from any real demo.
func TestIngest_SingleBucketHistogram(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	fixture := makeHistogramFixtureT("single.bucket", pmetric.AggregationTemporalityDelta, []histTestDP{{
		timestamp: base,
		bounds:    nil,
		counts:    []uint64{7},
		count:     7, sum: 3.5, min: 0.5, max: 0.5,
	}})

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture, s.FlushedIDs())
	}), "ingesting a single-bucket histogram must not fail")

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1, "the metric must be readable after ingest")

	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, summaries[0]["id"].(string),
			base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano(),
			0, nil, nil, 0, false, 0, 0, nil, "", nil, 0)
	})
	require.NoError(t, err)
	var metric map[string]any
	require.NoError(t, json.Unmarshal(raw, &metric))

	ts, _ := metric["timeseries"].([]any)
	require.Len(t, ts, 1)
	dps, _ := ts[0].(map[string]any)["datapoints"].([]any)
	require.Len(t, dps, 1, "the datapoint must survive the round trip")

	// The empty bounds vector must read back as an empty list, not null: the
	// renderer distinguishes "no bounds" from "bounds missing".
	dp, _ := dps[0].(map[string]any)
	require.Contains(t, dp, "explicitBounds")
	bounds, ok := dp["explicitBounds"].([]any)
	require.True(t, ok, "explicitBounds should be a list, got %T", dp["explicitBounds"])
	assert.Empty(t, bounds, "a single-bucket histogram has zero bounds")
}

// TestIngest_NilArraysReachingTheAppender pins the other two array shapes the
// demo produces that the F1 corpus never did: a histogram with no buckets at
// all, and an exponential histogram with no negative buckets -- the ordinary
// case for any latency instrument, since durations are never negative.
// pcommon renders both as nil, exactly as it does the empty bounds vector in
// TestIngest_SingleBucketHistogram.
//
// These two go to the appender rather than to a query parameter, and the
// appender converts a nil slice into an empty list instead of dereferencing
// it. That asymmetry between the two ways an array reaches DuckDB is the whole
// reason the bounds crash was confined to one column, and it is not obvious
// from either call site, so it is worth holding still: if the appender ever
// stops absorbing nil, this fails next to the code that would need the same
// normalisation AddBounds now does.
func TestIngest_NilArraysReachingTheAppender(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)

	t.Run("histogram with no buckets", func(t *testing.T) {
		s, ctx := storetest.New(t)

		md := pmetric.NewMetrics()
		rm := md.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("service.name", "nil-arrays")
		m := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		m.SetName("no.buckets")
		h := m.SetEmptyHistogram()
		h.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
		dp := h.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(base.UnixNano()))
		dp.SetCount(0)
		require.Nil(t, dp.BucketCounts().AsRaw(), "precondition: bucket_counts is nil, not empty")

		require.NoError(t, s.WithConn(func(c driver.Conn) error {
			return metrics.Ingest(ctx, c, md, s.FlushedIDs())
		}))
	})

	t.Run("exponential histogram with no negative buckets", func(t *testing.T) {
		s, ctx := storetest.New(t)

		md := pmetric.NewMetrics()
		rm := md.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("service.name", "nil-arrays")
		m := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		m.SetName("no.negatives")
		e := m.SetEmptyExponentialHistogram()
		e.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
		dp := e.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(base.UnixNano()))
		dp.SetCount(3)
		dp.SetScale(0)
		dp.Positive().BucketCounts().FromRaw([]uint64{1, 2})
		require.Nil(t, dp.Negative().BucketCounts().AsRaw(), "precondition: negative buckets are nil")

		require.NoError(t, s.WithConn(func(c driver.Conn) error {
			return metrics.Ingest(ctx, c, md, s.FlushedIDs())
		}))
	})
}

// TestMetricMetadataRoundTrip pins OTLP's Metric.metadata through the store.
//
// It was the last field of any signal that arrived and was discarded: a
// systematic diff of pdata's getters against the schema found nothing else
// missing once span and link flags landed. Metadata describes the instrument
// -- not the labels that identify a series -- so it lives on metric_ingests
// beside description, which varies per batch for the same reason.
//
// The scope matters as much as the value. Metadata attributes go into the
// dictionary under their own scope, which is deliberately absent from the
// allowlist in get_metric_attributes.sql: stored and displayed, but never
// offered as a search field. That exclusion is asserted here, because the
// allowlist is the only thing keeping it out and a later edit could widen it
// without anyone noticing.
func TestMetricMetadataRoundTrip(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "metadata-test")
	m := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("requests.total")
	m.SetDescription("inbound requests")
	m.Metadata().PutStr("owner", "checkout-team")
	m.Metadata().PutStr("slo", "99.9")
	dp := m.SetEmptyGauge().DataPoints().AppendEmpty()
	dp.SetTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
	dp.SetDoubleValue(42)
	dp.Attributes().PutStr("route", "/cart")

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, md, s.FlushedIDs())
	}))

	got := getMetricFullByName(t, s, ctx, "requests.total")

	meta, ok := got["metadata"].([]any)
	require.True(t, ok, "metadata must be present on the wire, got %T", got["metadata"])
	pairs := map[string]string{}
	for _, e := range meta {
		a := e.(map[string]any)
		pairs[a["key"].(string)] = a["value"].(string)
	}
	require.Equal(t, map[string]string{"owner": "checkout-team", "slo": "99.9"}, pairs)

	// The datapoint's own label must not have leaked into metadata, and vice
	// versa: they are different maps under different scopes.
	require.NotContains(t, pairs, "route")

	// Stored under its own scope...
	var n int
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRow(
			`select count(*) from attributes where scope = 'metadata'`).Scan(&n)
	}))
	require.Equal(t, 2, n, "both metadata attributes belong to the metadata scope")

	// ...and discovery offers it under that scope. This assertion used to be
	// inverted: discovery deliberately hid metadata because the search mapper
	// had no case for it, and a discovered-but-unsearchable field would have
	// been a dropdown entry that errors when picked. The mapper handles the
	// metadata scope now (see TestSearchSummariesByMetricMetadata), so hiding
	// it would be the dangling half.
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricAttributes(ctx, db, 0, maxNano)
	})
	require.NoError(t, err)
	var defs []map[string]any
	require.NoError(t, json.Unmarshal(raw, &defs))
	offered := false
	for _, d := range defs {
		if d["attributeScope"] == "metadata" && d["name"] == "owner" {
			offered = true
		}
	}
	require.True(t, offered, "metadata keys must be offered as searchable: %v", defs)
}

// Metric.metadata was stored (metric_ingests.metadata_ids) but reachable by
// no search: the mapper had no "metadata" scope and the discovery query did
// not list it, so its keys never appeared in the attribute dropdown and a
// hand-written condition was rejected. Pins the mapper's both paths -- the
// IDProbe equality fast path and the attr_value fallback -- plus discovery.
func TestSearchSummariesByMetricMetadata(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	base := time.Now().Add(-time.Minute)

	md := makeSumFixtureT("meta.metric", pmetric.AggregationTemporalityCumulative, []sumTestDP{{
		timestamp: base, value: 1, attrs: map[string]string{"r": "/x"},
	}})
	md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).
		Metadata().PutStr("workload.type", "batch")
	plain := makeSumFixtureT("meta.plain", pmetric.AggregationTemporalityCumulative, []sumTestDP{{
		timestamp: base, value: 1, attrs: map[string]string{"r": "/y"},
	}})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		if err := metrics.Ingest(ctx, conn, md, s.FlushedIDs()); err != nil {
			return err
		}
		return metrics.Ingest(ctx, conn, plain, s.FlushedIDs())
	}))
	end := time.Now().UnixNano() + int64(time.Hour)

	run := func(operator, value string) []map[string]any {
		query := &search.QueryNode{
			ID:   "q1",
			Type: "condition",
			Query: &search.Query{
				Field: &search.FieldDefinition{
					Name: "workload.type", SearchScope: "attribute",
					AttributeScope: "metadata", Type: "string",
				},
				FieldOperator: operator,
				Value:         value,
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, 0, end, query)
		})
		require.NoError(t, err)
		var out []map[string]any
		require.NoError(t, json.Unmarshal(raw, &out))
		return out
	}

	// Equality takes the IDProbe fast path; CONTAINS takes attr_value.
	eq := run("=", "batch")
	require.Len(t, eq, 1)
	assert.Equal(t, "meta.metric", eq[0]["name"])
	like := run("CONTAINS", "atc")
	require.Len(t, like, 1)
	assert.Equal(t, "meta.metric", like[0]["name"])
	assert.Empty(t, run("=", "interactive"))

	// Discovery lists the key under its scope, so the dropdown can offer it.
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricAttributes(ctx, db, 0, end)
	})
	require.NoError(t, err)
	var attrs []map[string]any
	require.NoError(t, json.Unmarshal(raw, &attrs))
	found := false
	for _, a := range attrs {
		if a["name"] == "workload.type" && a["attributeScope"] == "metadata" {
			found = true
		}
	}
	assert.True(t, found, "discovery must surface metadata keys: %v", attrs)
}
