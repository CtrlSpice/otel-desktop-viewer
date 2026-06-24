package metrics

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
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
	ErrInvalidMetricQuery   = errors.New("invalid metric search query")
	ErrMetricsStoreInternal = errors.New("metrics store internal error")
)

const flushIntervalMetrics = 100

// Ingest writes the metric data in m to the metric_streams,
// metric_ingests, datapoints, exemplars, and attributes tables. The
// caller must hold any required lock on the connection.
//
// Ingest runs in two passes:
//
//  1. First pass collects every distinct (resource, scope, metric)
//     identity in the request and upserts them into metric_streams,
//     resolving each to its UUID. This is the only round-trip-per-batch
//     step; the appender path that follows is constant per identity.
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
	// build the identity list eagerly so the upsert sees the whole batch
	// and can resolve everything in two round trips.
	type metricCoord struct {
		ri, si, mi int
	}
	type identityWithCoord struct {
		identity streamIdentity
		coord    metricCoord
	}

	var coords []identityWithCoord
	identitySet := make(map[streamIdentity]struct{})
	for ri, resourceMetric := range m.ResourceMetrics().All() {
		resource := resourceMetric.Resource()
		serviceName := serviceNameFromAttrs(resource.Attributes())
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
	identities := make([]streamIdentity, 0, len(identitySet))
	for id := range identitySet {
		identities = append(identities, id)
	}

	// Upsert metric_streams: INSERT ... ON CONFLICT DO NOTHING, then
	// SELECT back all ids. Two round-trips per batch, constant in
	// identity count. Same two-pass shape streams.go had, inlined here
	// to match the spans/logs single-file layout.
	dconn, ok := conn.(*duckdb.Conn)
	if !ok {
		return fmt.Errorf("Ingest: %w: connection is not a *duckdb.Conn", ErrMetricsStoreInternal)
	}
	prepareArg := func(v any) (driver.Value, error) {
		nv := driver.NamedValue{Value: v}
		err := dconn.CheckNamedValue(&nv)
		if err == nil {
			return nv.Value, nil
		}
		if !errors.Is(err, driver.ErrSkip) {
			return nil, err
		}
		return driver.DefaultParameterConverter.ConvertValue(v)
	}

	rowPlaceholders := make([]string, len(identities))
	insertArgs := make([]driver.NamedValue, 0, len(identities)*9)
	for i, id := range identities {
		newID := uuid.NewString()
		rowPlaceholders[i] = "(?::uuid, ?, ?, ?, ?, ?, ?, ?, ?)"
		var err error
		insertArgs, err = appendNamedValues(insertArgs, prepareArg,
			newID, id.Name, id.Unit, id.MetricType, id.AggregationTemporality,
			isMonotonicToBool(id.IsMonotonic), id.ScopeName, id.ScopeVersion, id.ServiceName,
		)
		if err != nil {
			return fmt.Errorf("Ingest: %w: prep insert arg: %w", ErrMetricsStoreInternal, err)
		}
	}

	insertSQL := fmt.Sprintf(
		`insert into metric_streams (id, name, unit, metric_type, aggregation_temporality, is_monotonic, scope_name, scope_version, service_name)
		 values %s
		 on conflict (name, unit, metric_type, aggregation_temporality, is_monotonic, scope_name, scope_version, service_name) do nothing`,
		strings.Join(rowPlaceholders, ", "),
	)
	if _, err := dconn.ExecContext(ctx, insertSQL, insertArgs); err != nil {
		return fmt.Errorf("Ingest: %w: stream insert: %w", ErrMetricsStoreInternal, err)
	}

	tupleClauses := make([]string, len(identities))
	selectArgs := make([]driver.NamedValue, 0, len(identities)*8)
	for i, id := range identities {
		tupleClauses[i] = `(name = ? and unit = ? and metric_type = ?
			and aggregation_temporality = ?
			and is_monotonic = ?
			and scope_name = ?
			and scope_version = ?
			and service_name = ?)`
		var err error
		selectArgs, err = appendNamedValues(selectArgs, prepareArg,
			id.Name, id.Unit, id.MetricType, id.AggregationTemporality,
			isMonotonicToBool(id.IsMonotonic), id.ScopeName, id.ScopeVersion, id.ServiceName,
		)
		if err != nil {
			return fmt.Errorf("Ingest: %w: prep select arg: %w", ErrMetricsStoreInternal, err)
		}
	}
	selectSQL := fmt.Sprintf(
		`select id, name, unit, metric_type, aggregation_temporality, is_monotonic, scope_name, scope_version, service_name
		 from metric_streams
		 where %s`,
		strings.Join(tupleClauses, " or "),
	)
	rows, err := dconn.QueryContext(ctx, selectSQL, selectArgs)
	if err != nil {
		return fmt.Errorf("Ingest: %w: stream select: %w", ErrMetricsStoreInternal, err)
	}

	streamIDs := make(map[streamIdentity]duckdb.UUID, len(identities))
	dest := make([]driver.Value, 9)
	for {
		if err := rows.Next(dest); err != nil {
			if err.Error() == "EOF" {
				break
			}
			rows.Close()
			return fmt.Errorf("Ingest: %w: stream scan: %w", ErrMetricsStoreInternal, err)
		}
		sid, err := decodeStreamID(dest[0])
		if err != nil {
			rows.Close()
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		metricType := stringOrEmpty(dest[3])
		key := streamIdentity{
			Name:                   stringOrEmpty(dest[1]),
			Unit:                   stringOrEmpty(dest[2]),
			MetricType:             metricType,
			AggregationTemporality: stringOrEmpty(dest[4]),
			IsMonotonic:            boolValueToIdentityString(dest[5], metricType),
			ScopeName:              stringOrEmpty(dest[6]),
			ScopeVersion:           stringOrEmpty(dest[7]),
			ServiceName:            stringOrEmpty(dest[8]),
		}
		streamIDs[key] = sid
	}
	rows.Close()

	if len(streamIDs) != len(identities) {
		return fmt.Errorf("Ingest: %w: resolved %d of %d stream identities", ErrMetricsStoreInternal, len(streamIDs), len(identities))
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
		serviceName := serviceNameFromAttrs(resource.Attributes())
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

// streamIdentity is the 8-field compound identity of a metric stream.
// All fields are strings (including IsMonotonic) so the struct is
// directly usable as a map key. Empty string means "not applicable."
type streamIdentity struct {
	Name                   string
	Unit                   string
	MetricType             string
	AggregationTemporality string
	IsMonotonic            string
	ScopeName              string
	ScopeVersion           string
	ServiceName            string
}

// serviceNameFromAttrs returns the value of the resource attribute
// service.name, or empty string if it isn't set.
func serviceNameFromAttrs(attrs pcommon.Map) string {
	if v, ok := attrs.Get("service.name"); ok {
		return v.AsString()
	}
	return ""
}

// streamIdentityFromMetric extracts the 8-field identity tuple from one
// metric in an OTLP request. aggregation_temporality and is_monotonic
// are encoded as strings (with empty string meaning "not applicable")
// so the result is comparable as a map key without juggling pointers.
func streamIdentityFromMetric(metric pmetric.Metric, scopeName, scopeVersion, serviceName string) streamIdentity {
	id := streamIdentity{
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
			datapointID, streamID, ingestID, int64(dp.Timestamp()), int64(dp.StartTimestamp()), uint32(dp.Flags()),
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
			datapointID, streamID, ingestID, int64(dp.Timestamp()), int64(dp.StartTimestamp()), uint32(dp.Flags()),
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
			datapointID, streamID, ingestID, int64(dp.Timestamp()), int64(dp.StartTimestamp()), uint32(dp.Flags()),
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
			datapointID, streamID, ingestID, int64(dp.Timestamp()), int64(dp.StartTimestamp()), uint32(dp.Flags()),
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

// SearchSummaries returns lightweight per-stream summaries for the drawer
// cards: identity fields, description, seriesCount, lastValue (Gauge/Sum),
// and lastSeen. One row per metric_streams row that has at least one
// in-range datapoint and matches the optional search criteria.
//
// Filtering reuses buildMetricSQL -- the same query builder the (now
// removed) full Search used -- so the WHERE clause is evaluated at the
// metric_ingests (per-batch) level: a stream appears if any of its
// ingests match. The summary aggregation then runs over the matched
// streams' in-range datapoints, identical to before.
func SearchSummaries(ctx context.Context, db *sql.DB, startTime, endTime int64, criteria any) (json.RawMessage, error) {
	var searchTree *search.QueryNode
	if criteria != nil {
		var err error
		searchTree, err = search.ParseQueryTree(criteria)
		if err != nil {
			return nil, fmt.Errorf("SearchSummaries: %w: %w", ErrInvalidMetricQuery, err)
		}
	}
	cteSQL, whereClause, args, err := buildMetricSQL(searchTree, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("SearchSummaries: %w: %w", ErrInvalidMetricQuery, err)
	}

	query := fmt.Sprintf(`%s,
		filtered_ingests as (
			select m.id, m.stream_id
			from metric_ingests m
			inner join metric_streams s on s.id = m.stream_id, search_params
			where %s
		),
		filtered_streams as (
			select s.* from metric_streams s
			where s.id in (select distinct stream_id from filtered_ingests)
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
		stream_datapoint_count as (
			select stream_id, count(*) as datapoint_count
			from filtered_dps
			group by stream_id
		),
		stream_last_value as (
			select
				d.stream_id,
				arg_max(coalesce(d.double_value, d.int_value), d.timestamp) as last_value
			from filtered_dps d
			inner join filtered_streams fs on fs.id = d.stream_id
			where fs.metric_type in ('Gauge', 'Sum')
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
			'dataPointCount', sdc.datapoint_count,
			'lastValue', slv.last_value,
			'lastSeen', sldp.last_dp_ts::varchar
		) order by sldp.last_dp_ts desc nulls last)), '[]') as varchar) as summaries
		from filtered_streams fs
		left join stream_latest_dp sldp on sldp.stream_id = fs.id
		left join stream_description sd on sd.stream_id = fs.id
		left join stream_series_count ssc on ssc.stream_id = fs.id
		left join stream_datapoint_count sdc on sdc.stream_id = fs.id
		left join stream_last_value slv on slv.stream_id = fs.id
	`, cteSQL, whereClause)
	var raw []byte
	if err := db.QueryRowContext(ctx, query, args...).Scan(&raw); err != nil {
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
				s.metric_type as metric_type,
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
			'metricType', s.metric_type,
			'aggregationTemporality', s.aggregation_temporality,
			'isMonotonic', s.is_monotonic,
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
// pass the same not-applicable convention streamIdentity uses
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
				(a.type = 'string[]' AND list_contains(TRY_CAST(a.value AS VARCHAR[]), CAST({RAW} AS VARCHAR))) OR
				(a.type = 'int64[]' AND list_contains(TRY_CAST(a.value AS BIGINT[]), TRY_CAST({RAW} AS BIGINT))) OR
				(a.type = 'float64[]' AND list_contains(TRY_CAST(a.value AS DOUBLE[]), TRY_CAST({RAW} AS DOUBLE))) OR
				(a.type = 'boolean[]' AND list_contains(TRY_CAST(a.value AS BOOLEAN[]), TRY_CAST({RAW} AS BOOLEAN)))
		)
	)`,
	}, nil
}

// appendNamedValues converts a positional argument list into the
// driver.NamedValue form that the duckdb driver's ExecContext /
// QueryContext expect, applying the supplied prep function to each value.
func appendNamedValues(args []driver.NamedValue, prep func(any) (driver.Value, error), vs ...any) ([]driver.NamedValue, error) {
	for _, v := range vs {
		val, err := prep(v)
		if err != nil {
			return nil, err
		}
		args = append(args, driver.NamedValue{
			Ordinal: len(args) + 1,
			Value:   val,
		})
	}
	return args, nil
}

func isMonotonicToBool(s string) bool {
	return s == "true"
}

// decodeStreamID normalizes a UUID coming back from the driver.Conn
// QueryContext path into a duckdb.UUID.
func decodeStreamID(v driver.Value) (duckdb.UUID, error) {
	switch t := v.(type) {
	case duckdb.UUID:
		return t, nil
	case [16]byte:
		return duckdb.UUID(t), nil
	case []byte:
		if len(t) != 16 {
			return duckdb.UUID{}, fmt.Errorf("decodeStreamID: expected 16 bytes, got %d", len(t))
		}
		var u duckdb.UUID
		copy(u[:], t)
		return u, nil
	case string:
		parsed, err := uuid.Parse(t)
		if err != nil {
			return duckdb.UUID{}, fmt.Errorf("decodeStreamID: parse %q: %w", t, err)
		}
		return duckdb.UUID(parsed), nil
	default:
		return duckdb.UUID{}, fmt.Errorf("decodeStreamID: unsupported value type %T", v)
	}
}

func stringOrEmpty(v driver.Value) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func boolValueToIdentityString(v driver.Value, metricType string) string {
	if metricType != "Sum" {
		return ""
	}
	if b, ok := v.(bool); ok {
		if b {
			return "true"
		}
		return "false"
	}
	return ""
}
