package store

import (
	"context"
	"fmt"

	"github.com/marcboeker/go-duckdb/v2"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/metrics"
)

type dbExemplar struct {
	Timestamp          int64      `db:"timestamp"`
	Value              float64    `db:"value"`
	TraceID            string     `db:"traceID,omitempty"`
	SpanID             string     `db:"spanID,omitempty"`
	FilteredAttributes duckdb.Map `db:"filteredAttributes"`
}

type dbGaugeDataPoint struct {
	Timestamp  int64        `db:"timestamp"`
	StartTime  int64        `db:"startTime"`
	Attributes duckdb.Map   `db:"attributes"`
	Flags      uint32       `db:"flags"`
	ValueType  string       `db:"valueType"`
	Value      float64      `db:"value"`
	Exemplars  []dbExemplar `db:"exemplars"`
}

type dbSumDataPoint struct {
	Timestamp              int64        `db:"timestamp"`
	StartTime              int64        `db:"startTime"`
	Attributes             duckdb.Map   `db:"attributes"`
	Flags                  uint32       `db:"flags"`
	ValueType              string       `db:"valueType"`
	Value                  float64      `db:"value"`
	IsMonotonic            bool         `db:"isMonotonic"`
	Exemplars              []dbExemplar `db:"exemplars"`
	AggregationTemporality string       `db:"aggregationTemporality"`
}

type dbHistogramDataPoint struct {
	Timestamp              int64        `db:"timestamp"`
	StartTime              int64        `db:"startTime"`
	Attributes             duckdb.Map   `db:"attributes"`
	Flags                  uint32       `db:"flags"`
	Count                  uint64       `db:"count"`
	Sum                    float64      `db:"sum"`
	Min                    float64      `db:"min"`
	Max                    float64      `db:"max"`
	BucketCounts           []uint64     `db:"bucketCounts"`
	ExplicitBounds         []float64    `db:"explicitBounds"`
	Exemplars              []dbExemplar `db:"exemplars"`
	AggregationTemporality string       `db:"aggregationTemporality"`
}

type dbExponentialHistogramDataPoint struct {
	Timestamp              int64           `db:"timestamp"`
	StartTime              int64           `db:"startTime"`
	Attributes             duckdb.Map      `db:"attributes"`
	Flags                  uint32          `db:"flags"`
	Count                  uint64          `db:"count"`
	Sum                    float64         `db:"sum"`
	Min                    float64         `db:"min"`
	Max                    float64         `db:"max"`
	Scale                  int32           `db:"scale"`
	ZeroCount              uint64          `db:"zeroCount"`
	Positive               metrics.Buckets `db:"positive"`
	Negative               metrics.Buckets `db:"negative"`
	Exemplars              []dbExemplar    `db:"exemplars"`
	AggregationTemporality string          `db:"aggregationTemporality"`
}

// AddMetrics appends a list of metrics to the store.
func (s *Store) AddMetrics(ctx context.Context, metricsData []metrics.MetricData) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf(ErrAddMetrics, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	appender, err := duckdb.NewAppender(s.conn, "", "", "metrics")
	if err != nil {
		return fmt.Errorf(ErrCreateAppender, err)
	}
	defer appender.Close()

	for i, metricData := range metricsData {
		err := appender.AppendRow(
			metricData.ID(),
			metricData.Name,
			metricData.Description,
			metricData.Unit,
			metricData.Type,
			toDbDataPoints(metricData.Type, metricData.DataPoints),
			toDbAttributes(metricData.Resource.Attributes),
			metricData.Resource.DroppedAttributesCount,
			metricData.Scope.Name,
			metricData.Scope.Version,
			toDbAttributes(metricData.Scope.Attributes),
			metricData.Scope.DroppedAttributesCount,
			metricData.Received,
		)
		if err != nil {
			return fmt.Errorf(ErrAppendRow, err)
		}

		// Flush every 10 metrics to prevent buffer overflow
		if (i+1)%10 == 0 {
			err = appender.Flush()
			if err != nil {
				return fmt.Errorf(ErrFlushAppender, err)
			}
		}
	}

	return nil
}

func toDbExemplars(exemplars []metrics.Exemplar) []dbExemplar {
	dbExemplars := make([]dbExemplar, len(exemplars))
	for i, exemplar := range exemplars {
		dbExemplars[i] = copyAndOverride[metrics.Exemplar, dbExemplar](exemplar, map[string]any{
			"FilteredAttributes": toDbAttributes(exemplar.FilteredAttributes),
		})
	}
	return dbExemplars
}

func toDbDataPoints(typeName string, dataPoints []metrics.MetricDataPoint) []duckdb.Union {
	switch typeName {
	case "Gauge":
		return toDbGaugeDataPoints(dataPoints)
	case "Sum":
		return toDbSumDataPoints(dataPoints)
	case "Histogram":
		return toDbHistogramDataPoints(dataPoints)
	case "ExponentialHistogram":
		return toDbExponentialHistogramDataPoints(dataPoints)
	default:
		return []duckdb.Union{}
	}
}

func toDbGaugeDataPoints(source []metrics.MetricDataPoint) []duckdb.Union {
	result := make([]duckdb.Union, len(source))
	for i, dataPoint := range source {
		if gaugePoint, ok := dataPoint.(metrics.GaugeDataPoint); ok {
			dbGaugePoint := copyAndOverride[metrics.GaugeDataPoint, dbGaugeDataPoint](gaugePoint, map[string]any{
				"Attributes": toDbAttributes(gaugePoint.Attributes),
				"Exemplars":  toDbExemplars(gaugePoint.Exemplars),
			})
			result[i] = duckdb.Union{Tag: "Gauge", Value: dbGaugePoint}
		} else {
			// Log error or handle invalid type assertion
			fmt.Printf("Warning: "+ErrMetricTypeMismatch+"\n", "GaugeDataPoint", dataPoint)
		}
	}
	return result
}

func toDbSumDataPoints(source []metrics.MetricDataPoint) []duckdb.Union {
	result := make([]duckdb.Union, len(source))
	for i, dataPoint := range source {
		if sumPoint, ok := dataPoint.(metrics.SumDataPoint); ok {
			dbSumPoint := copyAndOverride[metrics.SumDataPoint, dbSumDataPoint](sumPoint, map[string]any{
				"Attributes": toDbAttributes(sumPoint.Attributes),
				"Exemplars":  toDbExemplars(sumPoint.Exemplars),
			})
			result[i] = duckdb.Union{Tag: "Sum", Value: dbSumPoint}
		} else {
			// Log error or handle invalid type assertion
			fmt.Printf("Warning: "+ErrMetricTypeMismatch+"\n", "SumDataPoint", dataPoint)
		}
	}
	return result
}

func toDbHistogramDataPoints(source []metrics.MetricDataPoint) []duckdb.Union {
	result := make([]duckdb.Union, len(source))
	for i, dataPoint := range source {
		if histogramPoint, ok := dataPoint.(metrics.HistogramDataPoint); ok {
			dbHistogramPoint := copyAndOverride[metrics.HistogramDataPoint, dbHistogramDataPoint](histogramPoint, map[string]any{
				"Attributes": toDbAttributes(histogramPoint.Attributes),
				"Exemplars":  toDbExemplars(histogramPoint.Exemplars),
			})
			result[i] = duckdb.Union{Tag: "Histogram", Value: dbHistogramPoint}
		} else {
			// Log error or handle invalid type assertion
			fmt.Printf("Warning: "+ErrMetricTypeMismatch+"\n", "HistogramDataPoint", dataPoint)
		}
	}
	return result
}

func toDbExponentialHistogramDataPoints(source []metrics.MetricDataPoint) []duckdb.Union {
	result := make([]duckdb.Union, len(source))
	for i, dataPoint := range source {
		if expHistogramPoint, ok := dataPoint.(metrics.ExponentialHistogramDataPoint); ok {
			dbExpHistogramPoint := copyAndOverride[metrics.ExponentialHistogramDataPoint, dbExponentialHistogramDataPoint](expHistogramPoint, map[string]any{
				"Attributes": toDbAttributes(expHistogramPoint.Attributes),
				"Exemplars":  toDbExemplars(expHistogramPoint.Exemplars),
			})
			result[i] = duckdb.Union{Tag: "ExponentialHistogram", Value: dbExpHistogramPoint}
		} else {
			// Log error or handle invalid type assertion
			fmt.Printf("Warning: "+ErrMetricTypeMismatch+"\n", "ExponentialHistogramDataPoint", dataPoint)
		}
	}
	return result
}
