package store

// import (
// 	"context"
// 	"database/sql"
// 	"fmt"

// 	"github.com/marcboeker/go-duckdb/v2"

// 	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/metrics"
// 	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/resource"
// 	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/scope"
// )

// type dbExemplar struct {
// 	Timestamp          int64      `db:"timestamp"`
// 	Value              float64    `db:"value"`
// 	TraceID            string     `db:"traceID,omitempty"`
// 	SpanID             string     `db:"spanID,omitempty"`
// 	FilteredAttributes duckdb.Map `db:"filteredAttributes"`
// }

// type dbGauge struct {
// 	Timestamp  int64        `db:"timestamp"`
// 	StartTime  int64        `db:"startTime"`
// 	Attributes duckdb.Map   `db:"attributes"`
// 	Flags      uint32       `db:"flags"`
// 	ValueType  string       `db:"valueType"`
// 	Value      float64      `db:"value"`
// 	Exemplars  []dbExemplar `db:"exemplars"`
// }

// type dbSum struct {
// 	Timestamp              int64        `db:"timestamp"`
// 	StartTime              int64        `db:"startTime"`
// 	Attributes             duckdb.Map   `db:"attributes"`
// 	Flags                  uint32       `db:"flags"`
// 	ValueType              string       `db:"valueType"`
// 	Value                  float64      `db:"value"`
// 	IsMonotonic            bool         `db:"isMonotonic"`
// 	Exemplars              []dbExemplar `db:"exemplars"`
// 	AggregationTemporality string       `db:"aggregationTemporality"`
// }

// type dbHistogram struct {
// 	Timestamp              int64        `db:"timestamp"`
// 	StartTime              int64        `db:"startTime"`
// 	Attributes             duckdb.Map   `db:"attributes"`
// 	Flags                  uint32       `db:"flags"`
// 	Count                  uint64       `db:"count"`
// 	Sum                    float64      `db:"sum"`
// 	Min                    float64      `db:"min"`
// 	Max                    float64      `db:"max"`
// 	BucketCounts           []uint64     `db:"bucketCounts"`
// 	ExplicitBounds         []float64    `db:"explicitBounds"`
// 	Exemplars              []dbExemplar `db:"exemplars"`
// 	AggregationTemporality string       `db:"aggregationTemporality"`
// }

// type dbExponentialHistogram struct {
// 	Timestamp              int64           `db:"timestamp"`
// 	StartTime              int64           `db:"startTime"`
// 	Attributes             duckdb.Map      `db:"attributes"`
// 	Flags                  uint32          `db:"flags"`
// 	Count                  uint64          `db:"count"`
// 	Sum                    float64         `db:"sum"`
// 	Min                    float64         `db:"min"`
// 	Max                    float64         `db:"max"`
// 	Scale                  int32           `db:"scale"`
// 	ZeroCount              uint64          `db:"zeroCount"`
// 	Positive               metrics.Buckets `db:"positive"`
// 	Negative               metrics.Buckets `db:"negative"`
// 	Exemplars              []dbExemplar    `db:"exemplars"`
// 	AggregationTemporality string          `db:"aggregationTemporality"`
// }

// // AddMetrics appends a list of metrics to the store.
// func (s *Store) AddMetrics(ctx context.Context, metricsData []metrics.MetricData) error {
// 	if err := s.checkConnection(); err != nil {
// 		return fmt.Errorf(ErrAddMetrics, err)
// 	}

// 	s.mu.Lock()
// 	defer s.mu.Unlock()

// 	appender, err := duckdb.NewAppender(s.conn, "", "", "metrics")
// 	if err != nil {
// 		return fmt.Errorf(ErrCreateAppender, err)
// 	}
// 	defer appender.Close()

// 	for i, metricData := range metricsData {
// 		err := appender.AppendRow(
// 			metricData.ID(),
// 			metricData.Name,
// 			metricData.Description,
// 			metricData.Unit,
// 			metricData.Type,
// 			toDbDataPoints(metricData.Type, metricData.DataPoints),
// 			toDbAttributes(metricData.Resource.Attributes),
// 			metricData.Resource.DroppedAttributesCount,
// 			metricData.Scope.Name,
// 			metricData.Scope.Version,
// 			toDbAttributes(metricData.Scope.Attributes),
// 			metricData.Scope.DroppedAttributesCount,
// 			metricData.Received,
// 		)
// 		if err != nil {
// 			return fmt.Errorf(ErrAppendRow, err)
// 		}

// 		// Flush every 10 metrics to prevent buffer overflow
// 		if (i+1)%10 == 0 {
// 			err = appender.Flush()
// 			if err != nil {
// 				return fmt.Errorf(ErrFlushAppender, err)
// 			}
// 		}
// 	}

// 	return nil
// }

// func (s *Store) GetMetrics(ctx context.Context) ([]metrics.MetricData, error) {
// 	if err := s.checkConnection(); err != nil {
// 		return nil, fmt.Errorf(ErrGetMetrics, err)
// 	}

// 	metrics := []metrics.MetricData{}

// 	rows, err := s.db.QueryContext(ctx, SelectMetrics)
// 	if err != nil {
// 		return nil, fmt.Errorf(ErrGetMetrics, err)
// 	}
// 	defer rows.Close()

// 	for rows.Next() {
// 		metricData, err := scanMetricRow(rows)
// 		if err != nil {
// 			return nil, err
// 		}
// 		metrics = append(metrics, metricData)
// 	}

// 	return metrics, nil
// }

// // ClearMetrics truncates the metrics table.
// func (s *Store) ClearMetrics(ctx context.Context) error {
// 	if err := s.checkConnection(); err != nil {
// 		return fmt.Errorf(ErrClearMetrics, err)
// 	}

// 	if _, err := s.db.ExecContext(ctx, TruncateMetrics); err != nil {
// 		return fmt.Errorf(ErrClearMetrics, err)
// 	}
// 	return nil
// }

// func scanMetricRow(scanner interface{ Scan(dest ...any) error }) (metrics.MetricData, error) {
// 	var (
// 		rawDataPoints         duckdb.Union
// 		rawResourceAttributes duckdb.Map
// 		rawScopeAttributes    duckdb.Map
// 	)

// 	metricData := metrics.MetricData{
// 		Resource: &resource.ResourceData{
// 			Attributes:             map[string]any{},
// 			DroppedAttributesCount: 0,
// 		},
// 		Scope: &scope.ScopeData{
// 			Name:                   "",
// 			Version:                "",
// 			Attributes:             map[string]any{},
// 			DroppedAttributesCount: 0,
// 		},
// 	}

// 	if err := scanner.Scan(
// 		&metricData.Name,
// 		&metricData.Description,
// 		&metricData.Unit,
// 		&metricData.Type,
// 		&rawDataPoints,
// 		&rawResourceAttributes,
// 		&metricData.Resource.DroppedAttributesCount,
// 		&metricData.Scope.Name,
// 		&metricData.Scope.Version,
// 		&rawScopeAttributes,
// 		&metricData.Scope.DroppedAttributesCount,
// 		&metricData.Received,
// 	); err != nil {
// 		if err == sql.ErrNoRows {
// 			return metricData, ErrMetricIDNotFound
// 		}
// 		return metricData, fmt.Errorf(ErrScanMetricRow, err)
// 	}

// 	metricData.Resource.Attributes = fromDbAttributes(rawResourceAttributes)
// 	metricData.Scope.Attributes = fromDbAttributes(rawScopeAttributes)
// 	metricData.DataPoints = fromDbDataPoints(rawDataPoints)

// 	return metricData, nil
// }

// func toDbExemplars(exemplars []metrics.Exemplar) []dbExemplar {
// 	dbExemplars := make([]dbExemplar, len(exemplars))
// 	for i, exemplar := range exemplars {
// 		dbExemplars[i] = copyAndOverride[metrics.Exemplar, dbExemplar](exemplar, map[string]any{
// 			"FilteredAttributes": toDbAttributes(exemplar.FilteredAttributes),
// 		})
// 	}
// 	return dbExemplars
// }

// func toDbDataPoints(typeName string, dataPoints []metrics.MetricDataPoint) duckdb.Union {
// 	switch typeName {
// 	case "Gauge":
// 		return toDbGaugeDataPoints(dataPoints)
// 	case "Sum":
// 		return toDbSumDataPoints(dataPoints)
// 	case "Histogram":
// 		return toDbHistogramDataPoints(dataPoints)
// 	case "ExponentialHistogram":
// 		return toDbExponentialHistogramDataPoints(dataPoints)
// 	default:
// 		return duckdb.Union{}
// 	}
// }

// func toDbGaugeDataPoints(source []metrics.MetricDataPoint) duckdb.Union {
// 	tag := "Gauge"
// 	value := []dbGauge{}

// 	for _, dataPoint := range source {
// 		if gaugePoint, ok := dataPoint.(metrics.GaugeDataPoint); ok {
// 			value = append(value, copyAndOverride[metrics.GaugeDataPoint, dbGauge](gaugePoint, map[string]any{
// 				"Attributes": toDbAttributes(gaugePoint.Attributes),
// 				"Exemplars":  toDbExemplars(gaugePoint.Exemplars),
// 			}))
// 		} else {
// 			// Log error
// 			fmt.Printf("Warning: "+ErrMetricTypeMismatch+"\n", "GaugeDataPoint", dataPoint)
// 		}
// 	}
// 	return duckdb.Union{Tag: tag, Value: value}
// }

// func toDbSumDataPoints(source []metrics.MetricDataPoint) duckdb.Union {
// 	tag := "Sum"
// 	value := []dbSum{}

// 	for _, dataPoint := range source {
// 		if sumPoint, ok := dataPoint.(metrics.SumDataPoint); ok {
// 			value = append(value, copyAndOverride[metrics.SumDataPoint, dbSum](sumPoint, map[string]any{
// 				"Attributes": toDbAttributes(sumPoint.Attributes),
// 				"Exemplars":  toDbExemplars(sumPoint.Exemplars),
// 			}))
// 		} else {
// 			// Log error
// 			fmt.Printf("Warning: "+ErrMetricTypeMismatch+"\n", "SumDataPoint", dataPoint)
// 		}
// 	}
// 	return duckdb.Union{Tag: tag, Value: value}
// }

// func toDbHistogramDataPoints(source []metrics.MetricDataPoint) duckdb.Union {
// 	tag := "Histogram"
// 	value := []dbHistogram{}

// 	for _, dataPoint := range source {
// 		if histogramPoint, ok := dataPoint.(metrics.HistogramDataPoint); ok {
// 			dbHistogramPoint := copyAndOverride[metrics.HistogramDataPoint, dbHistogram](histogramPoint, map[string]any{
// 				"Attributes": toDbAttributes(histogramPoint.Attributes),
// 				"Exemplars":  toDbExemplars(histogramPoint.Exemplars),
// 			})
// 			value = append(value, dbHistogramPoint)
// 		} else {
// 			// Log error or handle invalid type assertion
// 			fmt.Printf("Warning: "+ErrMetricTypeMismatch+"\n", "HistogramDataPoint", dataPoint)
// 		}
// 	}
// 	return duckdb.Union{Tag: tag, Value: value}
// }

// func toDbExponentialHistogramDataPoints(source []metrics.MetricDataPoint) duckdb.Union {
// 	tag := "ExponentialHistogram"
// 	value := []dbExponentialHistogram{}

// 	for _, dataPoint := range source {
// 		if expHistogramPoint, ok := dataPoint.(metrics.ExponentialHistogramDataPoint); ok {
// 			dbExpHistogramPoint := copyAndOverride[metrics.ExponentialHistogramDataPoint, dbExponentialHistogram](expHistogramPoint, map[string]any{
// 				"Attributes": toDbAttributes(expHistogramPoint.Attributes),
// 				"Exemplars":  toDbExemplars(expHistogramPoint.Exemplars),
// 			})
// 			value = append(value, dbExpHistogramPoint)
// 		} else {
// 			// Log error or handle invalid type assertion
// 			fmt.Printf("Warning: "+ErrMetricTypeMismatch+"\n", "ExponentialHistogramDataPoint", dataPoint)
// 		}
// 	}
// 	return duckdb.Union{Tag: tag, Value: value}
// }

// func fromDbDataPoints(dataPoints duckdb.Union) []metrics.MetricDataPoint {
// 	switch dataPoints.Tag {
// 	case "Gauge":
// 		return fromDbGaugeDataPoints(dataPoints.Value)
// 	case "Sum":
// 		return fromDbSumDataPoints(dataPoints.Value)
// 	case "Histogram":
// 		return fromDbHistogramDataPoints(dataPoints.Value)
// 	case "ExponentialHistogram":
// 		return fromDbExponentialHistogramDataPoints(dataPoints.Value)
// 	default:
// 		return []metrics.MetricDataPoint{}
// 	}
// }

// func fromDbGaugeDataPoints(dbPoints any) []metrics.MetricDataPoint {
// 	fmt.Printf("DEBUG: fromDbGaugeDataPoints - value type: %T, value: %+v\n", dataPoints, dataPoints)

// 	result := []metrics.MetricDataPoint{}
// 	for _, dbPoint := range dbPoints {
// 		fmt.Printf("DEBUG: fromDbGaugeDataPoints - dbPoint type: %T, dbPoint: %+v\n", dbPoint, dbPoint)
// 		gaugePoint := copyAndOverride[dbGauge, metrics.GaugeDataPoint](dbPoint, map[string]any{
// 			"Attributes": fromDbAttributes(dbPoint.Attributes),
// 			"Exemplars":  fromDbExemplars(dbPoint.Exemplars),
// 		})
// 		result = append(result, metrics.MetricDataPoint(gaugePoint))
// 	}
// 	return result
// 	return []metrics.MetricDataPoint{}
// }

// func fromDbSumDataPoints(value any) []metrics.MetricDataPoint {
// 	if dbSumPoints, ok := value.([]dbSum); ok {
// 		result := make([]metrics.MetricDataPoint, len(dbSumPoints))
// 		for i, dbPoint := range dbSumPoints {
// 			attributes := map[string]any{}
// 			for k, v := range dbPoint.Attributes {
// 				if name, ok := k.(string); ok {
// 					if union, ok := v.(duckdb.Union); ok {
// 						attributes[name] = union.Value
// 					}
// 				}
// 			}

// 			sumPoint := copyAndOverride[dbSum, metrics.SumDataPoint](dbPoint, map[string]any{
// 				"Attributes": attributes,
// 				"Exemplars":  fromDbExemplars(dbPoint.Exemplars),
// 			})
// 			result[i] = sumPoint
// 		}
// 		return result
// 	}
// 	return []metrics.MetricDataPoint{}
// }

// func fromDbHistogramDataPoints(value any) []metrics.MetricDataPoint {
// 	if dbHistogramPoints, ok := value.([]dbHistogram); ok {
// 		result := make([]metrics.MetricDataPoint, len(dbHistogramPoints))
// 		for i, dbPoint := range dbHistogramPoints {
// 			attributes := map[string]any{}
// 			for k, v := range dbPoint.Attributes {
// 				if name, ok := k.(string); ok {
// 					if union, ok := v.(duckdb.Union); ok {
// 						attributes[name] = union.Value
// 					}
// 				}
// 			}

// 			histogramPoint := copyAndOverride[dbHistogram, metrics.HistogramDataPoint](dbPoint, map[string]any{
// 				"Attributes": attributes,
// 				"Exemplars":  fromDbExemplars(dbPoint.Exemplars),
// 			})
// 			result[i] = histogramPoint
// 		}
// 		return result
// 	}
// 	return []metrics.MetricDataPoint{}
// }

// func fromDbExponentialHistogramDataPoints(value any) []metrics.MetricDataPoint {
// 	if dbExpHistogramPoints, ok := value.([]dbExponentialHistogram); ok {
// 		result := make([]metrics.MetricDataPoint, len(dbExpHistogramPoints))
// 		for i, dbPoint := range dbExpHistogramPoints {
// 			attributes := map[string]any{}
// 			for k, v := range dbPoint.Attributes {
// 				if name, ok := k.(string); ok {
// 					if union, ok := v.(duckdb.Union); ok {
// 						attributes[name] = union.Value
// 					}
// 				}
// 			}

// 			expHistogramPoint := copyAndOverride[dbExponentialHistogram, metrics.ExponentialHistogramDataPoint](dbPoint, map[string]any{
// 				"Attributes": attributes,
// 				"Exemplars":  fromDbExemplars(dbPoint.Exemplars),
// 			})
// 			result[i] = expHistogramPoint
// 		}
// 		return result
// 	}
// 	return []metrics.MetricDataPoint{}
// }

// func fromDbExemplars(dbExemplars []dbExemplar) []metrics.Exemplar {
// 	result := make([]metrics.Exemplar, len(dbExemplars))
// 	for i, exemplar := range dbExemplars {
// 		result[i] = copyAndOverride[dbExemplar, metrics.Exemplar](exemplar, map[string]any{
// 			"FilteredAttributes": fromDbAttributes(exemplar.FilteredAttributes),
// 		})
// 	}
// 	return result
// }
