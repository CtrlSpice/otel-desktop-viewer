package metrics

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

var (
	ErrInvalidMetricQuery   = errors.New("invalid metric search query")
	ErrStreamIDNotFound     = errors.New("metric stream ID not found")
	ErrMetricsStoreInternal = errors.New("metrics store internal error")
)

// flushIntervalMetrics counts *metrics*, not datapoints -- a different unit
// from the span and log intervals, and already far coarser in rows. A single
// metric can carry thousands of datapoints, so 100 metrics is easily hundreds
// of thousands of appended rows between flushes: well past the point where
// flush overhead matters. Left alone deliberately rather than raised to match
// the others.
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
func Ingest(ctx context.Context, conn driver.Conn, m pmetric.Metrics, flushed *ingest.FlushedIDs) (err error) {
	tables := []string{"exemplars", "datapoints", "metric_ingests"}

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

	// The attribute dictionary is built in the same walk. Datapoint labels are
	// the bulk of it: on the reference capture they are 82% of all attribute
	// rows, resolving to 89 distinct sets across 294,607 datapoints.
	dict := ingest.NewDictionary(flushed)
	type scopeKey struct{ ri, si int }
	resourceIDs := map[int]duckdb.UUID{}
	scopeIDs := map[scopeKey]duckdb.UUID{}

	// One id array per datapoint, in walk order, handed to collectSeries below.
	var dpAttrIDs [][]duckdb.UUID

	for ri, resourceMetric := range m.ResourceMetrics().All() {
		resource := resourceMetric.Resource()
		serviceName := serviceNameFromAttrs(resource.Attributes())
		resourceIDs[ri] = dict.AddResource(resource)
		for si, scopeMetric := range resourceMetric.ScopeMetrics().All() {
			scope := scopeMetric.Scope()
			scopeIDs[scopeKey{ri, si}] = dict.AddScope(scope)
			for mi, metric := range scopeMetric.Metrics().All() {
				if err := ctx.Err(); err != nil {
					return err
				}
				identity := streamIdentityFromMetric(metric, scope.Name(), scope.Version(), serviceName)
				identitySet[identity] = struct{}{}
				coords = append(coords, identityWithCoord{identity: identity, coord: metricCoord{ri, si, mi}})
				dpAttrIDs = addMetricAttributes(dict, metric, dpAttrIDs)
			}
		}
	}
	if len(coords) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
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

	// Nine parallel arrays, one bound argument each, so the statement text is
	// the same whatever the batch holds. The previous form built one
	// "(?::uuid, ?, ...)" tuple per identity and appended nine arguments
	// beside it -- two counts that must agree, which SQL cannot check because
	// each is correct on its own.
	newStreamIDs := make([]string, 0, len(identities))
	names := make([]string, 0, len(identities))
	units := make([]string, 0, len(identities))
	types := make([]string, 0, len(identities))
	temporalities := make([]string, 0, len(identities))
	monotonics := make([]bool, 0, len(identities))
	scopeNames := make([]string, 0, len(identities))
	scopeVersions := make([]string, 0, len(identities))
	serviceNames := make([]string, 0, len(identities))
	for _, id := range identities {
		if err := ctx.Err(); err != nil {
			return err
		}
		newStreamIDs = append(newStreamIDs, uuid.NewString())
		names = append(names, id.Name)
		units = append(units, id.Unit)
		types = append(types, id.MetricType)
		temporalities = append(temporalities, id.AggregationTemporality)
		monotonics = append(monotonics, isMonotonicToBool(id.IsMonotonic))
		scopeNames = append(scopeNames, id.ScopeName)
		scopeVersions = append(scopeVersions, id.ScopeVersion)
		serviceNames = append(serviceNames, id.ServiceName)
	}

	insertArgs, err := appendNamedValues(nil, prepareArg,
		newStreamIDs, names, units, types, temporalities, monotonics,
		scopeNames, scopeVersions, serviceNames)
	if err != nil {
		return fmt.Errorf("Ingest: %w: prep insert arg: %w", ErrMetricsStoreInternal, err)
	}

	const insertSQL = `insert into metric_streams (id, name, unit, metric_type, aggregation_temporality, is_monotonic, scope_name, scope_version, service_name)
		 select unnest(?::varchar[])::uuid, unnest(?::varchar[]), unnest(?::varchar[]), unnest(?::varchar[]),
		        unnest(?::varchar[]), unnest(?::boolean[]), unnest(?::varchar[]), unnest(?::varchar[]), unnest(?::varchar[])
		 on conflict (name, unit, metric_type, aggregation_temporality, is_monotonic, scope_name, scope_version, service_name) do nothing`
	if _, err := dconn.ExecContext(ctx, insertSQL, insertArgs); err != nil {
		return fmt.Errorf("Ingest: %w: stream insert: %w", ErrMetricsStoreInternal, err)
	}

	// The same eight arrays, joined against rather than OR'd together. The
	// previous form was an OR of one eight-column conjunction per identity,
	// so both the text and the argument count grew with the batch.
	selectArgs, err := appendNamedValues(nil, prepareArg,
		names, units, types, temporalities, monotonics,
		scopeNames, scopeVersions, serviceNames)
	if err != nil {
		return fmt.Errorf("Ingest: %w: prep select arg: %w", ErrMetricsStoreInternal, err)
	}

	const selectSQL = `select s.id, s.name, s.unit, s.metric_type, s.aggregation_temporality,
		        s.is_monotonic, s.scope_name, s.scope_version, s.service_name
		 from metric_streams s
		 join (
			select unnest(?::varchar[]) as name, unnest(?::varchar[]) as unit,
			       unnest(?::varchar[]) as metric_type, unnest(?::varchar[]) as aggregation_temporality,
			       unnest(?::boolean[]) as is_monotonic, unnest(?::varchar[]) as scope_name,
			       unnest(?::varchar[]) as scope_version, unnest(?::varchar[]) as service_name
		 ) w on s.name = w.name and s.unit = w.unit and s.metric_type = w.metric_type
		    and s.aggregation_temporality = w.aggregation_temporality
		    and s.is_monotonic = w.is_monotonic and s.scope_name = w.scope_name
		    and s.scope_version = w.scope_version and s.service_name = w.service_name`
	rows, err := dconn.QueryContext(ctx, selectSQL, selectArgs)
	if err != nil {
		return fmt.Errorf("Ingest: %w: stream select: %w", ErrMetricsStoreInternal, err)
	}

	streamIDs := make(map[streamIdentity]duckdb.UUID, len(identities))
	dest := make([]driver.Value, 9)
	for {
		if err := ctx.Err(); err != nil {
			rows.Close()
			return err
		}
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
	if err := dict.Flush(ctx, conn); err != nil {
		return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
	}

	// Resolve every datapoint's series before any of them are appended.
	//
	// This has to sit here specifically: a series id needs the stream id, which
	// is only known after the upsert round trip above, and datapoints.series_id
	// is a foreign key, so the rows must be committed before the appender
	// flushes. Between the two is the only place it fits.
	//
	// The walk consumes the ids the dictionary walk already derived, and carries
	// them forward again so pass 2 reads them by position too. Neither this walk
	// nor pass 2 hashes a label set: datapoints are the highest-volume path in
	// the store, and one derivation each is all they get.
	dpIdents, seriesRows, err := collectSeries(ctx, m, streamIDs, resourceIDs, dpAttrIDs)
	if err != nil {
		return err
	}
	if err := insertSeries(ctx, dconn, prepareArg, seriesRows); err != nil {
		return err
	}
	dpCur := 0

	appenders, err := ingest.NewAppenders(conn, tables)
	if err != nil {
		return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
	}
	defer func() {
		err = errors.Join(err, ingest.CloseAppenders(appenders, tables))
	}()

	metricCount := 0
	for ri, resourceMetric := range m.ResourceMetrics().All() {
		resource := resourceMetric.Resource()
		serviceName := serviceNameFromAttrs(resource.Attributes())
		resourceID := resourceIDs[ri]
		for si, scopeMetric := range resourceMetric.ScopeMetrics().All() {
			scope := scopeMetric.Scope()
			scopeID := scopeIDs[scopeKey{ri, si}]
			for _, metric := range scopeMetric.Metrics().All() {
				if err := ctx.Err(); err != nil {
					return err
				}
				identity := streamIdentityFromMetric(metric, scope.Name(), scope.Version(), serviceName)
				streamID, ok := streamIDs[identity]
				if !ok {
					return fmt.Errorf("Ingest: %w: stream id missing for identity %+v", ErrMetricsStoreInternal, identity)
				}

				ingestID := duckdb.UUID(uuid.New())

				if err := appenders["metric_ingests"].AppendRow(
					ingestID,             // ID UUID
					streamID,             // StreamID UUID
					metric.Description(), // Description VARCHAR
					resourceID,           // ResourceID UUID
					scopeID,              // ScopeID UUID
					// Batch-level, from the OTLP wrappers rather than the
					// Resource / InstrumentationScope messages, neither of
					// which carries the field.
					resourceMetric.SchemaUrl(), // ResourceSchemaURL VARCHAR
					scopeMetric.SchemaUrl(),    // ScopeSchemaURL VARCHAR
				); err != nil {
					return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
				}

				switch metric.Type() {
				case pmetric.MetricTypeGauge:
					if err := ingestGaugeDatapoints(appenders, streamID, ingestID, metric.Gauge().DataPoints(), dpIdents, &dpCur); err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
					}
				case pmetric.MetricTypeSum:
					if err := ingestSumDatapoints(appenders, streamID, ingestID, metric.Sum().DataPoints(), dpIdents, &dpCur); err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
					}
				case pmetric.MetricTypeHistogram:
					if err := ingestHistogramDatapoints(appenders, streamID, ingestID, metric.Histogram().DataPoints(), dpIdents, &dpCur); err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
					}
				case pmetric.MetricTypeExponentialHistogram:
					if err := ingestExponentialHistogramDatapoints(appenders, streamID, ingestID, metric.ExponentialHistogram().DataPoints(), dpIdents, &dpCur); err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
					}
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

	// collectSeries and this walk must have visited the same datapoints in the
	// same order. A divergence would file every point past it under another
	// series -- wrong lines on a chart, with no error anywhere.
	if dpCur != len(dpIdents) {
		return fmt.Errorf("Ingest: %w: datapoint pass mismatch (%d/%d)",
			ErrMetricsStoreInternal, dpCur, len(dpIdents))
	}

	return nil
}

// addMetricAttributes records every datapoint and exemplar attribute set on a
// metric into the dictionary. Kept separate from the append pass so pass 1 can
// hash everything before any row references it.
//
// Appends each datapoint's id array to out, in walk order, and returns the
// extended slice. Those ids are the expensive part -- deriving one costs a
// sha256 per label plus a sort, and datapoints are the highest-volume path in
// the store -- so collectSeries reads them by position rather than hashing
// every label set a second time. It cannot register them itself: the dictionary
// is flushed before collectSeries runs, because series rows reference these ids
// and no foreign key can enforce that ordering into a uuid[].
func addMetricAttributes(dict *ingest.Dictionary, metric pmetric.Metric, out [][]duckdb.UUID) [][]duckdb.UUID {
	eachDatapoint(metric, func(attrs pcommon.Map, exemplars pmetric.ExemplarSlice) {
		out = append(out, ingest.NonNil(dict.AddAttributes(attrs, ingest.ScopeDatapoint)))
		addExemplarAttributes(dict, exemplars)
	})
	return out
}

func addExemplarAttributes(dict *ingest.Dictionary, exemplars pmetric.ExemplarSlice) {
	for _, ex := range exemplars.All() {
		dict.AddAttributes(ex.FilteredAttributes(), ingest.ScopeExemplar)
	}
}

// dpIdentity is what pass 1 works out for one datapoint and pass 2 writes.
type dpIdentity struct {
	series duckdb.UUID
	attrs  []duckdb.UUID
}

// seriesRow is a metric_series row awaiting insert.
type seriesRow struct {
	id       duckdb.UUID
	stream   duckdb.UUID
	resource duckdb.UUID
	attrs    []duckdb.UUID
}

// collectSeries walks every datapoint in the batch, in the same order pass 2
// will, and works out which series each belongs to.
//
// Returns one dpIdentity per datapoint in walk order -- pass 2 consumes them by
// position -- and the distinct series that need inserting. Ordering is the
// contract between the two walks; Ingest checks the cursor against the slice
// length afterwards so a divergence fails loudly rather than pairing datapoints
// with the wrong series.
func collectSeries(
	ctx context.Context,
	m pmetric.Metrics,
	streamIDs map[streamIdentity]duckdb.UUID,
	resourceIDs map[int]duckdb.UUID,
	dpAttrIDs [][]duckdb.UUID,
) ([]dpIdentity, map[duckdb.UUID]seriesRow, error) {
	idents := make([]dpIdentity, 0, len(dpAttrIDs))
	rows := map[duckdb.UUID]seriesRow{}
	cur := 0

	for ri, resourceMetric := range m.ResourceMetrics().All() {
		resource := resourceMetric.Resource()
		serviceName := serviceNameFromAttrs(resource.Attributes())
		resourceID := resourceIDs[ri]
		for _, scopeMetric := range resourceMetric.ScopeMetrics().All() {
			scope := scopeMetric.Scope()
			for _, metric := range scopeMetric.Metrics().All() {
				if err := ctx.Err(); err != nil {
					return nil, nil, err
				}
				identity := streamIdentityFromMetric(metric, scope.Name(), scope.Version(), serviceName)
				streamID, ok := streamIDs[identity]
				if !ok {
					return nil, nil, fmt.Errorf("collectSeries: %w: stream id missing for identity %+v",
						ErrMetricsStoreInternal, identity)
				}
				var overrun bool
				eachDatapoint(metric, func(_ pcommon.Map, _ pmetric.ExemplarSlice) {
					if cur >= len(dpAttrIDs) {
						overrun = true
						return
					}
					ids := dpAttrIDs[cur]
					cur++
					sid := ingest.SeriesID(streamID, resourceID, ids)
					idents = append(idents, dpIdentity{series: sid, attrs: ids})
					// resource_id is the resource this series was *first* seen
					// with, since the insert is on-conflict-do-nothing. When a
					// resource is enriched mid-stream the series now survives
					// intact, which is the point, but its stored resource stays
					// the earlier one. It is a representative, not an identity.
					rows[sid] = seriesRow{id: sid, stream: streamID, resource: resourceID, attrs: ids}
				})
				if overrun {
					return nil, nil, fmt.Errorf("collectSeries: %w: more datapoints than the dictionary walk saw (%d)",
						ErrMetricsStoreInternal, len(dpAttrIDs))
				}
			}
		}
	}
	if cur != len(dpAttrIDs) {
		return nil, nil, fmt.Errorf("collectSeries: %w: dictionary walk mismatch (%d/%d)",
			ErrMetricsStoreInternal, cur, len(dpAttrIDs))
	}
	return idents, rows, nil
}

// eachDatapoint visits a metric's datapoints in the order the ingest* helpers
// write them. Every walk goes through this, so they cannot drift apart -- which
// matters because the dictionary walk and collectSeries pair up by position,
// and a divergence would file every datapoint past it under another series.
func eachDatapoint(metric pmetric.Metric, fn func(attrs pcommon.Map, exemplars pmetric.ExemplarSlice)) {
	switch metric.Type() {
	case pmetric.MetricTypeGauge:
		for _, dp := range metric.Gauge().DataPoints().All() {
			fn(dp.Attributes(), dp.Exemplars())
		}
	case pmetric.MetricTypeSum:
		for _, dp := range metric.Sum().DataPoints().All() {
			fn(dp.Attributes(), dp.Exemplars())
		}
	case pmetric.MetricTypeHistogram:
		for _, dp := range metric.Histogram().DataPoints().All() {
			fn(dp.Attributes(), dp.Exemplars())
		}
	case pmetric.MetricTypeExponentialHistogram:
		for _, dp := range metric.ExponentialHistogram().DataPoints().All() {
			fn(dp.Attributes(), dp.Exemplars())
		}
	}
}

// insertSeries writes the distinct series, ignoring ones already present.
//
// Not through the appender, for the same reason the dictionary is not: the
// appender has no conflict handling, and a series recurs on every batch from
// the same sender, so almost every row would be a duplicate.
func insertSeries(
	ctx context.Context,
	dconn *duckdb.Conn,
	prepareArg func(any) (driver.Value, error),
	rows map[duckdb.UUID]seriesRow,
) error {
	if len(rows) == 0 {
		return nil
	}
	// Four parallel arrays, the last a list of lists: the label sets travel as
	// data rather than as UUIDListLiteral text spliced into the statement, so
	// the query no longer varies with the *contents* of a batch, not merely
	// its size.
	ids := make([]string, 0, len(rows))
	streams := make([]string, 0, len(rows))
	resources := make([]string, 0, len(rows))
	attrs := make([][]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, ingest.FormatUUID(r.id))
		streams = append(streams, ingest.FormatUUID(r.stream))
		resources = append(resources, ingest.FormatUUID(r.resource))
		set := make([]string, 0, len(r.attrs))
		for _, a := range r.attrs {
			set = append(set, ingest.FormatUUID(a))
		}
		attrs = append(attrs, set)
	}

	args, err := appendNamedValues(nil, prepareArg, ids, streams, resources, attrs)
	if err != nil {
		return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
	}

	const q = `insert into metric_series (id, stream_id, resource_id, attribute_ids)
		 select unnest(?::varchar[])::uuid, unnest(?::varchar[])::uuid, unnest(?::varchar[])::uuid,
		        list_transform(unnest(?::varchar[][]), x -> x::uuid)
		 on conflict (id) do nothing`
	if _, err := dconn.ExecContext(ctx, q, args); err != nil {
		return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
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
		_, exAttrIDs := ingest.AttributeSet(ex.FilteredAttributes(), ingest.ScopeExemplar)
		if err := appenders["exemplars"].AppendRow(
			exemplarID, datapointID, int64(ex.Timestamp()), ex.DoubleValue(), traceUUID, spanUUID,
			ingest.NonNil(exAttrIDs),
		); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
	}
	return nil
}

func ingestGaugeDatapoints(appenders map[string]*duckdb.Appender, streamID, ingestID duckdb.UUID, dps pmetric.NumberDataPointSlice, idents []dpIdentity, cur *int) error {
	for _, dp := range dps.All() {
		doubleVal, intVal, valType := numberDataPointValue(dp)
		datapointID := duckdb.UUID(uuid.New())
		ident := idents[*cur]
		*cur++
		if err := appenders["datapoints"].AppendRow(
			datapointID, streamID, ident.series, ingestID, int64(dp.Timestamp()), int64(dp.StartTimestamp()), uint32(dp.Flags()),
			doubleVal, intVal, valType, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			ident.attrs,
		); err != nil {
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

func ingestSumDatapoints(appenders map[string]*duckdb.Appender, streamID, ingestID duckdb.UUID, dps pmetric.NumberDataPointSlice, idents []dpIdentity, cur *int) error {
	for _, dp := range dps.All() {
		doubleVal, intVal, valType := numberDataPointValue(dp)
		datapointID := duckdb.UUID(uuid.New())
		ident := idents[*cur]
		*cur++
		if err := appenders["datapoints"].AppendRow(
			datapointID, streamID, ident.series, ingestID, int64(dp.Timestamp()), int64(dp.StartTimestamp()), uint32(dp.Flags()),
			doubleVal, intVal, valType,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			ident.attrs,
		); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		if err := ingestExemplars(appenders, ingestID, datapointID, dp.Exemplars()); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
	}
	return nil
}

func ingestHistogramDatapoints(appenders map[string]*duckdb.Appender, streamID, ingestID duckdb.UUID, dps pmetric.HistogramDataPointSlice, idents []dpIdentity, cur *int) error {
	for _, dp := range dps.All() {
		datapointID := duckdb.UUID(uuid.New())
		ident := idents[*cur]
		*cur++
		if err := appenders["datapoints"].AppendRow(
			datapointID, streamID, ident.series, ingestID, int64(dp.Timestamp()), int64(dp.StartTimestamp()), uint32(dp.Flags()),
			nil, nil, nil,
			dp.Count(), dp.Sum(), dp.Min(), dp.Max(), dp.BucketCounts().AsRaw(), dp.ExplicitBounds().AsRaw(),
			nil, nil, nil, nil, nil, nil, nil,
			ident.attrs,
		); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		if err := ingestExemplars(appenders, ingestID, datapointID, dp.Exemplars()); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
	}
	return nil
}

func ingestExponentialHistogramDatapoints(appenders map[string]*duckdb.Appender, streamID, ingestID duckdb.UUID, dps pmetric.ExponentialHistogramDataPointSlice, idents []dpIdentity, cur *int) error {
	for _, dp := range dps.All() {
		pos, neg := dp.Positive(), dp.Negative()
		datapointID := duckdb.UUID(uuid.New())
		ident := idents[*cur]
		*cur++
		if err := appenders["datapoints"].AppendRow(
			datapointID, streamID, ident.series, ingestID, int64(dp.Timestamp()), int64(dp.StartTimestamp()), uint32(dp.Flags()),
			nil, nil, nil,
			dp.Count(), dp.Sum(), dp.Min(), dp.Max(), nil, nil,
			dp.Scale(), dp.ZeroCount(), dp.ZeroThreshold(), pos.Offset(), pos.BucketCounts().AsRaw(), neg.Offset(), neg.BucketCounts().AsRaw(),
			ident.attrs,
		); err != nil {
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

	query, err := queries.Render(queries.SearchMetricSummaries, searchSummariesParams{
		CTEs:  cteSQL,
		From:  metricSearchFrom,
		Where: whereClause,
	})
	if err != nil {
		return nil, err
	}
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
// An unknown streamID returns ErrStreamIDNotFound; a known stream with no
// datapoints in the window returns valid MetricData with an empty timeseries
// list (the two are distinct: only the former is a "not found").
// targetBuckets is how many time buckets to reduce the window to; 0 means no
// reduction, and the caller gets every datapoint. Reduction is opt-in because
// it changes which datapoints exist in the response, and a caller that has not
// asked for it should not have to discover that.
func GetMetric(ctx context.Context, db *sql.DB, streamID string, startTime, endTime int64, targetBuckets int64) (json.RawMessage, error) {
	// Everything filters by stream_id.
	// matched_ingests is "ingests for this stream that produced at least
	// one datapoint in the time window." All identity columns the JSON
	// projection needs come from the metric_streams row directly via
	// the stream CTE.
	query, err := queries.Render(queries.GetMetric, nil)
	if err != nil {
		return nil, err
	}

	var raw []byte
	if err := db.QueryRowContext(ctx, query, streamID, startTime, endTime, targetBuckets).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetMetric: %w", ErrStreamIDNotFound)
		}
		return nil, fmt.Errorf("GetMetric: %w: %w", ErrMetricsStoreInternal, err)
	}
	// The projection is a non-null json_object, so a null here means the
	// query itself misbehaved -- an internal anomaly, not a missing stream.
	if raw == nil || string(raw) == "null" {
		return nil, fmt.Errorf("GetMetric: %w: query returned null", ErrMetricsStoreInternal)
	}
	return json.RawMessage(raw), nil
}

// GetMetricAttributes returns every metric-side attribute name/scope/type this
// store knows about. The time range is accepted and ignored -- see the note on
// spans.GetTraceAttributes.
//
// Datapoint and exemplar attributes are included, which the windowed version
// deliberately excluded: it was scoped to the per-batch resource/scope rows
// because reaching datapoint labels meant a second join through a table where
// they were 82% of the rows. From the dictionary they are the same select.
func GetMetricAttributes(ctx context.Context, db *sql.DB, startTime, endTime int64) (json.RawMessage, error) {
	query, err := queries.Render(queries.GetMetricAttributes, nil)
	if err != nil {
		return nil, err
	}

	var raw []byte
	if err := db.QueryRowContext(ctx, query).Scan(&raw); err != nil {
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
	// Attribute, resource and scope rows are shared across signals, so this
	// cannot know whether the ones it just abandoned are still in use.
	// ingest.SweepOrphans collects them; the store runs it once for all three
	// signals rather than three times here.
	for _, q := range []string{
		`delete from exemplars`,
		`delete from datapoints`,
		`delete from metric_series`,
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
// references it: metric_ingests, datapoints and exemplars. The
// dependency graph is a simple tree (streams -> ingests -> datapoints
// -> exemplars); attributes are no longer part of it, since they are
// shared dictionary rows collected by ingest.SweepOrphans rather than
// per-owner rows. This is a single, child-first cascade run on a
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
		`delete from exemplars where datapoint_id in (
			select id from datapoints where stream_id = ?::uuid
		)`,
		`delete from datapoints where stream_id = ?::uuid`,
		`delete from metric_series where stream_id = ?::uuid`,
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
	"id":          {},
	"description": {},
}

func metricFieldMapper() search.FieldMapper {
	return func(field *search.FieldDefinition, query *search.Query, params *[]search.NamedParam) ([]string, error) {
		switch field.SearchScope {
		case "field":
			expr, err := mapMetricFieldExpression(field)
			if err != nil {
				return nil, err
			}
			return []string{expr}, nil
		case "attribute":
			return mapMetricAttributeExpressions(field, query, params)
		case "global":
			return mapMetricGlobalExpressions()
		default:
			return nil, fmt.Errorf("unknown search scope %s: %w", field.SearchScope, ErrInvalidMetricQuery)
		}
	}
}

// matchIngestByLabel finds the ingests owning a datapoint (or exemplar) whose
// label matches, and it is written this way for a measured reason.
//
// The dictionary is resolved *first*: the inner select turns (key, condition)
// into a list of attribute ids by scanning ~488 rows, and the outer scan then
// tests each row's array for overlap with that list. One pass, no unnest, no
// join per row. The obvious alternatives cost 5x on the reference capture
// (229,196 datapoints):
//
//	correlated EXISTS per ingest         39.2 ms
//	hoisted IN with unnest + join        36.3 ms
//	hoisted IN with array overlap         7.7 ms   <- this
//
// The && operator is list_has_any. Note what the predicate means: it matches
// ingests having a datapoint that *carries* the label with a matching value. A
// null-check therefore matches nothing rather than finding datapoints missing
// the label -- unlike the resource and scope cases above, where attr_value
// returns NULL for an absent key and `IS NULL` means "lacks it". The asymmetry
// is inherent: a resource has one attribute set, an ingest has thousands of
// datapoints, so "the label is absent" is not a property of the ingest.
//
// The two %s are the source relation and the qualified array column: the
// exemplar form joins datapoints, and both tables have an attribute_ids, so the
// column has to be named explicitly.
const matchIngestByLabel = `m.id in (
			select d.metric_ingest_id from %s
			where %s && (
				select list(a.id) from attributes a where a.key = %s and a.value {COND}
			)
		)`

// metricSearchFrom is the FROM clause metric search predicates are written
// against. Mirrors spans.spanSearchFrom and logs.logSearchFrom.
//
// resources and scopes join through metric_ingests, not metric_streams: the
// stream's scope_name / scope_version are its coarse *identity* (stable across
// pod restarts, which is what keeps a counter one timeseries), while the
// resources and scopes rows are the precise per-batch record. Searching
// resource.* has to mean the latter.
const metricSearchFrom = `from search_params, metric_ingests m
			inner join metric_streams s on s.id = m.stream_id
			inner join resources r on r.id = m.resource_id
			inner join scopes sc on sc.id = m.scope_id`

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
	// The two dropped counts moved off metric_ingests onto the resources and
	// scopes rows it now references, so they resolve through the joins rather
	// than as columns on m.
	case "resource.droppedAttributesCount", "resourceDroppedAttributesCount":
		return "r.dropped_attributes_count", nil
	case "scope.droppedAttributesCount", "scopeDroppedAttributesCount":
		return "sc.dropped_attributes_count", nil
	default:
		col := util.CamelToSnake(name)
		if err := util.ValidateColumnName(col, metricColumns); err != nil {
			return "", fmt.Errorf("metric field %q: %w: %w", name, err, ErrInvalidMetricQuery)
		}
		return "m." + col, nil
	}
}

// mapMetricAttributeExpressions resolves an attribute by key against whichever
// array its scope names. The scope parameter the old form carried is gone:
// scope is implied by which array is searched.
//
// "metric" is kept as an alias for the resource array. It never denoted its own
// storage -- under the old schema all three scopes were rows hanging off the
// same metric_ingest_id -- and the search-field registry still emits it.
//
// Both predicates are hoisted into the owner table -- see
// spans.mapTraceAttributeExpressions for the measurement that motivates it.
func mapMetricAttributeExpressions(field *search.FieldDefinition, query *search.Query, params *[]search.NamedParam) ([]string, error) {
	// Fast path: equality on a string attribute is a membership test against a
	// content-derived id. IDProbe returns "" for anything it cannot answer
	// exactly, falling through to the value comparison below.
	switch field.AttributeScope {
	case "resource", "metric":
		if p := ingest.IDProbe("attribute_ids", field, query, ingest.ScopeResource); p != "" {
			return []string{search.Complete("m.resource_id in (select id from resources where " + p + ")")}, nil
		}
	case "scope":
		if p := ingest.IDProbe("attribute_ids", field, query, ingest.ScopeScope); p != "" {
			return []string{search.Complete("m.scope_id in (select id from scopes where " + p + ")")}, nil
		}
	case "datapoint":
		if p := ingest.IDProbe("d.attribute_ids", field, query, ingest.ScopeDatapoint); p != "" {
			return []string{search.Complete(
				"m.id in (select d.metric_ingest_id from datapoints d where " + p + ")")}, nil
		}
	case "exemplar":
		if p := ingest.IDProbe("e.attribute_ids", field, query, ingest.ScopeExemplar); p != "" {
			return []string{search.Complete(
				"m.id in (select d.metric_ingest_id from exemplars e" +
					" join datapoints d on d.id = e.datapoint_id where " + p + ")")}, nil
		}
	}

	keyParam := fmt.Sprintf("attr_key_%d", len(*params))
	*params = append(*params, search.NamedParam{Name: keyParam, Value: field.Name})

	switch field.AttributeScope {
	case "resource", "metric":
		return []string{fmt.Sprintf(
			"m.resource_id in (select id from resources where attr_value(attribute_ids, %s) {COND})",
			keyParam)}, nil
	case "scope":
		return []string{fmt.Sprintf(
			"m.scope_id in (select id from scopes where attr_value(attribute_ids, %s) {COND})",
			keyParam)}, nil
	case "datapoint":
		return []string{fmt.Sprintf(matchIngestByLabel,
			"datapoints d", "d.attribute_ids", keyParam)}, nil
	case "exemplar":
		return []string{fmt.Sprintf(matchIngestByLabel,
			"exemplars e join datapoints d on d.id = e.datapoint_id", "e.attribute_ids", keyParam)}, nil
	default:
		return nil, fmt.Errorf("unknown attribute scope %s: %w", field.AttributeScope, ErrInvalidMetricQuery)
	}
}

// matchIngestByAnyLabel is the free-text form of matchIngestByLabel: it matches
// on key, on value, or element-wise inside the four array types, mirroring what
// the span and log global matchers cover.
const matchIngestByAnyLabel = `m.id in (
			select d.metric_ingest_id from %s
			where %s && (
				select list(a.id) from attributes a where (
					a.key {COND} OR a.value {COND} OR
					(a.type = 'string[]' AND list_contains(TRY_CAST(a.value AS VARCHAR[]), CAST({RAW} AS VARCHAR))) OR
					(a.type = 'int64[]' AND list_contains(TRY_CAST(a.value AS BIGINT[]), TRY_CAST({RAW} AS BIGINT))) OR
					(a.type = 'float64[]' AND list_contains(TRY_CAST(a.value AS DOUBLE[]), TRY_CAST({RAW} AS DOUBLE))) OR
					(a.type = 'boolean[]' AND list_contains(TRY_CAST(a.value AS BOOLEAN[]), TRY_CAST({RAW} AS BOOLEAN)))
				)
			)
		)`

func mapMetricGlobalExpressions() ([]string, error) {
	return []string{
		"CAST(s.name AS VARCHAR) {COND}",
		"CAST(m.description AS VARCHAR) {COND}",
		"CAST(s.unit AS VARCHAR) {COND}",
		"CAST(s.scope_name AS VARCHAR) {COND}",
		"CAST(s.scope_version AS VARCHAR) {COND}",
		// Datapoint and exemplar labels, resolved through the dictionary first
		// so free-text search costs one array-overlap scan rather than a
		// per-ingest correlated walk of the datapoints table.
		fmt.Sprintf(matchIngestByAnyLabel, "datapoints d", "d.attribute_ids"),
		fmt.Sprintf(matchIngestByAnyLabel,
			"exemplars e join datapoints d on d.id = e.datapoint_id", "e.attribute_ids"),

		// The batch's resource and scope attributes -- the two sets that used
		// to share one metric_ingest_id in the attributes table.
		`EXISTS(
			SELECT 1
			FROM unnest(r.attribute_ids || sc.attribute_ids) AS t(aid)
			JOIN attributes a ON a.id = t.aid
			WHERE (
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

// searchSummariesParams are the fragments SearchSummaries assembles into
// queries/metrics/search_summaries.sql.
type searchSummariesParams struct {
	// CTEs is the search_params CTE holding the time bounds.
	CTEs string
	// From is the shared FROM/JOIN chain for metric search.
	From string
	// Where is the predicate, "true" when there are no criteria.
	Where string
}
