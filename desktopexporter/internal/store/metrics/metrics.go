package metrics

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

var (
	ErrInvalidMetricQuery           = errors.New("invalid metric search query")
	ErrMetricsStoreInternal         = errors.New("metrics store internal error")
	ErrMetricIDNotFound             = errors.New("metric ID not found")
	ErrDatapointIDNotFound          = errors.New("datapoint ID not found")
	ErrQuantilesNotSupportedForType = errors.New("quantiles are only supported for Histogram and ExponentialHistogram datapoints")
	ErrInvalidQuantileSeriesMode    = errors.New("invalid quantile series mode")
	ErrHistogramBoundsMismatch      = errors.New("aggregated Histogram has datapoints with mismatched explicit_bounds at the same timestamp")
	ErrInvalidTimeRange             = errors.New("invalid time range: endTs must be greater than startTs")
	ErrInvalidMaxPoints             = errors.New("invalid maxPoints: must be >= 1")
	ErrUnspecifiedTemporality           = errors.New("metric has Unspecified aggregation_temporality; cannot safely aggregate over time")
	ErrBucketSeriesNotSupportedForType = errors.New("bucket series are only supported for Histogram and ExponentialHistogram datapoints")
)

// histogramBoundsMismatchTag is the literal that aggregated-Histogram SQL
// raises via `error('histogram_bounds_mismatch')` when it detects mixed
// explicit_bounds within a timestamp group. We detect it on the Go side
// with strings.Contains because duckdb-go wraps SQL errors in driver-
// specific types we don't want to import here -- substring match keeps the
// coupling to "the literal we chose", which we own on both sides.
const histogramBoundsMismatchTag = "histogram_bounds_mismatch"

const flushIntervalMetrics = 100

// Ingest ingests metrics from pdata into the metrics table and related tables.
// The caller must hold any required lock on the connection.
func Ingest(ctx context.Context, conn driver.Conn, m pmetric.Metrics) (err error) {
	tables := []string{"attributes", "exemplars", "datapoints", "metrics"}
	appenders, err := ingest.NewAppenders(conn, tables)
	if err != nil {
		return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
	}
	defer func() {
		err = errors.Join(err, ingest.CloseAppenders(appenders, tables))
	}()

	metricCount := 0
	for _, resourceMetric := range m.ResourceMetrics().All() {
		resource := resourceMetric.Resource()
		for _, scopeMetric := range resourceMetric.ScopeMetrics().All() {
			scope := scopeMetric.Scope()
			for _, metric := range scopeMetric.Metrics().All() {
				metricID := duckdb.UUID(uuid.New())
				received := time.Now().UnixNano()
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
			doubleVal, intVal, valType, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
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
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
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
			nil, nil, nil, nil, nil, nil, nil,
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
			dp.Scale(), dp.ZeroCount(), dp.ZeroThreshold(), pos.Offset(), pos.BucketCounts().AsRaw(), neg.Offset(), neg.BucketCounts().AsRaw(),
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
		exemplar_attrs as (
			select a.exemplar_id,
				json_group_array(json_object('key', a.key, 'value', a.value, 'type', a.type::varchar)) as attrs
			from attributes a
			where a.exemplar_id is not null
				and a.datapoint_id in (select id from filtered_dps)
				and a.scope = 'exemplar'
			group by a.exemplar_id
		),
		exemplars_agg as (
			select e.datapoint_id, json_group_array(json_object(
				'timestamp', e.timestamp::varchar,
				'value', e.value,
				'traceID', replace(e.trace_id::varchar, '-', ''),
				'spanID', right(replace(e.span_id::varchar, '-', ''), 16),
				'filteredAttributes', coalesce(
					(select attrs from exemplar_attrs where exemplar_attrs.exemplar_id = e.id),
					json('[]')
				)
			)) as exemplars
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
			select d.metric_id, to_json(list(json_merge_patch(
				json_object(
					'id', d.id,
					'metricType', d.metric_type,
					'timestamp', d.timestamp::varchar,
					'startTime', d.start_time::varchar,
					'flags', d.flags,
					'attributes', coalesce((select attrs from dp_attrs_agg where dp_attrs_agg.datapoint_id = d.id), json('[]')),
					'exemplars', coalesce((select exemplars from exemplars_agg where exemplars_agg.datapoint_id = d.id), json('[]'))
				),
				case d.metric_type
					when 'Gauge' then json_object(
						'doubleValue', d.double_value,
						'intValue', d.int_value,
						'valueType', d.value_type
					)
					when 'Sum' then json_object(
						'doubleValue', d.double_value,
						'intValue', d.int_value,
						'valueType', d.value_type,
						'isMonotonic', d.is_monotonic,
						'aggregationTemporality', d.aggregation_temporality
					)
					when 'Histogram' then json_object(
						'count', d.count,
						'sum', d.sum,
						'min', d.min,
						'max', d.max,
						'bucketCounts', d.bucket_counts,
						'explicitBounds', d.explicit_bounds,
						'aggregationTemporality', d.aggregation_temporality
					)
					when 'ExponentialHistogram' then json_object(
						'count', d.count,
						'sum', d.sum,
						'min', d.min,
						'max', d.max,
						'scale', d.scale,
						'zeroCount', d.zero_count,
						'zeroThreshold', d.zero_threshold,
						'positiveBucketOffset', d.positive_bucket_offset,
						'positiveBucketCounts', d.positive_bucket_counts,
						'negativeBucketOffset', d.negative_bucket_offset,
						'negativeBucketCounts', d.negative_bucket_counts,
						'aggregationTemporality', d.aggregation_temporality
					)
				end
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

// GetMetricAttributes returns a JSON array of attribute names/scopes/types for metrics
// that have at least one datapoint in the given time range.
func GetMetricAttributes(ctx context.Context, db *sql.DB, startTime, endTime int64) (json.RawMessage, error) {
	query := `
		select cast(to_json(list(json_object('name', sub.key, 'attributeScope', sub.scope, 'type', sub.type::varchar)
			order by sub.key, sub.scope)) as varchar) as attributes
		from (
			select distinct a.key, a.scope, a.type
			from attributes a
			inner join metrics m on a.metric_id = m.id
			where a.datapoint_id is null and a.exemplar_id is null
			  and exists (
				select 1 from datapoints d
				where d.metric_id = m.id
				  and d.timestamp >= ? and d.timestamp <= ?
			  )
		) sub
	`
	var raw []byte
	if err := db.QueryRowContext(ctx, query, startTime, endTime).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetMetricAttributes: %w: %w", ErrMetricsStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// GetDatapointQuantiles returns a JSON object mapping each requested quantile
// (formatted as a string key, e.g. "0.5") to its interpolated value for the
// given datapoint. Histogram datapoints use linear interpolation; exponential
// histograms use log-linear (with a linear fallback in zero/sign-mismatch
// regions). Quantiles that the macro declines to compute (empty buckets,
// total count of zero) come back as JSON null.
//
// Returns ErrDatapointIDNotFound if no datapoint matches the ID, and
// ErrQuantilesNotSupportedForType if the datapoint exists but is a Gauge or
// Sum. Quantile values outside [0, 1] are passed through to the macros, which
// will produce nonsensical-but-non-erroring results -- callers are expected
// to validate inputs.
func GetDatapointQuantiles(ctx context.Context, db *sql.DB, datapointID string, quantiles []float64) (json.RawMessage, error) {
	if len(quantiles) == 0 {
		return json.RawMessage("{}"), nil
	}

	// Build the json_object key/value pairs. Keys are float literals formatted
	// in Go (safe -- we control the format) so they appear verbatim in the
	// output. Values dispatch on metric_type: Histogram -> hist_quantile,
	// ExponentialHistogram -> exp_hist_quantile, anything else -> NULL (which
	// Go uses to surface ErrQuantilesNotSupportedForType after the scan).
	pairs := make([]string, 0, len(quantiles))
	args := make([]any, 0, len(quantiles)*2+1)
	for _, q := range quantiles {
		key := strconv.FormatFloat(q, 'f', -1, 64)
		pairs = append(pairs, fmt.Sprintf(`'%s', case metric_type
			when 'Histogram' then hist_quantile(explicit_bounds, bucket_counts, ?)
			when 'ExponentialHistogram' then exp_hist_quantile(scale, negative_bucket_offset, negative_bucket_counts, zero_count, positive_bucket_offset, positive_bucket_counts, ?)
		end`, key))
		args = append(args, q, q)
	}
	args = append(args, datapointID)

	query := fmt.Sprintf(`
		select metric_type, cast(to_json(json_object(%s)) as varchar) as quantiles
		from datapoints
		where id = ?
	`, strings.Join(pairs, ", "))

	var metricType string
	var raw []byte
	if err := db.QueryRowContext(ctx, query, args...).Scan(&metricType, &raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetDatapointQuantiles: %w: %s", ErrDatapointIDNotFound, datapointID)
		}
		return nil, fmt.Errorf("GetDatapointQuantiles: %w: %w", ErrMetricsStoreInternal, err)
	}
	if metricType != "Histogram" && metricType != "ExponentialHistogram" {
		return nil, fmt.Errorf("GetDatapointQuantiles: %w: %s", ErrQuantilesNotSupportedForType, metricType)
	}
	return json.RawMessage(raw), nil
}

// GetMetricQuantileSeries returns a JSON array of one entry per series point
// for the given metric, with each entry containing the requested quantiles
// plus merged totals (count/sum/min/max). The shape matches the
// HistogramTrendChart frontend expectation; see the histogram-trend-chart
// plan for the full per-point object schema.
//
// Time bucketing is adaptive: the helper computes
// `bucket_ns = max(1ms, (endTs - startTs) / maxPoints)` in DuckDB, snaps each
// datapoint timestamp to its bucket start, and emits one entry per
// `(stream, bucket_start)` (per-stream) or per `bucket_start` (aggregated).
// In effect maxPoints is the chart's pixel width and the result has at most
// that many output rows. The window is `[startTs, endTs)` (start inclusive,
// end exclusive).
//
// Within-bucket time merging dispatches on the metric's
// aggregation_temporality:
//   - Delta: bucket counts sum across time within each (stream, bucket).
//   - Cumulative: take the latest sample per (stream, bucket) since each
//     row is already a running total -- summing would double-count.
//
// Unspecified temporality is rejected (ErrUnspecifiedTemporality) so we
// never silently mis-aggregate.
//
// Mode controls cross-stream behavior:
//   - "per-stream": one entry per (bucket_start, attribute set).
//   - "aggregated": (Histogram only for now; ExpHistogram lands in step 4)
//     one entry per bucket_start with all streams merged via the alignment
//     pipeline. Histogram needs uniform explicit_bounds within each bucket.
//
// Returns:
//   - ErrInvalidTimeRange if endTs <= startTs.
//   - ErrInvalidMaxPoints if maxPoints < 1.
//   - ErrMetricIDNotFound if the metric has no datapoints at all (across
//     the whole table, not the window).
//   - ErrQuantilesNotSupportedForType for Gauge/Sum.
//   - ErrUnspecifiedTemporality if the metric's aggregation_temporality is
//     Unspecified.
//
// Quantile values the macros decline to compute (empty buckets, zero total
// count) come back as JSON null. Empty quantile list returns "[]" without
// touching the database. A window with no datapoints returns "[]".
func GetMetricQuantileSeries(ctx context.Context, db *sql.DB, metricID string, quantiles []float64, mode string, startTs, endTs int64, maxPoints int) (json.RawMessage, error) {
	if len(quantiles) == 0 {
		return json.RawMessage("[]"), nil
	}
	if endTs <= startTs {
		return nil, fmt.Errorf("GetMetricQuantileSeries: %w: startTs=%d endTs=%d", ErrInvalidTimeRange, startTs, endTs)
	}
	if maxPoints < 1 {
		return nil, fmt.Errorf("GetMetricQuantileSeries: %w: %d", ErrInvalidMaxPoints, maxPoints)
	}
	// Mode is a static client-supplied string; reject it before touching the
	// DB so the handler can map it to InvalidParams without a wasted query.
	if mode != "per-stream" && mode != "aggregated" {
		return nil, fmt.Errorf("GetMetricQuantileSeries: %w: %q (want per-stream or aggregated)", ErrInvalidQuantileSeriesMode, mode)
	}

	// Pre-check: confirm the metric has datapoints and figure out its type
	// + temporality from the first datapoint. All datapoints for a given
	// metric_id share both (enforced at ingest), so a single sample suffices.
	// Gauge datapoints carry NULL aggregation_temporality (the schema only
	// requires it on Sum/Histogram/ExpHistogram), so we scan it as NullString
	// and only validate it for the histogram types we actually support.
	var metricType string
	var temporality sql.NullString
	err := db.QueryRowContext(ctx,
		`select metric_type, aggregation_temporality from datapoints where metric_id = ? limit 1`,
		metricID,
	).Scan(&metricType, &temporality)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetMetricQuantileSeries: %w: %s", ErrMetricIDNotFound, metricID)
		}
		return nil, fmt.Errorf("GetMetricQuantileSeries: %w: %w", ErrMetricsStoreInternal, err)
	}
	if metricType != "Histogram" && metricType != "ExponentialHistogram" {
		return nil, fmt.Errorf("GetMetricQuantileSeries: %w: %s", ErrQuantilesNotSupportedForType, metricType)
	}
	if !temporality.Valid || (temporality.String != "Delta" && temporality.String != "Cumulative") {
		return nil, fmt.Errorf("GetMetricQuantileSeries: %w: %s", ErrUnspecifiedTemporality, temporality.String)
	}

	// Mode was validated above; metric_type is Histogram or ExponentialHistogram.
	if mode == "per-stream" {
		return getPerStreamQuantileSeries(ctx, db, metricID, quantiles, temporality.String, startTs, endTs, maxPoints)
	}
	switch metricType {
	case "Histogram":
		return getAggregatedHistogramQuantileSeries(ctx, db, metricID, quantiles, temporality.String, startTs, endTs, maxPoints)
	case "ExponentialHistogram":
		return getAggregatedExpHistogramQuantileSeries(ctx, db, metricID, quantiles, temporality.String, startTs, endTs, maxPoints)
	}
	return nil, fmt.Errorf("GetMetricQuantileSeries: %w: %s", ErrQuantilesNotSupportedForType, metricType)
}

// quantileSeriesCTEs is the shared CTE preamble used by every quantile
// series variant. It defines:
//
//   - params: bind-parameter window (start_ts, end_ts) and computed
//     bucket_ns (clamped to >= 1 ms).
//   - dp_attrs: per-datapoint attribute array + stable "k=v|k=v" key
//     (the per-stream identity).
//   - bucketed: filtered (by metric_id and time window) datapoints with
//     attached attrs + bucket_start.
//   - time_merged: per (bucket_start, attrs_key) row, with within-bucket
//     time merging dispatched on temporality. Delta sums bucket vectors
//     across time; Cumulative takes the latest sample (each cumulative
//     row is a running total, summing would double-count).
//
// Variants downstream (per-stream final select, aggregated cross-stream
// merge, aggregated ExpHistogram alignment) all start from `time_merged`.
// Returns the SQL fragment ending after the time_merged CTE definition
// (with a trailing comma so callers can append more CTEs cleanly), plus
// the args slice for the 7 placeholders consumed here:
// (start_ts, end_ts, end_ts, start_ts, max_points, metric_id, metric_id).
func quantileSeriesCTEs(metricID string, startTs, endTs int64, maxPoints int, temporality string) (string, []any) {
	// When the UI sends start_ts=0 ("All" preset), the bucket width would be
	// computed over the entire epoch-to-now range, collapsing all datapoints
	// into a single bucket. Clamping to the metric's actual earliest timestamp
	// keeps the bucketing proportional to the real data span.
	const cteSharedHead = `
		with params as (
			select
				cast(? as bigint) as start_ts,
				cast(? as bigint) as end_ts,
				greatest(1000000::bigint,
					(cast(? as bigint) - greatest(cast(? as bigint),
						coalesce((select min(timestamp) from datapoints where metric_id = ?), cast(? as bigint))
					)) // cast(? as bigint)
				) as bucket_ns
		),
		dp_attrs as (
			select
				a.datapoint_id,
				to_json(list(
					json_object('key', a.key, 'value', a.value, 'type', a.type::varchar)
					order by a.key
				)) as attrs,
				coalesce(string_agg(a.key || '=' || a.value, '|' order by a.key), '') as attrs_key
			from attributes a
			where a.datapoint_id in (select id from datapoints where metric_id = ?)
			  and a.scope = 'datapoint'
			group by a.datapoint_id
		),
		bucketed as (
			select
				d.id, d.metric_type, d.timestamp,
				coalesce(da.attrs_key, '') as attrs_key,
				da.attrs,
				d.bucket_counts, d.explicit_bounds,
				d.scale, d.zero_count, d.zero_threshold,
				d.positive_bucket_offset, d.positive_bucket_counts,
				d.negative_bucket_offset, d.negative_bucket_counts,
				d.count, d.sum, d.min, d.max,
				(d.timestamp // p.bucket_ns) * p.bucket_ns as bucket_start
			from datapoints d
			left join dp_attrs da on da.datapoint_id = d.id
			cross join params p
			where d.metric_id = ?
			  and d.timestamp >= p.start_ts
			  and d.timestamp <  p.end_ts
		),`

	// Time-merge CTE differs by temporality. Both group by (bucket_start,
	// attrs_key) so the downstream shape is identical.
	const timeMergedDelta = `
		time_merged as (
			select
				bucket_start,
				attrs_key,
				any_value(attrs) as attrs,
				any_value(metric_type) as metric_type,
				sum_bucket_vectors(list(bucket_counts)) as bucket_counts,
				any_value(explicit_bounds) as explicit_bounds,
				any_value(scale) as scale,
				sum(zero_count) as zero_count,
				max(zero_threshold) as zero_threshold,
				any_value(positive_bucket_offset) as positive_bucket_offset,
				sum_bucket_vectors(list(positive_bucket_counts)) as positive_bucket_counts,
				any_value(negative_bucket_offset) as negative_bucket_offset,
				sum_bucket_vectors(list(negative_bucket_counts)) as negative_bucket_counts,
				sum(count) as count,
				sum(sum)   as sum,
				min(min)   as min,
				max(max)   as max
			from bucketed
			group by bucket_start, attrs_key
		)`

	const timeMergedCumulative = `
		time_merged as (
			select
				bucket_start,
				attrs_key,
				arg_max(attrs, timestamp) as attrs,
				any_value(metric_type) as metric_type,
				arg_max(bucket_counts, timestamp) as bucket_counts,
				arg_max(explicit_bounds, timestamp) as explicit_bounds,
				arg_max(scale, timestamp) as scale,
				arg_max(zero_count, timestamp) as zero_count,
				arg_max(zero_threshold, timestamp) as zero_threshold,
				arg_max(positive_bucket_offset, timestamp) as positive_bucket_offset,
				arg_max(positive_bucket_counts, timestamp) as positive_bucket_counts,
				arg_max(negative_bucket_offset, timestamp) as negative_bucket_offset,
				arg_max(negative_bucket_counts, timestamp) as negative_bucket_counts,
				arg_max(count, timestamp) as count,
				arg_max(sum,   timestamp) as sum,
				arg_max(min,   timestamp) as min,
				arg_max(max,   timestamp) as max
			from bucketed
			group by bucket_start, attrs_key
		)`

	var timeMerged string
	switch temporality {
	case "Cumulative":
		timeMerged = timeMergedCumulative
	default: // Delta
		timeMerged = timeMergedDelta
	}

	args := []any{startTs, endTs, endTs, startTs, metricID, startTs, maxPoints, metricID, metricID}
	return cteSharedHead + timeMerged, args
}

// getPerStreamQuantileSeries renders one entry per (bucket_start, attrs_key)
// after the shared bucketing + time-merge pipeline. Quantile dispatch is by
// metric_type (Histogram vs ExponentialHistogram) inside the json_object.
// The macros may return NULL for a quantile (e.g. empty buckets); that
// surfaces as JSON null.
func getPerStreamQuantileSeries(ctx context.Context, db *sql.DB, metricID string, quantiles []float64, temporality string, startTs, endTs int64, maxPoints int) (json.RawMessage, error) {
	cteSQL, cteArgs := quantileSeriesCTEs(metricID, startTs, endTs, maxPoints, temporality)
	pairsSQL, quantileArgs := buildPerStreamQuantilePairs(quantiles)
	args := make([]any, 0, len(cteArgs)+len(quantileArgs))
	args = append(args, cteArgs...)
	args = append(args, quantileArgs...)

	query := fmt.Sprintf(`%s
		select cast(coalesce(to_json(list(json_object(
			'timestamp', m.bucket_start::varchar,
			'attributesKey', m.attrs_key,
			'attributes', coalesce(m.attrs, json('[]')),
			'quantiles', json_object(%s),
			'count', m.count,
			'sum', m.sum,
			'min', m.min,
			'max', m.max
		) order by m.bucket_start, m.attrs_key)), '[]') as varchar) as series
		from time_merged m
	`, cteSQL, pairsSQL)

	var raw []byte
	if err := db.QueryRowContext(ctx, query, args...).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetMetricQuantileSeries: %w: %w", ErrMetricsStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// getAggregatedHistogramQuantileSeries merges all streams of a Histogram
// metric per timestamp and returns one entry per timestamp with quantiles
// computed over the merged bucket vector.
//
// Merge math: bucket counts add element-wise (sum_bucket_vectors), totals
// roll up via sum/min/max aggregates. This is mathematically valid only
// when every datapoint in a group shares the same explicit_bounds -- so we
// gate that with count(distinct explicit_bounds) and raise
// `error('histogram_bounds_mismatch')` from inside a CASE expression when
// the group has more than one bound shape. CASE is short-circuit, so the
// happy path doesn't pay for the error branch.
//
// The mismatch error is translated to ErrHistogramBoundsMismatch on the
// Go side via substring match, so the JSON-RPC layer can map it to a
// user-facing code.
func getAggregatedHistogramQuantileSeries(ctx context.Context, db *sql.DB, metricID string, quantiles []float64, temporality string, startTs, endTs int64, maxPoints int) (json.RawMessage, error) {
	cteSQL, cteArgs := quantileSeriesCTEs(metricID, startTs, endTs, maxPoints, temporality)
	pairsSQL, quantileArgs := buildAggregatedHistogramQuantilePairs(quantiles)
	args := make([]any, 0, len(cteArgs)+len(quantileArgs))
	args = append(args, cteArgs...)
	args = append(args, quantileArgs...)

	// `time_merged` already collapsed within each (stream, bucket); now
	// `grouped` collapses across streams within each bucket. The bounds
	// uniformity check fires here -- streams with mismatched explicit_bounds
	// at the same bucket are unmergeable, so we raise the sentinel error
	// inside the next CTE's CASE for short-circuit semantics. After the
	// merge, totals roll up via plain sum/min/max aggregates.
	query := fmt.Sprintf(`%s,
		grouped as (
			select
				bucket_start,
				any_value(explicit_bounds) as bounds,
				count(distinct explicit_bounds) as bound_variants,
				list(bucket_counts) as bucket_vectors,
				sum(count) as total_count,
				sum(sum)   as total_sum,
				min(min)   as total_min,
				max(max)   as total_max
			from time_merged
			group by bucket_start
		),
		merged as (
			select
				bucket_start,
				case
					when bound_variants > 1 then error('%s')
					else bounds
				end as bounds,
				sum_bucket_vectors(bucket_vectors) as merged_counts,
				total_count, total_sum, total_min, total_max
			from grouped
		)
		select cast(coalesce(to_json(list(json_object(
			'timestamp', m.bucket_start::varchar,
			'attributesKey', '',
			'attributes', json('[]'),
			'quantiles', json_object(%s),
			'count', m.total_count,
			'sum', m.total_sum,
			'min', m.total_min,
			'max', m.total_max
		) order by m.bucket_start)), '[]') as varchar) as series
		from merged m
	`, cteSQL, histogramBoundsMismatchTag, pairsSQL)

	var raw []byte
	if err := db.QueryRowContext(ctx, query, args...).Scan(&raw); err != nil {
		// DuckDB's error('histogram_bounds_mismatch') surfaces as a generic
		// driver error wrapping the literal -- match on the tag we own.
		if strings.Contains(err.Error(), histogramBoundsMismatchTag) {
			return nil, fmt.Errorf("GetMetricQuantileSeries: %w", ErrHistogramBoundsMismatch)
		}
		return nil, fmt.Errorf("GetMetricQuantileSeries: %w: %w", ErrMetricsStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// buildAggregatedHistogramQuantilePairs renders the per-quantile pairs for
// the aggregated-Histogram select. Unlike the per-stream variant, no
// metric_type CASE is needed -- the helper is only called after we've
// confirmed the metric is a Histogram, so every row in `merged` has the
// same shape. Each quantile contributes one `?` placeholder. Columns come
// from the `merged m` CTE row.
func buildAggregatedHistogramQuantilePairs(quantiles []float64) (string, []any) {
	pairs := make([]string, 0, len(quantiles))
	args := make([]any, 0, len(quantiles))
	for _, q := range quantiles {
		key := strconv.FormatFloat(q, 'f', -1, 64)
		pairs = append(pairs, fmt.Sprintf(`'%s', hist_quantile(m.bounds, m.merged_counts, ?)`, key))
		args = append(args, q)
	}
	return strings.Join(pairs, ", "), args
}

// getAggregatedExpHistogramQuantileSeries merges all streams of an
// ExponentialHistogram per bucket using the full alignment pipeline:
// downscale every stream to the bucket's minimum scale, left-pad to the
// bucket's minimum (post-downscale) offset, sum the bucket vectors, then
// fold buckets at or below the merged zero_threshold's cutoff index back
// into zero_count. Positive and negative sides run independently with the
// same shared cutoff (since |bucket k|'s magnitude is symmetric).
//
// CTE chain (after the shared bucketing/time_merge preamble):
//
//	bucket_targets   -- per bucket: target_scale = min(scale),
//	                  target_zero_threshold = max(zero_threshold), totals
//	downscaled       -- per (stream, bucket): pos_ds, neg_ds at target_scale
//	bucket_offsets   -- per bucket: target_pos_offset, target_neg_offset
//	padded           -- per (stream, bucket): left-pad to target offsets
//	summed           -- per bucket: sum_bucket_vectors across streams
//	folded           -- per bucket: cutoff -> fold_below_cutoff on each side
//	final            -- per bucket: roll up zero_count + folded amounts
//
// The cutoff for a target_zero_threshold T at target_scale s is the largest
// bucket index k such that the bucket's upper bound 2^((k+1)/2^s) <= T.
// Algebraically k = floor(log2(T) * 2^s) - 1. When T = 0, cutoff is NULL
// and fold_below_cutoff is a no-op (folded = 0, counts/offset unchanged).
func getAggregatedExpHistogramQuantileSeries(ctx context.Context, db *sql.DB, metricID string, quantiles []float64, temporality string, startTs, endTs int64, maxPoints int) (json.RawMessage, error) {
	cteSQL, cteArgs := quantileSeriesCTEs(metricID, startTs, endTs, maxPoints, temporality)
	pairsSQL, quantileArgs := buildAggregatedExpHistogramQuantilePairs(quantiles)
	args := make([]any, 0, len(cteArgs)+len(quantileArgs))
	args = append(args, cteArgs...)
	args = append(args, quantileArgs...)

	query := fmt.Sprintf(`%s,
		bucket_targets as (
			select
				bucket_start,
				min(scale) as target_scale,
				max(zero_threshold) as target_zero_threshold,
				sum(zero_count) as base_zero_count,
				sum(count) as total_count,
				sum(sum)   as total_sum,
				min(min)   as total_min,
				max(max)   as total_max
			from time_merged
			group by bucket_start
		),
		-- Two-step downscale: first materialize the per-row 'levels' value
		-- as a plain column, then call the macro. downscale_exp_buckets has
		-- its own internal CTE, and DuckDB refuses to bind cross-CTE
		-- correlated arguments into a subquery context. With levels in
		-- with_levels, every macro argument is a simple column ref.
		with_levels as (
			select
				tm.bucket_start,
				tm.positive_bucket_counts,
				tm.positive_bucket_offset,
				tm.negative_bucket_counts,
				tm.negative_bucket_offset,
				tm.scale - bt.target_scale as levels
			from time_merged tm
			join bucket_targets bt using (bucket_start)
		),
		downscaled as (
			select
				bucket_start,
				downscale_exp_buckets(positive_bucket_counts, positive_bucket_offset, levels) as pos_ds,
				downscale_exp_buckets(negative_bucket_counts, negative_bucket_offset, levels) as neg_ds
			from with_levels
		),
		bucket_offsets as (
			select
				bucket_start,
				min(pos_ds.offset) as pos_target_offset,
				min(neg_ds.offset) as neg_target_offset
			from downscaled
			group by bucket_start
		),
		padded as (
			select
				d.bucket_start,
				pad_left_to_offset(d.pos_ds.counts, d.pos_ds.offset, bo.pos_target_offset) as pos_padded,
				pad_left_to_offset(d.neg_ds.counts, d.neg_ds.offset, bo.neg_target_offset) as neg_padded
			from downscaled d
			join bucket_offsets bo using (bucket_start)
		),
		summed as (
			select
				bucket_start,
				sum_bucket_vectors(list(pos_padded)) as pos_summed,
				sum_bucket_vectors(list(neg_padded)) as neg_summed
			from padded
			group by bucket_start
		),
		folded as (
			select
				bt.bucket_start,
				bt.target_scale,
				bt.target_zero_threshold,
				bt.base_zero_count,
				bt.total_count, bt.total_sum, bt.total_min, bt.total_max,
				fold_below_cutoff(
					s.pos_summed, bo.pos_target_offset,
					case
						when bt.target_zero_threshold > 0
							then cast(floor(log2(bt.target_zero_threshold) * pow(2, bt.target_scale)) as bigint) - 1
						else null
					end
				) as pos_fold,
				fold_below_cutoff(
					s.neg_summed, bo.neg_target_offset,
					case
						when bt.target_zero_threshold > 0
							then cast(floor(log2(bt.target_zero_threshold) * pow(2, bt.target_scale)) as bigint) - 1
						else null
					end
				) as neg_fold
			from bucket_targets bt
			join bucket_offsets bo using (bucket_start)
			join summed s         using (bucket_start)
		),
		final as (
			select
				bucket_start,
				target_scale,
				coalesce(pos_fold.offset, 0)               as pos_offset,
				coalesce(pos_fold.counts, []::bigint[])    as pos_counts,
				coalesce(neg_fold.offset, 0)               as neg_offset,
				coalesce(neg_fold.counts, []::bigint[])    as neg_counts,
				base_zero_count
					+ coalesce(pos_fold.folded, 0)
					+ coalesce(neg_fold.folded, 0)         as final_zero_count,
				total_count, total_sum, total_min, total_max
			from folded
		)
		select cast(coalesce(to_json(list(json_object(
			'timestamp', m.bucket_start::varchar,
			'attributesKey', '',
			'attributes', json('[]'),
			'quantiles', json_object(%s),
			'count', m.total_count,
			'sum', m.total_sum,
			'min', m.total_min,
			'max', m.total_max
		) order by m.bucket_start)), '[]') as varchar) as series
		from final m
	`, cteSQL, pairsSQL)

	var raw []byte
	if err := db.QueryRowContext(ctx, query, args...).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetMetricQuantileSeries: %w: %w", ErrMetricsStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// buildAggregatedExpHistogramQuantilePairs renders the per-quantile pairs
// for the aggregated-ExpHistogram select. Each quantile contributes one `?`
// placeholder; columns come from the `final m` CTE row.
func buildAggregatedExpHistogramQuantilePairs(quantiles []float64) (string, []any) {
	pairs := make([]string, 0, len(quantiles))
	args := make([]any, 0, len(quantiles))
	for _, q := range quantiles {
		key := strconv.FormatFloat(q, 'f', -1, 64)
		pairs = append(pairs, fmt.Sprintf(`'%s', exp_hist_quantile(m.target_scale, m.neg_offset, m.neg_counts, m.final_zero_count, m.pos_offset, m.pos_counts, ?)`, key))
		args = append(args, q)
	}
	return strings.Join(pairs, ", "), args
}

// buildPerStreamQuantilePairs renders the comma-separated `'<key>', case ...`
// pairs that go inside json_object(...) for per-stream quantile selection.
// Each quantile contributes two `?` placeholders (one for hist_quantile, one
// for exp_hist_quantile), so the returned args slice is len(quantiles) * 2
// long and must be appended to whatever trailing args the caller has.
//
// Mirrors the dispatch pattern in GetDatapointQuantiles: case on metric_type,
// route Histogram to hist_quantile and ExponentialHistogram to
// exp_hist_quantile. Anything else falls through to NULL (callers gate this
// at the metric_type pre-check, so it shouldn't be reachable in practice).
// Columns come from the `time_merged m` CTE row.
func buildPerStreamQuantilePairs(quantiles []float64) (string, []any) {
	pairs := make([]string, 0, len(quantiles))
	args := make([]any, 0, len(quantiles)*2)
	for _, q := range quantiles {
		key := strconv.FormatFloat(q, 'f', -1, 64)
		pairs = append(pairs, fmt.Sprintf(`'%s', case m.metric_type
			when 'Histogram' then hist_quantile(m.explicit_bounds, m.bucket_counts, ?)
			when 'ExponentialHistogram' then exp_hist_quantile(m.scale, m.negative_bucket_offset, m.negative_bucket_counts, m.zero_count, m.positive_bucket_offset, m.positive_bucket_counts, ?)
		end`, key))
		args = append(args, q, q)
	}
	return strings.Join(pairs, ", "), args
}

// GetMetricBucketSeries returns a JSON array of one entry per time bucket
// for the given metric, with each entry containing the raw bucket vectors
// plus merged totals (count/sum/min/max). Unlike GetMetricQuantileSeries,
// no quantile computation is performed -- the caller receives the merged
// distribution data directly for heatmap rendering.
//
// Time bucketing, temporality dispatch, and mode semantics are identical to
// GetMetricQuantileSeries. See that function's doc comment for details.
//
// Returns:
//   - ErrInvalidTimeRange if endTs <= startTs.
//   - ErrInvalidMaxPoints if maxPoints < 1.
//   - ErrMetricIDNotFound if the metric has no datapoints.
//   - ErrBucketSeriesNotSupportedForType for Gauge/Sum.
//   - ErrUnspecifiedTemporality if aggregation_temporality is Unspecified.
//   - ErrHistogramBoundsMismatch if aggregated Histogram has mixed bounds.
func GetMetricBucketSeries(ctx context.Context, db *sql.DB, metricID string, mode string, startTs, endTs int64, maxPoints int) (json.RawMessage, error) {
	if endTs <= startTs {
		return nil, fmt.Errorf("GetMetricBucketSeries: %w: startTs=%d endTs=%d", ErrInvalidTimeRange, startTs, endTs)
	}
	if maxPoints < 1 {
		return nil, fmt.Errorf("GetMetricBucketSeries: %w: %d", ErrInvalidMaxPoints, maxPoints)
	}
	if mode != "per-stream" && mode != "aggregated" {
		return nil, fmt.Errorf("GetMetricBucketSeries: %w: %q (want per-stream or aggregated)", ErrInvalidQuantileSeriesMode, mode)
	}

	var metricType string
	var temporality sql.NullString
	err := db.QueryRowContext(ctx,
		`select metric_type, aggregation_temporality from datapoints where metric_id = ? limit 1`,
		metricID,
	).Scan(&metricType, &temporality)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetMetricBucketSeries: %w: %s", ErrMetricIDNotFound, metricID)
		}
		return nil, fmt.Errorf("GetMetricBucketSeries: %w: %w", ErrMetricsStoreInternal, err)
	}
	if metricType != "Histogram" && metricType != "ExponentialHistogram" {
		return nil, fmt.Errorf("GetMetricBucketSeries: %w: %s", ErrBucketSeriesNotSupportedForType, metricType)
	}
	if !temporality.Valid || (temporality.String != "Delta" && temporality.String != "Cumulative") {
		return nil, fmt.Errorf("GetMetricBucketSeries: %w: %s", ErrUnspecifiedTemporality, temporality.String)
	}

	if mode == "per-stream" {
		return getPerStreamBucketSeries(ctx, db, metricID, metricType, temporality.String, startTs, endTs, maxPoints)
	}
	switch metricType {
	case "Histogram":
		return getAggregatedHistogramBucketSeries(ctx, db, metricID, temporality.String, startTs, endTs, maxPoints)
	case "ExponentialHistogram":
		return getAggregatedExpHistogramBucketSeries(ctx, db, metricID, temporality.String, startTs, endTs, maxPoints)
	}
	return nil, fmt.Errorf("GetMetricBucketSeries: %w: %s", ErrBucketSeriesNotSupportedForType, metricType)
}

// getPerStreamBucketSeries emits one JSON entry per (bucket_start, attrs_key)
// with the raw bucket vectors and totals. The metric_type determines which
// fields are populated in the output object.
func getPerStreamBucketSeries(ctx context.Context, db *sql.DB, metricID, metricType, temporality string, startTs, endTs int64, maxPoints int) (json.RawMessage, error) {
	cteSQL, cteArgs := quantileSeriesCTEs(metricID, startTs, endTs, maxPoints, temporality)

	var selectSQL string
	switch metricType {
	case "Histogram":
		selectSQL = fmt.Sprintf(`%s
			select cast(coalesce(to_json(list(json_object(
				'kind', 'histogram',
				'timestamp', m.bucket_start::varchar,
				'attributesKey', m.attrs_key,
				'attributes', coalesce(m.attrs, json('[]')),
				'bounds', m.explicit_bounds,
				'counts', m.bucket_counts,
				'totals', json_object(
					'count', m.count,
					'sum', m.sum,
					'min', m.min,
					'max', m.max
				)
			) order by m.bucket_start, m.attrs_key)), '[]') as varchar) as series
			from time_merged m
		`, cteSQL)
	case "ExponentialHistogram":
		selectSQL = fmt.Sprintf(`%s
			select cast(coalesce(to_json(list(json_object(
				'kind', 'expHistogram',
				'timestamp', m.bucket_start::varchar,
				'attributesKey', m.attrs_key,
				'attributes', coalesce(m.attrs, json('[]')),
				'scale', m.scale,
				'zeroThreshold', m.zero_threshold,
				'zeroCount', m.zero_count,
				'positiveOffset', m.positive_bucket_offset,
				'positiveCounts', m.positive_bucket_counts,
				'negativeOffset', m.negative_bucket_offset,
				'negativeCounts', m.negative_bucket_counts,
				'totals', json_object(
					'count', m.count,
					'sum', m.sum,
					'min', m.min,
					'max', m.max
				)
			) order by m.bucket_start, m.attrs_key)), '[]') as varchar) as series
			from time_merged m
		`, cteSQL)
	}

	var raw []byte
	if err := db.QueryRowContext(ctx, selectSQL, cteArgs...).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetMetricBucketSeries: %w: %w", ErrMetricsStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// getAggregatedHistogramBucketSeries merges all streams of a Histogram
// metric per timestamp and returns the merged bucket vectors and totals.
// Uses the same grouped -> merged CTE chain as the quantile variant,
// including the bounds mismatch check.
func getAggregatedHistogramBucketSeries(ctx context.Context, db *sql.DB, metricID, temporality string, startTs, endTs int64, maxPoints int) (json.RawMessage, error) {
	cteSQL, cteArgs := quantileSeriesCTEs(metricID, startTs, endTs, maxPoints, temporality)

	query := fmt.Sprintf(`%s,
		grouped as (
			select
				bucket_start,
				any_value(explicit_bounds) as bounds,
				count(distinct explicit_bounds) as bound_variants,
				list(bucket_counts) as bucket_vectors,
				sum(count) as total_count,
				sum(sum)   as total_sum,
				min(min)   as total_min,
				max(max)   as total_max
			from time_merged
			group by bucket_start
		),
		merged as (
			select
				bucket_start,
				case
					when bound_variants > 1 then error('%s')
					else bounds
				end as bounds,
				sum_bucket_vectors(bucket_vectors) as merged_counts,
				total_count, total_sum, total_min, total_max
			from grouped
		)
		select cast(coalesce(to_json(list(json_object(
			'kind', 'histogram',
			'timestamp', m.bucket_start::varchar,
			'attributesKey', '',
			'attributes', json('[]'),
			'bounds', m.bounds,
			'counts', m.merged_counts,
			'totals', json_object(
				'count', m.total_count,
				'sum', m.total_sum,
				'min', m.total_min,
				'max', m.total_max
			)
		) order by m.bucket_start)), '[]') as varchar) as series
		from merged m
	`, cteSQL, histogramBoundsMismatchTag)

	var raw []byte
	if err := db.QueryRowContext(ctx, query, cteArgs...).Scan(&raw); err != nil {
		if strings.Contains(err.Error(), histogramBoundsMismatchTag) {
			return nil, fmt.Errorf("GetMetricBucketSeries: %w", ErrHistogramBoundsMismatch)
		}
		return nil, fmt.Errorf("GetMetricBucketSeries: %w: %w", ErrMetricsStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// getAggregatedExpHistogramBucketSeries merges all streams of an
// ExponentialHistogram per bucket using the full alignment pipeline
// (downscale, pad, sum, fold) and returns the aligned bucket arrays.
// Reuses the same CTE chain as the quantile variant but selects the
// raw vectors from `final` instead of computing quantiles.
func getAggregatedExpHistogramBucketSeries(ctx context.Context, db *sql.DB, metricID, temporality string, startTs, endTs int64, maxPoints int) (json.RawMessage, error) {
	cteSQL, cteArgs := quantileSeriesCTEs(metricID, startTs, endTs, maxPoints, temporality)

	query := fmt.Sprintf(`%s,
		bucket_targets as (
			select
				bucket_start,
				min(scale) as target_scale,
				max(zero_threshold) as target_zero_threshold,
				sum(zero_count) as base_zero_count,
				sum(count) as total_count,
				sum(sum)   as total_sum,
				min(min)   as total_min,
				max(max)   as total_max
			from time_merged
			group by bucket_start
		),
		with_levels as (
			select
				tm.bucket_start,
				tm.positive_bucket_counts,
				tm.positive_bucket_offset,
				tm.negative_bucket_counts,
				tm.negative_bucket_offset,
				tm.scale - bt.target_scale as levels
			from time_merged tm
			join bucket_targets bt using (bucket_start)
		),
		downscaled as (
			select
				bucket_start,
				downscale_exp_buckets(positive_bucket_counts, positive_bucket_offset, levels) as pos_ds,
				downscale_exp_buckets(negative_bucket_counts, negative_bucket_offset, levels) as neg_ds
			from with_levels
		),
		bucket_offsets as (
			select
				bucket_start,
				min(pos_ds.offset) as pos_target_offset,
				min(neg_ds.offset) as neg_target_offset
			from downscaled
			group by bucket_start
		),
		padded as (
			select
				d.bucket_start,
				pad_left_to_offset(d.pos_ds.counts, d.pos_ds.offset, bo.pos_target_offset) as pos_padded,
				pad_left_to_offset(d.neg_ds.counts, d.neg_ds.offset, bo.neg_target_offset) as neg_padded
			from downscaled d
			join bucket_offsets bo using (bucket_start)
		),
		summed as (
			select
				bucket_start,
				sum_bucket_vectors(list(pos_padded)) as pos_summed,
				sum_bucket_vectors(list(neg_padded)) as neg_summed
			from padded
			group by bucket_start
		),
		folded as (
			select
				bt.bucket_start,
				bt.target_scale,
				bt.target_zero_threshold,
				bt.base_zero_count,
				bt.total_count, bt.total_sum, bt.total_min, bt.total_max,
				fold_below_cutoff(
					s.pos_summed, bo.pos_target_offset,
					case
						when bt.target_zero_threshold > 0
							then cast(floor(log2(bt.target_zero_threshold) * pow(2, bt.target_scale)) as bigint) - 1
						else null
					end
				) as pos_fold,
				fold_below_cutoff(
					s.neg_summed, bo.neg_target_offset,
					case
						when bt.target_zero_threshold > 0
							then cast(floor(log2(bt.target_zero_threshold) * pow(2, bt.target_scale)) as bigint) - 1
						else null
					end
				) as neg_fold
			from bucket_targets bt
			join bucket_offsets bo using (bucket_start)
			join summed s         using (bucket_start)
		),
		final as (
			select
				bucket_start,
				target_scale,
				target_zero_threshold,
				coalesce(pos_fold.offset, 0)               as pos_offset,
				coalesce(pos_fold.counts, []::bigint[])    as pos_counts,
				coalesce(neg_fold.offset, 0)               as neg_offset,
				coalesce(neg_fold.counts, []::bigint[])    as neg_counts,
				base_zero_count
					+ coalesce(pos_fold.folded, 0)
					+ coalesce(neg_fold.folded, 0)         as final_zero_count,
				total_count, total_sum, total_min, total_max
			from folded
		)
		select cast(coalesce(to_json(list(json_object(
			'kind', 'expHistogram',
			'timestamp', m.bucket_start::varchar,
			'attributesKey', '',
			'attributes', json('[]'),
			'scale', m.target_scale,
			'zeroThreshold', m.target_zero_threshold,
			'zeroCount', m.final_zero_count,
			'positiveOffset', m.pos_offset,
			'positiveCounts', m.pos_counts,
			'negativeOffset', m.neg_offset,
			'negativeCounts', m.neg_counts,
			'totals', json_object(
				'count', m.total_count,
				'sum', m.total_sum,
				'min', m.total_min,
				'max', m.total_max
			)
		) order by m.bucket_start)), '[]') as varchar) as series
		from final m
	`, cteSQL)

	var raw []byte
	if err := db.QueryRowContext(ctx, query, cteArgs...).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetMetricBucketSeries: %w: %w", ErrMetricsStoreInternal, err)
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

var metricColumns = map[string]struct{}{
	"id":                                {},
	"name":                              {},
	"description":                       {},
	"unit":                              {},
	"type":                              {},
	"received":                          {},
	"resource_dropped_attributes_count": {},
	"scope_name":                        {},
	"scope_version":                     {},
	"scope_dropped_attributes_count":    {},
}

func metricFieldMapper() search.FieldMapper {
	return func(field *search.FieldDefinition, params *[]search.NamedParam) ([]string, error) {
		switch field.SearchScope {
		case "field":
			expr, err := mapMetricFieldExpression(field)
			if err != nil {
				return nil, err
			}
			return []string{expr}, nil
		case "attribute":
			return mapMetricAttributeExpressions(field, params)
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
		col := util.CamelToSnake(name)
		if err := util.ValidateColumnName(col, metricColumns); err != nil {
			return "", fmt.Errorf("metric field %q: %w: %w", name, err, ErrInvalidMetricQuery)
		}
		return "m." + col, nil
	}
}

func mapMetricAttributeExpressions(field *search.FieldDefinition, params *[]search.NamedParam) ([]string, error) {
	idx := len(*params)
	scopeParam := fmt.Sprintf("attr_scope_%d", idx)
	keyParam := fmt.Sprintf("attr_key_%d", idx+1)
	*params = append(*params,
		search.NamedParam{Name: scopeParam, Value: field.AttributeScope},
		search.NamedParam{Name: keyParam, Value: field.Name},
	)

	switch field.AttributeScope {
	case "resource", "scope", "metric":
		expr := fmt.Sprintf("(SELECT a.value FROM attributes a WHERE a.metric_id = m.id AND a.datapoint_id IS NULL AND a.exemplar_id IS NULL AND a.scope = %s AND a.key = %s LIMIT 1)", scopeParam, keyParam)
		return []string{expr}, nil
	default:
		return nil, fmt.Errorf("unknown attribute scope %s: %w", field.AttributeScope, ErrInvalidMetricQuery)
	}
}

func mapMetricGlobalExpressions() ([]string, error) {
	return []string{
		"CAST(m.name AS VARCHAR) {COND}",
		"CAST(m.description AS VARCHAR) {COND}",
		"CAST(m.unit AS VARCHAR) {COND}",
		"CAST(m.scope_name AS VARCHAR) {COND}",
		"CAST(m.scope_version AS VARCHAR) {COND}",
		`EXISTS(
			SELECT 1
			FROM attributes a
			WHERE a.metric_id = m.id AND (
				a.key {COND} OR a.value {COND} OR
				(a.type = 'string[]' AND list_contains(CAST(a.value AS VARCHAR[]), CAST({RAW} AS VARCHAR))) OR
				(a.type = 'int64[]' AND list_contains(CAST(a.value AS BIGINT[]), TRY_CAST({RAW} AS BIGINT))) OR
				(a.type = 'float64[]' AND list_contains(CAST(a.value AS DOUBLE[]), TRY_CAST({RAW} AS DOUBLE))) OR
				(a.type = 'boolean[]' AND list_contains(CAST(a.value AS BOOLEAN[]), TRY_CAST({RAW} AS BOOLEAN)))
			)
		)`,
	}, nil
}
