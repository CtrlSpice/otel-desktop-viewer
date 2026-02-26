package store

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/marcboeker/go-duckdb/v2"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

const flushIntervalMetrics = 100

// AddMetrics appends a list of metrics to the store.
func (s *Store) IngestMetrics(ctx context.Context, metrics pmetric.Metrics) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf("failed to add metrics: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tables := []string{"attributes", "exemplars", "datapoints", "metrics"}
	appenders, err := NewAppenders(s.conn, tables)
	if err != nil {
		return err
	}
	defer CloseAppenders(appenders, tables)

	metricCount := 0
	for _, resourceMetric := range metrics.ResourceMetrics().All() {
		resource := resourceMetric.Resource()

		for _, scopeMetric := range resourceMetric.ScopeMetrics().All() {
			scope := scopeMetric.Scope()

			for _, metric := range scopeMetric.Metrics().All() {
				metricID := duckdb.UUID(uuid.New())
				received := time.Now().UnixNano()
				metricSearchText := strings.Join([]string{
					metric.Name(),
					metric.Description(),
					metric.Unit(),
					scope.Name(),
					scope.Version(),
				}, " ")

				err = appenders["metrics"].AppendRow(
					metricID,                          // ID UUID
					metric.Name(),                     // Name VARCHAR
					metric.Description(),              // Description VARCHAR
					metric.Unit(),                     // Unit VARCHAR
					resource.DroppedAttributesCount(), // ResourceDroppedAttributesCount UINTEGER
					scope.Name(),                      // ScopeName VARCHAR
					scope.Version(),                   // ScopeVersion VARCHAR
					scope.DroppedAttributesCount(),    // ScopeDroppedAttributesCount UINTEGER
					received,                          // Received BIGINT
					metricSearchText,                  // SearchText VARCHAR
				)
				if err != nil {
					return fmt.Errorf("failed to append row: %w", err)
				}

				switch metric.Type() {
				case pmetric.MetricTypeGauge:
					if err := ingestGaugeDatapoints(appenders, metricID, metric.Gauge().DataPoints()); err != nil {
						return err
					}
				case pmetric.MetricTypeSum:
					if err := ingestSumDatapoints(appenders, metricID, metric.Sum()); err != nil {
						return err
					}
				case pmetric.MetricTypeHistogram:
					if err := ingestHistogramDatapoints(appenders, metricID, metric.Histogram()); err != nil {
						return err
					}
				case pmetric.MetricTypeExponentialHistogram:
					if err := ingestExponentialHistogramDatapoints(appenders, metricID, metric.ExponentialHistogram()); err != nil {
						return err
					}
				}
				ownerIDs := AttributeOwnerIDs{MetricID: &metricID}
				if err := IngestAttributes(appenders["attributes"], []AttributeBatchItem{
					{Attrs: resource.Attributes(), IDs: ownerIDs, Scope: "resource"},
					{Attrs: scope.Attributes(), IDs: ownerIDs, Scope: "scope"},
				}); err != nil {
					return err
				}
			}
		}

		// Flush every 10 metrics to prevent buffer overflow
		if metricCount%flushIntervalMetrics == 0 {
			if err := FlushAppenders(appenders, tables); err != nil {
				return fmt.Errorf("failed to flush appender: %w", err)
			}
		}
	}

	return nil

}

// ingestExemplars appends exemplar rows and their filtered attributes for a datapoint.
func ingestExemplars(appenders map[string]*duckdb.Appender, metricID, datapointID duckdb.UUID, exemplars pmetric.ExemplarSlice) error {
	for i := 0; i < exemplars.Len(); i++ {
		ex := exemplars.At(i)
		exemplarID := duckdb.UUID(uuid.New())
		traceIDStr := ""
		if tid := ex.TraceID(); !tid.IsEmpty() {
			traceIDStr = hex.EncodeToString(tid[:])
		}
		spanIDStr := ""
		if sid := ex.SpanID(); !sid.IsEmpty() {
			spanIDStr = hex.EncodeToString(sid[:])
		}
		if err := appenders["exemplars"].AppendRow(
			exemplarID,            // ID UUID
			datapointID,           // DataPointID UUID
			int64(ex.Timestamp()), // Timestamp BIGINT
			ex.DoubleValue(),      // Value DOUBLE
			traceIDStr,            // TraceID VARCHAR
			spanIDStr,             // SpanID VARCHAR
		); err != nil {
			return fmt.Errorf("failed to append exemplar row: %w", err)
		}
		exOwnerIDs := AttributeOwnerIDs{MetricID: &metricID, DataPointID: &datapointID, ExemplarID: &exemplarID}
		if err := IngestAttributes(appenders["attributes"], []AttributeBatchItem{
			{Attrs: ex.FilteredAttributes(), IDs: exOwnerIDs, Scope: "exemplar"},
		}); err != nil {
			return err
		}
	}
	return nil
}

// ingestGaugeDatapoints appends Gauge datapoint rows and their attributes/exemplars.
func ingestGaugeDatapoints(appenders map[string]*duckdb.Appender, metricID duckdb.UUID, dps pmetric.NumberDataPointSlice) error {
	for i := 0; i < dps.Len(); i++ {
		gp := dps.At(i)
		doubleVal, intVal, valType := numberDataPointValue(gp)
		datapointID := duckdb.UUID(uuid.New())
		if err := appenders["datapoints"].AppendRow(
			datapointID,                // ID UUID
			metricID,                   // MetricID UUID
			"Gauge",                    // MetricType VARCHAR
			int64(gp.Timestamp()),      // Timestamp BIGINT
			int64(gp.StartTimestamp()), // StartTime BIGINT
			uint32(gp.Flags()),         // Flags UINTEGER
			doubleVal,                  // DoubleValue DOUBLE
			intVal,                     // IntValue BIGINT
			valType,                    // ValueType VARCHAR
			nil,                        // IsMonotonic BOOLEAN
			nil,                        // AggregationTemporality VARCHAR
			nil, nil,                   // Count, Sum
			nil, nil, // Min, Max
			nil, nil, // BucketCounts, ExplicitBounds
			nil, nil, // Scale, ZeroCount
			nil, nil, // PositiveBucketOffset, PositiveBucketCounts
			nil, nil, // NegativeBucketOffset, NegativeBucketCounts
		); err != nil {
			return fmt.Errorf("failed to append datapoint row: %w", err)
		}
		dpOwnerIDs := AttributeOwnerIDs{MetricID: &metricID, DataPointID: &datapointID}
		if err := IngestAttributes(appenders["attributes"], []AttributeBatchItem{
			{Attrs: gp.Attributes(), IDs: dpOwnerIDs, Scope: "datapoint"},
		}); err != nil {
			return err
		}
		if err := ingestExemplars(appenders, metricID, datapointID, gp.Exemplars()); err != nil {
			return err
		}
	}
	return nil
}

// ingestSumDatapoints appends Sum datapoint rows and their attributes/exemplars.
func ingestSumDatapoints(appenders map[string]*duckdb.Appender, metricID duckdb.UUID, sum pmetric.Sum) error {
	dps := sum.DataPoints()
	for i := 0; i < dps.Len(); i++ {
		dp := dps.At(i)
		doubleVal, intVal, valType := numberDataPointValue(dp)
		datapointID := duckdb.UUID(uuid.New())
		if err := appenders["datapoints"].AppendRow(
			datapointID,                           // ID UUID
			metricID,                              // MetricID UUID
			"Sum",                                 // MetricType VARCHAR
			int64(dp.Timestamp()),                 // Timestamp BIGINT
			int64(dp.StartTimestamp()),            // StartTime BIGINT
			uint32(dp.Flags()),                    // Flags UINTEGER
			doubleVal,                             // DoubleValue DOUBLE
			intVal,                                // IntValue BIGINT
			valType,                               // ValueType VARCHAR
			sum.IsMonotonic(),                     // IsMonotonic BOOLEAN
			sum.AggregationTemporality().String(), // AggregationTemporality VARCHAR
			nil, nil, nil, nil,                    // Count, Sum, Min, Max
			nil, nil, // BucketCounts, ExplicitBounds
			nil, nil, // Scale, ZeroCount
			nil, nil, // PositiveBucketOffset, PositiveBucketCounts
			nil, nil, // NegativeBucketOffset, NegativeBucketCounts
		); err != nil {
			return fmt.Errorf("failed to append datapoint row: %w", err)
		}
		dpOwnerIDs := AttributeOwnerIDs{MetricID: &metricID, DataPointID: &datapointID}
		if err := IngestAttributes(appenders["attributes"], []AttributeBatchItem{
			{Attrs: dp.Attributes(), IDs: dpOwnerIDs, Scope: "datapoint"},
		}); err != nil {
			return err
		}
		if err := ingestExemplars(appenders, metricID, datapointID, dp.Exemplars()); err != nil {
			return err
		}
	}
	return nil
}

// ingestHistogramDatapoints appends Histogram datapoint rows and their attributes/exemplars.
func ingestHistogramDatapoints(appenders map[string]*duckdb.Appender, metricID duckdb.UUID, hist pmetric.Histogram) error {
	for i := 0; i < hist.DataPoints().Len(); i++ {
		dp := hist.DataPoints().At(i)
		datapointID := duckdb.UUID(uuid.New())
		if err := appenders["datapoints"].AppendRow(
			datapointID,                // ID UUID
			metricID,                   // MetricID UUID
			"Histogram",                // MetricType VARCHAR
			int64(dp.Timestamp()),      // Timestamp BIGINT
			int64(dp.StartTimestamp()), // StartTime BIGINT
			uint32(dp.Flags()),         // Flags UINTEGER
			nil, nil,                   // DoubleValue, IntValue
			nil,                                    // ValueType VARCHAR
			nil,                                    // IsMonotonic BOOLEAN
			hist.AggregationTemporality().String(), // AggregationTemporality VARCHAR
			dp.Count(),                             // Count UBIGINT
			dp.Sum(),                               // Sum DOUBLE
			dp.Min(),                               // Min DOUBLE
			dp.Max(),                               // Max DOUBLE
			dp.BucketCounts().AsRaw(),              // BucketCounts UBIGINT[]
			dp.ExplicitBounds().AsRaw(),            // ExplicitBounds DOUBLE[]
			nil, nil,                               // Scale, ZeroCount
			nil, nil, // PositiveBucketOffset, PositiveBucketCounts
			nil, nil, // NegativeBucketOffset, NegativeBucketCounts
		); err != nil {
			return fmt.Errorf("failed to append datapoint row: %w", err)
		}
		dpOwnerIDs := AttributeOwnerIDs{MetricID: &metricID, DataPointID: &datapointID}
		if err := IngestAttributes(appenders["attributes"], []AttributeBatchItem{
			{Attrs: dp.Attributes(), IDs: dpOwnerIDs, Scope: "datapoint"},
		}); err != nil {
			return err
		}
		if err := ingestExemplars(appenders, metricID, datapointID, dp.Exemplars()); err != nil {
			return err
		}
	}
	return nil
}

// ingestExponentialHistogramDatapoints appends ExponentialHistogram datapoint rows and their attributes/exemplars.
func ingestExponentialHistogramDatapoints(appenders map[string]*duckdb.Appender, metricID duckdb.UUID, exp pmetric.ExponentialHistogram) error {
	for i := 0; i < exp.DataPoints().Len(); i++ {
		dp := exp.DataPoints().At(i)
		pos, neg := dp.Positive(), dp.Negative()
		datapointID := duckdb.UUID(uuid.New())
		if err := appenders["datapoints"].AppendRow(
			datapointID,                // ID UUID
			metricID,                   // MetricID UUID
			"ExponentialHistogram",     // MetricType VARCHAR
			int64(dp.Timestamp()),      // Timestamp BIGINT
			int64(dp.StartTimestamp()), // StartTime BIGINT
			uint32(dp.Flags()),         // Flags UINTEGER
			nil, nil,                   // DoubleValue, IntValue
			nil,                                   // ValueType VARCHAR
			nil,                                   // IsMonotonic BOOLEAN
			exp.AggregationTemporality().String(), // AggregationTemporality VARCHAR
			dp.Count(),                            // Count UBIGINT
			dp.Sum(),                              // Sum DOUBLE
			dp.Min(),                              // Min DOUBLE
			dp.Max(),                              // Max DOUBLE
			nil, nil,                              // BucketCounts, ExplicitBounds
			dp.Scale(),                 // Scale INTEGER
			dp.ZeroCount(),             // ZeroCount UBIGINT
			pos.Offset(),               // PositiveBucketOffset INTEGER
			pos.BucketCounts().AsRaw(), // PositiveBucketCounts UBIGINT[]
			neg.Offset(),               // NegativeBucketOffset INTEGER
			neg.BucketCounts().AsRaw(), // NegativeBucketCounts UBIGINT[]
		); err != nil {
			return fmt.Errorf("failed to append datapoint row: %w", err)
		}
		dpOwnerIDs := AttributeOwnerIDs{MetricID: &metricID, DataPointID: &datapointID}
		if err := IngestAttributes(appenders["attributes"], []AttributeBatchItem{
			{Attrs: dp.Attributes(), IDs: dpOwnerIDs, Scope: "datapoint"},
		}); err != nil {
			return err
		}
		if err := ingestExemplars(appenders, metricID, datapointID, dp.Exemplars()); err != nil {
			return err
		}
	}
	return nil
}

func numberDataPointValue(dp pmetric.NumberDataPoint) (doubleVal any, intVal any, typeStr string) {
	typeStr = dp.ValueType().String()
	switch dp.ValueType() {
	case pmetric.NumberDataPointValueTypeDouble:
		return dp.DoubleValue(), nil, typeStr
	case pmetric.NumberDataPointValueTypeInt:
		return nil, dp.IntValue(), typeStr
	default:
		return nil, nil, typeStr
	}
}

// SearchMetrics returns metrics that have at least one datapoint in [startTime, endTime],
// matching the optional query tree, as a JSON array of metric objects (DB-generated JSON).
func (s *Store) SearchMetrics(ctx context.Context, startTime, endTime int64, query any) (json.RawMessage, error) {
	if err := s.checkConnection(); err != nil {
		return nil, fmt.Errorf("failed to search metrics: %w", err)
	}

	var queryTree *QueryNode
	if query != nil {
		var err error
		queryTree, err = ParseQueryTree(query)
		if err != nil {
			return nil, fmt.Errorf("failed to parse query tree: %w", err)
		}
	}

	cteSQL, whereClause, args, err := BuildMetricSQL(queryTree, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to build metric SQL: %w", err)
	}

	finalQuery := fmt.Sprintf(`%s
		filtered_metrics AS (
			SELECT m.* FROM metrics m, search_params
			WHERE %s
		),
		filtered_dps AS (
			SELECT d.* FROM datapoints d
			INNER JOIN filtered_metrics fm ON d.MetricID = fm.ID, search_params
			WHERE d.Timestamp >= time_start AND d.Timestamp <= time_end
		),
		dp_attrs_agg AS (
			SELECT a.DataPointID, json_group_array(json_object('key', a.Key, 'value', a.Value, 'type', a.Type::VARCHAR)) AS attrs
			FROM attributes a
			WHERE a.DataPointID IN (SELECT ID FROM filtered_dps) AND a.Scope = 'datapoint'
			GROUP BY a.DataPointID
		),
		exemplars_agg AS (
			SELECT e.DataPointID, json_group_array(json_object('timestamp', e.Timestamp, 'value', e.Value, 'traceID', e.TraceID, 'spanID', e.SpanID)) AS exemplars
			FROM exemplars e
			WHERE e.DataPointID IN (SELECT ID FROM filtered_dps)
			GROUP BY e.DataPointID
		),
		metric_res_attrs AS (
			SELECT a.MetricID, json_group_array(json_object('key', a.Key, 'value', a.Value, 'type', a.Type::VARCHAR)) AS attrs
			FROM attributes a
			WHERE a.MetricID IN (SELECT ID FROM filtered_metrics) AND a.Scope = 'resource' AND a.DataPointID IS NULL AND a.ExemplarID IS NULL
			GROUP BY a.MetricID
		),
		metric_scope_attrs AS (
			SELECT a.MetricID, json_group_array(json_object('key', a.Key, 'value', a.Value, 'type', a.Type::VARCHAR)) AS attrs
			FROM attributes a
			WHERE a.MetricID IN (SELECT ID FROM filtered_metrics) AND a.Scope = 'scope' AND a.DataPointID IS NULL AND a.ExemplarID IS NULL
			GROUP BY a.MetricID
		),
		datapoints_agg AS (
			SELECT d.MetricID, json_group_array(json_object(
				'id', d.ID, 'metricType', d.MetricType, 'timestamp', d.Timestamp, 'startTime', d.StartTime, 'flags', d.Flags,
				'doubleValue', d.DoubleValue, 'intValue', d.IntValue, 'valueType', d.ValueType,
				'isMonotonic', d.IsMonotonic, 'aggregationTemporality', d.AggregationTemporality,
				'count', d.Count, 'sum', d.Sum, 'min', d.Min, 'max', d.Max,
				'bucketCounts', d.BucketCounts, 'explicitBounds', d.ExplicitBounds,
				'scale', d.Scale, 'zeroCount', d.ZeroCount,
				'positiveBucketOffset', d.PositiveBucketOffset, 'positiveBucketCounts', d.PositiveBucketCounts,
				'negativeBucketOffset', d.NegativeBucketOffset, 'negativeBucketCounts', d.NegativeBucketCounts,
				'attributes', COALESCE((SELECT attrs FROM dp_attrs_agg WHERE dp_attrs_agg.DataPointID = d.ID), json('[]')),
				'exemplars', COALESCE((SELECT exemplars FROM exemplars_agg WHERE exemplars_agg.DataPointID = d.ID), json('[]'))
			)) AS datapoints
			FROM filtered_dps d
			GROUP BY d.MetricID
		)
		SELECT COALESCE(json_group_array(json_object(
			'id', m.ID, 'name', m.Name, 'description', m.Description, 'unit', m.Unit,
			'resourceDroppedAttributesCount', m.ResourceDroppedAttributesCount,
			'resource', json_object('attributes', COALESCE(res.attrs, json('[]')), 'droppedAttributesCount', m.ResourceDroppedAttributesCount),
			'scopeName', m.ScopeName, 'scopeVersion', m.ScopeVersion, 'scopeDroppedAttributesCount', m.ScopeDroppedAttributesCount,
			'scope', json_object('name', m.ScopeName, 'version', m.ScopeVersion, 'attributes', COALESCE(scope_attrs.attrs, json('[]')), 'droppedAttributesCount', m.ScopeDroppedAttributesCount),
			'received', m.Received,
			'datapoints', COALESCE(dp_agg.datapoints, json('[]'))
		)), '[]') AS metrics
		FROM filtered_metrics m
		LEFT JOIN metric_res_attrs res ON res.MetricID = m.ID
		LEFT JOIN metric_scope_attrs scope_attrs ON scope_attrs.MetricID = m.ID
		LEFT JOIN datapoints_agg dp_agg ON dp_agg.MetricID = m.ID
		ORDER BY m.Received DESC`,
		cteSQL,
		whereClause,
	)

	var raw []byte
	if err := s.db.QueryRowContext(ctx, finalQuery, args...).Scan(&raw); err != nil {
		return nil, fmt.Errorf("failed to search metrics: %w", err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// ClearMetrics truncates the metrics table and all child tables (datapoints, exemplars, and their attributes).
func (s *Store) ClearMetrics(ctx context.Context) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf("failed to clear metrics: %w", err)
	}

	childQueries := []string{
		`DELETE FROM attributes WHERE MetricID IS NOT NULL`,
		`TRUNCATE TABLE exemplars`,
		`TRUNCATE TABLE datapoints`,
		`TRUNCATE TABLE metrics`,
	}
	for _, query := range childQueries {
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("failed to clear metrics: %w", err)
		}
	}
	return nil
}

// DeleteMetricByID deletes a specific metric by its ID, including child datapoints, exemplars, and attributes.
func (s *Store) DeleteMetricByID(ctx context.Context, metricID string) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf("failed to delete metric by ID: %w", err)
	}

	childQueries := []string{
		`DELETE FROM attributes WHERE MetricID = ?`,
		`DELETE FROM exemplars WHERE DataPointID IN (SELECT ID FROM datapoints WHERE MetricID = ?)`,
		`DELETE FROM datapoints WHERE MetricID = ?`,
		`DELETE FROM metrics WHERE ID = ?`,
	}
	for _, query := range childQueries {
		if _, err := s.db.ExecContext(ctx, query, metricID); err != nil {
			return fmt.Errorf("failed to delete metric by ID: %w", err)
		}
	}

	return nil
}

// DeleteMetricsByIDs deletes multiple metrics by their IDs, including child datapoints, exemplars, and attributes.
func (s *Store) DeleteMetricsByIDs(ctx context.Context, metricIDs []any) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf("failed to delete metrics by ID: %w", err)
	}

	if len(metricIDs) == 0 {
		return nil
	}

	placeholders := buildPlaceholders(len(metricIDs))
	childQueries := []string{
		fmt.Sprintf(`DELETE FROM attributes WHERE MetricID IN (%s)`, placeholders),
		fmt.Sprintf(`DELETE FROM exemplars WHERE DataPointID IN (SELECT ID FROM datapoints WHERE MetricID IN (%s))`, placeholders),
		fmt.Sprintf(`DELETE FROM datapoints WHERE MetricID IN (%s)`, placeholders),
		fmt.Sprintf(`DELETE FROM metrics WHERE ID IN (%s)`, placeholders),
	}
	for _, query := range childQueries {
		if _, err := s.db.ExecContext(ctx, query, metricIDs...); err != nil {
			return fmt.Errorf("failed to delete metrics by ID: %w", err)
		}
	}

	return nil
}

// BuildMetricSQL converts a QueryNode into a parameterized CTE, WHERE clause, and args for metric search.
// Metrics are filtered by those that have at least one datapoint in [startTime, endTime]; conditions apply to metric columns.
func BuildMetricSQL(queryNode *QueryNode, startTime, endTime int64) (cteSQL string, whereSQL string, args []any, err error) {
	timeCondition := "EXISTS (SELECT 1 FROM datapoints d WHERE d.MetricID = m.ID AND d.Timestamp >= time_start AND d.Timestamp <= time_end)"
	return BuildSearchSQL(queryNode, startTime, endTime, metricFieldMapper(), timeCondition)
}

func metricFieldMapper() FieldMapper {
	return func(field *FieldDefinition) ([]string, error) {
		switch field.SearchScope {
		case "field":
			expr, err := mapMetricFieldExpression(field)
			if err != nil {
				return nil, err
			}
			return []string{expr}, nil
		case "attribute":
			return mapMetricAttributeExpressions(field)
		case "global":
			return mapMetricGlobalExpressions()
		default:
			return nil, fmt.Errorf("unknown search scope: %s", field.SearchScope)
		}
	}
}

func mapMetricFieldExpression(field *FieldDefinition) (string, error) {
	name := field.Name
	if name == "" {
		return "", fmt.Errorf("empty field name")
	}
	switch name {
	case "name":
		return "m.Name", nil
	case "description":
		return "m.Description", nil
	case "unit":
		return "m.Unit", nil
	case "scope.name", "scopeName":
		return "m.ScopeName", nil
	case "scope.version", "scopeVersion":
		return "m.ScopeVersion", nil
	default:
		cap := strings.ToUpper(name[:1]) + name[1:]
		return "m." + cap, nil
	}
}

func mapMetricAttributeExpressions(field *FieldDefinition) ([]string, error) {
	switch field.AttributeScope {
	case "resource", "scope", "metric":
		expr := fmt.Sprintf("(SELECT a.Value FROM attributes a WHERE a.MetricID = m.ID AND a.DataPointID IS NULL AND a.ExemplarID IS NULL AND a.Scope = '%s' AND a.Key = '%s' LIMIT 1)", field.AttributeScope, field.Name)
		return []string{expr}, nil
	default:
		return nil, fmt.Errorf("unknown attribute scope: %s", field.AttributeScope)
	}
}

func mapMetricGlobalExpressions() ([]string, error) {
	return []string{
		"m.SearchText LIKE ?",
		"EXISTS(SELECT 1 FROM attributes a WHERE a.MetricID = m.ID AND (a.Key LIKE ? OR a.Value LIKE ?))",
	}, nil
}
