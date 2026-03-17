package metrics

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/google/uuid"
	"github.com/duckdb/duckdb-go/v2"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

var (
	ErrInvalidMetricQuery   = errors.New("invalid metric search query")
	ErrMetricsStoreInternal = errors.New("metrics store internal error")
	ErrMetricIDNotFound     = errors.New("metric ID not found")
)

const flushIntervalMetrics = 100

// Ingest ingests metrics from pdata into the metrics table and related tables.
// The caller must hold any required lock on the connection.
func Ingest(ctx context.Context, conn driver.Conn, m pmetric.Metrics) error {
	tables := []string{"attributes", "exemplars", "datapoints", "metrics"}
	appenders, err := ingest.NewAppenders(conn, tables)
	if err != nil {
		return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
	}
	defer ingest.CloseAppenders(appenders, tables)

	metricCount := 0
	for _, resourceMetric := range m.ResourceMetrics().All() {
		resource := resourceMetric.Resource()
		for _, scopeMetric := range resourceMetric.ScopeMetrics().All() {
			scope := scopeMetric.Scope()
			for _, metric := range scopeMetric.Metrics().All() {
				metricID := duckdb.UUID(uuid.New())
				received := time.Now().UnixNano()
				metricSearchText := strings.Join([]string{
					metric.Name(), metric.Description(), metric.Unit(),
					scope.Name(), scope.Version(),
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
					return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
				}
				switch metric.Type() {
				case pmetric.MetricTypeGauge:
					if err := ingestGaugeDatapoints(appenders, metricID, metric.Gauge().DataPoints()); err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
					}
				case pmetric.MetricTypeSum:
					if err := ingestSumDatapoints(appenders, metricID, metric.Sum()); err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
					}
				case pmetric.MetricTypeHistogram:
					if err := ingestHistogramDatapoints(appenders, metricID, metric.Histogram()); err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
					}
				case pmetric.MetricTypeExponentialHistogram:
					if err := ingestExponentialHistogramDatapoints(appenders, metricID, metric.ExponentialHistogram()); err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
					}
				}
				ownerIDs := ingest.AttributeOwnerIDs{MetricID: &metricID}
				if err := ingest.IngestAttributes(appenders["attributes"], []ingest.AttributeBatchItem{
					{Attrs: resource.Attributes(), IDs: ownerIDs, Scope: "resource"},
					{Attrs: scope.Attributes(), IDs: ownerIDs, Scope: "scope"},
				}); err != nil {
					return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
				}
				metricCount++
				if metricCount%flushIntervalMetrics == 0 {
					if err := ingest.FlushAppenders(appenders, tables); err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
					}
				}
			}
		}
	}

	if err := ingest.FlushAppenders(appenders, tables); err != nil {
		return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
	}
	return nil
}

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
			exemplarID, datapointID, int64(ex.Timestamp()), ex.DoubleValue(), traceUUID, spanUUID,
		); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		exOwnerIDs := ingest.AttributeOwnerIDs{MetricID: &metricID, DataPointID: &datapointID, ExemplarID: &exemplarID}
		if err := ingest.IngestAttributes(appenders["attributes"], []ingest.AttributeBatchItem{
			{Attrs: ex.FilteredAttributes(), IDs: exOwnerIDs, Scope: "exemplar"},
		}); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
	}
	return nil
}

func ingestGaugeDatapoints(appenders map[string]*duckdb.Appender, metricID duckdb.UUID, dps pmetric.NumberDataPointSlice) error {
	for _, dp := range dps.All() {
		doubleVal, intVal, valType := numberDataPointValue(dp)
		datapointID := duckdb.UUID(uuid.New())
		if err := appenders["datapoints"].AppendRow(
			datapointID, metricID, "Gauge", int64(dp.Timestamp()), int64(dp.StartTimestamp()), uint32(dp.Flags()),
			doubleVal, intVal, valType, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		dpOwnerIDs := ingest.AttributeOwnerIDs{MetricID: &metricID, DataPointID: &datapointID}
		if err := ingest.IngestAttributes(appenders["attributes"], []ingest.AttributeBatchItem{
			{Attrs: dp.Attributes(), IDs: dpOwnerIDs, Scope: "datapoint"},
		}); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		if err := ingestExemplars(appenders, metricID, datapointID, dp.Exemplars()); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
	}
	return nil
}

func ingestSumDatapoints(appenders map[string]*duckdb.Appender, metricID duckdb.UUID, sum pmetric.Sum) error {
	for _, dp := range sum.DataPoints().All() {
		doubleVal, intVal, valType := numberDataPointValue(dp)
		datapointID := duckdb.UUID(uuid.New())
		if err := appenders["datapoints"].AppendRow(
			datapointID, metricID, "Sum", int64(dp.Timestamp()), int64(dp.StartTimestamp()), uint32(dp.Flags()),
			doubleVal, intVal, valType, sum.IsMonotonic(), sum.AggregationTemporality().String(),
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		dpOwnerIDs := ingest.AttributeOwnerIDs{MetricID: &metricID, DataPointID: &datapointID}
		if err := ingest.IngestAttributes(appenders["attributes"], []ingest.AttributeBatchItem{
			{Attrs: dp.Attributes(), IDs: dpOwnerIDs, Scope: "datapoint"},
		}); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		if err := ingestExemplars(appenders, metricID, datapointID, dp.Exemplars()); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
	}
	return nil
}

func ingestHistogramDatapoints(appenders map[string]*duckdb.Appender, metricID duckdb.UUID, hist pmetric.Histogram) error {
	for _, dp := range hist.DataPoints().All() {
		datapointID := duckdb.UUID(uuid.New())
		if err := appenders["datapoints"].AppendRow(
			datapointID, metricID, "Histogram", int64(dp.Timestamp()), int64(dp.StartTimestamp()), uint32(dp.Flags()),
			nil, nil, nil, nil, hist.AggregationTemporality().String(),
			dp.Count(), dp.Sum(), dp.Min(), dp.Max(), dp.BucketCounts().AsRaw(), dp.ExplicitBounds().AsRaw(),
			nil, nil, nil, nil, nil, nil,
		); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		dpOwnerIDs := ingest.AttributeOwnerIDs{MetricID: &metricID, DataPointID: &datapointID}
		if err := ingest.IngestAttributes(appenders["attributes"], []ingest.AttributeBatchItem{
			{Attrs: dp.Attributes(), IDs: dpOwnerIDs, Scope: "datapoint"},
		}); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		if err := ingestExemplars(appenders, metricID, datapointID, dp.Exemplars()); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
	}
	return nil
}

func ingestExponentialHistogramDatapoints(appenders map[string]*duckdb.Appender, metricID duckdb.UUID, exp pmetric.ExponentialHistogram) error {
	for _, dp := range exp.DataPoints().All() {
		pos, neg := dp.Positive(), dp.Negative()
		datapointID := duckdb.UUID(uuid.New())
		if err := appenders["datapoints"].AppendRow(
			datapointID, metricID, "ExponentialHistogram", int64(dp.Timestamp()), int64(dp.StartTimestamp()), uint32(dp.Flags()),
			nil, nil, nil, nil, exp.AggregationTemporality().String(),
			dp.Count(), dp.Sum(), dp.Min(), dp.Max(), nil, nil,
			dp.Scale(), dp.ZeroCount(), pos.Offset(), pos.BucketCounts().AsRaw(), neg.Offset(), neg.BucketCounts().AsRaw(),
		); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		dpOwnerIDs := ingest.AttributeOwnerIDs{MetricID: &metricID, DataPointID: &datapointID}
		if err := ingest.IngestAttributes(appenders["attributes"], []ingest.AttributeBatchItem{
			{Attrs: dp.Attributes(), IDs: dpOwnerIDs, Scope: "datapoint"},
		}); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		if err := ingestExemplars(appenders, metricID, datapointID, dp.Exemplars()); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
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

// Search returns metrics that have at least one datapoint in [startTime, endTime], matching the optional criteria.
func Search(ctx context.Context, db *sql.DB, startTime, endTime int64, criteria any) (json.RawMessage, error) {
	var searchTree *search.QueryNode
	if criteria != nil {
		var err error
		searchTree, err = search.ParseQueryTree(criteria)
		if err != nil {
			return nil, fmt.Errorf("Search: %w: %w", ErrInvalidMetricQuery, err)
		}
	}
	cteSQL, whereClause, args, err := buildMetricSQL(searchTree, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("Search: %w: %w", ErrInvalidMetricQuery, err)
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
		cteSQL, whereClause,
	)
	var raw []byte
	if err := db.QueryRowContext(ctx, finalQuery, args...).Scan(&raw); err != nil {
		return nil, fmt.Errorf("Search: %w: %w", ErrMetricsStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// Clear truncates the metrics table and all child tables.
func Clear(ctx context.Context, db *sql.DB) error {
	for _, q := range []string{
		`delete from attributes where metric_id is not null`,
		`truncate table exemplars`,
		`truncate table datapoints`,
		`truncate table metrics`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("Clear: %w: %w", ErrMetricsStoreInternal, err)
		}
	}
	return nil
}

// DeleteMetricByID deletes a specific metric by its ID.
func DeleteMetricByID(ctx context.Context, db *sql.DB, metricID string) error {
	for _, q := range []string{
		`delete from attributes where metric_id = ?`,
		`delete from exemplars where datapoint_id in (select id from datapoints where metric_id = ?)`,
		`delete from datapoints where metric_id = ?`,
		`delete from metrics where id = ?`,
	} {
		if _, err := db.ExecContext(ctx, q, metricID); err != nil {
			return fmt.Errorf("DeleteMetricByID: %w: %w", ErrMetricsStoreInternal, err)
		}
	}
	return nil
}

// DeleteMetricsByIDs deletes multiple metrics by their IDs.
func DeleteMetricsByIDs(ctx context.Context, db *sql.DB, metricIDs []any) error {
	if len(metricIDs) == 0 {
		return nil
	}
	placeholders := util.BuildPlaceholders(len(metricIDs))
	for _, q := range []string{
		fmt.Sprintf(`delete from attributes where metric_id in (%s)`, placeholders),
		fmt.Sprintf(`delete from exemplars where datapoint_id in (select id from datapoints where metric_id in (%s))`, placeholders),
		fmt.Sprintf(`delete from datapoints where metric_id in (%s)`, placeholders),
		fmt.Sprintf(`delete from metrics where id in (%s)`, placeholders),
	} {
		if _, err := db.ExecContext(ctx, q, metricIDs...); err != nil {
			return fmt.Errorf("DeleteMetricsByIDs: %w: %w", ErrMetricsStoreInternal, err)
		}
	}
	return nil
}

func buildMetricSQL(queryNode *search.QueryNode, startTime, endTime int64) (cteSQL string, whereSQL string, args []any, err error) {
	timeCondition := "exists (select 1 from datapoints d where d.metric_id = m.id and d.timestamp >= time_start and d.timestamp <= time_end)"
	return search.BuildSearchSQL(queryNode, startTime, endTime, metricFieldMapper(), timeCondition)
}

func metricFieldMapper() search.FieldMapper {
	return func(field *search.FieldDefinition) ([]string, error) {
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
			return nil, fmt.Errorf("unknown search scope %s: %w", field.SearchScope, ErrInvalidMetricQuery)
		}
	}
}

func mapMetricFieldExpression(field *search.FieldDefinition) (string, error) {
	name := field.Name
	if name == "" {
		return "", fmt.Errorf("empty field name: %w", ErrInvalidMetricQuery)
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
		return "m." + util.CamelToSnake(name), nil
	}
}

func mapMetricAttributeExpressions(field *search.FieldDefinition) ([]string, error) {
	switch field.AttributeScope {
	case "resource", "scope", "metric":
		expr := fmt.Sprintf("(SELECT a.value FROM attributes a WHERE a.metric_id = m.id AND a.datapoint_id IS NULL AND a.exemplar_id IS NULL AND a.scope = '%s' AND a.key = '%s' LIMIT 1)", field.AttributeScope, field.Name)
		return []string{expr}, nil
	default:
		return nil, fmt.Errorf("unknown attribute scope %s: %w", field.AttributeScope, ErrInvalidMetricQuery)
	}
}

func mapMetricGlobalExpressions() ([]string, error) {
	return []string{
		"m.search_text = ?",
		"EXISTS(SELECT 1 FROM attributes a WHERE a.metric_id = m.id AND (a.key = ? OR a.value = ?))",
	}, nil
}
