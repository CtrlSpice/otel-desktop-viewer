package metrics

import (
	"encoding/json"
	"strconv"

	"go.opentelemetry.io/collector/pdata/pmetric"
)

// SumDataPoint represents a sum metric data point
type SumDataPoint struct {
	Timestamp              int64          `json:"-"`
	StartTime              int64          `json:"-"`
	Attributes             map[string]any `json:"attributes"`
	Flags                  uint32         `json:"flags"`
	Value                  float64        `json:"value"`
	IsMonotonic            bool           `json:"isMonotonic"`
	Exemplars              []Exemplar     `json:"exemplars,omitempty"`
	AggregationTemporality string         `json:"aggregationTemporality"`
}

// extractSumDataPoints extracts sum data points from OpenTelemetry number data point slices
func extractSumDataPoints(source pmetric.Sum) []MetricDataPoint {
	points := make([]MetricDataPoint, source.DataPoints().Len())
	for i := range source.DataPoints().Len() {
		sourcePoint := source.DataPoints().At(i)
		point := SumDataPoint{
			Timestamp:              sourcePoint.Timestamp().AsTime().UnixNano(),
			StartTime:              sourcePoint.StartTimestamp().AsTime().UnixNano(),
			Attributes:             sourcePoint.Attributes().AsRaw(),
			Flags:                  uint32(sourcePoint.Flags()),
			IsMonotonic:            source.IsMonotonic(),
			Exemplars:              extractExemplars(sourcePoint.Exemplars()),
			AggregationTemporality: source.AggregationTemporality().String(),
		}

		switch sourcePoint.ValueType() {
		case pmetric.NumberDataPointValueTypeDouble:
			point.Value = sourcePoint.DoubleValue()
		case pmetric.NumberDataPointValueTypeInt:
			point.Value = float64(sourcePoint.IntValue())
		}
		points[i] = point
	}
	return points
}

// MarshalJSON implementations for timestamp serialization
func (dataPoint SumDataPoint) MarshalJSON() ([]byte, error) {
	type Alias SumDataPoint // Avoid recursive MarshalJSON calls
	return json.Marshal(&struct {
		Alias
		Timestamp         string `json:"timestamp"`
		StartTimeUnixNano string `json:"startTimeUnixNano"`
	}{
		Alias:             Alias(dataPoint),
		Timestamp:         strconv.FormatInt(dataPoint.Timestamp, 10),
		StartTimeUnixNano: strconv.FormatInt(dataPoint.StartTime, 10),
	})
}
