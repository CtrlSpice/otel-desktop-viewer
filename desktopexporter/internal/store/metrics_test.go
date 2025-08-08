package store

import (
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/resource"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/scope"
	"github.com/stretchr/testify/assert"
)

// createTestMetrics creates a comprehensive set of test metrics
func createTestMetrics() []metrics.MetricData {
	baseTime := time.Now().UnixNano()
	startTime := baseTime - time.Hour.Nanoseconds()

	return []metrics.MetricData{
		{
			Name:        "gauge_metric",
			Description: "Current memory usage",
			Unit:        "bytes",
			DataPoints: metrics.DataPoints{
				Type: metrics.MetricTypeGauge,
				Points: []metrics.MetricDataPoint{
					metrics.GaugeDataPoint{
						Timestamp:  baseTime,
						StartTime:  startTime,
						Attributes: map[string]any{"memory.type": "heap", "service.instance": "instance-1"},
						Flags:      0,
						ValueType:  "Double",
						Value:      1024.5,
						Exemplars: []metrics.Exemplar{
							{
								Timestamp:          baseTime + 30*time.Second.Nanoseconds(),
								Value:              1024.5,
								TraceID:            "0123456789abcdef0123456789abcdef",
								SpanID:             "0123456789abcdef",
								FilteredAttributes: map[string]any{"user.id": "user123", "request.id": "req456"},
							},
						},
					},
					metrics.GaugeDataPoint{
						Timestamp:  baseTime + time.Minute.Nanoseconds(),
						StartTime:  startTime + time.Minute.Nanoseconds(),
						Attributes: map[string]any{"memory.type": "stack", "service.instance": "instance-2"},
						Flags:      0,
						ValueType:  "Double",
						Value:      2048.0,
						Exemplars: []metrics.Exemplar{
							{
								Timestamp:          baseTime + time.Minute.Nanoseconds() + 15*time.Second.Nanoseconds(),
								Value:              2048.0,
								TraceID:            "fedcba9876543210fedcba9876543210",
								SpanID:             "fedcba9876543210",
								FilteredAttributes: map[string]any{"user.id": "user789", "session.id": "sess101"},
							},
						},
					},
				},
			},
			Resource: &resource.ResourceData{
				Attributes: map[string]any{
					"service.name":    "test-service",
					"service.version": "1.0.0",
				},
				DroppedAttributesCount: 0,
			},
			Scope: &scope.ScopeData{
				Name:                   "test-scope",
				Version:                "v1.0.0",
				Attributes:             map[string]any{},
				DroppedAttributesCount: 0,
			},
			Received: baseTime,
		},
		{
			Name:        "sum_metric",
			Description: "Total requests processed",
			Unit:        "requests",
			DataPoints: metrics.DataPoints{
				Type: metrics.MetricTypeSum,
				Points: []metrics.MetricDataPoint{
					metrics.SumDataPoint{
						Timestamp:   baseTime + 2*time.Minute.Nanoseconds(),
						StartTime:   startTime + 2*time.Minute.Nanoseconds(),
						Attributes:  map[string]any{"http.method": "POST", "http.status_code": int64(200)},
						Flags:       0,
						ValueType:   "Double",
						Value:       1500.0,
						IsMonotonic: true,
						Exemplars: []metrics.Exemplar{
							{
								Timestamp:          baseTime + 2*time.Minute.Nanoseconds() + 10*time.Second.Nanoseconds(),
								Value:              1500.0,
								TraceID:            "abcdef0123456789abcdef0123456789",
								SpanID:             "abcdef0123456789",
								FilteredAttributes: map[string]any{"endpoint": "/api/create", "user.role": "admin"},
							},
						},
						AggregationTemporality: "Cumulative",
					},
					metrics.SumDataPoint{
						Timestamp:              baseTime + 3*time.Minute.Nanoseconds(),
						StartTime:              startTime + 3*time.Minute.Nanoseconds(),
						Attributes:             map[string]any{"http.method": "GET", "http.status_code": int64(404)},
						Flags:                  0,
						ValueType:              "Double",
						Value:                  2500.0,
						IsMonotonic:            true,
						Exemplars:              []metrics.Exemplar{},
						AggregationTemporality: "Cumulative",
					},
				},
			},
			Resource: &resource.ResourceData{
				Attributes: map[string]any{
					"service.name":    "test-service",
					"service.version": "1.0.0",
				},
				DroppedAttributesCount: 0,
			},
			Scope: &scope.ScopeData{
				Name:                   "test-scope",
				Version:                "v1.0.0",
				Attributes:             map[string]any{},
				DroppedAttributesCount: 0,
			},
			Received: baseTime + time.Minute.Nanoseconds(),
		},
		{
			Name:        "histogram_metric",
			Description: "Request duration distribution",
			Unit:        "seconds",
			DataPoints: metrics.DataPoints{
				Type: metrics.MetricTypeHistogram,
				Points: []metrics.MetricDataPoint{
					metrics.HistogramDataPoint{
						Timestamp:      baseTime + 4*time.Minute.Nanoseconds(),
						StartTime:      startTime + 4*time.Minute.Nanoseconds(),
						Attributes:     map[string]any{"http.method": "GET", "http.route": "/api/users"},
						Flags:          0,
						Count:          100,
						Sum:            25.5,
						Min:            0.1,
						Max:            2.5,
						BucketCounts:   []uint64{10, 20, 30, 25, 15},
						ExplicitBounds: []float64{0.5, 1.0, 1.5, 2.0},
						Exemplars: []metrics.Exemplar{
							{
								Timestamp:          baseTime + 4*time.Minute.Nanoseconds() + 5*time.Second.Nanoseconds(),
								Value:              1.25,
								TraceID:            "1111222233334444aaaa bbbbccccdddd",
								SpanID:             "1111222233334444",
								FilteredAttributes: map[string]any{"bucket": "1.0-1.5", "slow_request": true},
							},
							{
								Timestamp:          baseTime + 4*time.Minute.Nanoseconds() + 20*time.Second.Nanoseconds(),
								Value:              2.5,
								TraceID:            "5555666677778888eeeeffff00001111",
								SpanID:             "5555666677778888",
								FilteredAttributes: map[string]any{"bucket": "2.0+", "outlier": true},
							},
						},
						AggregationTemporality: "Delta",
					},
					metrics.HistogramDataPoint{
						Timestamp:              baseTime + 5*time.Minute.Nanoseconds(),
						StartTime:              startTime + 5*time.Minute.Nanoseconds(),
						Attributes:             map[string]any{"http.method": "POST", "http.route": "/api/orders"},
						Flags:                  0,
						Count:                  50,
						Sum:                    15.0,
						Min:                    0.05,
						Max:                    1.0,
						BucketCounts:           []uint64{5, 10, 15, 10, 10},
						ExplicitBounds:         []float64{0.2, 0.4, 0.6, 0.8},
						Exemplars:              []metrics.Exemplar{},
						AggregationTemporality: "Delta",
					},
				},
			},
			Resource: &resource.ResourceData{
				Attributes: map[string]any{
					"service.name":    "test-service",
					"service.version": "1.0.0",
				},
				DroppedAttributesCount: 0,
			},
			Scope: &scope.ScopeData{
				Name:                   "test-scope",
				Version:                "v1.0.0",
				Attributes:             map[string]any{},
				DroppedAttributesCount: 0,
			},
			Received: baseTime + 2*time.Minute.Nanoseconds(),
		},
		{
			Name:        "exponential_histogram_metric",
			Description: "Response size distribution",
			Unit:        "bytes",
			DataPoints: metrics.DataPoints{
				Type: metrics.MetricTypeExponentialHistogram,
				Points: []metrics.MetricDataPoint{
					metrics.ExponentialHistogramDataPoint{
						Timestamp:  baseTime + 6*time.Minute.Nanoseconds(),
						StartTime:  startTime + 6*time.Minute.Nanoseconds(),
						Attributes: map[string]any{"content.type": "application/json", "http.method": "POST"},
						Flags:      0,
						Count:      50,
						Sum:        10240.0,
						Min:        100.0,
						Max:        2048.0,
						Scale:      2,
						ZeroCount:  5,
						Positive: metrics.Buckets{
							BucketOffset: 1,
							BucketCounts: []uint64{5, 10, 15, 10, 5},
						},
						Negative: metrics.Buckets{
							BucketOffset: 0,
							BucketCounts: []uint64{2, 3},
						},
						Exemplars: []metrics.Exemplar{
							{
								Timestamp:          baseTime + 6*time.Minute.Nanoseconds() + 8*time.Second.Nanoseconds(),
								Value:              2048.0,
								TraceID:            "9999aaaabbbbcccc9999aaaabbbbcccc",
								SpanID:             "9999aaaabbbbcccc",
								FilteredAttributes: map[string]any{"response.size": "large", "compression": "gzip"},
							},
						},
						AggregationTemporality: "Delta",
					},
					metrics.ExponentialHistogramDataPoint{
						Timestamp:  baseTime + 7*time.Minute.Nanoseconds(),
						StartTime:  startTime + 7*time.Minute.Nanoseconds(),
						Attributes: map[string]any{"content.type": "text/plain", "http.method": "GET"},
						Flags:      0,
						Count:      25,
						Sum:        5120.0,
						Min:        50.0,
						Max:        1024.0,
						Scale:      1,
						ZeroCount:  3,
						Positive: metrics.Buckets{
							BucketOffset: 0,
							BucketCounts: []uint64{3, 7, 10, 5},
						},
						Negative: metrics.Buckets{
							BucketOffset: 0,
							BucketCounts: []uint64{1, 1},
						},
						Exemplars:              []metrics.Exemplar{},
						AggregationTemporality: "Delta",
					},
				},
			},
			Resource: &resource.ResourceData{
				Attributes: map[string]any{
					"service.name":    "test-service",
					"service.version": "1.0.0",
				},
				DroppedAttributesCount: 0,
			},
			Scope: &scope.ScopeData{
				Name:                   "test-scope",
				Version:                "v1.0.0",
				Attributes:             map[string]any{},
				DroppedAttributesCount: 0,
			},
			Received: baseTime + 3*time.Minute.Nanoseconds(),
		},
	}
}

// TestMetricSuite runs a comprehensive suite of tests on metrics
func TestMetricSuite(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	// Add test metrics
	metricsData := createTestMetrics()
	err := helper.Store.AddMetrics(helper.Ctx, metricsData)
	assert.NoError(t, err, "failed to add test metrics")

	t.Run("MetricRetrieval", func(t *testing.T) {
		retrievedMetrics, err := helper.Store.GetMetrics(helper.Ctx)
		assert.NoError(t, err)
		assert.Len(t, retrievedMetrics, 4, "should have four metrics")

		// Verify metrics are ordered by received time (newest first)
		assert.Equal(t, "exponential_histogram_metric", retrievedMetrics[0].Name)
		assert.Equal(t, "histogram_metric", retrievedMetrics[1].Name)
		assert.Equal(t, "sum_metric", retrievedMetrics[2].Name)
		assert.Equal(t, "gauge_metric", retrievedMetrics[3].Name)
	})

	t.Run("GaugeMetric", func(t *testing.T) {
		retrievedMetrics, err := helper.Store.GetMetrics(helper.Ctx)
		assert.NoError(t, err)

		gaugeMetric := retrievedMetrics[3] // Last in the list (oldest)
		assert.Equal(t, "gauge_metric", gaugeMetric.Name)
		assert.Equal(t, "Current memory usage", gaugeMetric.Description)
		assert.Equal(t, "bytes", gaugeMetric.Unit)
		assert.Equal(t, metrics.MetricTypeGauge, gaugeMetric.DataPoints.Type)

		// Verify gauge data points
		assert.Len(t, gaugeMetric.DataPoints.Points, 2)
		gaugePoint1 := gaugeMetric.DataPoints.Points[0].(metrics.GaugeDataPoint)
		assert.Equal(t, "Double", gaugePoint1.ValueType)
		assert.Equal(t, 1024.5, gaugePoint1.Value)
		assert.Equal(t, uint32(0), gaugePoint1.Flags)
		assert.Equal(t, "heap", gaugePoint1.Attributes["memory.type"])
		assert.Equal(t, "instance-1", gaugePoint1.Attributes["service.instance"])

		gaugePoint2 := gaugeMetric.DataPoints.Points[1].(metrics.GaugeDataPoint)
		assert.Equal(t, 2048.0, gaugePoint2.Value)
		assert.Equal(t, "stack", gaugePoint2.Attributes["memory.type"])
		assert.Equal(t, "instance-2", gaugePoint2.Attributes["service.instance"])

		// Verify exemplars
		assert.Len(t, gaugePoint1.Exemplars, 1)
		exemplar1 := gaugePoint1.Exemplars[0]
		assert.Equal(t, 1024.5, exemplar1.Value)
		assert.Equal(t, "0123456789abcdef0123456789abcdef", exemplar1.TraceID)
		assert.Equal(t, "0123456789abcdef", exemplar1.SpanID)
		assert.Greater(t, exemplar1.Timestamp, int64(0))
		assert.Equal(t, "user123", exemplar1.FilteredAttributes["user.id"])
		assert.Equal(t, "req456", exemplar1.FilteredAttributes["request.id"])

		assert.Len(t, gaugePoint2.Exemplars, 1)
		exemplar2 := gaugePoint2.Exemplars[0]
		assert.Equal(t, 2048.0, exemplar2.Value)
		assert.Equal(t, "fedcba9876543210fedcba9876543210", exemplar2.TraceID)
		assert.Equal(t, "fedcba9876543210", exemplar2.SpanID)
		assert.Greater(t, exemplar2.Timestamp, int64(0))
		assert.Equal(t, "user789", exemplar2.FilteredAttributes["user.id"])
		assert.Equal(t, "sess101", exemplar2.FilteredAttributes["session.id"])
	})

	t.Run("SumMetric", func(t *testing.T) {
		retrievedMetrics, err := helper.Store.GetMetrics(helper.Ctx)
		assert.NoError(t, err)

		sumMetric := retrievedMetrics[2]
		assert.Equal(t, "sum_metric", sumMetric.Name)
		assert.Equal(t, "Total requests processed", sumMetric.Description)
		assert.Equal(t, "requests", sumMetric.Unit)
		assert.Equal(t, metrics.MetricTypeSum, sumMetric.DataPoints.Type)

		// Verify sum data points
		assert.Len(t, sumMetric.DataPoints.Points, 2)
		sumPoint1 := sumMetric.DataPoints.Points[0].(metrics.SumDataPoint)
		assert.Equal(t, 1500.0, sumPoint1.Value)
		assert.Equal(t, true, sumPoint1.IsMonotonic)
		assert.Equal(t, "Cumulative", sumPoint1.AggregationTemporality)
		assert.Equal(t, "POST", sumPoint1.Attributes["http.method"])
		assert.Equal(t, int64(200), sumPoint1.Attributes["http.status_code"])

		sumPoint2 := sumMetric.DataPoints.Points[1].(metrics.SumDataPoint)
		assert.Equal(t, 2500.0, sumPoint2.Value)
		assert.Equal(t, "GET", sumPoint2.Attributes["http.method"])
		assert.Equal(t, int64(404), sumPoint2.Attributes["http.status_code"])

		// Verify exemplars
		assert.Len(t, sumPoint1.Exemplars, 1)
		sumExemplar := sumPoint1.Exemplars[0]
		assert.Equal(t, 1500.0, sumExemplar.Value)
		assert.Equal(t, "abcdef0123456789abcdef0123456789", sumExemplar.TraceID)
		assert.Equal(t, "abcdef0123456789", sumExemplar.SpanID)
		assert.Greater(t, sumExemplar.Timestamp, int64(0))
		assert.Equal(t, "/api/create", sumExemplar.FilteredAttributes["endpoint"])
		assert.Equal(t, "admin", sumExemplar.FilteredAttributes["user.role"])
	})

	t.Run("HistogramMetric", func(t *testing.T) {
		retrievedMetrics, err := helper.Store.GetMetrics(helper.Ctx)
		assert.NoError(t, err)

		histogramMetric := retrievedMetrics[1]
		assert.Equal(t, "histogram_metric", histogramMetric.Name)
		assert.Equal(t, "Request duration distribution", histogramMetric.Description)
		assert.Equal(t, "seconds", histogramMetric.Unit)
		assert.Equal(t, metrics.MetricTypeHistogram, histogramMetric.DataPoints.Type)

		// Verify histogram data points
		assert.Len(t, histogramMetric.DataPoints.Points, 2)
		histogramPoint1 := histogramMetric.DataPoints.Points[0].(metrics.HistogramDataPoint)
		assert.Equal(t, uint64(100), histogramPoint1.Count)
		assert.Equal(t, 25.5, histogramPoint1.Sum)
		assert.Equal(t, 0.1, histogramPoint1.Min)
		assert.Equal(t, 2.5, histogramPoint1.Max)
		assert.Equal(t, "Delta", histogramPoint1.AggregationTemporality)
		assert.Equal(t, "GET", histogramPoint1.Attributes["http.method"])
		assert.Equal(t, "/api/users", histogramPoint1.Attributes["http.route"])
		assert.Equal(t, []uint64{10, 20, 30, 25, 15}, histogramPoint1.BucketCounts)
		assert.Equal(t, []float64{0.5, 1.0, 1.5, 2.0}, histogramPoint1.ExplicitBounds)

		histogramPoint2 := histogramMetric.DataPoints.Points[1].(metrics.HistogramDataPoint)
		assert.Equal(t, uint64(50), histogramPoint2.Count)
		assert.Equal(t, 15.0, histogramPoint2.Sum)
		assert.Equal(t, 0.05, histogramPoint2.Min)
		assert.Equal(t, 1.0, histogramPoint2.Max)
		assert.Equal(t, "POST", histogramPoint2.Attributes["http.method"])
		assert.Equal(t, "/api/orders", histogramPoint2.Attributes["http.route"])

		// Verify exemplars
		assert.Len(t, histogramPoint1.Exemplars, 2)
		histExemplar1 := histogramPoint1.Exemplars[0]
		assert.Equal(t, 1.25, histExemplar1.Value)
		assert.Equal(t, "1111222233334444aaaa bbbbccccdddd", histExemplar1.TraceID)
		assert.Equal(t, "1111222233334444", histExemplar1.SpanID)
		assert.Greater(t, histExemplar1.Timestamp, int64(0))
		assert.Equal(t, "1.0-1.5", histExemplar1.FilteredAttributes["bucket"])
		assert.Equal(t, true, histExemplar1.FilteredAttributes["slow_request"])

		histExemplar2 := histogramPoint1.Exemplars[1]
		assert.Equal(t, 2.5, histExemplar2.Value)
		assert.Equal(t, "5555666677778888eeeeffff00001111", histExemplar2.TraceID)
		assert.Equal(t, "5555666677778888", histExemplar2.SpanID)
		assert.Greater(t, histExemplar2.Timestamp, int64(0))
		assert.Equal(t, "2.0+", histExemplar2.FilteredAttributes["bucket"])
		assert.Equal(t, true, histExemplar2.FilteredAttributes["outlier"])
	})

	t.Run("ExponentialHistogramMetric", func(t *testing.T) {
		retrievedMetrics, err := helper.Store.GetMetrics(helper.Ctx)
		assert.NoError(t, err)

		expHistogramMetric := retrievedMetrics[0]
		assert.Equal(t, "exponential_histogram_metric", expHistogramMetric.Name)
		assert.Equal(t, "Response size distribution", expHistogramMetric.Description)
		assert.Equal(t, "bytes", expHistogramMetric.Unit)
		assert.Equal(t, metrics.MetricTypeExponentialHistogram, expHistogramMetric.DataPoints.Type)

		// Verify exponential histogram data points
		assert.Len(t, expHistogramMetric.DataPoints.Points, 2)
		expHistogramPoint1 := expHistogramMetric.DataPoints.Points[0].(metrics.ExponentialHistogramDataPoint)
		assert.Equal(t, uint64(50), expHistogramPoint1.Count)
		assert.Equal(t, 10240.0, expHistogramPoint1.Sum)
		assert.Equal(t, 100.0, expHistogramPoint1.Min)
		assert.Equal(t, 2048.0, expHistogramPoint1.Max)
		assert.Equal(t, int32(2), expHistogramPoint1.Scale)
		assert.Equal(t, uint64(5), expHistogramPoint1.ZeroCount)
		assert.Equal(t, "Delta", expHistogramPoint1.AggregationTemporality)
		assert.Equal(t, "application/json", expHistogramPoint1.Attributes["content.type"])
		assert.Equal(t, "POST", expHistogramPoint1.Attributes["http.method"])

		// Verify positive buckets
		assert.Equal(t, int32(1), expHistogramPoint1.Positive.BucketOffset)
		assert.Equal(t, []uint64{5, 10, 15, 10, 5}, expHistogramPoint1.Positive.BucketCounts)

		// Verify negative buckets
		assert.Equal(t, int32(0), expHistogramPoint1.Negative.BucketOffset)
		assert.Equal(t, []uint64{2, 3}, expHistogramPoint1.Negative.BucketCounts)

		expHistogramPoint2 := expHistogramMetric.DataPoints.Points[1].(metrics.ExponentialHistogramDataPoint)
		assert.Equal(t, uint64(25), expHistogramPoint2.Count)
		assert.Equal(t, 5120.0, expHistogramPoint2.Sum)
		assert.Equal(t, 50.0, expHistogramPoint2.Min)
		assert.Equal(t, 1024.0, expHistogramPoint2.Max)
		assert.Equal(t, int32(1), expHistogramPoint2.Scale)
		assert.Equal(t, uint64(3), expHistogramPoint2.ZeroCount)
		assert.Equal(t, "text/plain", expHistogramPoint2.Attributes["content.type"])
		assert.Equal(t, "GET", expHistogramPoint2.Attributes["http.method"])

		// Verify exemplars
		assert.Len(t, expHistogramPoint1.Exemplars, 1)
		expExemplar := expHistogramPoint1.Exemplars[0]
		assert.Equal(t, 2048.0, expExemplar.Value)
		assert.Equal(t, "9999aaaabbbbcccc9999aaaabbbbcccc", expExemplar.TraceID)
		assert.Equal(t, "9999aaaabbbbcccc", expExemplar.SpanID)
		assert.Greater(t, expExemplar.Timestamp, int64(0))
		assert.Equal(t, "large", expExemplar.FilteredAttributes["response.size"])
		assert.Equal(t, "gzip", expExemplar.FilteredAttributes["compression"])
	})

	t.Run("MetricResourceAndScope", func(t *testing.T) {
		retrievedMetrics, err := helper.Store.GetMetrics(helper.Ctx)
		assert.NoError(t, err)

		// Verify resource and scope are consistent across all metrics
		for i, metric := range retrievedMetrics {
			assert.Equal(t, "test-service", metric.Resource.Attributes["service.name"], "metric %d service name", i)
			assert.Equal(t, "1.0.0", metric.Resource.Attributes["service.version"], "metric %d service version", i)
			assert.Equal(t, uint32(0), metric.Resource.DroppedAttributesCount, "metric %d resource dropped count", i)
			assert.Equal(t, "test-scope", metric.Scope.Name, "metric %d scope name", i)
			assert.Equal(t, "v1.0.0", metric.Scope.Version, "metric %d scope version", i)
			assert.Empty(t, metric.Scope.Attributes, "metric %d scope attributes", i)
			assert.Equal(t, uint32(0), metric.Scope.DroppedAttributesCount, "metric %d scope dropped count", i)
		}
	})

	t.Run("MetricTimestamps", func(t *testing.T) {
		retrievedMetrics, err := helper.Store.GetMetrics(helper.Ctx)
		assert.NoError(t, err)

		// Verify metrics are ordered by received time (newest first)
		for i := 0; i < len(retrievedMetrics)-1; i++ {
			assert.GreaterOrEqual(t, retrievedMetrics[i].Received, retrievedMetrics[i+1].Received,
				"metrics should be ordered by received time (newest first)")
		}

		// Verify all received timestamps are valid (greater than 0)
		for i, metric := range retrievedMetrics {
			assert.Greater(t, metric.Received, int64(0), "metric %d received time should be valid", i)

			// Verify data point timestamps are valid
			for j, dataPoint := range metric.DataPoints.Points {
				switch dp := dataPoint.(type) {
				case metrics.GaugeDataPoint:
					assert.Greater(t, dp.Timestamp, int64(0), "gauge data point %d timestamp", j)
					assert.Greater(t, dp.StartTime, int64(0), "gauge data point %d start time", j)
				case metrics.SumDataPoint:
					assert.Greater(t, dp.Timestamp, int64(0), "sum data point %d timestamp", j)
					assert.Greater(t, dp.StartTime, int64(0), "sum data point %d start time", j)
				case metrics.HistogramDataPoint:
					assert.Greater(t, dp.Timestamp, int64(0), "histogram data point %d timestamp", j)
					assert.Greater(t, dp.StartTime, int64(0), "histogram data point %d start time", j)
				case metrics.ExponentialHistogramDataPoint:
					assert.Greater(t, dp.Timestamp, int64(0), "exponential histogram data point %d timestamp", j)
					assert.Greater(t, dp.StartTime, int64(0), "exponential histogram data point %d start time", j)
				}
			}
		}
	})
}

// TestEmptyMetrics verifies handling of empty metric lists and empty stores
func TestEmptyMetrics(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	// Test adding empty metric list
	err := helper.Store.AddMetrics(helper.Ctx, []metrics.MetricData{})
	assert.NoError(t, err)

	// Test getting metrics from empty store
	metrics, err := helper.Store.GetMetrics(helper.Ctx)
	assert.NoError(t, err)
	assert.Empty(t, metrics)
}

// TestClearMetrics verifies that all metrics can be cleared from the store
func TestClearMetrics(t *testing.T) {
	helper, teardown := SetupTest(t)
	defer teardown()

	// Add test metrics
	metricsData := createTestMetrics()
	err := helper.Store.AddMetrics(helper.Ctx, metricsData)
	assert.NoError(t, err)

	// Verify metrics exist
	retrievedMetrics, err := helper.Store.GetMetrics(helper.Ctx)
	assert.NoError(t, err)
	assert.Len(t, retrievedMetrics, 4)

	// Clear metrics
	err = helper.Store.ClearMetrics(helper.Ctx)
	assert.NoError(t, err)

	// Verify store is empty
	retrievedMetrics, err = helper.Store.GetMetrics(helper.Ctx)
	assert.NoError(t, err)
	assert.Empty(t, retrievedMetrics)
}
