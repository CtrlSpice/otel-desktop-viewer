package metrics

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/attributes"
	"github.com/mitchellh/mapstructure"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

// GaugeDataPoint represents a gauge metric data point
type GaugeDataPoint struct {
	Timestamp  int64                 `json:"-"`
	StartTime  int64                 `json:"-"`
	Attributes attributes.Attributes `json:"attributes"`
	Flags      uint32                `json:"flags"`
	ValueType  string                `json:"valueType"`
	Value      float64               `json:"value"`
	Exemplars  []Exemplar            `json:"exemplars,omitempty"`
}

// extractGaugeDataPoints extracts gauge data points from OpenTelemetry number data point slices
func extractGaugeDataPoints(source pmetric.Gauge) []MetricDataPoint {
	points := make([]MetricDataPoint, source.DataPoints().Len())
	for i := range source.DataPoints().Len() {
		sourcePoint := source.DataPoints().At(i)
		point := GaugeDataPoint{
			Timestamp:  sourcePoint.Timestamp().AsTime().UnixNano(),
			StartTime:  sourcePoint.StartTimestamp().AsTime().UnixNano(),
			Attributes: attributes.Attributes(sourcePoint.Attributes().AsRaw()),
			Flags:      uint32(sourcePoint.Flags()),
			ValueType:  sourcePoint.ValueType().String(),
			Value:      sourcePoint.DoubleValue(),
			Exemplars:  extractExemplars(sourcePoint.Exemplars()),
		}
		points[i] = point
	}
	return points
}

// MarshalJSON implementations for timestamp serialization
func (dataPoint GaugeDataPoint) MarshalJSON() ([]byte, error) {
	type Alias GaugeDataPoint // Avoid recursive MarshalJSON calls
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

func (dataPoint *GaugeDataPoint) Scan(src any) error {
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
		return fmt.Errorf("failed to decode GaugeDataPoint: %w", err)
	}

	return nil
}
