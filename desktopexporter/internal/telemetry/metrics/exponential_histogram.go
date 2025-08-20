package metrics

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/attributes"
	"github.com/go-viper/mapstructure/v2"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

// Buckets represents histogram buckets
type Buckets struct {
	BucketOffset int32    `json:"bucketOffset"`
	BucketCounts []uint64 `json:"bucketCounts"`
}

// ExponentialHistogramDataPoint represents an exponential histogram metric data point
type ExponentialHistogramDataPoint struct {
	Timestamp              int64                 `json:"-"`
	StartTime              int64                 `json:"-"`
	Attributes             attributes.Attributes `json:"attributes"`
	Flags                  uint32                `json:"flags"`
	Count                  uint64                `json:"-"`
	Sum                    float64               `json:"sum"`
	Min                    float64               `json:"min"`
	Max                    float64               `json:"max"`
	Scale                  int32                 `json:"scale"`
	ZeroCount              uint64                `json:"-"`
	Positive               Buckets               `json:"positive"`
	Negative               Buckets               `json:"negative"`
	Exemplars              []Exemplar            `json:"exemplars,omitempty"`
	AggregationTemporality string                `json:"aggregationTemporality"`
}

// extractExponentialHistogramDataPoints extracts exponential histogram data points from OpenTelemetry exponential histogram data point slices
func extractExponentialHistogramDataPoints(source pmetric.ExponentialHistogram) []MetricDataPoint {
	points := make([]MetricDataPoint, source.DataPoints().Len())
	for i := range source.DataPoints().Len() {
		sourcePoint := source.DataPoints().At(i)
		point := ExponentialHistogramDataPoint{
			Timestamp:  sourcePoint.Timestamp().AsTime().UnixNano(),
			StartTime:  sourcePoint.StartTimestamp().AsTime().UnixNano(),
			Attributes: attributes.Attributes(sourcePoint.Attributes().AsRaw()),
			Flags:      uint32(sourcePoint.Flags()),
			Count:      sourcePoint.Count(),
			Sum:        sourcePoint.Sum(),
			Min:        sourcePoint.Min(),
			Max:        sourcePoint.Max(),
			Scale:      sourcePoint.Scale(),
			ZeroCount:  sourcePoint.ZeroCount(),
			Positive: Buckets{
				BucketOffset: sourcePoint.Positive().Offset(),
				BucketCounts: sourcePoint.Positive().BucketCounts().AsRaw(),
			},
			Negative: Buckets{
				BucketOffset: sourcePoint.Negative().Offset(),
				BucketCounts: sourcePoint.Negative().BucketCounts().AsRaw(),
			},
			Exemplars:              extractExemplars(sourcePoint.Exemplars()),
			AggregationTemporality: source.AggregationTemporality().String(),
		}
		points[i] = point
	}
	return points
}

// MarshalJSON implementations for timestamp, count, and zeroCount serialization
func (dataPoint ExponentialHistogramDataPoint) MarshalJSON() ([]byte, error) {
	type Alias ExponentialHistogramDataPoint // Avoid recursive MarshalJSON calls
	return json.Marshal(&struct {
		Alias
		Timestamp         string `json:"timestamp"`
		StartTimeUnixNano string `json:"startTimeUnixNano"`
		Count             string `json:"count"`
		ZeroCount         string `json:"zeroCount"`
	}{
		Alias:             Alias(dataPoint),
		Timestamp:         strconv.FormatInt(dataPoint.Timestamp, 10),
		StartTimeUnixNano: strconv.FormatInt(dataPoint.StartTime, 10),
		Count:             strconv.FormatUint(dataPoint.Count, 10),
		ZeroCount:         strconv.FormatUint(dataPoint.ZeroCount, 10),
	})
}

func (buckets Buckets) MarshalJSON() ([]byte, error) {
	type Alias Buckets // Avoid recursive MarshalJSON calls
	bucketCountsStr := make([]string, len(buckets.BucketCounts))
	for i, count := range buckets.BucketCounts {
		bucketCountsStr[i] = strconv.FormatUint(count, 10)
	}
	return json.Marshal(&struct {
		Alias
		BucketCounts []string `json:"bucketCounts"`
	}{
		Alias:        Alias(buckets),
		BucketCounts: bucketCountsStr,
	})
}

func (dataPoint *ExponentialHistogramDataPoint) Scan(src any) error {
	if src == nil {
		return nil
	}

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: attributes.AttributesDecodeHook,
		Result:     dataPoint,
	})
	if err != nil {
		return fmt.Errorf("failed to create decoder: %w", err)
	}

	if err := decoder.Decode(src); err != nil {
		return fmt.Errorf("failed to decode ExponentialHistogramDataPoint: %w", err)
	}

	return nil
}
