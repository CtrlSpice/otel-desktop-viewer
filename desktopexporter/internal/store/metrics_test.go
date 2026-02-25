package store

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

const maxNano = 1<<63 - 1

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

	return metrics
}

// searchMetricsAll returns SearchMetrics with wide time range and nil query; parses JSON to slice of maps.
func searchMetricsAll(t *testing.T, helper *TestHelper) []map[string]any {
	t.Helper()
	raw, err := helper.Store.SearchMetrics(helper.Ctx, 0, maxNano, nil)
	assert.NoError(t, err)
	var out []map[string]any
	assert.NoError(t, json.Unmarshal(raw, &out))
	return out
}

// TestMetricSuite runs tests on ingested metrics using SearchMetrics (DB-generated JSON).
func TestMetricSuite(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	err := helper.Store.IngestMetrics(helper.Ctx, createTestMetricsPdata())
	assert.NoError(t, err, "ingest test metrics")

	t.Run("MetricRetrieval", func(t *testing.T) {
		metrics := searchMetricsAll(t, helper)
		assert.Len(t, metrics, 4, "should have four metrics")
		names := make([]string, len(metrics))
		for i, m := range metrics {
			if n, ok := m["name"].(string); ok {
				names[i] = n
			}
		}
		assert.Contains(t, names, "gauge_metric")
		assert.Contains(t, names, "sum_metric")
		assert.Contains(t, names, "histogram_metric")
		assert.Contains(t, names, "exponential_histogram_metric")
	})

	t.Run("GaugeMetric", func(t *testing.T) {
		metrics := searchMetricsAll(t, helper)
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

	t.Run("SumMetric", func(t *testing.T) {
		metrics := searchMetricsAll(t, helper)
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
		metrics := searchMetricsAll(t, helper)
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
		metrics := searchMetricsAll(t, helper)
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
		metrics := searchMetricsAll(t, helper)
		for i, m := range metrics {
			resource, _ := m["resource"].(map[string]any)
			assert.NotNil(t, resource, "metric %d resource", i)
			scope, _ := m["scope"].(map[string]any)
			assert.NotNil(t, scope, "metric %d scope", i)
			assert.Equal(t, "test-scope", scope["name"], "metric %d scope name", i)
			assert.Equal(t, "v1.0.0", scope["version"], "metric %d scope version", i)
		}
	})
}

func requireMetric(t *testing.T, m map[string]any, name string) {
	t.Helper()
	assert.NotNil(t, m, "metric %q not found", name)
	if m != nil {
		assert.Equal(t, name, m["name"], "metric name")
	}
}

// TestDeleteMetricByID verifies that a single metric can be deleted by its ID.
func TestDeleteMetricByID(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	err := helper.Store.IngestMetrics(helper.Ctx, createTestMetricsPdata())
	assert.NoError(t, err)

	metrics := searchMetricsAll(t, helper)
	assert.Len(t, metrics, 4)

	targetID, ok := metrics[0]["id"].(string)
	assert.True(t, ok, "metric ID should be a string")
	assert.NotEmpty(t, targetID)

	err = helper.Store.DeleteMetricByID(helper.Ctx, targetID)
	assert.NoError(t, err)

	metrics = searchMetricsAll(t, helper)
	assert.Len(t, metrics, 3)
	for _, m := range metrics {
		assert.NotEqual(t, targetID, m["id"])
	}
}

// TestDeleteMetricsByIDs verifies that multiple metrics can be deleted by their IDs.
func TestDeleteMetricsByIDs(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	err := helper.Store.IngestMetrics(helper.Ctx, createTestMetricsPdata())
	assert.NoError(t, err)

	metrics := searchMetricsAll(t, helper)
	assert.Len(t, metrics, 4)

	idsToDelete := []any{metrics[0]["id"], metrics[1]["id"]}
	err = helper.Store.DeleteMetricsByIDs(helper.Ctx, idsToDelete)
	assert.NoError(t, err)

	metrics = searchMetricsAll(t, helper)
	assert.Len(t, metrics, 2)
}

// TestDeleteMetricsByIDs_Empty verifies that deleting with an empty list is a no-op.
func TestDeleteMetricsByIDs_Empty(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	err := helper.Store.DeleteMetricsByIDs(helper.Ctx, []any{})
	assert.NoError(t, err)
}

// TestEmptyMetrics verifies empty metric list and empty store.
func TestEmptyMetrics(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	err := helper.Store.IngestMetrics(helper.Ctx, pmetric.NewMetrics())
	assert.NoError(t, err)

	metrics := searchMetricsAll(t, helper)
	assert.Empty(t, metrics)
}

// TestClearMetrics verifies that all metrics can be cleared.
func TestClearMetrics(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	err := helper.Store.IngestMetrics(helper.Ctx, createTestMetricsPdata())
	assert.NoError(t, err)

	metrics := searchMetricsAll(t, helper)
	assert.Len(t, metrics, 4)

	err = helper.Store.ClearMetrics(helper.Ctx)
	assert.NoError(t, err)

	metrics = searchMetricsAll(t, helper)
	assert.Empty(t, metrics)
}
