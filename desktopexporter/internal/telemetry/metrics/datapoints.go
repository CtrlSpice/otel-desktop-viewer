package metrics

import (
	"database/sql/driver"
	"fmt"

	"github.com/marcboeker/go-duckdb/v2"
)

// MetricType represents the different types of metrics data points
type MetricType string

const (
	MetricTypeEmpty                MetricType = "Empty"
	MetricTypeGauge                MetricType = "Gauge"
	MetricTypeSum                  MetricType = "Sum"
	MetricTypeHistogram            MetricType = "Histogram"
	MetricTypeExponentialHistogram MetricType = "ExponentialHistogram"
)

// MetricDataPoint represents the different types of metric data points
// This is how we get around the lack of algebraic data types in Go
type MetricDataPoint interface {
	isMetricDataPoint()
}

type DataPoints struct {
	Type   MetricType        `json:"type"`
	Points []MetricDataPoint `json:"points"`
}

func (dataPoints DataPoints) Value() (driver.Value, error) {
	return duckdb.Union{Tag: string(dataPoints.Type), Value: dataPoints.Points}, nil
}

func (dataPoints *DataPoints) Scan(src any) error {

	if src == nil {
		*dataPoints = DataPoints{}
		return nil
	}

	if union, ok := src.(duckdb.Union); ok {
		resultType := MetricType(union.Tag)

		switch resultType {
		case MetricTypeGauge:
			srcSlice := union.Value.([]any)
			points := make([]MetricDataPoint, len(srcSlice))
			for i, src := range srcSlice {
				point := GaugeDataPoint{}
				if err := point.Scan(src); err != nil {
					return fmt.Errorf("failed to scan GaugeDataPoint: %w", err)
				}
				points[i] = point
			}
			*dataPoints = DataPoints{Type: resultType, Points: points}
		case MetricTypeSum:
			srcSlice := union.Value.([]any)
			points := make([]MetricDataPoint, len(srcSlice))
			for i, src := range srcSlice {
				point := SumDataPoint{}
				if err := point.Scan(src); err != nil {
					return fmt.Errorf("failed to scan SumDataPoint: %w", err)
				}
				points[i] = point
			}
			*dataPoints = DataPoints{Type: resultType, Points: points}
		case MetricTypeExponentialHistogram:
			srcSlice := union.Value.([]any)
			points := make([]MetricDataPoint, len(srcSlice))
			for i, src := range srcSlice {
				point := ExponentialHistogramDataPoint{}
				if err := point.Scan(src); err != nil {
					return fmt.Errorf("failed to scan ExponentialHistogramDataPoint: %w", err)
				}
				points[i] = point
			}
			*dataPoints = DataPoints{Type: resultType, Points: points}
		case MetricTypeHistogram:
			srcSlice := union.Value.([]any)
			points := make([]MetricDataPoint, len(srcSlice))
			for i, src := range srcSlice {
				point := HistogramDataPoint{}
				if err := point.Scan(src); err != nil {
					return fmt.Errorf("failed to scan HistogramDataPoint: %w", err)
				}
				points[i] = point
			}
			*dataPoints = DataPoints{Type: resultType, Points: points}
		}
		return nil
	}

	return fmt.Errorf("DataPoints: cannot scan from %T", src)
}

// Interface implementation methods
func (GaugeDataPoint) isMetricDataPoint()                {}
func (SumDataPoint) isMetricDataPoint()                  {}
func (HistogramDataPoint) isMetricDataPoint()            {}
func (ExponentialHistogramDataPoint) isMetricDataPoint() {}
