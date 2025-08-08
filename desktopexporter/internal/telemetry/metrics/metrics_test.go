package metrics

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var metrics []MetricData

func init() {
	metrics = GenerateSampleMetrics()
}

func TestMetricExtraction(t *testing.T) {
	tests := []struct {
		name     string
		validate func(t *testing.T, metrics []MetricData)
	}{
		{
			name: "extracts correct number of metrics",
			validate: func(t *testing.T, metrics []MetricData) {
				assert.Len(t, metrics, 4)
			},
		},
		{
			name: "validates gauge metric",
			validate: func(t *testing.T, metrics []MetricData) {
				gaugeMetric := metrics[0]
				assert.Equal(t, "memory.usage", gaugeMetric.Name)
				assert.Equal(t, "Current memory usage", gaugeMetric.Description)
				assert.Equal(t, "bytes", gaugeMetric.Unit)
				assert.Equal(t, MetricTypeGauge, gaugeMetric.DataPoints.Type)

				// Validate gauge data point
				gaugeDataPoint := gaugeMetric.DataPoints.Points[0].(GaugeDataPoint)
				assert.Equal(t, time.Date(2023, 02, 01, 20, 25, 36, 179472007, time.UTC), time.Unix(0, gaugeDataPoint.Timestamp).In(time.UTC))
				assert.Equal(t, uint32(0), gaugeDataPoint.Flags)
				assert.Equal(t, "Double", gaugeDataPoint.ValueType)
				assert.Equal(t, 1024.5, gaugeDataPoint.Value)

				expectedAttrs := map[string]any{
					"memory.type":      "heap",
					"service.instance": "currencyservice-1",
				}

				for key, expected := range expectedAttrs {
					assert.Equal(t, expected, gaugeDataPoint.Attributes[key], "gauge attribute %s", key)
				}
			},
		},
		{
			name: "validates sum metric",
			validate: func(t *testing.T, metrics []MetricData) {
				sumMetric := metrics[1]
				assert.Equal(t, "requests.total", sumMetric.Name)
				assert.Equal(t, "Total requests processed", sumMetric.Description)
				assert.Equal(t, "requests", sumMetric.Unit)
				assert.Equal(t, MetricTypeSum, sumMetric.DataPoints.Type)

				// Validate sum data point
				sumDataPoint := sumMetric.DataPoints.Points[0].(SumDataPoint)
				assert.Equal(t, time.Date(2023, 02, 01, 20, 25, 36, 179472007, time.UTC), time.Unix(0, sumDataPoint.Timestamp).In(time.UTC))
				assert.Equal(t, uint32(0), sumDataPoint.Flags)
				assert.Equal(t, 1500.0, sumDataPoint.Value)
				assert.Equal(t, true, sumDataPoint.IsMonotonic)
				assert.Equal(t, "Cumulative", sumDataPoint.AggregationTemporality)

				expectedAttrs := map[string]any{
					"http.method":      "POST",
					"http.status_code": int64(200),
				}

				for key, expected := range expectedAttrs {
					assert.Equal(t, expected, sumDataPoint.Attributes[key], "sum attribute %s", key)
				}
			},
		},
		{
			name: "validates histogram metric",
			validate: func(t *testing.T, metrics []MetricData) {
				histogramMetric := metrics[2]
				assert.Equal(t, "request.duration", histogramMetric.Name)
				assert.Equal(t, "Request duration distribution", histogramMetric.Description)
				assert.Equal(t, "seconds", histogramMetric.Unit)
				assert.Equal(t, MetricTypeHistogram, histogramMetric.DataPoints.Type)

				// Validate histogram data point
				histogramDataPoint := histogramMetric.DataPoints.Points[0].(HistogramDataPoint)
				assert.Equal(t, time.Date(2023, 02, 01, 20, 25, 36, 179472007, time.UTC), time.Unix(0, histogramDataPoint.Timestamp).In(time.UTC))
				assert.Equal(t, uint32(0), histogramDataPoint.Flags)
				assert.Equal(t, uint64(100), histogramDataPoint.Count)
				assert.Equal(t, 25.5, histogramDataPoint.Sum)
				assert.Equal(t, 0.1, histogramDataPoint.Min)
				assert.Equal(t, 2.5, histogramDataPoint.Max)
				assert.Equal(t, "Delta", histogramDataPoint.AggregationTemporality)

				// Validate bucket counts and bounds
				expectedBucketCounts := []uint64{10, 20, 30, 25, 15}
				assert.Equal(t, expectedBucketCounts, histogramDataPoint.BucketCounts)

				expectedExplicitBounds := []float64{0.5, 1.0, 1.5, 2.0}
				assert.Equal(t, expectedExplicitBounds, histogramDataPoint.ExplicitBounds)

				expectedAttrs := map[string]any{
					"http.method": "GET",
					"http.route":  "/api/convert",
				}

				for key, expected := range expectedAttrs {
					assert.Equal(t, expected, histogramDataPoint.Attributes[key], "histogram attribute %s", key)
				}
			},
		},
		{
			name: "validates exponential histogram metric",
			validate: func(t *testing.T, metrics []MetricData) {
				expHistogramMetric := metrics[3]
				assert.Equal(t, "response.size", expHistogramMetric.Name)
				assert.Equal(t, "Response size distribution", expHistogramMetric.Description)
				assert.Equal(t, "bytes", expHistogramMetric.Unit)
				assert.Equal(t, MetricTypeExponentialHistogram, expHistogramMetric.DataPoints.Type)

				// Validate exponential histogram data point
				expHistogramDataPoint := expHistogramMetric.DataPoints.Points[0].(ExponentialHistogramDataPoint)
				assert.Equal(t, time.Date(2023, 02, 01, 20, 25, 36, 179472007, time.UTC), time.Unix(0, expHistogramDataPoint.Timestamp).In(time.UTC))
				assert.Equal(t, uint32(0), expHistogramDataPoint.Flags)
				assert.Equal(t, uint64(50), expHistogramDataPoint.Count)
				assert.Equal(t, 10240.0, expHistogramDataPoint.Sum)
				assert.Equal(t, 100.0, expHistogramDataPoint.Min)
				assert.Equal(t, 2048.0, expHistogramDataPoint.Max)
				assert.Equal(t, int32(2), expHistogramDataPoint.Scale)
				assert.Equal(t, uint64(5), expHistogramDataPoint.ZeroCount)
				assert.Equal(t, "Delta", expHistogramDataPoint.AggregationTemporality)

				// Validate positive buckets
				assert.Equal(t, int32(1), expHistogramDataPoint.Positive.BucketOffset)
				expectedPositiveBucketCounts := []uint64{5, 10, 15, 10, 5}
				assert.Equal(t, expectedPositiveBucketCounts, expHistogramDataPoint.Positive.BucketCounts)

				// Validate negative buckets
				assert.Equal(t, int32(0), expHistogramDataPoint.Negative.BucketOffset)
				expectedNegativeBucketCounts := []uint64{2, 3}
				assert.Equal(t, expectedNegativeBucketCounts, expHistogramDataPoint.Negative.BucketCounts)

				expectedAttrs := map[string]any{
					"content.type": "application/json",
					"http.method":  "POST",
				}

				for key, expected := range expectedAttrs {
					assert.Equal(t, expected, expHistogramDataPoint.Attributes[key], "exponential histogram attribute %s", key)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.validate(t, metrics)
		})
	}
}

func TestMetricMarshaling(t *testing.T) {
	metric := metrics[0] // Use gauge metric for testing

	jsonBytes, err := metric.MarshalJSON()
	assert.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(jsonBytes, &result)
	assert.NoError(t, err)

	tests := []struct {
		name     string
		validate func(t *testing.T, result map[string]any)
	}{
		{
			name: "validates timestamp format",
			validate: func(t *testing.T, result map[string]any) {
				// Validate received timestamp is encoded as string
				received := result["received"].(string)
				expectedReceived := strconv.FormatInt(metric.Received, 10)
				assert.Equal(t, expectedReceived, received, "received timestamp should be encoded as string nanoseconds")
			},
		},
		{
			name: "validates basic fields",
			validate: func(t *testing.T, result map[string]any) {
				assert.Equal(t, metric.Name, result["name"])
				assert.Equal(t, metric.Description, result["description"])
				assert.Equal(t, metric.Unit, result["unit"])
			},
		},
		{
			name: "validates data points",
			validate: func(t *testing.T, result map[string]any) {
				dataPoints := result["dataPoints"].(map[string]any)

				// Check type is nested in dataPoints
				assert.Equal(t, string(metric.DataPoints.Type), dataPoints["type"])

				assert.Len(t, dataPoints["points"], len(metric.DataPoints.Points))

				// Validate first data point timestamp format
				firstDataPoint := dataPoints["points"].([]any)[0].(map[string]any)
				timestamp := firstDataPoint["timestamp"].(string)
				expectedTimestamp := "1675283136179472007"
				assert.Equal(t, expectedTimestamp, timestamp, "data point timestamp should be encoded as string nanoseconds")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.validate(t, result)
		})
	}
}

func TestDataPointMarshaling(t *testing.T) {
	tests := []struct {
		name      string
		dataPoint MetricDataPoint
		validate  func(t *testing.T, result map[string]any)
	}{
		{
			name:      "validates gauge data point marshaling",
			dataPoint: metrics[0].DataPoints.Points[0],
			validate: func(t *testing.T, result map[string]any) {
				// Validate timestamp format
				timestamp := result["timestamp"].(string)
				expectedTimestamp := "1675283136179472007"
				assert.Equal(t, expectedTimestamp, timestamp, "gauge timestamp should be encoded as string")

				// Validate start time format
				startTime := result["startTimeUnixNano"].(string)
				assert.NotEmpty(t, startTime, "gauge start time should be encoded as string")

				// Validate basic fields
				assert.Equal(t, "Double", result["valueType"])
				assert.Equal(t, 1024.5, result["value"])
				assert.Equal(t, float64(0), result["flags"])
			},
		},
		{
			name:      "validates sum data point marshaling",
			dataPoint: metrics[1].DataPoints.Points[0],
			validate: func(t *testing.T, result map[string]any) {
				// Validate timestamp format
				timestamp := result["timestamp"].(string)
				expectedTimestamp := "1675283136179472007"
				assert.Equal(t, expectedTimestamp, timestamp, "sum timestamp should be encoded as string")

				// Validate basic fields
				assert.Equal(t, 1500.0, result["value"])
				assert.Equal(t, true, result["isMonotonic"])
				assert.Equal(t, "Cumulative", result["aggregationTemporality"])
			},
		},
		{
			name:      "validates histogram data point marshaling",
			dataPoint: metrics[2].DataPoints.Points[0],
			validate: func(t *testing.T, result map[string]any) {
				// Validate timestamp format
				timestamp := result["timestamp"].(string)
				expectedTimestamp := "1675283136179472007"
				assert.Equal(t, expectedTimestamp, timestamp, "histogram timestamp should be encoded as string")

				// Validate count and bucket counts as strings
				count := result["count"].(string)
				assert.Equal(t, "100", count, "histogram count should be encoded as string")

				bucketCounts := result["bucketCounts"].([]any)
				expectedBucketCounts := []string{"10", "20", "30", "25", "15"}
				for i, expected := range expectedBucketCounts {
					assert.Equal(t, expected, bucketCounts[i], "histogram bucket count %d", i)
				}

				// Validate basic fields
				assert.Equal(t, 25.5, result["sum"])
				assert.Equal(t, 0.1, result["min"])
				assert.Equal(t, 2.5, result["max"])
				assert.Equal(t, "Delta", result["aggregationTemporality"])
			},
		},
		{
			name:      "validates exponential histogram data point marshaling",
			dataPoint: metrics[3].DataPoints.Points[0],
			validate: func(t *testing.T, result map[string]any) {
				// Validate timestamp format
				timestamp := result["timestamp"].(string)
				expectedTimestamp := "1675283136179472007"
				assert.Equal(t, expectedTimestamp, timestamp, "exponential histogram timestamp should be encoded as string")

				// Validate count and zero count as strings
				count := result["count"].(string)
				assert.Equal(t, "50", count, "exponential histogram count should be encoded as string")

				zeroCount := result["zeroCount"].(string)
				assert.Equal(t, "5", zeroCount, "exponential histogram zero count should be encoded as string")

				// Validate basic fields
				assert.Equal(t, 10240.0, result["sum"])
				assert.Equal(t, 100.0, result["min"])
				assert.Equal(t, 2048.0, result["max"])
				assert.Equal(t, float64(2), result["scale"])
				assert.Equal(t, "Delta", result["aggregationTemporality"])

				// Validate positive buckets
				positive := result["positive"].(map[string]any)
				assert.Equal(t, float64(1), positive["bucketOffset"])
				positiveBucketCounts := positive["bucketCounts"].([]any)
				expectedPositiveBucketCounts := []string{"5", "10", "15", "10", "5"}
				for i, expected := range expectedPositiveBucketCounts {
					assert.Equal(t, expected, positiveBucketCounts[i], "positive bucket count %d", i)
				}

				// Validate negative buckets
				negative := result["negative"].(map[string]any)
				assert.Equal(t, float64(0), negative["bucketOffset"])
				negativeBucketCounts := negative["bucketCounts"].([]any)
				expectedNegativeBucketCounts := []string{"2", "3"}
				for i, expected := range expectedNegativeBucketCounts {
					assert.Equal(t, expected, negativeBucketCounts[i], "negative bucket count %d", i)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBytes, err := json.Marshal(tt.dataPoint)
			assert.NoError(t, err)

			var result map[string]any
			err = json.Unmarshal(jsonBytes, &result)
			assert.NoError(t, err)

			tt.validate(t, result)
		})
	}
}
