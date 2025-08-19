package metrics

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/attributes"
	"github.com/go-viper/mapstructure/v2"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

// SumDataPoint represents a sum metric data point
type SumDataPoint struct {
	Timestamp              int64                 `json:"-"`
	StartTime              int64                 `json:"-"`
	Attributes             attributes.Attributes `json:"attributes"`
	Flags                  uint32                `json:"flags"`
	ValueType              string                `json:"valueType"`
	Value                  float64               `json:"value"`
	IsMonotonic            bool                  `json:"isMonotonic"`
	Exemplars              []Exemplar            `json:"exemplars,omitempty"`
	AggregationTemporality string                `json:"aggregationTemporality"`
}

// extractSumDataPoints extracts sum data points from OpenTelemetry number data point slices
func extractSumDataPoints(source pmetric.Sum) []MetricDataPoint {
	points := make([]MetricDataPoint, source.DataPoints().Len())
	for i := range source.DataPoints().Len() {
		sourcePoint := source.DataPoints().At(i)
		point := SumDataPoint{
			Timestamp:              sourcePoint.Timestamp().AsTime().UnixNano(),
			StartTime:              sourcePoint.StartTimestamp().AsTime().UnixNano(),
			Attributes:             attributes.Attributes(sourcePoint.Attributes().AsRaw()),
			Flags:                  uint32(sourcePoint.Flags()),
			ValueType:              sourcePoint.ValueType().String(),
			Value:                  sourcePoint.DoubleValue(),
			IsMonotonic:            source.IsMonotonic(),
			Exemplars:              extractExemplars(sourcePoint.Exemplars()),
			AggregationTemporality: source.AggregationTemporality().String(),
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

func (dataPoint *SumDataPoint) Scan(src any) error {
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
		return fmt.Errorf("failed to decode SumDataPoint: %w", err)
	}

	return nil
}
