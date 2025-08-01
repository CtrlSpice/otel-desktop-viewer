package store_test

// import (
// 	"testing"
// 	"time"

// 	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/metrics"
// 	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/resource"
// 	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/scope"
// 	"github.com/marcboeker/go-duckdb/v2"
// 	"github.com/stretchr/testify/assert"
// )

// // TestMetricSuite runs a comprehensive suite of tests on metrics
// func TestMetricSuite(t *testing.T) {
// 	helper, teardown := setupTest(t)
// 	defer teardown()

// 	// Add test metrics
// 	metricsData := createTestMetrics()
// 	err := helper.store.AddMetrics(helper.ctx, metricsData)
// 	assert.NoError(t, err, "failed to add test metrics")

// 	t.Run("MetricRetrieval", func(t *testing.T) {
// 		retrievedMetrics, err := helper.store.GetMetrics(helper.ctx)
// 		assert.NoError(t, err)
// 		assert.Len(t, retrievedMetrics, 4, "should have four metrics")

// 		// Verify metrics are ordered by received time (newest first)
// 		assert.Equal(t, "exponential_histogram_metric", retrievedMetrics[0].Name)
// 		assert.Equal(t, "histogram_metric", retrievedMetrics[1].Name)
// 		assert.Equal(t, "sum_metric", retrievedMetrics[2].Name)
// 		assert.Equal(t, "gauge_metric", retrievedMetrics[3].Name)
// 	})

// 	t.Run("GaugeMetric", func(t *testing.T) {
// 		retrievedMetrics, err := helper.store.GetMetrics(helper.ctx)
// 		assert.NoError(t, err)

// 		gaugeMetric := retrievedMetrics[3] // Last in the list (oldest)
// 		assert.Equal(t, "gauge_metric", gaugeMetric.Name)
// 		assert.Equal(t, "Current memory usage", gaugeMetric.Description)
// 		assert.Equal(t, "bytes", gaugeMetric.Unit)
// 		assert.Equal(t, "Gauge", gaugeMetric.Type)

// 		// Verify gauge data points
// 		assert.Len(t, gaugeMetric.DataPoints, 2)
// 		gaugePoint1 := gaugeMetric.DataPoints[0].(metrics.GaugeDataPoint)
// 		assert.Equal(t, "Double", gaugePoint1.ValueType)
// 		assert.Equal(t, 1024.5, gaugePoint1.Value)
// 		assert.Equal(t, uint32(0), gaugePoint1.Flags)
// 		assert.Equal(t, "heap", gaugePoint1.Attributes["memory.type"])
// 		assert.Equal(t, "instance-1", gaugePoint1.Attributes["service.instance"])

// 		gaugePoint2 := gaugeMetric.DataPoints[1].(metrics.GaugeDataPoint)
// 		assert.Equal(t, 2048.0, gaugePoint2.Value)
// 		assert.Equal(t, "stack", gaugePoint2.Attributes["memory.type"])
// 		assert.Equal(t, "instance-2", gaugePoint2.Attributes["service.instance"])
// 	})

// 	t.Run("SumMetric", func(t *testing.T) {
// 		retrievedMetrics, err := helper.store.GetMetrics(helper.ctx)
// 		assert.NoError(t, err)

// 		sumMetric := retrievedMetrics[2]
// 		assert.Equal(t, "sum_metric", sumMetric.Name)
// 		assert.Equal(t, "Total requests processed", sumMetric.Description)
// 		assert.Equal(t, "requests", sumMetric.Unit)
// 		assert.Equal(t, "Sum", sumMetric.Type)

// 		// Verify sum data points
// 		assert.Len(t, sumMetric.DataPoints, 2)
// 		sumPoint1 := sumMetric.DataPoints[0].(metrics.SumDataPoint)
// 		assert.Equal(t, 1500.0, sumPoint1.Value)
// 		assert.Equal(t, true, sumPoint1.IsMonotonic)
// 		assert.Equal(t, "Cumulative", sumPoint1.AggregationTemporality)
// 		assert.Equal(t, "POST", sumPoint1.Attributes["http.method"])
// 		assert.Equal(t, int64(200), sumPoint1.Attributes["http.status_code"])

// 		sumPoint2 := sumMetric.DataPoints[1].(metrics.SumDataPoint)
// 		assert.Equal(t, 2500.0, sumPoint2.Value)
// 		assert.Equal(t, "GET", sumPoint2.Attributes["http.method"])
// 		assert.Equal(t, int64(404), sumPoint2.Attributes["http.status_code"])
// 	})

// 	t.Run("HistogramMetric", func(t *testing.T) {
// 		retrievedMetrics, err := helper.store.GetMetrics(helper.ctx)
// 		assert.NoError(t, err)

// 		histogramMetric := retrievedMetrics[1]
// 		assert.Equal(t, "histogram_metric", histogramMetric.Name)
// 		assert.Equal(t, "Request duration distribution", histogramMetric.Description)
// 		assert.Equal(t, "seconds", histogramMetric.Unit)
// 		assert.Equal(t, "Histogram", histogramMetric.Type)

// 		// Verify histogram data points
// 		assert.Len(t, histogramMetric.DataPoints, 2)
// 		histogramPoint1 := histogramMetric.DataPoints[0].(metrics.HistogramDataPoint)
// 		assert.Equal(t, uint64(100), histogramPoint1.Count)
// 		assert.Equal(t, 25.5, histogramPoint1.Sum)
// 		assert.Equal(t, 0.1, histogramPoint1.Min)
// 		assert.Equal(t, 2.5, histogramPoint1.Max)
// 		assert.Equal(t, "Delta", histogramPoint1.AggregationTemporality)
// 		assert.Equal(t, "GET", histogramPoint1.Attributes["http.method"])
// 		assert.Equal(t, "/api/users", histogramPoint1.Attributes["http.route"])
// 		assert.Equal(t, []uint64{10, 20, 30, 25, 15}, histogramPoint1.BucketCounts)
// 		assert.Equal(t, []float64{0.5, 1.0, 1.5, 2.0}, histogramPoint1.ExplicitBounds)

// 		histogramPoint2 := histogramMetric.DataPoints[1].(metrics.HistogramDataPoint)
// 		assert.Equal(t, uint64(50), histogramPoint2.Count)
// 		assert.Equal(t, 15.0, histogramPoint2.Sum)
// 		assert.Equal(t, 0.05, histogramPoint2.Min)
// 		assert.Equal(t, 1.0, histogramPoint2.Max)
// 		assert.Equal(t, "POST", histogramPoint2.Attributes["http.method"])
// 		assert.Equal(t, "/api/orders", histogramPoint2.Attributes["http.route"])
// 	})

// 	t.Run("ExponentialHistogramMetric", func(t *testing.T) {
// 		retrievedMetrics, err := helper.store.GetMetrics(helper.ctx)
// 		assert.NoError(t, err)

// 		expHistogramMetric := retrievedMetrics[0]
// 		assert.Equal(t, "exponential_histogram_metric", expHistogramMetric.Name)
// 		assert.Equal(t, "Response size distribution", expHistogramMetric.Description)
// 		assert.Equal(t, "bytes", expHistogramMetric.Unit)
// 		assert.Equal(t, "ExponentialHistogram", expHistogramMetric.Type)

// 		// Verify exponential histogram data points
// 		assert.Len(t, expHistogramMetric.DataPoints, 2)
// 		expHistogramPoint1 := expHistogramMetric.DataPoints[0].(metrics.ExponentialHistogramDataPoint)
// 		assert.Equal(t, uint64(50), expHistogramPoint1.Count)
// 		assert.Equal(t, 10240.0, expHistogramPoint1.Sum)
// 		assert.Equal(t, 100.0, expHistogramPoint1.Min)
// 		assert.Equal(t, 2048.0, expHistogramPoint1.Max)
// 		assert.Equal(t, int32(2), expHistogramPoint1.Scale)
// 		assert.Equal(t, uint64(5), expHistogramPoint1.ZeroCount)
// 		assert.Equal(t, "Delta", expHistogramPoint1.AggregationTemporality)
// 		assert.Equal(t, "application/json", expHistogramPoint1.Attributes["content.type"])
// 		assert.Equal(t, "POST", expHistogramPoint1.Attributes["http.method"])

// 		// Verify positive buckets
// 		assert.Equal(t, int32(1), expHistogramPoint1.Positive.BucketOffset)
// 		assert.Equal(t, []uint64{5, 10, 15, 10, 5}, expHistogramPoint1.Positive.BucketCounts)

// 		// Verify negative buckets
// 		assert.Equal(t, int32(0), expHistogramPoint1.Negative.BucketOffset)
// 		assert.Equal(t, []uint64{2, 3}, expHistogramPoint1.Negative.BucketCounts)

// 		expHistogramPoint2 := expHistogramMetric.DataPoints[1].(metrics.ExponentialHistogramDataPoint)
// 		assert.Equal(t, uint64(25), expHistogramPoint2.Count)
// 		assert.Equal(t, 5120.0, expHistogramPoint2.Sum)
// 		assert.Equal(t, 50.0, expHistogramPoint2.Min)
// 		assert.Equal(t, 1024.0, expHistogramPoint2.Max)
// 		assert.Equal(t, int32(1), expHistogramPoint2.Scale)
// 		assert.Equal(t, uint64(3), expHistogramPoint2.ZeroCount)
// 		assert.Equal(t, "text/plain", expHistogramPoint2.Attributes["content.type"])
// 		assert.Equal(t, "GET", expHistogramPoint2.Attributes["http.method"])
// 	})

// 	t.Run("MetricResourceAndScope", func(t *testing.T) {
// 		retrievedMetrics, err := helper.store.GetMetrics(helper.ctx)
// 		assert.NoError(t, err)

// 		// Verify resource and scope are consistent across all metrics
// 		for i, metric := range retrievedMetrics {
// 			assert.Equal(t, "test-service", metric.Resource.Attributes["service.name"], "metric %d service name", i)
// 			assert.Equal(t, "1.0.0", metric.Resource.Attributes["service.version"], "metric %d service version", i)
// 			assert.Equal(t, uint32(0), metric.Resource.DroppedAttributesCount, "metric %d resource dropped count", i)
// 			assert.Equal(t, "test-scope", metric.Scope.Name, "metric %d scope name", i)
// 			assert.Equal(t, "v1.0.0", metric.Scope.Version, "metric %d scope version", i)
// 			assert.Empty(t, metric.Scope.Attributes, "metric %d scope attributes", i)
// 			assert.Equal(t, uint32(0), metric.Scope.DroppedAttributesCount, "metric %d scope dropped count", i)
// 		}
// 	})

// 	t.Run("MetricTimestamps", func(t *testing.T) {
// 		retrievedMetrics, err := helper.store.GetMetrics(helper.ctx)
// 		assert.NoError(t, err)

// 		baseTime := time.Now().UnixNano()
// 		tolerance := int64(5 * time.Second.Nanoseconds()) // 5 second tolerance for test timing

// 		for i, metric := range retrievedMetrics {
// 			// Verify received timestamp is recent
// 			assert.Greater(t, metric.Received, baseTime-tolerance, "metric %d received time should be recent", i)
// 			assert.Less(t, metric.Received, baseTime+tolerance, "metric %d received time should be recent", i)

// 			// Verify data point timestamps
// 			for j, dataPoint := range metric.DataPoints {
// 				switch dp := dataPoint.(type) {
// 				case metrics.GaugeDataPoint:
// 					assert.Greater(t, dp.Timestamp, int64(0), "gauge data point %d timestamp", j)
// 					assert.Greater(t, dp.StartTime, int64(0), "gauge data point %d start time", j)
// 				case metrics.SumDataPoint:
// 					assert.Greater(t, dp.Timestamp, int64(0), "sum data point %d timestamp", j)
// 					assert.Greater(t, dp.StartTime, int64(0), "sum data point %d start time", j)
// 				case metrics.HistogramDataPoint:
// 					assert.Greater(t, dp.Timestamp, int64(0), "histogram data point %d timestamp", j)
// 					assert.Greater(t, dp.StartTime, int64(0), "histogram data point %d start time", j)
// 				case metrics.ExponentialHistogramDataPoint:
// 					assert.Greater(t, dp.Timestamp, int64(0), "exponential histogram data point %d timestamp", j)
// 					assert.Greater(t, dp.StartTime, int64(0), "exponential histogram data point %d start time", j)
// 				}
// 			}
// 		}
// 	})
// }

// // TestEmptyMetrics verifies handling of empty metric lists and empty stores
// func TestEmptyMetrics(t *testing.T) {
// 	helper, teardown := setupTest(t)
// 	defer teardown()

// 	// Test adding empty metric list
// 	err := helper.store.AddMetrics(helper.ctx, []metrics.MetricData{})
// 	assert.NoError(t, err)

// 	// Test getting metrics from empty store
// 	metrics, err := helper.store.GetMetrics(helper.ctx)
// 	assert.NoError(t, err)
// 	assert.Empty(t, metrics)
// }

// // TestClearMetrics verifies that all metrics can be cleared from the store
// func TestClearMetrics(t *testing.T) {
// 	helper, teardown := setupTest(t)
// 	defer teardown()

// 	// Add test metrics
// 	metricsData := createTestMetrics()
// 	err := helper.store.AddMetrics(helper.ctx, metricsData)
// 	assert.NoError(t, err)

// 	// Verify metrics exist
// 	retrievedMetrics, err := helper.store.GetMetrics(helper.ctx)
// 	assert.NoError(t, err)
// 	assert.Len(t, retrievedMetrics, 4)

// 	// Clear metrics
// 	err = helper.store.ClearMetrics(helper.ctx)
// 	assert.NoError(t, err)

// 	// Verify store is empty
// 	retrievedMetrics, err = helper.store.GetMetrics(helper.ctx)
// 	assert.NoError(t, err)
// 	assert.Empty(t, retrievedMetrics)
// }

// // TestToDbExemplars tests the toDbExemplars helper function
// func TestToDbExemplars(t *testing.T) {
// 	now := time.Now().UnixNano()
// 	tests := []struct {
// 		name     string
// 		input    []metrics.Exemplar
// 		expected []dbExemplar
// 	}{
// 		{
// 			name:     "empty exemplars",
// 			input:    []metrics.Exemplar{},
// 			expected: []dbExemplar{},
// 		},
// 		{
// 			name: "single exemplar",
// 			input: []metrics.Exemplar{
// 				{
// 					Timestamp:          now,
// 					Value:              42.5,
// 					TraceID:            "trace123",
// 					SpanID:             "span456",
// 					FilteredAttributes: map[string]any{"key": "value"},
// 				},
// 			},
// 			expected: []dbExemplar{
// 				{
// 					Timestamp:          now,
// 					Value:              42.5,
// 					TraceID:            "trace123",
// 					SpanID:             "span456",
// 					FilteredAttributes: duckdb.Map{"key": duckdb.Union{Tag: "str", Value: "value"}},
// 				},
// 			},
// 		},
// 		{
// 			name: "multiple exemplars",
// 			input: []metrics.Exemplar{
// 				{
// 					Timestamp:          now,
// 					Value:              10.0,
// 					TraceID:            "trace1",
// 					SpanID:             "span1",
// 					FilteredAttributes: map[string]any{"attr1": "value1"},
// 				},
// 				{
// 					Timestamp:          now + time.Second.Nanoseconds(),
// 					Value:              20.0,
// 					TraceID:            "trace2",
// 					SpanID:             "span2",
// 					FilteredAttributes: map[string]any{"attr2": int64(42)},
// 				},
// 			},
// 			expected: []dbExemplar{
// 				{
// 					Timestamp:          now,
// 					Value:              10.0,
// 					TraceID:            "trace1",
// 					SpanID:             "span1",
// 					FilteredAttributes: duckdb.Map{"attr1": duckdb.Union{Tag: "str", Value: "value1"}},
// 				},
// 				{
// 					Timestamp:          now + time.Second.Nanoseconds(),
// 					Value:              20.0,
// 					TraceID:            "trace2",
// 					SpanID:             "span2",
// 					FilteredAttributes: duckdb.Map{"attr2": duckdb.Union{Tag: "bigint", Value: int64(42)}},
// 				},
// 			},
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			result := toDbExemplars(tt.input)
// 			assert.Equal(t, len(tt.expected), len(result))

// 			for i, expected := range tt.expected {
// 				assert.Equal(t, expected.Timestamp, result[i].Timestamp)
// 				assert.Equal(t, expected.Value, result[i].Value)
// 				assert.Equal(t, expected.TraceID, result[i].TraceID)
// 				assert.Equal(t, expected.SpanID, result[i].SpanID)
// 				assert.Equal(t, len(expected.FilteredAttributes), len(result[i].FilteredAttributes))
// 			}
// 		})
// 	}
// }

// // TestFromDbExemplars tests the fromDbExemplars helper function
// func TestFromDbExemplars(t *testing.T) {
// 	now := time.Now().UnixNano()
// 	tests := []struct {
// 		name     string
// 		input    []dbExemplar
// 		expected []metrics.Exemplar
// 	}{
// 		{
// 			name:     "empty exemplars",
// 			input:    []dbExemplar{},
// 			expected: []metrics.Exemplar{},
// 		},
// 		{
// 			name: "single exemplar with attributes",
// 			input: []dbExemplar{
// 				{
// 					Timestamp: now,
// 					Value:     42.5,
// 					TraceID:   "trace123",
// 					SpanID:    "span456",
// 					FilteredAttributes: duckdb.Map{
// 						"string_attr": duckdb.Union{Tag: "str", Value: "hello"},
// 						"int_attr":    duckdb.Union{Tag: "bigint", Value: int64(42)},
// 						"bool_attr":   duckdb.Union{Tag: "boolean", Value: true},
// 					},
// 				},
// 			},
// 			expected: []metrics.Exemplar{
// 				{
// 					Timestamp: now,
// 					Value:     42.5,
// 					TraceID:   "trace123",
// 					SpanID:    "span456",
// 					FilteredAttributes: map[string]any{
// 						"string_attr": "hello",
// 						"int_attr":    int64(42),
// 						"bool_attr":   true,
// 					},
// 				},
// 			},
// 		},
// 		{
// 			name: "multiple exemplars",
// 			input: []dbExemplar{
// 				{
// 					Timestamp:          now,
// 					Value:              10.0,
// 					TraceID:            "trace1",
// 					SpanID:             "span1",
// 					FilteredAttributes: duckdb.Map{"attr1": duckdb.Union{Tag: "str", Value: "value1"}},
// 				},
// 				{
// 					Timestamp:          now + time.Second.Nanoseconds(),
// 					Value:              20.0,
// 					TraceID:            "trace2",
// 					SpanID:             "span2",
// 					FilteredAttributes: duckdb.Map{"attr2": duckdb.Union{Tag: "double", Value: 3.14}},
// 				},
// 			},
// 			expected: []metrics.Exemplar{
// 				{
// 					Timestamp:          now,
// 					Value:              10.0,
// 					TraceID:            "trace1",
// 					SpanID:             "span1",
// 					FilteredAttributes: map[string]any{"attr1": "value1"},
// 				},
// 				{
// 					Timestamp:          now + time.Second.Nanoseconds(),
// 					Value:              20.0,
// 					TraceID:            "trace2",
// 					SpanID:             "span2",
// 					FilteredAttributes: map[string]any{"attr2": 3.14},
// 				},
// 			},
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			result := fromDbExemplars(tt.input)
// 			assert.Equal(t, tt.expected, result)
// 		})
// 	}
// }

// // createTestMetrics creates a comprehensive set of test metrics
// func createTestMetrics() []metrics.MetricData {
// 	baseTime := time.Now().UnixNano()
// 	startTime := baseTime - time.Hour.Nanoseconds()

// 	return []metrics.MetricData{
// 		{
// 			Name:        "gauge_metric",
// 			Description: "Current memory usage",
// 			Unit:        "bytes",
// 			Type:        "Gauge",
// 			DataPoints: []metrics.MetricDataPoint{
// 				metrics.GaugeDataPoint{
// 					Timestamp:  baseTime,
// 					StartTime:  startTime,
// 					Attributes: map[string]any{"memory.type": "heap", "service.instance": "instance-1"},
// 					Flags:      0,
// 					ValueType:  "Double",
// 					Value:      1024.5,
// 					Exemplars:  []metrics.Exemplar{},
// 				},
// 				metrics.GaugeDataPoint{
// 					Timestamp:  baseTime + time.Minute.Nanoseconds(),
// 					StartTime:  startTime + time.Minute.Nanoseconds(),
// 					Attributes: map[string]any{"memory.type": "stack", "service.instance": "instance-2"},
// 					Flags:      0,
// 					ValueType:  "Double",
// 					Value:      2048.0,
// 					Exemplars:  []metrics.Exemplar{},
// 				},
// 			},
// 			Resource: &resource.ResourceData{
// 				Attributes: map[string]any{
// 					"service.name":    "test-service",
// 					"service.version": "1.0.0",
// 				},
// 				DroppedAttributesCount: 0,
// 			},
// 			Scope: &scope.ScopeData{
// 				Name:                   "test-scope",
// 				Version:                "v1.0.0",
// 				Attributes:             map[string]any{},
// 				DroppedAttributesCount: 0,
// 			},
// 			Received: baseTime,
// 		},
// 		{
// 			Name:        "sum_metric",
// 			Description: "Total requests processed",
// 			Unit:        "requests",
// 			Type:        "Sum",
// 			DataPoints: []metrics.MetricDataPoint{
// 				metrics.SumDataPoint{
// 					Timestamp:              baseTime + 2*time.Minute.Nanoseconds(),
// 					StartTime:              startTime + 2*time.Minute.Nanoseconds(),
// 					Attributes:             map[string]any{"http.method": "POST", "http.status_code": int64(200)},
// 					Flags:                  0,
// 					ValueType:              "Double",
// 					Value:                  1500.0,
// 					IsMonotonic:            true,
// 					Exemplars:              []metrics.Exemplar{},
// 					AggregationTemporality: "Cumulative",
// 				},
// 				metrics.SumDataPoint{
// 					Timestamp:              baseTime + 3*time.Minute.Nanoseconds(),
// 					StartTime:              startTime + 3*time.Minute.Nanoseconds(),
// 					Attributes:             map[string]any{"http.method": "GET", "http.status_code": int64(404)},
// 					Flags:                  0,
// 					ValueType:              "Double",
// 					Value:                  2500.0,
// 					IsMonotonic:            true,
// 					Exemplars:              []metrics.Exemplar{},
// 					AggregationTemporality: "Cumulative",
// 				},
// 			},
// 			Resource: &resource.ResourceData{
// 				Attributes: map[string]any{
// 					"service.name":    "test-service",
// 					"service.version": "1.0.0",
// 				},
// 				DroppedAttributesCount: 0,
// 			},
// 			Scope: &scope.ScopeData{
// 				Name:                   "test-scope",
// 				Version:                "v1.0.0",
// 				Attributes:             map[string]any{},
// 				DroppedAttributesCount: 0,
// 			},
// 			Received: baseTime + time.Minute.Nanoseconds(),
// 		},
// 		{
// 			Name:        "histogram_metric",
// 			Description: "Request duration distribution",
// 			Unit:        "seconds",
// 			Type:        "Histogram",
// 			DataPoints: []metrics.MetricDataPoint{
// 				metrics.HistogramDataPoint{
// 					Timestamp:              baseTime + 4*time.Minute.Nanoseconds(),
// 					StartTime:              startTime + 4*time.Minute.Nanoseconds(),
// 					Attributes:             map[string]any{"http.method": "GET", "http.route": "/api/users"},
// 					Flags:                  0,
// 					Count:                  100,
// 					Sum:                    25.5,
// 					Min:                    0.1,
// 					Max:                    2.5,
// 					BucketCounts:           []uint64{10, 20, 30, 25, 15},
// 					ExplicitBounds:         []float64{0.5, 1.0, 1.5, 2.0},
// 					Exemplars:              []metrics.Exemplar{},
// 					AggregationTemporality: "Delta",
// 				},
// 				metrics.HistogramDataPoint{
// 					Timestamp:              baseTime + 5*time.Minute.Nanoseconds(),
// 					StartTime:              startTime + 5*time.Minute.Nanoseconds(),
// 					Attributes:             map[string]any{"http.method": "POST", "http.route": "/api/orders"},
// 					Flags:                  0,
// 					Count:                  50,
// 					Sum:                    15.0,
// 					Min:                    0.05,
// 					Max:                    1.0,
// 					BucketCounts:           []uint64{5, 10, 15, 10, 10},
// 					ExplicitBounds:         []float64{0.2, 0.4, 0.6, 0.8},
// 					Exemplars:              []metrics.Exemplar{},
// 					AggregationTemporality: "Delta",
// 				},
// 			},
// 			Resource: &resource.ResourceData{
// 				Attributes: map[string]any{
// 					"service.name":    "test-service",
// 					"service.version": "1.0.0",
// 				},
// 				DroppedAttributesCount: 0,
// 			},
// 			Scope: &scope.ScopeData{
// 				Name:                   "test-scope",
// 				Version:                "v1.0.0",
// 				Attributes:             map[string]any{},
// 				DroppedAttributesCount: 0,
// 			},
// 			Received: baseTime + 2*time.Minute.Nanoseconds(),
// 		},
// 		{
// 			Name:        "exponential_histogram_metric",
// 			Description: "Response size distribution",
// 			Unit:        "bytes",
// 			Type:        "ExponentialHistogram",
// 			DataPoints: []metrics.MetricDataPoint{
// 				metrics.ExponentialHistogramDataPoint{
// 					Timestamp:  baseTime + 6*time.Minute.Nanoseconds(),
// 					StartTime:  startTime + 6*time.Minute.Nanoseconds(),
// 					Attributes: map[string]any{"content.type": "application/json", "http.method": "POST"},
// 					Flags:      0,
// 					Count:      50,
// 					Sum:        10240.0,
// 					Min:        100.0,
// 					Max:        2048.0,
// 					Scale:      2,
// 					ZeroCount:  5,
// 					Positive: metrics.Buckets{
// 						BucketOffset: 1,
// 						BucketCounts: []uint64{5, 10, 15, 10, 5},
// 					},
// 					Negative: metrics.Buckets{
// 						BucketOffset: 0,
// 						BucketCounts: []uint64{2, 3},
// 					},
// 					Exemplars:              []metrics.Exemplar{},
// 					AggregationTemporality: "Delta",
// 				},
// 				metrics.ExponentialHistogramDataPoint{
// 					Timestamp:  baseTime + 7*time.Minute.Nanoseconds(),
// 					StartTime:  startTime + 7*time.Minute.Nanoseconds(),
// 					Attributes: map[string]any{"content.type": "text/plain", "http.method": "GET"},
// 					Flags:      0,
// 					Count:      25,
// 					Sum:        5120.0,
// 					Min:        50.0,
// 					Max:        1024.0,
// 					Scale:      1,
// 					ZeroCount:  3,
// 					Positive: metrics.Buckets{
// 						BucketOffset: 0,
// 						BucketCounts: []uint64{3, 7, 10, 5},
// 					},
// 					Negative: metrics.Buckets{
// 						BucketOffset: 0,
// 						BucketCounts: []uint64{1, 1},
// 					},
// 					Exemplars:              []metrics.Exemplar{},
// 					AggregationTemporality: "Delta",
// 				},
// 			},
// 			Resource: &resource.ResourceData{
// 				Attributes: map[string]any{
// 					"service.name":    "test-service",
// 					"service.version": "1.0.0",
// 				},
// 				DroppedAttributesCount: 0,
// 			},
// 			Scope: &scope.ScopeData{
// 				Name:                   "test-scope",
// 				Version:                "v1.0.0",
// 				Attributes:             map[string]any{},
// 				DroppedAttributesCount: 0,
// 			},
// 			Received: baseTime + 3*time.Minute.Nanoseconds(),
// 		},
// 	}
// }
