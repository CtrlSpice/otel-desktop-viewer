package store

import (
	"context"
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

				metricCount++
				if metricCount%flushIntervalMetrics == 0 {
					if err := FlushAppenders(appenders, tables); err != nil {
						return fmt.Errorf("failed to flush appender: %w", err)
					}
				}
			}
		}
	}

	return nil

}

// ingestExemplars appends exemplar rows and their filtered attributes for a datapoint.
func ingestExemplars(appenders map[string]*duckdb.Appender, metricID, datapointID duckdb.UUID, exemplars pmetric.ExemplarSlice) error {
	for _, ex := range exemplars.All() {
		exemplarID := duckdb.UUID(uuid.New())
		var traceUUID *duckdb.UUID
		if tid := ex.TraceID(); !tid.IsEmpty() {
			u := duckdb.UUID(tid)
			traceUUID = &u
		}
		var spanUUID *duckdb.UUID
		if sid := ex.SpanID(); !sid.IsEmpty() {
			var padded [16]byte
			copy(padded[8:], sid[:])
			u := duckdb.UUID(padded)
			spanUUID = &u
		}
		if err := appenders["exemplars"].AppendRow(
			exemplarID,            // ID UUID
			datapointID,           // DataPointID UUID
			int64(ex.Timestamp()), // Timestamp BIGINT
			ex.DoubleValue(),      // Value DOUBLE
			traceUUID,             // TraceID UUID
			spanUUID,              // SpanID UUID
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
	for _, dp := range dps.All() {
		doubleVal, intVal, valType := numberDataPointValue(dp)
		datapointID := duckdb.UUID(uuid.New())
		if err := appenders["datapoints"].AppendRow(
			datapointID,                // ID UUID
			metricID,                   // MetricID UUID
			"Gauge",                    // MetricType VARCHAR
			int64(dp.Timestamp()),      // Timestamp BIGINT
			int64(dp.StartTimestamp()), // StartTime BIGINT
			uint32(dp.Flags()),         // Flags UINTEGER
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

// ingestSumDatapoints appends Sum datapoint rows and their attributes/exemplars.
func ingestSumDatapoints(appenders map[string]*duckdb.Appender, metricID duckdb.UUID, sum pmetric.Sum) error {
	dps := sum.DataPoints()
	for _, dp := range dps.All() {
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
	for _, dp := range hist.DataPoints().All() {
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
	for _, dp := range exp.DataPoints().All() {
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

	finalQuery := fmt.Sprintf(`%s,
		filtered_metrics as (
			select m.* from metrics m, search_params
			where %s
		),
		filtered_dps as (
			select d.* from datapoints d
			inner join filtered_metrics fm on d.metric_id = fm.id, search_params
			where d.timestamp >= time_start and d.timestamp <= time_end
		),
		dp_attrs_agg as (
			select a.datapoint_id, json_group_array(json_object('key', a.key, 'value', a.value, 'type', a.type::varchar)) as attrs
			from attributes a
			where a.datapoint_id in (select id from filtered_dps) and a.scope = 'datapoint'
			group by a.datapoint_id
		),
		exemplars_agg as (
			select e.datapoint_id, json_group_array(json_object('timestamp', e.timestamp, 'value', e.value, 'traceID', e.trace_id, 'spanID', e.span_id)) as exemplars
			from exemplars e
			where e.datapoint_id in (select id from filtered_dps)
			group by e.datapoint_id
		),
		metric_res_attrs as (
			select a.metric_id, json_group_array(json_object('key', a.key, 'value', a.value, 'type', a.type::varchar)) as attrs
			from attributes a
			where a.metric_id in (select id from filtered_metrics) and a.scope = 'resource' and a.datapoint_id is null and a.exemplar_id is null
			group by a.metric_id
		),
		metric_scope_attrs as (
			select a.metric_id, json_group_array(json_object('key', a.key, 'value', a.value, 'type', a.type::varchar)) as attrs
			from attributes a
			where a.metric_id in (select id from filtered_metrics) and a.scope = 'scope' and a.datapoint_id is null and a.exemplar_id is null
			group by a.metric_id
		),
		datapoints_agg as (
			select d.metric_id, to_json(list(json_object(
				'id', d.id, 'metricType', d.metric_type, 'timestamp', d.timestamp, 'startTime', d.start_time, 'flags', d.flags,
				'doubleValue', d.double_value, 'intValue', d.int_value, 'valueType', d.value_type,
				'isMonotonic', d.is_monotonic, 'aggregationTemporality', d.aggregation_temporality,
				'count', d.count, 'sum', d.sum, 'min', d.min, 'max', d.max,
				'bucketCounts', d.bucket_counts, 'explicitBounds', d.explicit_bounds,
				'scale', d.scale, 'zeroCount', d.zero_count,
				'positiveBucketOffset', d.positive_bucket_offset, 'positiveBucketCounts', d.positive_bucket_counts,
				'negativeBucketOffset', d.negative_bucket_offset, 'negativeBucketCounts', d.negative_bucket_counts,
				'attributes', coalesce((select attrs from dp_attrs_agg where dp_attrs_agg.datapoint_id = d.id), json('[]')),
				'exemplars', coalesce((select exemplars from exemplars_agg where exemplars_agg.datapoint_id = d.id), json('[]'))
			) order by d.timestamp desc)) as datapoints
			from filtered_dps d
			group by d.metric_id
		)
		select cast(coalesce(to_json(list(json_object(
			'id', m.id, 'name', m.name, 'description', m.description, 'unit', m.unit,
			'resourceDroppedAttributesCount', m.resource_dropped_attributes_count,
			'resource', json_object('attributes', coalesce(res.attrs, json('[]')), 'droppedAttributesCount', m.resource_dropped_attributes_count),
			'scopeName', m.scope_name, 'scopeVersion', m.scope_version, 'scopeDroppedAttributesCount', m.scope_dropped_attributes_count,
			'scope', json_object('name', m.scope_name, 'version', m.scope_version, 'attributes', coalesce(scope_attrs.attrs, json('[]')), 'droppedAttributesCount', m.scope_dropped_attributes_count),
			'received', m.received,
			'datapoints', coalesce(dp_agg.datapoints, json('[]'))
		) order by m.received desc)), '[]') as varchar) as metrics
		from filtered_metrics m
		left join metric_res_attrs res on res.metric_id = m.id
		left join metric_scope_attrs scope_attrs on scope_attrs.metric_id = m.id
		left join datapoints_agg dp_agg on dp_agg.metric_id = m.id`,
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
		`delete from attributes where metric_id is not null`,
		`truncate table exemplars`,
		`truncate table datapoints`,
		`truncate table metrics`,
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
		`delete from attributes where metric_id = ?`,
		`delete from exemplars where datapoint_id in (select id from datapoints where metric_id = ?)`,
		`delete from datapoints where metric_id = ?`,
		`delete from metrics where id = ?`,
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
		fmt.Sprintf(`delete from attributes where metric_id in (%s)`, placeholders),
		fmt.Sprintf(`delete from exemplars where datapoint_id in (select id from datapoints where metric_id in (%s))`, placeholders),
		fmt.Sprintf(`delete from datapoints where metric_id in (%s)`, placeholders),
		fmt.Sprintf(`delete from metrics where id in (%s)`, placeholders),
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
	timeCondition := "exists (select 1 from datapoints d where d.metric_id = m.id and d.timestamp >= time_start and d.timestamp <= time_end)"
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
		return "m.name", nil
	case "description":
		return "m.description", nil
	case "unit":
		return "m.unit", nil
	case "scope.name", "scopeName":
		return "m.scope_name", nil
	case "scope.version", "scopeVersion":
		return "m.scope_version", nil
	default:
		return "m." + camelToSnake(name), nil
	}
}

func mapMetricAttributeExpressions(field *FieldDefinition) ([]string, error) {
	switch field.AttributeScope {
	case "resource", "scope", "metric":
		expr := fmt.Sprintf("(SELECT a.value FROM attributes a WHERE a.metric_id = m.id AND a.datapoint_id IS NULL AND a.exemplar_id IS NULL AND a.scope = '%s' AND a.key = '%s' LIMIT 1)", field.AttributeScope, field.Name)
		return []string{expr}, nil
	default:
		return nil, fmt.Errorf("unknown attribute scope: %s", field.AttributeScope)
	}
}

// mapMetricGlobalExpressions returns all SQL expressions for a global search across metrics.
//
// The "= ?" placeholders are conventions: BuildOperatorCondition replaces "= ?" with the
// actual operator and a named CTE parameter (e.g. "LIKE value_0") based on the query's
// FieldOperator.
//
// See BuildOperatorCondition in query_tree.go.
func mapMetricGlobalExpressions() ([]string, error) {
	return []string{
		"m.search_text = ?",
		"EXISTS(SELECT 1 FROM attributes a WHERE a.metric_id = m.id AND (a.key = ? OR a.value = ?))",
	}, nil
}
