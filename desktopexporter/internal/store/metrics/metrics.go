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

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

var (
	ErrInvalidMetricQuery           = errors.New("invalid metric search query")
	ErrMetricsStoreInternal         = errors.New("metrics store internal error")
	ErrMetricIDNotFound             = errors.New("metric ID not found")
	ErrDatapointIDNotFound          = errors.New("datapoint ID not found")
	ErrQuantilesNotSupportedForType = errors.New("quantiles are only supported for Histogram and ExponentialHistogram datapoints")
	ErrInvalidQuantileSeriesMode    = errors.New("invalid quantile series mode")
	ErrHistogramBoundsMismatch      = errors.New("merged Histogram has datapoints with mismatched explicit_bounds at the same timestamp")
	ErrInvalidTimeRange             = errors.New("invalid time range: endTs must be greater than startTs")
	ErrInvalidMaxPoints             = errors.New("invalid maxPoints: must be >= 1")
	ErrUnspecifiedTemporality           = errors.New("metric has Unspecified aggregation_temporality; cannot safely aggregate over time")
	ErrBucketSeriesNotSupportedForType = errors.New("bucket series are only supported for Histogram and ExponentialHistogram datapoints")
)

// histogramBoundsMismatchTag is the literal that merged-Histogram SQL
// raises via `error('histogram_bounds_mismatch')` when it detects mixed
// explicit_bounds within a timestamp group. We detect it on the Go side
// with strings.Contains because duckdb-go wraps SQL errors in driver-
// specific types we don't want to import here -- substring match keeps the
// coupling to "the literal we chose", which we own on both sides.
const histogramBoundsMismatchTag = "histogram_bounds_mismatch"

const flushIntervalMetrics = 100

// Ingest writes the metric data in m to the metric_streams,
// metric_ingests, datapoints, exemplars, and attributes tables. The
// caller must hold any required lock on the connection.
//
// Ingest runs in two passes:
//
//  1. First pass collects every distinct (resource, scope, metric)
//     identity in the request and resolves them to metric_streams.id
//     UUIDs via upsertMetricStreams. This is the only round-trip-per-
//     batch step; the appender path that follows is constant per
//     identity.
//  2. Second pass walks the same hierarchy again, this time writing
//     a metric_ingests row per (resource, scope, metric) and the
//     datapoints / exemplars / attributes for each. Datapoints carry
//     both stream_id (the hot lookup key) and metric_ingest_id
//     (provenance back to the originating batch).
//
// The two-pass shape exists so the upsert sees ALL identities at once
// and resolves them in one round-trip; doing the upsert per metric
// would be O(metrics) round-trips per batch.
func Ingest(ctx context.Context, conn driver.Conn, m pmetric.Metrics) (err error) {
	tables := []string{"attributes", "exemplars", "datapoints", "metric_ingests"}

	// Pass 1: collect every distinct identity in this OTLP request, plus
	// per-identity service_name (denormalized onto metric_streams). We
	// build the identity list eagerly so upsertMetricStreams sees the
	// whole batch and can resolve everything in two round trips.
	type metricCoord struct {
		ri, si, mi int
	}
	type identityWithCoord struct {
		identity StreamIdentity
		coord    metricCoord
	}

	var coords []identityWithCoord
	identitySet := make(map[StreamIdentity]struct{})
	for ri, resourceMetric := range m.ResourceMetrics().All() {
		resource := resourceMetric.Resource()
		serviceName := ServiceNameFromAttrs(resource.Attributes())
		for si, scopeMetric := range resourceMetric.ScopeMetrics().All() {
			scope := scopeMetric.Scope()
			for mi, metric := range scopeMetric.Metrics().All() {
				identity := streamIdentityFromMetric(metric, scope.Name(), scope.Version(), serviceName)
				identitySet[identity] = struct{}{}
				coords = append(coords, identityWithCoord{identity: identity, coord: metricCoord{ri, si, mi}})
			}
		}
	}
	if len(coords) == 0 {
		return nil
	}
	identities := make([]StreamIdentity, 0, len(identitySet))
	for id := range identitySet {
		identities = append(identities, id)
	}
	streamIDs, err := upsertMetricStreams(ctx, conn, identities)
	if err != nil {
		return fmt.Errorf("Ingest: %w", err)
	}

	// Pass 2: open the appenders and walk the request again, writing
	// metric_ingests + datapoints + attributes. We resolve each metric's
	// stream_id from streamIDs by re-deriving its identity (cheap; the
	// alternative would be carrying stream IDs alongside coords, but
	// the lookup is O(1) and keeps the inner loop self-contained).
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
		serviceName := ServiceNameFromAttrs(resource.Attributes())
		for _, scopeMetric := range resourceMetric.ScopeMetrics().All() {
			scope := scopeMetric.Scope()
			for _, metric := range scopeMetric.Metrics().All() {
				identity := streamIdentityFromMetric(metric, scope.Name(), scope.Version(), serviceName)
				streamID, ok := streamIDs[identity]
				if !ok {
					return fmt.Errorf("Ingest: %w: stream id missing for identity %+v", ErrMetricsStoreInternal, identity)
				}

				ingestID := duckdb.UUID(uuid.New())

				if err := appenders["metric_ingests"].AppendRow(
					ingestID,                          // ID UUID
					streamID,                          // StreamID UUID
					metric.Description(),              // Description VARCHAR
					resource.DroppedAttributesCount(), // ResourceDroppedAttributesCount UINTEGER
					scope.DroppedAttributesCount(),    // ScopeDroppedAttributesCount UINTEGER
				); err != nil {
					return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
				}

				switch metric.Type() {
				case pmetric.MetricTypeGauge:
					if err := ingestGaugeDatapoints(appenders, streamID, ingestID, metric.Gauge().DataPoints()); err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
					}
				case pmetric.MetricTypeSum:
					if err := ingestSumDatapoints(appenders, streamID, ingestID, metric.Sum().DataPoints()); err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
					}
				case pmetric.MetricTypeHistogram:
					if err := ingestHistogramDatapoints(appenders, streamID, ingestID, metric.Histogram().DataPoints()); err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
					}
				case pmetric.MetricTypeExponentialHistogram:
					if err := ingestExponentialHistogramDatapoints(appenders, streamID, ingestID, metric.ExponentialHistogram().DataPoints()); err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
					}
				}
				ownerIDs := ingest.AttributeOwnerIDs{MetricIngestID: &ingestID}
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

// streamIdentityFromMetric extracts the 8-field identity tuple from one
// metric in an OTLP request. aggregation_temporality and is_monotonic
// are encoded as strings (with empty string meaning "not applicable")
// so the result is comparable as a map key without juggling pointers.
func streamIdentityFromMetric(metric pmetric.Metric, scopeName, scopeVersion, serviceName string) StreamIdentity {
	id := StreamIdentity{
		Name:         metric.Name(),
		Unit:         metric.Unit(),
		MetricType:   metric.Type().String(),
		ScopeName:    scopeName,
		ScopeVersion: scopeVersion,
		ServiceName:  serviceName,
	}
	switch metric.Type() {
	case pmetric.MetricTypeSum:
		id.AggregationTemporality = metric.Sum().AggregationTemporality().String()
		if metric.Sum().IsMonotonic() {
			id.IsMonotonic = "true"
		} else {
			id.IsMonotonic = "false"
		}
	case pmetric.MetricTypeHistogram:
		id.AggregationTemporality = metric.Histogram().AggregationTemporality().String()
	case pmetric.MetricTypeExponentialHistogram:
		id.AggregationTemporality = metric.ExponentialHistogram().AggregationTemporality().String()
	}
	return id
}

// ingestExemplars writes the FilteredAttributes-bearing exemplars for one
// datapoint, plus their attributes. streamID and ingestID propagate from
// the parent metric so an exemplar's attribute row can join back through
// the datapoint to identify its stream cheaply.
func ingestExemplars(appenders map[string]*duckdb.Appender, ingestID, datapointID duckdb.UUID, exemplars pmetric.ExemplarSlice) error {
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
		exOwnerIDs := ingest.AttributeOwnerIDs{MetricIngestID: &ingestID, DataPointID: &datapointID, ExemplarID: &exemplarID}
		if err := ingest.IngestAttributes(appenders["attributes"], []ingest.AttributeBatchItem{
			{Attrs: ex.FilteredAttributes(), IDs: exOwnerIDs, Scope: "exemplar"},
		}); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
	}
	return nil
}

// nullableCanonical returns the canonical "key=value|..." form of an
// attribute set (see ingest.AttrsCanonical) when non-empty, or nil so
// the resulting datapoints.attrs_canonical column is SQL NULL when
// empty. Storing NULL for the empty case matches our query
// convention: "no attributes" is a distinct stream identity from
// "any specific attribute set" and gets coalesced to a sentinel
// empty-string by readers.
func nullableCanonical(attrs pcommon.Map) any {
	if attrs.Len() == 0 {
		return nil
	}
	return ingest.AttrsCanonical(attrs)
}

func ingestGaugeDatapoints(appenders map[string]*duckdb.Appender, streamID, ingestID duckdb.UUID, dps pmetric.NumberDataPointSlice) error {
	for _, dp := range dps.All() {
		doubleVal, intVal, valType := numberDataPointValue(dp)
		datapointID := duckdb.UUID(uuid.New())
		if err := appenders["datapoints"].AppendRow(
			datapointID, streamID, ingestID, "Gauge", int64(dp.Timestamp()), int64(dp.StartTimestamp()), uint32(dp.Flags()),
			doubleVal, intVal, valType, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nullableCanonical(dp.Attributes()),
		); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		dpOwnerIDs := ingest.AttributeOwnerIDs{MetricIngestID: &ingestID, DataPointID: &datapointID}
		if err := ingest.IngestAttributes(appenders["attributes"], []ingest.AttributeBatchItem{
			{Attrs: dp.Attributes(), IDs: dpOwnerIDs, Scope: "datapoint"},
		}); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		if err := ingestExemplars(appenders, ingestID, datapointID, dp.Exemplars()); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
	}
	return nil
}

// Sum/Histogram/ExpHistogram all share the same datapoint-iteration shape
// now that aggregation_temporality and is_monotonic are stored on
// metric_streams (one place per stream) instead of being copied to every
// datapoint. The per-type functions just differ in which datapoint
// columns they populate.

func ingestSumDatapoints(appenders map[string]*duckdb.Appender, streamID, ingestID duckdb.UUID, dps pmetric.NumberDataPointSlice) error {
	for _, dp := range dps.All() {
		doubleVal, intVal, valType := numberDataPointValue(dp)
		datapointID := duckdb.UUID(uuid.New())
		if err := appenders["datapoints"].AppendRow(
			datapointID, streamID, ingestID, "Sum", int64(dp.Timestamp()), int64(dp.StartTimestamp()), uint32(dp.Flags()),
			doubleVal, intVal, valType,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nullableCanonical(dp.Attributes()),
		); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		dpOwnerIDs := ingest.AttributeOwnerIDs{MetricIngestID: &ingestID, DataPointID: &datapointID}
		if err := ingest.IngestAttributes(appenders["attributes"], []ingest.AttributeBatchItem{
			{Attrs: dp.Attributes(), IDs: dpOwnerIDs, Scope: "datapoint"},
		}); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		if err := ingestExemplars(appenders, ingestID, datapointID, dp.Exemplars()); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
	}
	return nil
}

func ingestHistogramDatapoints(appenders map[string]*duckdb.Appender, streamID, ingestID duckdb.UUID, dps pmetric.HistogramDataPointSlice) error {
	for _, dp := range dps.All() {
		datapointID := duckdb.UUID(uuid.New())
		if err := appenders["datapoints"].AppendRow(
			datapointID, streamID, ingestID, "Histogram", int64(dp.Timestamp()), int64(dp.StartTimestamp()), uint32(dp.Flags()),
			nil, nil, nil,
			dp.Count(), dp.Sum(), dp.Min(), dp.Max(), dp.BucketCounts().AsRaw(), dp.ExplicitBounds().AsRaw(),
			nil, nil, nil, nil, nil, nil, nil,
			nullableCanonical(dp.Attributes()),
		); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		dpOwnerIDs := ingest.AttributeOwnerIDs{MetricIngestID: &ingestID, DataPointID: &datapointID}
		if err := ingest.IngestAttributes(appenders["attributes"], []ingest.AttributeBatchItem{
			{Attrs: dp.Attributes(), IDs: dpOwnerIDs, Scope: "datapoint"},
		}); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		if err := ingestExemplars(appenders, ingestID, datapointID, dp.Exemplars()); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
	}
	return nil
}

func ingestExponentialHistogramDatapoints(appenders map[string]*duckdb.Appender, streamID, ingestID duckdb.UUID, dps pmetric.ExponentialHistogramDataPointSlice) error {
	for _, dp := range dps.All() {
		pos, neg := dp.Positive(), dp.Negative()
		datapointID := duckdb.UUID(uuid.New())
		if err := appenders["datapoints"].AppendRow(
			datapointID, streamID, ingestID, "ExponentialHistogram", int64(dp.Timestamp()), int64(dp.StartTimestamp()), uint32(dp.Flags()),
			nil, nil, nil,
			dp.Count(), dp.Sum(), dp.Min(), dp.Max(), nil, nil,
			dp.Scale(), dp.ZeroCount(), dp.ZeroThreshold(), pos.Offset(), pos.BucketCounts().AsRaw(), neg.Offset(), neg.BucketCounts().AsRaw(),
			nullableCanonical(dp.Attributes()),
		); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		dpOwnerIDs := ingest.AttributeOwnerIDs{MetricIngestID: &ingestID, DataPointID: &datapointID}
		if err := ingest.IngestAttributes(appenders["attributes"], []ingest.AttributeBatchItem{
			{Attrs: dp.Attributes(), IDs: dpOwnerIDs, Scope: "datapoint"},
		}); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		if err := ingestExemplars(appenders, ingestID, datapointID, dp.Exemplars()); err != nil {
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

// Search returns metrics that have at least one datapoint in [startTime,
// endTime], matching the optional criteria. Each result is one
// metric_ingests row joined to its metric_streams row, projected as the
// "metric" shape the frontend expects (the union of identity-from-stream
// and per-batch fields like description / dropped counts).
//
// The per-row granularity is preserved: a long-lived counter that's
// reported every batch still produces one Search result per batch, just
// like before. Identity columns (name, unit, ...) are read from
// metric_streams via the join; the rest comes from metric_ingests.
//
// Ordering is by latest datapoint timestamp per ingest (newest data
// first). This is the source's notion of recency -- when a datapoint
// was actually observed -- rather than the collector's wall clock at
// batch arrival.
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
	// filtered_ingests is the new per-batch row source. Field mapper
	// expressions written against alias "m" still work because the join
	// resolves identity columns through "s". WHERE predicates on the old
	// identity columns (m.name, m.unit, ...) are rewritten by the field
	// mapper to s.name, s.unit, ... -- see metricFieldMapper.
	finalQuery := fmt.Sprintf(`%s,
		filtered_ingests as (
			select m.*, s.name, s.unit, s.metric_type, s.aggregation_temporality, s.is_monotonic,
				s.scope_name, s.scope_version, s.service_name
			from metric_ingests m
			inner join metric_streams s on s.id = m.stream_id, search_params
			where %s
		),
		-- Datapoints inherit aggregation_temporality / is_monotonic from
		-- the stream (single source of truth per the OTel data model), so
		-- the per-type JSON projection below references fi.<field> rather
		-- than re-joining metric_streams.
		filtered_dps as (
			select d.*,
				fi.aggregation_temporality as aggregation_temporality,
				fi.is_monotonic as is_monotonic
			from datapoints d
			inner join filtered_ingests fi on d.metric_ingest_id = fi.id, search_params
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
		ingest_res_attrs as (
			select a.metric_ingest_id, json_group_array(json_object('key', a.key, 'value', a.value, 'type', a.type::varchar)) as attrs
			from attributes a
			where a.metric_ingest_id in (select id from filtered_ingests) and a.scope = 'resource' and a.datapoint_id is null and a.exemplar_id is null
			group by a.metric_ingest_id
		),
		ingest_scope_attrs as (
			select a.metric_ingest_id, json_group_array(json_object('key', a.key, 'value', a.value, 'type', a.type::varchar)) as attrs
			from attributes a
			where a.metric_ingest_id in (select id from filtered_ingests) and a.scope = 'scope' and a.datapoint_id is null and a.exemplar_id is null
			group by a.metric_ingest_id
		),
		-- One row per (ingest, attribute-set). Same lift-attributes-out
		-- pattern as GetMetric, but here we keep the extra
		-- metric_ingest_id grouping because Search returns per-ingest
		-- result rows. attribute_sample picks any one dp's attributes
		-- from the group -- they're identical by the grouping criterion.
		ts_dps_agg as (
			select
				d.metric_ingest_id,
				coalesce(d.attrs_canonical, '') as attrs_key,
				any_value(coalesce((select attrs from dp_attrs_agg where dp_attrs_agg.datapoint_id = d.id), json('[]'))) as attributes_sample,
				max(d.timestamp) as latest_ts,
				to_json(list(json_merge_patch(
					json_object(
						'id', d.id,
						'metricType', d.metric_type,
						'timestamp', d.timestamp::varchar,
						'startTime', d.start_time::varchar,
						'flags', d.flags,
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
			group by d.metric_ingest_id, coalesce(d.attrs_canonical, '')
		),
		-- Roll the per-(ingest, timeseries) rows up into one
		-- timeseries[] per ingest. Timeseries within an ingest are
		-- ordered by latest dp timestamp desc -- mirrors GetMetric and
		-- gives the search-results UI a "newest activity first" feel
		-- by default.
		timeseries_agg as (
			select metric_ingest_id, to_json(list(json_object(
				'attributesKey', attrs_key,
				'attributes', attributes_sample,
				'datapoints', datapoints
			) order by latest_ts desc)) as timeseries
			from ts_dps_agg
			group by metric_ingest_id
		),
		-- Latest datapoint timestamp per ingest, used to order the
		-- top-level results "newest data first." We aggregate the
		-- per-(ingest, timeseries) latest_ts values rather than re-
		-- scanning filtered_dps -- cheaper and gives the same answer.
		ingest_latest_dp as (
			select metric_ingest_id, max(latest_ts) as last_dp_ts
			from ts_dps_agg
			group by metric_ingest_id
		)
		-- "id" exposed to the frontend is the STREAM id, not the per-
		-- batch ingest id. Any client-side action that wants to fan out
		-- across all batches of a metric (quantile-series, bucket-series,
		-- delete) keys off this. The actual ingest UUID stays available
		-- as ingestId for callers that care about provenance.
		select cast(coalesce(to_json(list(json_object(
			'id', fi.stream_id, 'ingestId', fi.id, 'name', fi.name, 'description', fi.description, 'unit', fi.unit,
			'resourceDroppedAttributesCount', fi.resource_dropped_attributes_count,
			'resource', json_object('attributes', coalesce(res.attrs, json('[]')), 'droppedAttributesCount', fi.resource_dropped_attributes_count),
			'scopeName', fi.scope_name, 'scopeVersion', fi.scope_version, 'scopeDroppedAttributesCount', fi.scope_dropped_attributes_count,
			'scope', json_object('name', fi.scope_name, 'version', fi.scope_version, 'attributes', coalesce(scope_attrs.attrs, json('[]')), 'droppedAttributesCount', fi.scope_dropped_attributes_count),
			'timeseries', coalesce(ts.timeseries, json('[]'))
		) order by ild.last_dp_ts desc nulls last)), '[]') as varchar) as metrics
		from filtered_ingests fi
		left join ingest_res_attrs res on res.metric_ingest_id = fi.id
		left join ingest_scope_attrs scope_attrs on scope_attrs.metric_ingest_id = fi.id
		left join timeseries_agg ts on ts.metric_ingest_id = fi.id
		left join ingest_latest_dp ild on ild.metric_ingest_id = fi.id`,
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

// SearchSummaries returns lightweight per-stream summaries for the drawer
// cards: identity fields, description, seriesCount, lastValue (Gauge/Sum),
// and lastSeen.
// One row per metric_streams row with at least one in-range datapoint.
func SearchSummaries(ctx context.Context, db *sql.DB, startTime, endTime int64) (json.RawMessage, error) {
	query := `
		with search_params as (select ? as time_start, ? as time_end),
		filtered_streams as (
			select s.* from metric_streams s, search_params
			where exists (
				select 1 from datapoints d
				where d.stream_id = s.id
				  and d.timestamp >= time_start and d.timestamp <= time_end
			)
		),
		filtered_dps as (
			select d.* from datapoints d
			inner join filtered_streams fs on d.stream_id = fs.id, search_params
			where d.timestamp >= time_start and d.timestamp <= time_end
		),
		stream_latest_dp as (
			select stream_id, max(timestamp) as last_dp_ts
			from filtered_dps
			group by stream_id
		),
		ingest_latest_dp as (
			select metric_ingest_id, max(timestamp) as last_dp_ts
			from filtered_dps
			group by metric_ingest_id
		),
		stream_description as (
			select mi.stream_id,
				arg_max(mi.description, ild.last_dp_ts) as description
			from metric_ingests mi
			inner join ingest_latest_dp ild on ild.metric_ingest_id = mi.id
			where mi.stream_id in (select id from filtered_streams)
			group by mi.stream_id
		),
		stream_series_count as (
			select stream_id, count(distinct coalesce(attrs_canonical, '')) as series_count
			from filtered_dps
			group by stream_id
		),
		stream_last_value as (
			select
				d.stream_id,
				arg_max(coalesce(d.double_value, d.int_value), d.timestamp) as last_value
			from filtered_dps d
			where d.metric_type in ('Gauge', 'Sum')
			group by d.stream_id
		)
		select cast(coalesce(to_json(list(json_object(
			'id', cast(fs.id as varchar),
			'name', fs.name,
			'description', sd.description,
			'unit', fs.unit,
			'metricType', fs.metric_type,
			'aggregationTemporality', fs.aggregation_temporality,
			'isMonotonic', case
				when fs.metric_type = 'Sum' then fs.is_monotonic
				else null
			end,
			'serviceName', fs.service_name,
			'seriesCount', ssc.series_count,
			'lastValue', slv.last_value,
			'lastSeen', sldp.last_dp_ts::varchar
		) order by sldp.last_dp_ts desc nulls last)), '[]') as varchar) as summaries
		from filtered_streams fs
		left join stream_latest_dp sldp on sldp.stream_id = fs.id
		left join stream_description sd on sd.stream_id = fs.id
		left join stream_series_count ssc on ssc.stream_id = fs.id
		left join stream_last_value slv on slv.stream_id = fs.id
	`
	var raw []byte
	if err := db.QueryRowContext(ctx, query, startTime, endTime).Scan(&raw); err != nil {
		return nil, fmt.Errorf("SearchSummaries: %w: %w", ErrMetricsStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// GetMetric returns full MetricData for a metric stream in the time window.
func GetMetric(ctx context.Context, db *sql.DB, streamID string, startTime, endTime int64) (json.RawMessage, error) {
	// Everything filters by stream_id.
	// matched_ingests is "ingests for this stream that produced at least
	// one datapoint in the time window." All identity columns the JSON
	// projection needs come from the metric_streams row directly via
	// the stream CTE.
	query := `
		with input as (
			select ?::uuid as stream_id,
				?::bigint as time_start,
				?::bigint as time_end
		),
		stream as (
			select s.* from metric_streams s, input
			where s.id = input.stream_id
		),
		matched_ingests as (
			select m.* from metric_ingests m, input
			where m.stream_id = input.stream_id
			  and exists (
				select 1 from datapoints d
				where d.metric_ingest_id = m.id
				  and d.timestamp >= input.time_start and d.timestamp <= input.time_end
			  )
		),
		-- Datapoints inherit aggregation_temporality / is_monotonic from
		-- the stream so the per-type JSON projection below doesn't need
		-- a per-row join.
		filtered_dps as (
			select d.*,
				s.aggregation_temporality as aggregation_temporality,
				s.is_monotonic as is_monotonic
			from datapoints d, input, stream s
			where d.stream_id = input.stream_id
			  and d.timestamp >= input.time_start and d.timestamp <= input.time_end
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
		-- Per-ingest latest datapoint timestamp over the queried window
		-- -- the recency proxy we use to pick a "representative" ingest
		-- for description / dropped counts. These per-batch fields can
		-- drift across ingests; we prefer the most recently-observed
		-- sender's view (newest data, not newest wall-clock arrival).
		ingest_latest_dp as (
			select metric_ingest_id, max(timestamp) as last_dp_ts
			from filtered_dps
			group by metric_ingest_id
		),
		-- Most recent matched ingest is the source of variable-but-
		-- non-identifying fields (description, dropped counts).
		representative as (
			select mi.* from matched_ingests mi
			inner join ingest_latest_dp ild on ild.metric_ingest_id = mi.id
			order by ild.last_dp_ts desc nulls last
			limit 1
		),
		ingest_res_attrs as (
			select json_group_array(json_object('key', a.key, 'value', a.value, 'type', a.type::varchar)) as attrs
			from attributes a
			where a.metric_ingest_id in (select id from matched_ingests)
			  and a.scope = 'resource'
			  and a.datapoint_id is null
			  and a.exemplar_id is null
		),
		ingest_scope_attrs as (
			select json_group_array(json_object('key', a.key, 'value', a.value, 'type', a.type::varchar)) as attrs
			from attributes a
			where a.metric_ingest_id in (select id from matched_ingests)
			  and a.scope = 'scope'
			  and a.datapoint_id is null
			  and a.exemplar_id is null
		),
		-- One row per (metric, attribute-set) -- i.e. per OTel stream.
		-- The attribute set itself is owned by the stream (lifted out of
		-- the per-dp objects), and the dp objects inside are pure OTLP
		-- measurement payloads: timestamp, type-specific value fields,
		-- exemplars, flags. attrs_canonical is the grouping key; we
		-- coalesce NULL (no-attrs case) to "" so all attribute-less
		-- points collapse into one timeseries rather than scattering.
		--
		-- attributes_sample picks any one datapoint's attributes from
		-- this timeseries. Within a timeseries they're identical by
		-- definition (it's the grouping criterion), so any() / first() /
		-- arg_max all yield the same answer; we use any_value for clarity.
		ts_dps_agg as (
			select
				coalesce(d.attrs_canonical, '') as attrs_key,
				any_value(coalesce((select attrs from dp_attrs_agg where dp_attrs_agg.datapoint_id = d.id), json('[]'))) as attributes_sample,
				max(d.timestamp) as latest_ts,
				to_json(list(json_merge_patch(
					json_object(
						'id', d.id,
						'metricType', d.metric_type,
						'timestamp', d.timestamp::varchar,
						'startTime', d.start_time::varchar,
						'flags', d.flags,
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
			group by coalesce(d.attrs_canonical, '')
		),
		-- Pack each timeseries into the wire shape and order them so
		-- the most recently active timeseries sorts first -- mirrors
		-- the "newest first" feel of the old flat datapoint list,
		-- which is what the detail panel's legend reads top-down.
		-- Empty list (no dps in window) collapses to '[]' via the
		-- outer coalesce.
		timeseries_agg as (
			select to_json(list(json_object(
				'attributesKey', attrs_key,
				'attributes', attributes_sample,
				'datapoints', datapoints
			) order by latest_ts desc)) as timeseries
			from ts_dps_agg
		)
		select cast(json_object(
			'id', s.id, 'name', s.name, 'description', r.description, 'unit', s.unit,
			'resourceDroppedAttributesCount', r.resource_dropped_attributes_count,
			'resource', json_object(
				'attributes', coalesce((select attrs from ingest_res_attrs), json('[]')),
				'droppedAttributesCount', r.resource_dropped_attributes_count
			),
			'scopeName', s.scope_name, 'scopeVersion', s.scope_version,
			'scopeDroppedAttributesCount', r.scope_dropped_attributes_count,
			'scope', json_object(
				'name', s.scope_name, 'version', s.scope_version,
				'attributes', coalesce((select attrs from ingest_scope_attrs), json('[]')),
				'droppedAttributesCount', r.scope_dropped_attributes_count
			),
			'timeseries', coalesce((select timeseries from timeseries_agg), json('[]'))
		) as varchar) as metric
		from representative r, stream s
	`
	var raw []byte
	if err := db.QueryRowContext(ctx, query, streamID, startTime, endTime).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return json.RawMessage("null"), nil
		}
		return nil, fmt.Errorf("GetMetric: %w: %w", ErrMetricsStoreInternal, err)
	}
	if raw == nil || string(raw) == "null" {
		return json.RawMessage("null"), nil
	}
	return json.RawMessage(raw), nil
}

// resolveStreamID maps an 8-field OTel metric identity to its
// metric_streams.id. Returns sql.ErrNoRows if no stream matches; callers
// translate that into a "not found" response (e.g. JSON null at the
// JSON-RPC layer). Empty strings on the nullable identity fields
// (aggregation_temporality, is_monotonic) match NULL columns via
// `is not distinct from`.
//
// Returned id is a string in canonical UUID form so we can pass it back
// into subsequent queries via the same `?::uuid` casting used everywhere
// else; we deliberately don't return a duckdb.UUID because callers (the
// JSON-RPC layer) and downstream queries both prefer the string shape.
// resolveStreamID looks up metric_streams.id for the 8-field identity
// tuple supplied by the JSON-RPC layer. All identity columns in
// metric_streams are NOT NULL (with empty-string / false defaults
// representing "not applicable"), so plain equality is safe -- callers
// pass the same not-applicable convention StreamIdentity uses
// internally. is_monotonic is the only field that needs translation:
// the wire form is the string "true"/"false"/"" while the column is
// boolean, with metric types that don't carry monotonicity (everything
// other than Sum) stored as the false default.
func resolveStreamID(ctx context.Context, db *sql.DB, name, unit, metricType, aggregationTemporality, isMonotonic, scopeName, scopeVersion, serviceName string) (string, error) {
	const q = `
		select id::varchar from metric_streams
		where name = ?
		  and unit = ?
		  and metric_type = ?
		  and aggregation_temporality = ?
		  and is_monotonic = ?
		  and scope_name = ?
		  and scope_version = ?
		  and service_name = ?
		limit 1
	`
	var id string
	err := db.QueryRowContext(ctx, q,
		name, unit, metricType, aggregationTemporality, isMonotonic == "true",
		scopeName, scopeVersion, serviceName,
	).Scan(&id)
	return id, err
}

// GetMetricAttributes returns a JSON array of attribute names/scopes/types
// for metrics that have at least one datapoint in the given time range.
// Uses the renamed metric_ingest_id column on attributes; the existence
// check lives on metric_ingests (the per-batch table) since we want
// attributes from any batch whose datapoints land in the window.
func GetMetricAttributes(ctx context.Context, db *sql.DB, startTime, endTime int64) (json.RawMessage, error) {
	query := `
		select cast(to_json(list(json_object('name', sub.key, 'attributeScope', sub.scope, 'type', sub.type::varchar)
			order by sub.key, sub.scope)) as varchar) as attributes
		from (
			select distinct a.key, a.scope, a.type
			from attributes a
			inner join metric_ingests m on a.metric_ingest_id = m.id
			where a.datapoint_id is null and a.exemplar_id is null
			  and exists (
				select 1 from datapoints d
				where d.metric_ingest_id = m.id
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
// `(timeseries, bucket_start)` (per-attribute) or per `bucket_start`
// (merged). In effect maxPoints is the chart's pixel width and the
// result has at most that many output rows. The window is
// `[startTs, endTs)` (start inclusive, end exclusive).
//
// Within-bucket time merging dispatches on the metric's
// aggregation_temporality:
//   - Delta: bucket counts sum across time within each (timeseries, bucket).
//   - Cumulative: take the latest sample per (timeseries, bucket) since
//     each row is already a running total -- summing would double-count.
//
// Unspecified temporality is rejected (ErrUnspecifiedTemporality) so we
// never silently mis-aggregate.
//
// Mode controls cross-timeseries behavior:
//   - "per-attribute": one entry per (bucket_start, attribute set).
//   - "merged": one entry per bucket_start with all timeseries merged
//     via the alignment pipeline. Histogram needs uniform
//     explicit_bounds within each bucket; ExpHistogram realigns scales.
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
	if mode != "per-attribute" && mode != "merged" {
		return nil, fmt.Errorf("GetMetricQuantileSeries: %w: %q (want per-attribute or merged)", ErrInvalidQuantileSeriesMode, mode)
	}

	// Pre-check: confirm the stream exists and read its metric_type +
	// aggregation_temporality. Both live on metric_streams now (one row
	// per logical metric), so this is a simple lookup -- we no longer
	// need to join through datapoints to discriminate metric_type.
	// Gauge streams carry NULL aggregation_temporality so we scan it as
	// NullString and only validate it for the histogram types we
	// actually support.
	var metricType string
	var temporality sql.NullString
	err := db.QueryRowContext(ctx,
		`select metric_type, aggregation_temporality
		   from metric_streams
		  where id = ?::uuid`,
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
	if mode == "per-attribute" {
		return getPerAttributeQuantileSeries(ctx, db, metricID, quantiles, temporality.String, startTs, endTs, maxPoints)
	}
	switch metricType {
	case "Histogram":
		return getMergedHistogramQuantileSeries(ctx, db, metricID, quantiles, temporality.String, startTs, endTs, maxPoints)
	case "ExponentialHistogram":
		return getMergedExpHistogramQuantileSeries(ctx, db, metricID, quantiles, temporality.String, startTs, endTs, maxPoints)
	}
	return nil, fmt.Errorf("GetMetricQuantileSeries: %w: %s", ErrQuantilesNotSupportedForType, metricType)
}

// quantileSeriesCTEs is the shared CTE preamble used by every quantile
// series variant. It defines:
//
//   - params: bind-parameter window (start_ts, end_ts) and computed
//     bucket_ns (clamped to >= 1 ms).
//   - dp_attrs_proj: per-datapoint attribute array (only). The "stream
//     within a stream" key used to be a string_agg over attribute rows;
//     it's now the precomputed datapoints.attrs_canonical, so we
//     only build the human-readable attrs JSON here.
//   - bucketed: filtered (by stream_id and time window) datapoints with
//     attrs JSON attached + bucket_start.
//   - time_merged: per (bucket_start, attrs_canonical) row, with
//     within-bucket time merging dispatched on temporality. Delta sums
//     bucket vectors across time; Cumulative takes the latest sample
//     (each cumulative row is a running total, summing would
//     double-count).
//
// Variants downstream (per-attribute final select, merged Histogram
// alignment, merged ExpHistogram alignment) all start from time_merged.
// Returns the SQL fragment ending after the time_merged CTE definition
// (with a trailing comma so callers can append more CTEs cleanly), plus
// the args slice for the 7 placeholders consumed here:
// (start_ts, end_ts, end_ts, start_ts, stream_id, start_ts, max_points,
// stream_id, stream_id).
//
// metricID is interpreted as a metric_streams.id; that's the new "hot
// key" for cross-batch metric queries.
func quantileSeriesCTEs(metricID string, startTs, endTs int64, maxPoints int, temporality string) (string, []any) {
	const cteSharedHead = `
		with params as (
			select
				cast(? as bigint) as start_ts,
				cast(? as bigint) as end_ts,
				greatest(1000000::bigint,
					(cast(? as bigint) - greatest(cast(? as bigint),
						coalesce((select min(timestamp) from datapoints where stream_id = ?::uuid), cast(? as bigint))
					)) // cast(? as bigint)
				) as bucket_ns
		),
		dp_attrs_proj as (
			select
				a.datapoint_id,
				to_json(list(
					json_object('key', a.key, 'value', a.value, 'type', a.type::varchar)
					order by a.key
				)) as attrs
			from attributes a
			where a.datapoint_id in (select id from datapoints where stream_id = ?::uuid)
			  and a.scope = 'datapoint'
			group by a.datapoint_id
		),
		bucketed as (
			select
				d.id, d.metric_type, d.timestamp,
				-- attrs_key is the stable identifier the frontend uses to
				-- group / dedup per-attribute streams within one logical
				-- metric. We expose the precomputed attrs_canonical
				-- column directly: it's already the
				-- "key=value|key=value|..." form (sorted ascending) the
				-- frontend computes locally for raw datapoints, so the
				-- two paths share one identity encoding without any
				-- translation. NULL (datapoint with no attrs) becomes
				-- the empty string -- preserving the historical "" convention
				-- so existing tests and frontend merged-mode comparisons
				-- keep working.
				coalesce(d.attrs_canonical, '') as attrs_key,
				da.attrs,
				d.bucket_counts, d.explicit_bounds,
				d.scale, d.zero_count, d.zero_threshold,
				d.positive_bucket_offset, d.positive_bucket_counts,
				d.negative_bucket_offset, d.negative_bucket_counts,
				d.count, d.sum, d.min, d.max,
				(d.timestamp // p.bucket_ns) * p.bucket_ns as bucket_start
			from datapoints d
			left join dp_attrs_proj da on da.datapoint_id = d.id
			cross join params p
			where d.stream_id = ?::uuid
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

// getPerAttributeQuantileSeries renders one entry per (bucket_start, attrs_key)
// after the shared bucketing + time-merge pipeline. Quantile dispatch is by
// metric_type (Histogram vs ExponentialHistogram) inside the json_object.
// The macros may return NULL for a quantile (e.g. empty buckets); that
// surfaces as JSON null.
func getPerAttributeQuantileSeries(ctx context.Context, db *sql.DB, metricID string, quantiles []float64, temporality string, startTs, endTs int64, maxPoints int) (json.RawMessage, error) {
	cteSQL, cteArgs := quantileSeriesCTEs(metricID, startTs, endTs, maxPoints, temporality)
	pairsSQL, quantileArgs := buildPerAttributeQuantilePairs(quantiles)
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

// getMergedHistogramQuantileSeries merges all per-attribute timeseries
// of a Histogram metric per timestamp and returns one entry per
// timestamp with quantiles computed over the merged bucket vector.
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
func getMergedHistogramQuantileSeries(ctx context.Context, db *sql.DB, metricID string, quantiles []float64, temporality string, startTs, endTs int64, maxPoints int) (json.RawMessage, error) {
	cteSQL, cteArgs := quantileSeriesCTEs(metricID, startTs, endTs, maxPoints, temporality)
	pairsSQL, quantileArgs := buildMergedHistogramQuantilePairs(quantiles)
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

// buildMergedHistogramQuantilePairs renders the per-quantile pairs for
// the merged-Histogram select. Unlike the per-attribute variant, no
// metric_type CASE is needed -- the helper is only called after we've
// confirmed the metric is a Histogram, so every row in `merged` has the
// same shape. Each quantile contributes one `?` placeholder. Columns come
// from the `merged m` CTE row.
func buildMergedHistogramQuantilePairs(quantiles []float64) (string, []any) {
	pairs := make([]string, 0, len(quantiles))
	args := make([]any, 0, len(quantiles))
	for _, q := range quantiles {
		key := strconv.FormatFloat(q, 'f', -1, 64)
		pairs = append(pairs, fmt.Sprintf(`'%s', hist_quantile(m.bounds, m.merged_counts, ?)`, key))
		args = append(args, q)
	}
	return strings.Join(pairs, ", "), args
}

// getMergedExpHistogramQuantileSeries merges all per-attribute
// timeseries of an ExponentialHistogram per bucket using the full
// alignment pipeline: downscale every timeseries to the bucket's
// minimum scale, left-pad to the bucket's minimum (post-downscale)
// offset, sum the bucket vectors, then fold buckets at or below the
// merged zero_threshold's cutoff index back into zero_count. Positive
// and negative sides run independently with the same shared cutoff
// (since |bucket k|'s magnitude is symmetric).
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
func getMergedExpHistogramQuantileSeries(ctx context.Context, db *sql.DB, metricID string, quantiles []float64, temporality string, startTs, endTs int64, maxPoints int) (json.RawMessage, error) {
	cteSQL, cteArgs := quantileSeriesCTEs(metricID, startTs, endTs, maxPoints, temporality)
	pairsSQL, quantileArgs := buildMergedExpHistogramQuantilePairs(quantiles)
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

// buildMergedExpHistogramQuantilePairs renders the per-quantile pairs
// for the merged-ExpHistogram select. Each quantile contributes one `?`
// placeholder; columns come from the `final m` CTE row.
func buildMergedExpHistogramQuantilePairs(quantiles []float64) (string, []any) {
	pairs := make([]string, 0, len(quantiles))
	args := make([]any, 0, len(quantiles))
	for _, q := range quantiles {
		key := strconv.FormatFloat(q, 'f', -1, 64)
		pairs = append(pairs, fmt.Sprintf(`'%s', exp_hist_quantile(m.target_scale, m.neg_offset, m.neg_counts, m.final_zero_count, m.pos_offset, m.pos_counts, ?)`, key))
		args = append(args, q)
	}
	return strings.Join(pairs, ", "), args
}

// buildPerAttributeQuantilePairs renders the comma-separated `'<key>', case ...`
// pairs that go inside json_object(...) for per-attribute quantile selection.
// Each quantile contributes two `?` placeholders (one for hist_quantile, one
// for exp_hist_quantile), so the returned args slice is len(quantiles) * 2
// long and must be appended to whatever trailing args the caller has.
//
// Mirrors the dispatch pattern in GetDatapointQuantiles: case on metric_type,
// route Histogram to hist_quantile and ExponentialHistogram to
// exp_hist_quantile. Anything else falls through to NULL (callers gate this
// at the metric_type pre-check, so it shouldn't be reachable in practice).
// Columns come from the `time_merged m` CTE row.
func buildPerAttributeQuantilePairs(quantiles []float64) (string, []any) {
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

// GetMetricMergedQuantiles computes a single set of quantile values for
// a Histogram or ExponentialHistogram metric over the entire [startTs,
// endTs) window, with all per-attribute timeseries merged. Returns one
// `{q -> value}` JSON object, mirroring GetDatapointQuantiles' return
// shape so the frontend can share rendering logic between "snapshot of
// one datapoint" and "merged across the whole metric."
//
// Approach: reuse GetMetricQuantileSeries(mode='merged', maxPoints=1).
// That path's bucket-width math (`(endTs - startTs) // maxPoints`)
// reduces to "one bucket spanning the whole window" when maxPoints=1,
// so the existing time_merged + cross-timeseries merge pipeline
// produces exactly one output row whose `quantiles` field is the
// answer. The mathematical guarantee we need ("same quantiles as the
// per-time-bucket series, just computed once over the full window")
// falls out for free because we go through the same SQL.
//
// Trade-off: we go through one round of JSON encode/decode to unwrap the
// single-element list. Negligible vs. running the full pipeline.
//
// Returns:
//   - ErrInvalidTimeRange if endTs <= startTs.
//   - ErrMetricIDNotFound if the metric has no datapoints at all.
//   - ErrQuantilesNotSupportedForType for Gauge/Sum.
//   - ErrUnspecifiedTemporality if aggregation_temporality is Unspecified.
//   - ErrHistogramBoundsMismatch if the Histogram has mixed bounds across
//     datapoints (per spec, you can't merge mismatched bucket vectors).
//
// An empty quantile list returns "{}" without touching the database. A
// window with no datapoints (but metric exists) also returns "{}" -- the
// inner series query returns an empty list and we surface it as "no
// quantiles to compute."
func GetMetricMergedQuantiles(ctx context.Context, db *sql.DB, metricID string, quantiles []float64, startTs, endTs int64) (json.RawMessage, error) {
	if len(quantiles) == 0 {
		return json.RawMessage("{}"), nil
	}
	if endTs <= startTs {
		return nil, fmt.Errorf("GetMetricMergedQuantiles: %w: startTs=%d endTs=%d", ErrInvalidTimeRange, startTs, endTs)
	}

	// Delegate to the series query with maxPoints=1: one bucket covering
	// the whole window means time_merged sees every dp as belonging to the
	// same bucket_start, then the cross-timeseries merge collapses to a
	// single output row. All the temporality/type/bounds-mismatch error
	// paths come along automatically because they live in the same code
	// path.
	seriesRaw, err := GetMetricQuantileSeries(ctx, db, metricID, quantiles, "merged", startTs, endTs, 1)
	if err != nil {
		// Re-wrap so the caller sees this function's name in the error
		// chain. errors.Is(err, ErrFoo) still works because we wrap with %w.
		return nil, fmt.Errorf("GetMetricMergedQuantiles: %w", err)
	}

	// seriesRaw is a JSON array of either zero or one element. We need to
	// pull out just the `quantiles` field. Decoding to a typed struct
	// keeps the unwrap honest -- if the inner shape changes we'll get a
	// compile-style failure at decode time.
	var series []struct {
		Quantiles json.RawMessage `json:"quantiles"`
	}
	if err := json.Unmarshal(seriesRaw, &series); err != nil {
		return nil, fmt.Errorf("GetMetricMergedQuantiles: %w: decode series: %w", ErrMetricsStoreInternal, err)
	}
	if len(series) == 0 {
		return json.RawMessage("{}"), nil
	}
	return series[0].Quantiles, nil
}

// GetMetricSummary returns derived summary statistics for a metric over a
// time window. The shape is uniform across kinds: a top-level object
// with `kind` (and `isMonotonic` for Sum) plus a `timeseries` array.
// Each timeseries entry carries its own per-attribute window-level
// aggregates, so the frontend can match them up with the chart legend
// by `attributesKey` instead of having to re-aggregate data it already
// rendered:
//
//   - Gauge        → { kind, timeseries: [{ attributesKey, current, min, max, lastReceived }, ...] }
//   - Sum          → { kind, isMonotonic, timeseries: [{ attributesKey, current, delta, min, max, lastReceived }, ...] }
//   - Histogram    → { kind, timeseries: [{ attributesKey: "__merged__", count, sum, min, max, quantiles, lastReceived }] }
//   - ExpHistogram → { kind, timeseries: [{ attributesKey: "__merged__", count, sum, min, max, quantiles, lastReceived }] }
//
// Why per-timeseries for Gauge/Sum: aggregating `current`/`min`/`max`
// across per-attribute timeseries of a Gauge or Sum is meaningless --
// "current" would just pick whichever attribute combination reported
// last, and "min/max" would mix unrelated time series. We hand the
// frontend the per-timeseries rows and let it display whatever the
// user's legend selection is asking for.
//
// Why merged for Histograms: bucket vectors merge cleanly across
// timeseries, so a single "merged distribution" summary genuinely
// answers "what's the overall p99 in this window across all
// attributes." Per-timeseries histogram summaries aren't produced
// today; the heatmap UI has no single-timeseries-selection mode yet,
// so there's no place to render them.
//
// For Sum, `delta` is reset-aware on Cumulative timeseries (see
// `getSumSummary` for the run-detection logic) and a plain `sum(value)`
// per timeseries on Delta timeseries. `current`/`min`/`max` are
// observed values, not increments, so they don't need reset adjustment.
//
// `isMonotonic` is a metric-level Sum property, so it lives at the top
// of the response (not on each timeseries). Gauge and Histogram omit it.
//
// `lastReceived` is the timestamp of each timeseries' most recent
// datapoint in the window, in nanoseconds. A timeseries with no
// datapoints in the window is simply omitted from `timeseries`; the
// empty array is the "no data in window" signal.
//
// Returns:
//   - ErrInvalidTimeRange if endTs <= startTs.
//   - ErrMetricIDNotFound if the metric stream doesn't exist.
//   - ErrUnspecifiedTemporality (Histogram/ExpHist/Sum with Unspecified).
//   - ErrHistogramBoundsMismatch (Histogram with mixed bounds).
//   - Empty window (metric exists, no datapoints in range): returns the
//     uniform shape with `timeseries: []`. The frontend renders "no data
//     in window" instead of fabricated zeros.
func GetMetricSummary(ctx context.Context, db *sql.DB, metricID string, startTs, endTs int64) (json.RawMessage, error) {
	if endTs <= startTs {
		return nil, fmt.Errorf("GetMetricSummary: %w: startTs=%d endTs=%d", ErrInvalidTimeRange, startTs, endTs)
	}

	// Pre-check: same single-row lookup the bucket-series and metric
	// fetches do. Need metric_type to dispatch and is_monotonic +
	// aggregation_temporality for the Sum branch.
	var metricType string
	var temporality sql.NullString
	var isMonotonic sql.NullBool
	err := db.QueryRowContext(ctx,
		`select metric_type, aggregation_temporality, is_monotonic
		   from metric_streams
		  where id = ?::uuid`,
		metricID,
	).Scan(&metricType, &temporality, &isMonotonic)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetMetricSummary: %w: %s", ErrMetricIDNotFound, metricID)
		}
		return nil, fmt.Errorf("GetMetricSummary: %w: %w", ErrMetricsStoreInternal, err)
	}

	switch metricType {
	case "Gauge":
		return getGaugeSummary(ctx, db, metricID, startTs, endTs)
	case "Sum":
		return getSumSummary(ctx, db, metricID, temporality.String, isMonotonic.Bool, startTs, endTs)
	case "Histogram", "ExponentialHistogram":
		return getHistogramSummary(ctx, db, metricID, metricType, startTs, endTs)
	}
	return nil, fmt.Errorf("GetMetricSummary: %w: %s", ErrMetricsStoreInternal, metricType)
}

// getGaugeSummary: per-timeseries latest value + min/max across the window.
//
// We group by attrs_canonical because aggregating Gauge values across
// per-attribute timeseries is meaningless -- "current" would pick
// whichever timeseries reported last, and min/max would mix unrelated
// series. Each per-attribute timeseries gets its own summary row; the
// frontend matches these up with chart legend entries by `attributesKey`.
//
// `value` resolves to whichever of double_value/int_value is set: the
// ingest pipeline guarantees exactly one of the two is non-null for
// Gauge/Sum, so coalescing in this order produces the original number.
//
// Empty window returns `{"kind":"gauge","timeseries":[]}` -- the frontend
// renders "no data in window" rather than fabricated zeros.
func getGaugeSummary(ctx context.Context, db *sql.DB, metricID string, startTs, endTs int64) (json.RawMessage, error) {
	const q = `
		with vals as (
			select
				coalesce(attrs_canonical, '') as attrs_key,
				timestamp,
				coalesce(double_value, int_value::double) as value
			from datapoints
			where stream_id = ?::uuid
			  and timestamp >= ?::bigint
			  and timestamp <  ?::bigint
		),
		per_ts as (
			select
				attrs_key,
				arg_max(value, timestamp) as current,
				min(value) as min_v,
				max(value) as max_v,
				max(timestamp) as last_ts
			from vals
			group by attrs_key
		)
		select cast(json_object(
			'kind', 'gauge',
			'timeseries', coalesce((
				select json_group_array(json_object(
					'attributesKey', attrs_key,
					'current',       current,
					'min',           min_v,
					'max',           max_v,
					'lastReceived',  cast(last_ts as varchar)
				))
				from per_ts
			), json('[]'))
		) as varchar)`
	var raw []byte
	if err := db.QueryRowContext(ctx, q, metricID, startTs, endTs).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetMetricSummary: %w: %w", ErrMetricsStoreInternal, err)
	}
	return json.RawMessage(raw), nil
}

// getSumSummary computes per-timeseries window-level aggregates
// (current value, delta, min, max) for a Sum metric. Like Gauge,
// summing across per-attribute timeseries is meaningless for Sum --
// "current" would pick whichever timeseries reported last, and
// "min/max across the window" only makes sense within a single
// timeseries. Each per-attribute timeseries gets its own summary row;
// the frontend matches these up with chart legend entries by
// `attributesKey`. `isMonotonic` is a metric-level property (one bool
// per metric, not per timeseries), so it lives at the top level
// alongside `kind` and `timeseries`.
//
// The interesting case is `delta` for Cumulative temporality, which has
// to account for counter resets. A reset happens when an exporter
// process restarts (or otherwise re-initialises a counter): the
// counter resumes from zero, and `last - first` over the whole window
// would either undercount (positive but smaller than the real total)
// or even go negative (the naive value at the end is below the value
// at the start because of the restart).
//
// OTel signals a reset via `start_time`: every dp carries the start
// time of its accumulation period, so `start_time` changing within a
// timeseries marks a new accumulation. We also defensively detect
// "soft resets" -- samples where the value dropped below the previous
// one without a corresponding `start_time` change, which can happen
// with buggy exporters or counters that overflow. Either condition
// begins a new "run" within the timeseries.
//
// Per run, the contribution to delta is `max(value) - min(value)`
// within that run. For a timeseries' first run, this is the same as
// "what we observed accumulate during the window," which matches
// Prometheus's increase() and OTel's specified semantics for the
// "started observing mid-flight" case.
//
// Per-timeseries delta = sum(per-run deltas) across that timeseries'
// runs. `current`, `min`, `max` are window-level over raw values per
// timeseries (no reset adjustment needed -- they're observed values,
// not increments).
//
// For temporality='Delta', each datapoint IS an increment, so the
// per-timeseries delta is just `sum(value)` per timeseries, no reset
// handling required.
//
// Unspecified temporality: errors. Without knowing whether each value
// is a delta or a running total, we can't compute "delta over window"
// or "current value" without lying.
//
// Empty window returns `{"kind":"sum","isMonotonic":...,"timeseries":[]}`.
func getSumSummary(ctx context.Context, db *sql.DB, metricID, temporality string, isMonotonic bool, startTs, endTs int64) (json.RawMessage, error) {
	if temporality != "Delta" && temporality != "Cumulative" {
		return nil, fmt.Errorf("GetMetricSummary: %w: %s", ErrUnspecifiedTemporality, temporality)
	}

	// per_ts_delta is a CTE keyed by attrs_key giving each per-attribute
	// timeseries' window delta. The two temporality paths produce the
	// same shape (one row per attrs_key with a `delta` column) so the
	// outer per_ts join doesn't need to know which path ran.
	var perTsDeltaCTE string
	switch temporality {
	case "Delta":
		// Each dp is its own increment. Per-timeseries window delta is
		// just the sum of that timeseries' dps in the window.
		perTsDeltaCTE = `
		per_ts_delta as (
			select attrs_key, coalesce(sum(value), 0) as delta
			from vals
			group by attrs_key
		)`
	case "Cumulative":
		// Reset-aware per-timeseries delta. `runs` tags each sample with
		// a run_id derived from the cumulative count of "this is a new
		// run" boundaries within attrs_key, then per_run takes max - min
		// within each run, and per_ts_delta sums across runs within each
		// attrs_key. Boundary cases:
		//   - first sample in a timeseries (lag(...) over w is null)
		//   - start_time changed (exporter signalled a reset)
		//   - value went down (soft reset -- defensive)
		perTsDeltaCTE = `
		runs as (
			select
				attrs_key,
				value,
				sum(case
					when lag(value) over w is null then 1
					when start_time <> lag(start_time) over w then 1
					when value < lag(value) over w then 1
					else 0
				end) over (
					partition by attrs_key order by timestamp
					rows between unbounded preceding and current row
				) as run_id
			from vals
			window w as (partition by attrs_key order by timestamp)
		),
		per_run as (
			select attrs_key, run_id, max(value) - min(value) as run_delta
			from runs
			group by attrs_key, run_id
		),
		per_ts_delta as (
			select attrs_key, coalesce(sum(run_delta), 0) as delta
			from per_run
			group by attrs_key
		)`
	}

	q := fmt.Sprintf(`
		with vals as (
			select
				coalesce(attrs_canonical, '') as attrs_key,
				timestamp,
				start_time,
				coalesce(double_value, int_value::double) as value
			from datapoints
			where stream_id = ?::uuid
			  and timestamp >= ?::bigint
			  and timestamp <  ?::bigint
		),%s,
		per_ts as (
			select
				v.attrs_key,
				arg_max(v.value, v.timestamp) as current,
				min(v.value) as min_v,
				max(v.value) as max_v,
				max(v.timestamp) as last_ts,
				any_value(d.delta) as delta
			from vals v
			left join per_ts_delta d using (attrs_key)
			group by v.attrs_key
		)
		select cast(json_object(
			'kind', 'sum',
			'isMonotonic', ?::boolean,
			'timeseries', coalesce((
				select json_group_array(json_object(
					'attributesKey', attrs_key,
					'current',       current,
					'delta',         delta,
					'min',           min_v,
					'max',           max_v,
					'lastReceived',  cast(last_ts as varchar)
				))
				from per_ts
			), json('[]'))
		) as varchar)`, perTsDeltaCTE)
	var raw []byte
	if err := db.QueryRowContext(ctx, q, metricID, startTs, endTs, isMonotonic).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetMetricSummary: %w: %w", ErrMetricsStoreInternal, err)
	}
	return json.RawMessage(raw), nil
}

// MergedTimeseriesKey is the synthetic `attributesKey` used for the
// single merged-distribution entry returned by histogram summaries.
// Real canonical keys always contain `=`, so this sentinel is
// unambiguous. Frontend code can treat any timeseries with this key as
// "this is the cross-timeseries merge, not a real per-attribute series."
const MergedTimeseriesKey = "__merged__"

// getHistogramSummary: delegates to the existing merged bucket-series
// + merged-quantiles paths and stitches the two JSON payloads into
// the unified `timeseries: [...]` shape.
//
// Histograms are the one metric kind where cross-timeseries aggregation
// is genuinely meaningful: merging count/sum/min/max + bucket vectors
// across per-attribute timeseries yields a real overall distribution.
// To signal that this is a merged result rather than a per-attribute
// timeseries, we emit a single entry with `attributesKey = MergedTimeseriesKey`.
//
// Per-timeseries histogram summaries are deliberately not produced
// today; the heatmap UI doesn't have a single-timeseries-selection
// mode yet, and adding the per-timeseries variant before there's a
// place to render it would just be code we have to maintain. When that
// UI lands we'll add a per-timeseries branch alongside the merged one.
//
// Authoring a third copy of the bucket-series CTE chain just to also
// emit quantiles in one query would mean duplicating ~80 lines of SQL
// for a 5% perf win on a local tool; not worth the maintenance burden.
// Both delegated calls share the same pre-checks (temporality,
// bounds-mismatch, type) so an error from either is correctly typed
// and gets re-wrapped with this function's name in the chain.
//
// kind discriminator: 'histogram' or 'expHistogram'. We keep the same
// camelCase the BucketSeriesPoint discriminated union uses on the wire.
//
// Empty window returns `{"kind":"...","timeseries":[]}` -- consistent
// with Gauge and Sum, and lets the frontend render "no data in window"
// instead of fabricated zeros.
func getHistogramSummary(ctx context.Context, db *sql.DB, metricID, metricType string, startTs, endTs int64) (json.RawMessage, error) {
	bucketRaw, err := GetMetricBucketSeries(ctx, db, metricID, "merged", startTs, endTs, 1)
	if err != nil {
		return nil, fmt.Errorf("GetMetricSummary: %w", err)
	}
	quantileRaw, err := GetMetricMergedQuantiles(ctx, db, metricID, []float64{0.5, 0.95, 0.99}, startTs, endTs)
	if err != nil {
		return nil, fmt.Errorf("GetMetricSummary: %w", err)
	}

	// Decode just the totals + timestamp from the single-element
	// bucket-series array. Empty window => empty array => empty
	// `timeseries` array in the output.
	var bucket []struct {
		Timestamp string `json:"timestamp"`
		Totals    struct {
			Count int      `json:"count"`
			Sum   float64  `json:"sum"`
			Min   *float64 `json:"min"`
			Max   *float64 `json:"max"`
		} `json:"totals"`
	}
	if err := json.Unmarshal(bucketRaw, &bucket); err != nil {
		return nil, fmt.Errorf("GetMetricSummary: %w: decode bucket: %w", ErrMetricsStoreInternal, err)
	}

	kind := "histogram"
	if metricType == "ExponentialHistogram" {
		kind = "expHistogram"
	}

	timeseries := []map[string]any{}
	if len(bucket) == 1 {
		merged := map[string]any{
			"attributesKey": MergedTimeseriesKey,
			"count":         bucket[0].Totals.Count,
			"sum":           bucket[0].Totals.Sum,
			"min":           0,
			"max":           0,
			"quantiles":     json.RawMessage(quantileRaw),
			"lastReceived":  bucket[0].Timestamp,
		}
		if bucket[0].Totals.Min != nil {
			merged["min"] = *bucket[0].Totals.Min
		}
		if bucket[0].Totals.Max != nil {
			merged["max"] = *bucket[0].Totals.Max
		}
		timeseries = append(timeseries, merged)
	}

	out := map[string]any{
		"kind":       kind,
		"timeseries": timeseries,
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("GetMetricSummary: %w: encode summary: %w", ErrMetricsStoreInternal, err)
	}
	return raw, nil
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
//   - ErrHistogramBoundsMismatch if merged Histogram has mixed bounds.
func GetMetricBucketSeries(ctx context.Context, db *sql.DB, metricID string, mode string, startTs, endTs int64, maxPoints int) (json.RawMessage, error) {
	if endTs <= startTs {
		return nil, fmt.Errorf("GetMetricBucketSeries: %w: startTs=%d endTs=%d", ErrInvalidTimeRange, startTs, endTs)
	}
	if maxPoints < 1 {
		return nil, fmt.Errorf("GetMetricBucketSeries: %w: %d", ErrInvalidMaxPoints, maxPoints)
	}
	if mode != "per-attribute" && mode != "merged" {
		return nil, fmt.Errorf("GetMetricBucketSeries: %w: %q (want per-attribute or merged)", ErrInvalidQuantileSeriesMode, mode)
	}

	// Pre-check: read metric_type + temporality straight from the
	// metric_streams row. Both fields are stream-level under the new
	// schema, so this is a single-row lookup; we no longer need to
	// peer into datapoints just to discriminate the type.
	var metricType string
	var temporality sql.NullString
	err := db.QueryRowContext(ctx,
		`select metric_type, aggregation_temporality
		   from metric_streams
		  where id = ?::uuid`,
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

	if mode == "per-attribute" {
		return getPerAttributeBucketSeries(ctx, db, metricID, metricType, temporality.String, startTs, endTs, maxPoints)
	}
	switch metricType {
	case "Histogram":
		return getMergedHistogramBucketSeries(ctx, db, metricID, temporality.String, startTs, endTs, maxPoints)
	case "ExponentialHistogram":
		return getMergedExpHistogramBucketSeries(ctx, db, metricID, temporality.String, startTs, endTs, maxPoints)
	}
	return nil, fmt.Errorf("GetMetricBucketSeries: %w: %s", ErrBucketSeriesNotSupportedForType, metricType)
}

// getPerAttributeBucketSeries emits one JSON entry per (bucket_start, attrs_key)
// with the raw bucket vectors and totals. The metric_type determines which
// fields are populated in the output object.
func getPerAttributeBucketSeries(ctx context.Context, db *sql.DB, metricID, metricType, temporality string, startTs, endTs int64, maxPoints int) (json.RawMessage, error) {
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

// getMergedHistogramBucketSeries merges all per-attribute timeseries
// of a Histogram metric per timestamp and returns the merged bucket
// vectors and totals. Uses the same grouped -> merged CTE chain as
// the quantile variant, including the bounds mismatch check.
func getMergedHistogramBucketSeries(ctx context.Context, db *sql.DB, metricID, temporality string, startTs, endTs int64, maxPoints int) (json.RawMessage, error) {
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

// getMergedExpHistogramBucketSeries merges all per-attribute
// timeseries of an ExponentialHistogram per bucket using the full
// alignment pipeline (downscale, pad, sum, fold) and returns the
// aligned bucket arrays. Reuses the same CTE chain as the quantile
// variant but selects the raw vectors from `final` instead of
// computing quantiles.
func getMergedExpHistogramBucketSeries(ctx context.Context, db *sql.DB, metricID, temporality string, startTs, endTs int64, maxPoints int) (json.RawMessage, error) {
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
// Clear removes every metric record from the database: streams, ingests,
// datapoints, exemplars, and the attribute rows that hang off them.
//
// Order matters: child tables go first, then parents, so each statement
// has its FK targets still present when it runs. We tear down the
// per-owner attribute rows in three passes (exemplar / datapoint /
// metric_ingest) because chk_attributes_one_owner forces each row to
// belong to exactly one family -- a single OR'd delete would still hit
// the right rows but its plan is much heavier and the per-family
// version is easier to read against the FK graph.
//
// We don't TRUNCATE the parent tables (metric_streams, metric_ingests)
// because TRUNCATE in DuckDB doesn't run FK checks, but plain DELETE
// keeps us in lockstep with the FK cascade conventions used everywhere
// else in this package.
func Clear(ctx context.Context, db *sql.DB) error {
	for _, q := range []string{
		`delete from attributes where exemplar_id is not null`,
		`delete from attributes where datapoint_id is not null`,
		`delete from attributes where metric_ingest_id is not null`,
		`delete from exemplars`,
		`delete from datapoints`,
		`delete from metric_ingests`,
		`delete from metric_streams`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("Clear: %w: %w", ErrMetricsStoreInternal, err)
		}
	}
	return nil
}

// DeleteMetricStream removes a metric stream and every row that
// references it: metric_ingests, datapoints, exemplars, and attribute
// rows owned by any of those. Now that identity is normalized into
// metric_streams, the dependency graph is a simple tree
// (streams -> ingests -> datapoints -> exemplars, with attributes
// hanging off each). This is a single, child-first cascade run on a
// pinned connection.
//
// We still can't wrap this in a transaction: DuckDB issue #13819 still
// fires "phantom" FK violations for in-tx cascades. The pinned-conn
// auto-commit pattern works around it -- worst-case partial failure
// leaves orphaned attribute rows for an otherwise-cleaned stream, which
// a retry of DeleteMetricStream(streamID) will collect on the next pass.
//
// Returns nil if the stream does not exist (idempotent delete).
func DeleteMetricStream(ctx context.Context, db *sql.DB, streamID string) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("DeleteMetricStream: %w: acquire conn: %w", ErrMetricsStoreInternal, err)
	}
	defer conn.Close()

	// Each statement names the doomed stream in its own WHERE clause so
	// they're independent at the FK layer. Order: leaves first.
	for _, q := range []string{
		`delete from attributes where exemplar_id in (
			select id from exemplars where datapoint_id in (
				select id from datapoints where stream_id = ?::uuid
			)
		)`,
		`delete from attributes where datapoint_id in (
			select id from datapoints where stream_id = ?::uuid
		)`,
		`delete from attributes where metric_ingest_id in (
			select id from metric_ingests where stream_id = ?::uuid
		)`,
		`delete from exemplars where datapoint_id in (
			select id from datapoints where stream_id = ?::uuid
		)`,
		`delete from datapoints where stream_id = ?::uuid`,
		`delete from metric_ingests where stream_id = ?::uuid`,
		`delete from metric_streams where id = ?::uuid`,
	} {
		if _, err := conn.ExecContext(ctx, q, streamID); err != nil {
			return fmt.Errorf("DeleteMetricStream: %w: %w", ErrMetricsStoreInternal, err)
		}
	}
	return nil
}

// buildMetricSQL builds the WHERE clause for the metric Search query.
// It runs against the join of metric_ingests m + metric_streams s, so:
//
//   - identity columns (name, unit, scope_name, scope_version) come from s
//   - per-batch columns (description, dropped counts) come from m
//   - the time predicate joins through metric_ingests.id
//
// Search-level field expressions still use the "m.<col>" / "s.<col>"
// shape so callers don't need to know about the internal join.
func buildMetricSQL(queryNode *search.QueryNode, startTime, endTime int64) (cteSQL string, whereSQL string, args []any, err error) {
	timeCondition := "exists (select 1 from datapoints d where d.metric_ingest_id = m.id and d.timestamp >= time_start and d.timestamp <= time_end)"
	return search.BuildSearchSQL(queryNode, startTime, endTime, metricFieldMapper(), timeCondition)
}

// metricColumns lists field names the search expression syntax can
// reference. All identity columns now resolve through metric_streams (s);
// description / *_dropped_attributes_count remain on metric_ingests (m).
var metricColumns = map[string]struct{}{
	"id":                                {},
	"description":                       {},
	"resource_dropped_attributes_count": {},
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
		return "s.name", nil
	case "unit":
		return "s.unit", nil
	case "scope.name", "scopeName":
		return "s.scope_name", nil
	case "scope.version", "scopeVersion":
		return "s.scope_version", nil
	case "description":
		return "m.description", nil
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
	// "resource"/"scope"/"metric" all denote attributes attached to a
	// metric_ingest row (the per-batch record), so they live on the
	// renamed metric_ingest_id column. Datapoint and exemplar
	// attribute scopes are handled elsewhere.
	case "resource", "scope", "metric":
		expr := fmt.Sprintf("(SELECT a.value FROM attributes a WHERE a.metric_ingest_id = m.id AND a.datapoint_id IS NULL AND a.exemplar_id IS NULL AND a.scope = %s AND a.key = %s LIMIT 1)", scopeParam, keyParam)
		return []string{expr}, nil
	default:
		return nil, fmt.Errorf("unknown attribute scope %s: %w", field.AttributeScope, ErrInvalidMetricQuery)
	}
}

func mapMetricGlobalExpressions() ([]string, error) {
	return []string{
		"CAST(s.name AS VARCHAR) {COND}",
		"CAST(m.description AS VARCHAR) {COND}",
		"CAST(s.unit AS VARCHAR) {COND}",
		"CAST(s.scope_name AS VARCHAR) {COND}",
		"CAST(s.scope_version AS VARCHAR) {COND}",
		`EXISTS(
			SELECT 1
			FROM attributes a
			WHERE a.metric_ingest_id = m.id AND (
				a.key {COND} OR a.value {COND} OR
				(a.type = 'string[]' AND list_contains(CAST(a.value AS VARCHAR[]), CAST({RAW} AS VARCHAR))) OR
				(a.type = 'int64[]' AND list_contains(CAST(a.value AS BIGINT[]), TRY_CAST({RAW} AS BIGINT))) OR
				(a.type = 'float64[]' AND list_contains(CAST(a.value AS DOUBLE[]), TRY_CAST({RAW} AS DOUBLE))) OR
				(a.type = 'boolean[]' AND list_contains(CAST(a.value AS BOOLEAN[]), TRY_CAST({RAW} AS BOOLEAN)))
			)
		)`,
	}, nil
}
