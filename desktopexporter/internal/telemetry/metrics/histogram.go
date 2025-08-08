package metrics

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/attributes"
	"github.com/mitchellh/mapstructure"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

// HistogramDataPoint represents a histogram metric data point
type HistogramDataPoint struct {
	Timestamp              int64                 `json:"-"`
	StartTime              int64                 `json:"-"`
	Attributes             attributes.Attributes `json:"attributes"`
	Flags                  uint32                `json:"flags"`
	Count                  uint64                `json:"-"`
	Sum                    float64               `json:"sum"`
	Min                    float64               `json:"min"`
	Max                    float64               `json:"max"`
	BucketCounts           []uint64              `json:"-"`
	ExplicitBounds         []float64             `json:"explicitBounds"`
	Exemplars              []Exemplar            `json:"exemplars,omitempty"`
	AggregationTemporality string                `json:"aggregationTemporality"`
}

func extractHistogramDataPoints(source pmetric.Histogram) []MetricDataPoint {
	points := make([]MetricDataPoint, source.DataPoints().Len())
	for i := range source.DataPoints().Len() {
		sourcePoint := source.DataPoints().At(i)
		point := HistogramDataPoint{
			Timestamp:              sourcePoint.Timestamp().AsTime().UnixNano(),
			StartTime:              sourcePoint.StartTimestamp().AsTime().UnixNano(),
			Attributes:             attributes.Attributes(sourcePoint.Attributes().AsRaw()),
			Flags:                  uint32(sourcePoint.Flags()),
			Count:                  sourcePoint.Count(),
			Sum:                    sourcePoint.Sum(),
			Min:                    sourcePoint.Min(),
			Max:                    sourcePoint.Max(),
			BucketCounts:           sourcePoint.BucketCounts().AsRaw(),
			ExplicitBounds:         sourcePoint.ExplicitBounds().AsRaw(),
			Exemplars:              extractExemplars(sourcePoint.Exemplars()),
			AggregationTemporality: source.AggregationTemporality().String(),
		}
		points[i] = point
	}
	return points
}

// MarshalJSON implementations for timestamp, count, and bucketCounts serialization
func (dataPoint HistogramDataPoint) MarshalJSON() ([]byte, error) {
	type Alias HistogramDataPoint // Avoid recursive MarshalJSON calls

	// Convert bucket counts to strings
	bucketCountsStr := make([]string, len(dataPoint.BucketCounts))
	for i, count := range dataPoint.BucketCounts {
		bucketCountsStr[i] = strconv.FormatUint(count, 10)
	}

	return json.Marshal(&struct {
		Alias
		Timestamp         string   `json:"timestamp"`
		StartTimeUnixNano string   `json:"startTimeUnixNano"`
		Count             string   `json:"count"`
		BucketCounts      []string `json:"bucketCounts"`
	}{
		Alias:             Alias(dataPoint),
		Timestamp:         strconv.FormatInt(dataPoint.Timestamp, 10),
		StartTimeUnixNano: strconv.FormatInt(dataPoint.StartTime, 10),
		Count:             strconv.FormatUint(dataPoint.Count, 10),
		BucketCounts:      bucketCountsStr,
	})
}

func (dataPoint *HistogramDataPoint) Scan(src any) error {
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
		return fmt.Errorf("failed to decode HistogramDataPoint: %w", err)
	}

	return nil
}
