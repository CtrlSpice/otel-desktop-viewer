package metrics

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

var (
	ErrInvalidMetricQuery    = errors.New("invalid metric search query")
	ErrInvalidMetricLimit    = errors.New("invalid metric search limit")
	ErrStreamIDNotFound      = errors.New("metric stream ID not found")
	ErrUnsupportedMetricType = errors.New("unsupported metric type")
	ErrMetricsStoreInternal  = errors.New("metrics store internal error")
)

// flushIntervalMetrics counts *metrics*, not datapoints -- a different unit
// from the span and log intervals, and already far coarser in rows. A single
// metric can carry thousands of datapoints, so 100 metrics is easily hundreds
// of thousands of appended rows between flushes: well past the point where
// flush overhead matters. Left alone deliberately rather than raised to match
// the others.
const flushIntervalMetrics = 100

// scopeKey identifies a scope by position: the ri'th resource's si'th scope.
type scopeKey struct{ ri, si int }

// countMetrics counts metrics, the unit bisection blames. Walked rather than
// read from pass 1's coords, which skips unresolvable identities and so would
// not line up with pass 2's ordinals.
func countMetrics(m pmetric.Metrics) int {
	n := 0
	for _, rm := range m.ResourceMetrics().All() {
		for _, sm := range rm.ScopeMetrics().All() {
			n += sm.Metrics().Len()
		}
	}
	return n
}

// metricDatapointCount mirrors appendPass's type switch, so a skipped metric
// steps dpCur over exactly the datapoints that pass would have written.
func metricDatapointCount(metric pmetric.Metric) int {
	switch metric.Type() {
	case pmetric.MetricTypeGauge:
		return metric.Gauge().DataPoints().Len()
	case pmetric.MetricTypeSum:
		return metric.Sum().DataPoints().Len()
	case pmetric.MetricTypeHistogram:
		return metric.Histogram().DataPoints().Len()
	case pmetric.MetricTypeExponentialHistogram:
		return metric.ExponentialHistogram().DataPoints().Len()
	}
	return 0
}

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
// Ingest is IngestReport for callers with nowhere to put the report. Metrics
// the store refused are still skipped rather than failing the batch; this form
// just cannot say how many there were.
func Ingest(ctx context.Context, conn driver.Conn, m pmetric.Metrics, flushed *ingest.FlushedIDs) error {
	_, err := IngestReport(ctx, conn, m, flushed)
	return err
}

// IngestReport ingests a batch and reports the metrics it could not write. A
// non-empty Rejected is not a failure: the batch landed, minus those metrics.
func IngestReport(ctx context.Context, conn driver.Conn, m pmetric.Metrics, flushed *ingest.FlushedIDs) (rejected ingest.Rejected, err error) {
	defer func() { err = ingest.InterruptedContextError(ctx, err) }()
	var identities []streamIdentity
	identityIndexes := make(map[streamLookupKey]int)
	var seriesRows []seriesRow
	cleanupArmed := false

	// Pass 1: collect every distinct identity in this OTLP request, plus
	// per-identity service_name (denormalized onto metric_streams). We
	// build the identity list eagerly so resolution sees the whole batch
	// and can resolve everything in two round trips.
	// The attribute dictionary is built in the same walk. Datapoint labels are
	// the bulk of it: on the reference capture they are 82% of all attribute
	// rows, resolving to 89 distinct sets across 294,607 datapoints.
	dict := ingest.NewDictionary(flushed)
	resourceIDs := map[int]duckdb.UUID{}
	resourceAttributeIDs := map[int][]duckdb.UUID{}
	scopeIDs := map[scopeKey]duckdb.UUID{}
	scopeAttributeIDs := map[scopeKey][]duckdb.UUID{}

	// One id array per datapoint, in walk order, handed to collectSeries below.
	var dpAttrIDs [][]duckdb.UUID

	for ri, resourceMetric := range m.ResourceMetrics().All() {
		resource := resourceMetric.Resource()
		serviceName := serviceNameFromAttrs(resource.Attributes())
		resourceIDs[ri] = dict.AddResource(resource)
		_, resourceAttributeIDs[ri] = ingest.AttributeSet(resource.Attributes(), ingest.ScopeResource)
		for si, scopeMetric := range resourceMetric.ScopeMetrics().All() {
			scope := scopeMetric.Scope()
			key := scopeKey{ri, si}
			scopeIDs[key] = dict.AddScope(scope)
			_, scopeAttributeIDs[key] = ingest.AttributeSet(scope.Attributes(), ingest.ScopeScope)
			for _, metric := range scopeMetric.Metrics().All() {
				if err := ctx.Err(); err != nil {
					return ingest.Rejected{}, err
				}
				identity := streamIdentityFromMetric(metric, resourceAttributeIDs[ri], scope.Name(), scope.Version(),
					scopeMetric.SchemaUrl(), scopeAttributeIDs[key], serviceName)
				identityKey := identity.lookupKey()
				if _, exists := identityIndexes[identityKey]; !exists {
					identityIndexes[identityKey] = len(identities)
					identities = append(identities, identity)
				}
				dpAttrIDs = addMetricAttributes(dict, metric, dpAttrIDs)
			}
		}
	}
	if len(identities) == 0 {
		return ingest.Rejected{}, nil
	}
	if err := ctx.Err(); err != nil {
		return ingest.Rejected{}, err
	}
	// Find or insert metric_streams by their exact stored keys. Unique indexes
	// enforce identity; UUIDs are generated only for keys not already present.
	dconn, ok := conn.(*duckdb.Conn)
	if !ok {
		return ingest.Rejected{}, fmt.Errorf("Ingest: %w: connection is not a *duckdb.Conn", ErrMetricsStoreInternal)
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
	defer func() {
		if cleanupArmed && (err != nil || rejected.Count() != 0) {
			err = errors.Join(err, cleanupProvisionalIdentities(
				context.WithoutCancel(ctx), dconn, prepareArg, flushed, identities, seriesRows))
		}
	}()

	// Attribute rows must exist before the stream identities that reference
	// them. DuckDB cannot enforce foreign keys into the UUID arrays.
	cleanupArmed = true
	if err := dict.Flush(ctx, conn); err != nil {
		return ingest.Rejected{}, fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
	}
	if err := resolveStreamIDs(ctx, dconn, prepareArg, identities); err != nil {
		return ingest.Rejected{}, err
	}

	// Pass 2: open the appenders and walk the request again, writing
	// metric_ingests and datapoints. We resolve each metric's stream_id by
	// matching its exact identity in the resolved batch.

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
	dpIdents, collectedSeries, err := collectSeries(ctx, m, identities, identityIndexes, resourceAttributeIDs, scopeAttributeIDs, dpAttrIDs)
	if err != nil {
		return ingest.Rejected{}, err
	}
	seriesRows = collectedSeries
	if err := insertSeries(ctx, dconn, prepareArg, seriesRows); err != nil {
		return ingest.Rejected{}, err
	}
	for i := range dpIdents {
		dpIdents[i].series = seriesRows[dpIdents[i].seriesIndex].id
	}
	// Pass 2: append, retrying in halves so a bad metric costs only itself.
	return ingest.BisectingWrite(ctx, countMetrics(m), nil, func(lo, hi int) error {
		return ingest.InTransaction(ctx, conn, func() error {
			return appendPass(ctx, conn, m, identities, identityIndexes, resourceIDs, scopeIDs, resourceAttributeIDs, scopeAttributeIDs, dpIdents,
				func(ordinal int) bool { return ordinal >= lo && ordinal < hi })
		})
	})
}

// appendPass writes the metrics keep selects, by ordinal. Skipped metrics still
// step dpCur over their datapoints, since dpIdents is consumed by position.
func appendPass(
	ctx context.Context,
	conn driver.Conn,
	m pmetric.Metrics,
	streamIDs []streamIdentity,
	streamIndexes map[streamLookupKey]int,
	resourceIDs map[int]duckdb.UUID,
	scopeIDs map[scopeKey]duckdb.UUID,
	resourceAttributeIDs map[int][]duckdb.UUID,
	scopeAttributeIDs map[scopeKey][]duckdb.UUID,
	dpIdents []dpIdentity,
	keep func(ordinal int) bool,
) (err error) {
	tables := []string{"exemplars", "datapoints", "metric_ingests"}
	dpCur := 0

	appenders, err := ingest.NewAppenders(conn, tables)
	if err != nil {
		return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
	}
	defer func() {
		err = errors.Join(err, ingest.CloseAppenders(appenders, tables))
	}()

	metricCount := 0
	metricOrdinal := 0
	for ri, resourceMetric := range m.ResourceMetrics().All() {
		resource := resourceMetric.Resource()
		serviceName := serviceNameFromAttrs(resource.Attributes())
		resourceID := resourceIDs[ri]
		for si, scopeMetric := range resourceMetric.ScopeMetrics().All() {
			scope := scopeMetric.Scope()
			key := scopeKey{ri, si}
			scopeID := scopeIDs[key]
			for _, metric := range scopeMetric.Metrics().All() {
				if err := ctx.Err(); err != nil {
					return err
				}

				ordinal := metricOrdinal
				metricOrdinal++
				if !keep(ordinal) {
					dpCur += metricDatapointCount(metric)
					continue
				}

				identity := streamIdentityFromMetric(metric, resourceAttributeIDs[ri], scope.Name(), scope.Version(),
					scopeMetric.SchemaUrl(), scopeAttributeIDs[key], serviceName)
				streamIndex, ok := streamIndexes[identity.lookupKey()]
				if !ok {
					return fmt.Errorf("Ingest: %w: stream id missing for identity %+v", ErrMetricsStoreInternal, identity)
				}
				streamID := streamIDs[streamIndex].ID

				ingestID := duckdb.UUID(uuid.New())

				// Hashed in pass 1; NonNil because AttributeSet returns nil for an
				// empty map while the column is NOT NULL.
				_, metadataIDs := ingest.AttributeSet(metric.Metadata(), ingest.ScopeMetricMetadata)
				if err := appenders["metric_ingests"].AppendRow(
					ingestID,                   // ID UUID
					streamID,                   // StreamID UUID
					metric.Description(),       // Description VARCHAR
					ingest.NonNil(metadataIDs), // MetadataIDs UUID[] (NOT NULL; [] when absent)
					resourceID,                 // ResourceID UUID
					scopeID,                    // ScopeID UUID
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
		// ErrNotRowFault: our bug, not a row's, so bisection must not search.
		return fmt.Errorf("Ingest: %w: %w: datapoint pass mismatch (%d/%d)",
			ErrMetricsStoreInternal, ingest.ErrNotRowFault, dpCur, len(dpIdents))
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
	// Bounds vectors are dictionary rows the datapoints will reference, so
	// they must be registered in this pass: the dictionary flushes before the
	// appenders open, and a foreign key holds datapoints to it. Pass 2 does
	// not need the ids handed over -- BoundsID is a pure hash, so the appender
	// recomputes it from the same bytes.
	if metric.Type() == pmetric.MetricTypeHistogram {
		for _, dp := range metric.Histogram().DataPoints().All() {
			dict.AddBounds(dp.ExplicitBounds().AsRaw())
		}
	}
	// The metric's own metadata map, which is not a datapoint label: it
	// describes the instrument rather than identifying a series, so it takes
	// its own scope and never enters series identity.
	dict.AddAttributes(metric.Metadata(), ingest.ScopeMetricMetadata)

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
	series      duckdb.UUID
	seriesIndex int
	attrs       []duckdb.UUID
}

// seriesRow is a metric_series row awaiting insert.
type seriesRow struct {
	id             duckdb.UUID
	stream         duckdb.UUID
	attrs          []duckdb.UUID
	existed        bool
	existenceKnown bool
}

func seriesKey(row seriesRow) string {
	// UUID strings have a fixed width, so concatenation is an exact in-memory
	// lookup key. It is not stored or exposed as the series ID.
	var key strings.Builder
	key.Grow(36 * (len(row.attrs) + 1))
	key.WriteString(ingest.FormatUUID(row.stream))
	for _, id := range row.attrs {
		key.WriteString(ingest.FormatUUID(id))
	}
	return key.String()
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
	streamIDs []streamIdentity,
	streamIndexes map[streamLookupKey]int,
	resourceAttributeIDs map[int][]duckdb.UUID,
	scopeAttributeIDs map[scopeKey][]duckdb.UUID,
	dpAttrIDs [][]duckdb.UUID,
) ([]dpIdentity, []seriesRow, error) {
	idents := make([]dpIdentity, 0, len(dpAttrIDs))
	var rows []seriesRow
	rowIndexes := make(map[string]int)
	cur := 0

	for ri, resourceMetric := range m.ResourceMetrics().All() {
		resource := resourceMetric.Resource()
		serviceName := serviceNameFromAttrs(resource.Attributes())
		for si, scopeMetric := range resourceMetric.ScopeMetrics().All() {
			scope := scopeMetric.Scope()
			key := scopeKey{ri, si}
			for _, metric := range scopeMetric.Metrics().All() {
				if err := ctx.Err(); err != nil {
					return nil, nil, err
				}
				identity := streamIdentityFromMetric(metric, resourceAttributeIDs[ri], scope.Name(), scope.Version(),
					scopeMetric.SchemaUrl(), scopeAttributeIDs[key], serviceName)
				streamIndex, ok := streamIndexes[identity.lookupKey()]
				if !ok {
					return nil, nil, fmt.Errorf("collectSeries: %w: stream id missing for identity %+v",
						ErrMetricsStoreInternal, identity)
				}
				streamID := streamIDs[streamIndex].ID
				var overrun bool
				eachDatapoint(metric, func(_ pcommon.Map, _ pmetric.ExemplarSlice) {
					if cur >= len(dpAttrIDs) {
						overrun = true
						return
					}
					ids := dpAttrIDs[cur]
					cur++
					row := seriesRow{stream: streamID, attrs: ids}
					rowKey := seriesKey(row)
					rowIndex, ok := rowIndexes[rowKey]
					if !ok {
						rowIndex = len(rows)
						rows = append(rows, row)
						rowIndexes[rowKey] = rowIndex
					}
					idents = append(idents, dpIdentity{seriesIndex: rowIndex, attrs: ids})
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
	rows []seriesRow,
) error {
	if len(rows) == 0 {
		return nil
	}
	// Three parallel arrays, the last a list of lists: the label sets travel as
	// data rather than as UUIDListLiteral text spliced into the statement, so
	// the query no longer varies with the *contents* of a batch, not merely
	// its size.
	ordinals := make([]int32, 0, len(rows))
	streams := make([]string, 0, len(rows))
	attrs := make([][]string, 0, len(rows))
	for i, r := range rows {
		ordinals = append(ordinals, int32(i))
		streams = append(streams, ingest.FormatUUID(r.stream))
		set := make([]string, 0, len(r.attrs))
		for _, a := range r.attrs {
			set = append(set, ingest.FormatUUID(a))
		}
		attrs = append(attrs, set)
	}

	args, err := appendNamedValues(nil, prepareArg, streams, attrs)
	if err != nil {
		return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
	}

	const q = `insert into metric_series (id, stream_id, attribute_ids)
		 select uuid(), w.stream_id, w.attribute_ids
		 from (
			select unnest(?::varchar[])::uuid as stream_id,
			       list_transform(unnest(?::varchar[][]), x -> x::uuid) as attribute_ids
		 ) w
		 where not exists (
			select 1 from metric_series s
			where s.stream_id = w.stream_id and s.attribute_ids = w.attribute_ids
		 )`
	selectArgs, err := appendNamedValues(nil, prepareArg, ordinals, streams, attrs)
	if err != nil {
		return fmt.Errorf("Ingest: %w: prep series select: %w", ErrMetricsStoreInternal, err)
	}
	const resolve = `select w.ordinal::bigint, s.id::varchar from metric_series s join (
		select unnest(?::integer[]) as ordinal,
		       unnest(?::varchar[])::uuid as stream_id,
		       list_transform(unnest(?::varchar[][]), x -> x::uuid) as attribute_ids
	) w on s.stream_id = w.stream_id and s.attribute_ids = w.attribute_ids`
	resolveRows := func(markExisting, requireAll bool) error {
		checkRows, err := dconn.QueryContext(ctx, resolve, selectArgs)
		if err != nil {
			return fmt.Errorf("Ingest: %w: series select: %w", ErrMetricsStoreInternal, err)
		}
		resolved := make([]bool, len(rows))
		defer checkRows.Close()
		for {
			dest := []driver.Value{nil, nil}
			err := checkRows.Next(dest)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return fmt.Errorf("Ingest: %w: series select: %w", ErrMetricsStoreInternal, err)
			}
			ordinal := int(dest[0].(int64))
			if resolved[ordinal] {
				return fmt.Errorf("Ingest: %w: duplicate exact metric series key", ErrMetricsStoreInternal)
			}
			parsed, err := uuid.Parse(dest[1].(string))
			if err != nil {
				return fmt.Errorf("Ingest: %w: parse series ID: %w", ErrMetricsStoreInternal, err)
			}
			rows[ordinal].id = duckdb.UUID(parsed)
			rows[ordinal].existed = rows[ordinal].existed || markExisting
			resolved[ordinal] = true
		}
		if requireAll && slices.Contains(resolved, false) {
			return fmt.Errorf("Ingest: %w: metric series ID not resolved", ErrMetricsStoreInternal)
		}
		return nil
	}
	if err := resolveRows(true, false); err != nil {
		return err
	}
	for i := range rows {
		rows[i].existenceKnown = true
	}
	if _, err := dconn.ExecContext(ctx, q, args); err != nil {
		return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
	}
	if err := resolveRows(false, true); err != nil {
		return err
	}
	return nil
}

// streamIdentity is the complete stored identity tuple for one OTel Metric.
type streamIdentity struct {
	ID                     duckdb.UUID
	Existed                bool
	ExistenceKnown         bool
	ResourceAttributeIDs   []duckdb.UUID
	Name                   string
	Unit                   string
	MetricType             string
	AggregationTemporality int32
	IsMonotonic            string
	ScopeName              string
	ScopeVersion           string
	ScopeSchemaURL         string
	ScopeAttributeIDs      []duckdb.UUID
	ServiceName            string
}

type streamLookupKey struct {
	resourceAttributes     string
	name                   string
	unit                   string
	metricType             string
	aggregationTemporality int32
	isMonotonic            string
	scopeName              string
	scopeVersion           string
	scopeSchemaURL         string
	scopeAttributes        string
}

func uuidListKey(ids []duckdb.UUID) string {
	var key strings.Builder
	key.Grow(36 * len(ids))
	for _, id := range ids {
		key.WriteString(ingest.FormatUUID(id))
	}
	return key.String()
}

func (s streamIdentity) lookupKey() streamLookupKey {
	return streamLookupKey{
		resourceAttributes:     uuidListKey(s.ResourceAttributeIDs),
		name:                   s.Name,
		unit:                   s.Unit,
		metricType:             s.MetricType,
		aggregationTemporality: s.AggregationTemporality,
		isMonotonic:            s.IsMonotonic,
		scopeName:              s.ScopeName,
		scopeVersion:           s.ScopeVersion,
		scopeSchemaURL:         s.ScopeSchemaURL,
		scopeAttributes:        uuidListKey(s.ScopeAttributeIDs),
	}
}

func resolveStreamIDs(
	ctx context.Context,
	dconn *duckdb.Conn,
	prepareArg func(any) (driver.Value, error),
	identities []streamIdentity,
) error {
	ordinals := make([]int32, 0, len(identities))
	resourceAttrs := make([][]string, 0, len(identities))
	names := make([]string, 0, len(identities))
	units := make([]string, 0, len(identities))
	types := make([]string, 0, len(identities))
	temporalities := make([]int32, 0, len(identities))
	monotonics := make([]bool, 0, len(identities))
	scopeNames := make([]string, 0, len(identities))
	scopeVersions := make([]string, 0, len(identities))
	scopeSchemaURLs := make([]string, 0, len(identities))
	scopeAttrs := make([][]string, 0, len(identities))
	serviceNames := make([]string, 0, len(identities))
	for i, identity := range identities {
		if err := ctx.Err(); err != nil {
			return err
		}
		ordinals = append(ordinals, int32(i))
		resourceAttrs = append(resourceAttrs, uuidStrings(identity.ResourceAttributeIDs))
		names = append(names, identity.Name)
		units = append(units, identity.Unit)
		types = append(types, identity.MetricType)
		temporalities = append(temporalities, identity.AggregationTemporality)
		monotonics = append(monotonics, isMonotonicToBool(identity.IsMonotonic))
		scopeNames = append(scopeNames, identity.ScopeName)
		scopeVersions = append(scopeVersions, identity.ScopeVersion)
		scopeSchemaURLs = append(scopeSchemaURLs, identity.ScopeSchemaURL)
		scopeAttrs = append(scopeAttrs, uuidStrings(identity.ScopeAttributeIDs))
		serviceNames = append(serviceNames, identity.ServiceName)
	}

	keyArgs := []any{resourceAttrs, names, units, types, temporalities, monotonics,
		scopeNames, scopeVersions, scopeSchemaURLs, scopeAttrs}
	selectArgs, err := appendNamedValues(nil, prepareArg, append([]any{ordinals}, keyArgs...)...)
	if err != nil {
		return fmt.Errorf("Ingest: %w: prep stream select: %w", ErrMetricsStoreInternal, err)
	}
	const selectSQL = `select w.ordinal::bigint, s.id::varchar from metric_streams s join (
		select unnest(?::integer[]) as ordinal,
		       list_transform(unnest(?::varchar[][]), x -> x::uuid) as resource_attribute_ids,
		       unnest(?::varchar[]) as name, unnest(?::varchar[]) as unit,
		       unnest(?::varchar[]) as metric_type, unnest(?::integer[]) as aggregation_temporality,
		       unnest(?::boolean[]) as is_monotonic, unnest(?::varchar[]) as scope_name,
		       unnest(?::varchar[]) as scope_version, unnest(?::varchar[]) as scope_schema_url,
		       list_transform(unnest(?::varchar[][]), x -> x::uuid) as scope_attribute_ids
	) w on s.resource_attribute_ids = w.resource_attribute_ids and s.name = w.name
	   and s.unit = w.unit and s.metric_type = w.metric_type
	   and s.aggregation_temporality = w.aggregation_temporality
	   and s.is_monotonic = w.is_monotonic and s.scope_name = w.scope_name
	   and s.scope_version = w.scope_version and s.scope_schema_url = w.scope_schema_url
	   and s.scope_attribute_ids = w.scope_attribute_ids`
	resolveRows := func(markExisting, requireAll bool) error {
		rows, err := dconn.QueryContext(ctx, selectSQL, selectArgs)
		if err != nil {
			return fmt.Errorf("Ingest: %w: stream select: %w", ErrMetricsStoreInternal, err)
		}
		resolved := make([]bool, len(identities))
		defer rows.Close()
		for {
			dest := []driver.Value{nil, nil}
			err := rows.Next(dest)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return fmt.Errorf("Ingest: %w: stream select: %w", ErrMetricsStoreInternal, err)
			}
			ordinal := int(dest[0].(int64))
			if resolved[ordinal] {
				return fmt.Errorf("Ingest: %w: duplicate exact metric stream key", ErrMetricsStoreInternal)
			}
			parsed, err := uuid.Parse(dest[1].(string))
			if err != nil {
				return fmt.Errorf("Ingest: %w: parse stream ID: %w", ErrMetricsStoreInternal, err)
			}
			identities[ordinal].ID = duckdb.UUID(parsed)
			identities[ordinal].Existed = identities[ordinal].Existed || markExisting
			resolved[ordinal] = true
		}
		if requireAll && slices.Contains(resolved, false) {
			return fmt.Errorf("Ingest: %w: metric stream ID not resolved", ErrMetricsStoreInternal)
		}
		return nil
	}
	if err := resolveRows(true, false); err != nil {
		return err
	}
	for i := range identities {
		identities[i].ExistenceKnown = true
	}
	insertArgs, err := appendNamedValues(nil, prepareArg, append(keyArgs, serviceNames)...)
	if err != nil {
		return fmt.Errorf("Ingest: %w: prep stream insert: %w", ErrMetricsStoreInternal, err)
	}
	const insertSQL = `insert into metric_streams (id, resource_attribute_ids, name, unit, metric_type, aggregation_temporality, is_monotonic, scope_name, scope_version, scope_schema_url, scope_attribute_ids, service_name)
		 select uuid(), w.resource_attribute_ids, w.name, w.unit, w.metric_type,
		        w.aggregation_temporality, w.is_monotonic, w.scope_name, w.scope_version,
		        w.scope_schema_url, w.scope_attribute_ids, w.service_name
		 from (
			select list_transform(unnest(?::varchar[][]), x -> x::uuid) as resource_attribute_ids,
			       unnest(?::varchar[]) as name, unnest(?::varchar[]) as unit,
			       unnest(?::varchar[]) as metric_type, unnest(?::integer[]) as aggregation_temporality,
			       unnest(?::boolean[]) as is_monotonic, unnest(?::varchar[]) as scope_name,
			       unnest(?::varchar[]) as scope_version, unnest(?::varchar[]) as scope_schema_url,
			       list_transform(unnest(?::varchar[][]), x -> x::uuid) as scope_attribute_ids,
			       unnest(?::varchar[]) as service_name
		 ) w
		 where not exists (
			select 1 from metric_streams s
			where s.resource_attribute_ids = w.resource_attribute_ids and s.name = w.name
			  and s.unit = w.unit and s.metric_type = w.metric_type
			  and s.aggregation_temporality = w.aggregation_temporality
			  and s.is_monotonic = w.is_monotonic and s.scope_name = w.scope_name
			  and s.scope_version = w.scope_version and s.scope_schema_url = w.scope_schema_url
			  and s.scope_attribute_ids = w.scope_attribute_ids
		 )`
	if _, err := dconn.ExecContext(ctx, insertSQL, insertArgs); err != nil {
		return fmt.Errorf("Ingest: %w: stream insert: %w", ErrMetricsStoreInternal, err)
	}
	return resolveRows(false, true)
}

func cleanupProvisionalIdentities(
	ctx context.Context,
	dconn *duckdb.Conn,
	prepareArg func(any) (driver.Value, error),
	flushed *ingest.FlushedIDs,
	identities []streamIdentity,
	seriesRows []seriesRow,
) error {
	var cleanupErr error
	for _, row := range seriesRows {
		if !row.existenceKnown || row.existed {
			continue
		}
		args, err := appendNamedValues(nil, prepareArg,
			ingest.FormatUUID(row.stream), uuidStrings(row.attrs))
		if err != nil {
			cleanupErr = errors.Join(cleanupErr,
				fmt.Errorf("cleanupProvisionalIdentities: %w: %w", ErrMetricsStoreInternal, err))
			continue
		}
		_, err = dconn.ExecContext(ctx, `delete from metric_series s
			where s.stream_id = ?::uuid
			  and s.attribute_ids = list_transform(?::varchar[], x -> x::uuid)
			  and not exists (select 1 from datapoints d where d.series_id = s.id)`, args)
		if err != nil {
			cleanupErr = errors.Join(cleanupErr,
				fmt.Errorf("cleanupProvisionalIdentities: %w: %w", ErrMetricsStoreInternal, err))
		}
	}
	for _, identity := range identities {
		if !identity.ExistenceKnown || identity.Existed {
			continue
		}
		args, err := appendNamedValues(nil, prepareArg,
			uuidStrings(identity.ResourceAttributeIDs), identity.Name, identity.Unit,
			identity.MetricType, identity.AggregationTemporality,
			isMonotonicToBool(identity.IsMonotonic), identity.ScopeName,
			identity.ScopeVersion, identity.ScopeSchemaURL,
			uuidStrings(identity.ScopeAttributeIDs))
		if err != nil {
			cleanupErr = errors.Join(cleanupErr,
				fmt.Errorf("cleanupProvisionalIdentities: %w: %w", ErrMetricsStoreInternal, err))
			continue
		}
		_, err = dconn.ExecContext(ctx, `delete from metric_streams s
			where s.resource_attribute_ids = list_transform(?::varchar[], x -> x::uuid)
			  and s.name = ? and s.unit = ? and s.metric_type = ?
			  and s.aggregation_temporality = ? and s.is_monotonic = ?
			  and s.scope_name = ? and s.scope_version = ? and s.scope_schema_url = ?
			  and s.scope_attribute_ids = list_transform(?::varchar[], x -> x::uuid)
			  and not exists (select 1 from metric_ingests mi where mi.stream_id = s.id)`, args)
		if err != nil {
			cleanupErr = errors.Join(cleanupErr,
				fmt.Errorf("cleanupProvisionalIdentities: %w: %w", ErrMetricsStoreInternal, err))
		}
	}
	return errors.Join(cleanupErr, ingest.SweepOrphansConn(ctx, dconn, flushed))
}

// serviceNameFromAttrs returns the value of the resource attribute
// service.name, or empty string if it isn't set.
func serviceNameFromAttrs(attrs pcommon.Map) string {
	if v, ok := attrs.Get("service.name"); ok {
		return v.AsString()
	}
	return ""
}

// streamIdentityFromMetric extracts the exact identity tuple from one Metric.
// aggregation_temporality remains its received signed int32 code; metric type
// distinguishes non-applicable stored placeholders from received values.
func streamIdentityFromMetric(
	metric pmetric.Metric,
	resourceAttributeIDs []duckdb.UUID,
	scopeName, scopeVersion, scopeSchemaURL string,
	scopeAttributeIDs []duckdb.UUID,
	serviceName string,
) streamIdentity {
	id := streamIdentity{
		ResourceAttributeIDs: resourceAttributeIDs,
		Name:                 metric.Name(), Unit: metric.Unit(), MetricType: metric.Type().String(),
		ScopeName: scopeName, ScopeVersion: scopeVersion, ScopeSchemaURL: scopeSchemaURL,
		ScopeAttributeIDs: scopeAttributeIDs, ServiceName: serviceName,
	}
	switch metric.Type() {
	case pmetric.MetricTypeSum:
		id.AggregationTemporality = int32(metric.Sum().AggregationTemporality())
		mono := metric.Sum().IsMonotonic()
		id.IsMonotonic = strconv.FormatBool(mono)
	case pmetric.MetricTypeHistogram:
		id.AggregationTemporality = int32(metric.Histogram().AggregationTemporality())
	case pmetric.MetricTypeExponentialHistogram:
		id.AggregationTemporality = int32(metric.ExponentialHistogram().AggregationTemporality())
	}
	return id
}

func uuidStrings(ids []duckdb.UUID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = ingest.FormatUUID(id)
	}
	return out
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
		var spanID driver.Value
		if sid := ex.SpanID(); !sid.IsEmpty() {
			spanID = util.SpanIDUint64(sid)
		}
		_, exAttrIDs := ingest.AttributeSet(ex.FilteredAttributes(), ingest.ScopeExemplar)
		doubleVal, intVal := exemplarValue(ex)
		if err := appenders["exemplars"].AppendRow(
			exemplarID, datapointID, uint64(ex.Timestamp()), doubleVal, intVal, traceUUID, spanID,
			ingest.NonNil(exAttrIDs),
		); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
	}
	return nil
}

func exemplarValue(ex pmetric.Exemplar) (doubleVal any, intVal any) {
	switch ex.ValueType() {
	case pmetric.ExemplarValueTypeDouble:
		return ex.DoubleValue(), nil
	case pmetric.ExemplarValueTypeInt:
		return nil, ex.IntValue()
	default:
		return nil, nil
	}
}

func ingestGaugeDatapoints(appenders map[string]*duckdb.Appender, streamID, ingestID duckdb.UUID, dps pmetric.NumberDataPointSlice, idents []dpIdentity, cur *int) error {
	for _, dp := range dps.All() {
		doubleVal, intVal, valType := numberDataPointValue(dp)
		datapointID := duckdb.UUID(uuid.New())
		ident := idents[*cur]
		*cur++
		if err := appenders["datapoints"].AppendRow(
			datapointID, streamID, ident.series, ingestID, uint64(dp.Timestamp()), uint64(dp.StartTimestamp()), uint32(dp.Flags()),
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
			datapointID, streamID, ident.series, ingestID, uint64(dp.Timestamp()), uint64(dp.StartTimestamp()), uint32(dp.Flags()),
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
			datapointID, streamID, ident.series, ingestID, uint64(dp.Timestamp()), uint64(dp.StartTimestamp()), uint32(dp.Flags()),
			nil, nil, nil,
			dp.Count(), optionalFloat64(dp.HasSum(), dp.Sum()), optionalFloat64(dp.HasMin(), dp.Min()), optionalFloat64(dp.HasMax(), dp.Max()), dp.BucketCounts().AsRaw(), ingest.BoundsID(dp.ExplicitBounds().AsRaw()),
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
			datapointID, streamID, ident.series, ingestID, uint64(dp.Timestamp()), uint64(dp.StartTimestamp()), uint32(dp.Flags()),
			nil, nil, nil,
			dp.Count(), optionalFloat64(dp.HasSum(), dp.Sum()), optionalFloat64(dp.HasMin(), dp.Min()), optionalFloat64(dp.HasMax(), dp.Max()), nil, nil,
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

func optionalFloat64(present bool, value float64) any {
	if !present {
		return nil
	}
	return value
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
func SearchSummaries(ctx context.Context, db *sql.DB, timeRange timerange.TimeRange, criteria any) (json.RawMessage, error) {
	return searchSummaries(ctx, db, timeRange, criteria, search.ResultOptions{})
}

// SearchSummariesWithLimit returns at most limit metric stream summaries.
func SearchSummariesWithLimit(ctx context.Context, db *sql.DB, timeRange timerange.TimeRange, criteria any, limit int64) (json.RawMessage, error) {
	return searchSummaries(ctx, db, timeRange, criteria, search.ResultOptions{Limit: &limit})
}

func SearchSummariesWithOptions(ctx context.Context, db *sql.DB, timeRange timerange.TimeRange, criteria any, options search.ResultOptions) (json.RawMessage, error) {
	return searchSummaries(ctx, db, timeRange, criteria, options)
}

func searchSummaries(ctx context.Context, db *sql.DB, timeRange timerange.TimeRange, criteria any, options search.ResultOptions) (json.RawMessage, error) {
	var searchTree *search.QueryNode
	if criteria != nil {
		var err error
		searchTree, err = search.ParseQueryTree(criteria)
		if err != nil {
			return nil, fmt.Errorf("SearchSummaries: %w: %w", ErrInvalidMetricQuery, err)
		}
	}
	cteSQL, whereClause, args, err := buildMetricSQL(searchTree, timeRange)
	if err != nil {
		return nil, fmt.Errorf("SearchSummaries: %w: %w", ErrInvalidMetricQuery, err)
	}

	candidateOrder, summaryOrder, earlyLimit, err := metricSummaryOrderBy(options.Sort)
	if err != nil {
		return nil, err
	}
	candidateLimit := ""
	summaryLimit := ""
	if options.Limit != nil {
		if *options.Limit < 1 {
			return nil, fmt.Errorf("SearchSummaries: limit must be positive: %w", ErrInvalidMetricLimit)
		}
		if earlyLimit {
			candidateLimit = "\n\t\t\tlimit ?"
		} else {
			summaryLimit = "\n\t\t\tlimit ?"
		}
		args = append(args, *options.Limit)
	}
	query, err := queries.Render(queries.SearchMetricSummaries, searchSummariesParams{
		CTEs:           cteSQL,
		From:           metricSearchFrom,
		Where:          whereClause,
		DatapointWhere: metricDatapointWhere(timeRange),
		CandidateOrder: candidateOrder,
		CandidateLimit: candidateLimit,
		SummaryOrder:   summaryOrder,
		SummaryLimit:   summaryLimit,
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

func metricSummaryOrderBy(sortOption *search.Sort) (candidateOrder, summaryOrder string, earlyLimit bool, err error) {
	field := "lastSeen"
	direction := "desc"
	if sortOption != nil {
		field = sortOption.Field
		direction, err = search.SortDirectionSQL(sortOption.Direction)
		if err != nil {
			return "", "", false, err
		}
	}
	type metricSortSpec struct {
		candidate string
		summary   string
		early     bool
	}
	spec, ok := map[string]metricSortSpec{
		"lastSeen":       {candidate: "sldp.last_dp_ts", summary: "last_dp_ts", early: true},
		"name":           {candidate: "fs.name", summary: "name", early: true},
		"metricType":     {candidate: "fs.metric_type", summary: "metric_type", early: true},
		"serviceName":    {candidate: "fs.service_name", summary: "service_name", early: true},
		"description":    {summary: "coalesce(description, '')"},
		"dataPointCount": {summary: "coalesce(datapoint_count, 0)"},
		"seriesCount":    {summary: "coalesce(series_count, 0)"},
	}[field]
	if !ok {
		return "", "", false, fmt.Errorf("unsupported metric sort field %q: %w", field, search.ErrInvalidSort)
	}
	if spec.early {
		candidateOrder = fmt.Sprintf("%s %s nulls last, fs.id asc", spec.candidate, direction)
	}
	summaryOrder = fmt.Sprintf("%s %s nulls last, id asc", spec.summary, direction)
	return candidateOrder, summaryOrder, spec.early, nil
}

// fieldValueQueries names the metric columns whose values the search box may
// complete, keyed by the search grammar's field name. An allowlist of whole
// baked queries rather than an identifier spliced into one: a field name from
// the wire never becomes SQL. metric_streams holds one row per logical
// stream, so these scan a tiny table, not the datapoints.
var fieldValueQueries = map[string]string{
	"name": `
		select cast(coalesce(to_json(list(sub.v order by sub.v)), to_json([])) as varchar)
		from (
			select distinct name as v
			from metric_streams
			where name ilike '%' || ? || '%' escape '\'
			order by name
			limit ?
		) sub
	`,
	"unit": `
		select cast(coalesce(to_json(list(sub.v order by sub.v)), to_json([])) as varchar)
		from (
			select distinct unit as v
			from metric_streams
			where unit <> '' and unit ilike '%' || ? || '%' escape '\'
			order by unit
			limit ?
		) sub
	`,
}

// GetFieldValues returns distinct values of one completable metric column
// matching term. Same contract as spans.GetFieldValues; alphabetical rather
// than by frequency, because metric_streams has no row count to rank by and
// stream names read best sorted.
func GetFieldValues(ctx context.Context, db *sql.DB, field, term string, limit int64) (json.RawMessage, error) {
	query, ok := fieldValueQueries[field]
	if !ok {
		return nil, fmt.Errorf("GetFieldValues: %w: field %q has no value completion", ErrInvalidMetricQuery, field)
	}
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(term)
	var raw []byte
	if err := db.QueryRowContext(ctx, query, escaped, limit).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetFieldValues: %w: %w", ErrMetricsStoreInternal, err)
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
// seriesIDs narrows the response to those series; nil returns every series and
// an empty slice returns none. The filter is applied before the reduction, so
// a caller asking for two of ten series pays for two.
// quantiles are computed per histogram datapoint and returned keyed by the
// quantile; empty skips the work.
// viewBuckets is the resolution the Sum / Average / Rate views aggregate onto,
// which is a different question from targetBuckets: the election thins while
// keeping the line's shape, the views bucket for a chart.
// sparklineBuckets is the resolution of the per-series sparkline, a third
// question again: it fits a list row rather than a chart, and the reduction
// keeps each bucket's min and max so a spike survives at that size. 0 skips it
// and every series comes back with a null sparkline. It is computed for every
// series in the response, including unselected ones, because the sparkline is
// how a user decides which series to select.
// selectedSeriesIDs names the pool the Selected cross-series line folds. It
// narrows nothing else: the All line keeps folding every series in the stream,
// and nil means nothing is checked, so only the All line is drawn.
// datapointSeriesIDs and datapointSeriesLimit decide which series ship their
// datapoints -- almost the whole payload. Every series keeps its row, stats,
// view buckets and sparkline regardless, so the panel can still list the ones
// nobody is drawing and the All aggregate can still fold them. The limit is
// for a caller that cannot name the series it wants because it picks them from
// this very response; it takes the first N in the response's own order. Nil
// and 0 mean every series ships them.
// A null endpoint is omitted from filtering and derived from the filtered data
// extent. A concrete endpoint remains the effective endpoint.
func GetMetric(ctx context.Context, db *sql.DB, streamID string, timeRange timerange.TimeRange, targetBuckets int64, seriesIDs []string, quantiles []float64, tzOffsetNs int64, viewBuckets int64, sparklineBuckets int64, selectedSeriesIDs []string, tzName string, datapointSeriesIDs []string, datapointSeriesLimit int64) (json.RawMessage, error) {
	return getMetric(ctx, db, getMetricParams{}, streamID, timeRange, targetBuckets, seriesIDs, quantiles, tzOffsetNs, viewBuckets, sparklineBuckets, selectedSeriesIDs, tzName, datapointSeriesIDs, datapointSeriesLimit)
}

// GetMetricOTLP returns all retained reports for a metric stream as a standard
// OTLP JSON object. Reports with matching resource, scope, description, and
// metadata are combined while distinct source contexts remain separate. Callers
// must transport the returned bytes unchanged because re-encoding can lose -0.0.
func GetMetricOTLP(ctx context.Context, db *sql.DB, streamID string) (json.RawMessage, error) {
	query, err := queries.Render(queries.GetMetricOTLP, nil)
	if err != nil {
		return nil, fmt.Errorf("GetMetricOTLP: %w: %w", ErrMetricsStoreInternal, err)
	}

	var metricType string
	var raw []byte
	if err := db.QueryRowContext(ctx, query, streamID).Scan(&metricType, &raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetMetricOTLP: %w", ErrStreamIDNotFound)
		}
		return nil, fmt.Errorf("GetMetricOTLP: %w: %w", ErrMetricsStoreInternal, err)
	}
	if raw == nil {
		switch metricType {
		case "Gauge", "Sum", "Histogram", "ExponentialHistogram":
			return nil, fmt.Errorf("GetMetricOTLP: %w: query returned null for %s", ErrMetricsStoreInternal, metricType)
		default:
			return nil, fmt.Errorf("GetMetricOTLP: %w: %s", ErrUnsupportedMetricType, metricType)
		}
	}
	return json.RawMessage(raw), nil
}

// getMetric runs the query in whichever shape params asks for. Both shapes take
// the same arguments and the same CTEs; only the projection differs.
func getMetric(ctx context.Context, db *sql.DB, params getMetricParams, streamID string, timeRange timerange.TimeRange, targetBuckets int64, seriesIDs []string, quantiles []float64, tzOffsetNs int64, viewBuckets int64, sparklineBuckets int64, selectedSeriesIDs []string, tzName string, datapointSeriesIDs []string, datapointSeriesLimit int64) (json.RawMessage, error) {
	// Deduplicate the quantile list, keeping first-occurrence order.
	//
	// The quantile CTEs build the wire object with map(), and DuckDB raises
	// "Map keys must be unique" on a duplicate -- so a request carrying the
	// same quantile twice would fail whole. The old json_group_object path
	// tolerated that silently, and nothing between the RPC handler and here
	// dedupes, so this is where the tolerance lives now. Order is preserved
	// because the object's keys deliberately follow request order.
	if len(quantiles) > 1 {
		seen := make(map[float64]bool, len(quantiles))
		deduped := quantiles[:0:0]
		for _, q := range quantiles {
			if !seen[q] {
				seen[q] = true
				deduped = append(deduped, q)
			}
		}
		quantiles = deduped
	}

	// Everything filters by stream_id.
	// matched_ingests is "ingests for this stream that produced at least
	// one datapoint in the time window." All identity columns the JSON
	// projection needs come from the metric_streams row directly via
	// the stream CTE.
	params.TimeFilter = metricDetailTimeFilter(timeRange)
	query, err := queries.Render(queries.GetMetric, params)
	if err != nil {
		return nil, err
	}

	var raw []byte
	// A nil slice binds as an empty array, not SQL NULL, so "all series" has
	// to travel as an untyped nil. Empty then keeps its own meaning: no series.
	var seriesArg any
	if seriesIDs != nil {
		seriesArg = seriesIDs
	}
	if quantiles == nil {
		quantiles = []float64{}
	}
	// Same untyped-nil dance as seriesArg, and for a different meaning: null is
	// "nothing checked", so the Selected pool is empty and the chart draws All
	// alone. An empty slice would bind as an empty array and say the same thing,
	// but going through nil keeps the two parameters' conventions identical.
	var selectedArg any
	if len(selectedSeriesIDs) > 0 {
		selectedArg = selectedSeriesIDs
	}
	// nil and empty are different questions here, as they are for seriesArg:
	// nil means the caller is not naming series, so the limit decides, while an
	// empty list names no series and ships no datapoints at all.
	// Empty means "no zone named", which the query reads as a null and answers
	// with the single offset instead -- so a caller that sends no zone keeps the
	// behaviour it had.
	var tzArg any
	if tzName != "" {
		tzArg = tzName
	}
	var datapointArg any
	if datapointSeriesIDs != nil {
		datapointArg = datapointSeriesIDs
	}
	var startTime, endTime any
	if timeRange.Start != nil {
		startTime = []uint64{*timeRange.Start}
	}
	if timeRange.End != nil {
		endTime = []uint64{*timeRange.End}
	}
	if err := db.QueryRowContext(ctx, query, streamID, startTime, endTime, targetBuckets, seriesArg, quantiles, tzOffsetNs, viewBuckets, sparklineBuckets, selectedArg, tzArg, datapointArg, datapointSeriesLimit).Scan(&raw); err != nil {
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

// GetMetricAggregate returns only the cross-series aggregate: the selected
// series merged into one histogram per time bucket.
//
// Exists as its own call because the two halves of a metric response have
// different lifetimes. Per-series quantiles are additive -- fetch them once for
// every series and any subset's lines are already in hand -- while the
// aggregate is specific to the selection and has to be recomputed when the
// legend changes. Binding both to one fetch would either re-ship the per-series
// payload on every toggle, or make selecting a metric fetch twice, because the
// legend selection is seeded from the response it would depend on.
//
// Runs the same query and keeps one field. The per-series work happens either
// way -- the aggregate is built from the merged series -- so the saving is
// payload, not computation.
// It serves both metric shapes, and they use different parameters to say
// different things -- which is the part to get right.
//
// A histogram merges the *checked* series into one histogram per bucket, so the
// caller narrows with seriesIDs and the merge sees only those.
//
// A scalar needs both pools at once: the checked series and every series in the
// stream. So the caller passes no seriesIDs at all and names the checked set in
// selectedSeriesIDs instead. Narrowing here would not merely trim the payload,
// it would redefine the answer -- "All" computed over a narrowed set is "all of
// the checked ones", which is wrong and looks entirely plausible on a chart.
//
// The rule the two share: narrowing decides what is *sent*, never what is
// *aggregated*.
func GetMetricAggregate(ctx context.Context, db *sql.DB, streamID string, timeRange timerange.TimeRange, targetBuckets int64, seriesIDs []string, quantiles []float64, tzOffsetNs int64, viewBuckets int64, selectedSeriesIDs []string, tzName string) (json.RawMessage, error) {
	// Ask SQL for the aggregate shape rather than the whole metric.
	//
	// This used to call GetMetric and then unmarshal its response in Go to
	// keep two fields. That was the store's only place parsing JSON on the way
	// back out -- everywhere else a query's JSON is passed through untouched --
	// and it made a legend toggle pay for the entire metric.
	//
	// The saving is not the discarded bytes, it is the plan. DuckDB prunes the
	// CTEs a projection never reads, so dropping `timeseries` drops the
	// per-series pipelines feeding it. Measured on a 21-series histogram:
	// 314ms -> 150ms, of which planning 212ms -> 67ms.
	//
	// 0 sparkline buckets and an empty datapoint list are still passed, so a
	// caller reading this does not have to work out that the pruning already
	// covers them.
	raw, err := getMetric(ctx, db, aggregateShapeFor(ctx, db, streamID),
		streamID, timeRange, targetBuckets, seriesIDs, quantiles,
		tzOffsetNs, viewBuckets, 0, selectedSeriesIDs, tzName,
		[]string{}, 0)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// GetMetricAttributes returns every metric-side attribute name/scope/type this
// store knows about. See the note on spans.GetTraceAttributes.
//
// Datapoint and exemplar attributes are included, which the windowed version
// deliberately excluded: it was scoped to the per-batch resource/scope rows
// because reaching datapoint labels meant a second join through a table where
// they were 82% of the rows. From the dictionary they are the same select.
func GetMetricAttributes(ctx context.Context, db *sql.DB) (json.RawMessage, error) {
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
// auto-commit pattern works around it. A retry of DeleteMetricStream
// completes any remaining stream-table cascade; orphaned dictionary rows
// are collected by a later SweepOrphans call after Clear or during retention.
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
func buildMetricSQL(queryNode *search.QueryNode, timeRange timerange.TimeRange) (cteSQL string, whereSQL string, args []any, err error) {
	dpCondition, timeParams := search.TimePredicate("d.timestamp", timeRange.Start, timeRange.End)
	timeCondition := "exists (select 1 from datapoints d where d.metric_ingest_id = m.id"
	if dpCondition != "" {
		timeCondition += " and " + dpCondition
	}
	timeCondition += ")"
	return search.BuildSearchSQL(queryNode, metricFieldMapper(), timeCondition, timeParams)
}

func metricDatapointWhere(timeRange timerange.TimeRange) string {
	predicate, _ := search.TimePredicate("d.timestamp", timeRange.Start, timeRange.End)
	if predicate == "" {
		return ""
	}
	return "where " + predicate
}

func metricDetailTimeFilter(timeRange timerange.TimeRange) string {
	predicate, _ := search.TimePredicate("d.timestamp", timeRange.Start, timeRange.End)
	if predicate == "" {
		return ""
	}
	predicate = strings.ReplaceAll(predicate, "time_start", "input.time_start")
	predicate = strings.ReplaceAll(predicate, "time_end", "input.time_end")
	return "and " + predicate
}

// metricColumns lists field names the search expression syntax can
// reference. All identity columns now resolve through metric_streams (s);
// description / *_dropped_attributes_count remain on metric_ingests (m).
var metricColumns = map[string]struct{}{
	"id":          {},
	"description": {},
}

func metricFieldMapper() search.FieldMapper {
	return func(field *search.FieldDefinition, query *search.Query, params *[]search.NamedParam) ([]search.ResolvedExpression, error) {
		switch field.SearchScope {
		case "field":
			expr, err := mapMetricFieldExpression(field)
			if err != nil {
				return nil, err
			}
			return []search.ResolvedExpression{expr}, nil
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
			where exists (select 1 from unnest(%s) t(aid) join attributes a on a.id = t.aid
			where a.key = %s%s and %s {COND})
		)`

// metricSearchFrom is the FROM clause metric search predicates are written
// against. Mirrors spans.spanSearchFrom and logs.logSearchFrom.
//
// resources and scopes join through metric_ingests, not metric_streams. The
// stream stores the exact identifying Resource and Scope fields, while the
// resources and scopes rows preserve each received payload, including dropped
// counts. Searching resource.* and scope.* has to use those received rows.
const metricSearchFrom = `from search_params, metric_ingests m
			inner join metric_streams s on s.id = m.stream_id
			inner join resources r on r.id = m.resource_id
			inner join scopes sc on sc.id = m.scope_id`

func mapMetricFieldExpression(field *search.FieldDefinition) (search.ResolvedExpression, error) {
	name := field.Name
	if name == "" {
		return search.ResolvedExpression{}, fmt.Errorf("empty field name: %w", ErrInvalidMetricQuery)
	}
	switch name {
	case "name":
		return search.Text("s.name"), nil
	case "unit":
		return search.Text("s.unit"), nil
	case "type":
		return search.Text("s.metric_type"), nil
	case "scope.name":
		return search.Text("s.scope_name"), nil
	case "scope.version":
		return search.Text("s.scope_version"), nil
	case "description":
		return search.Text("m.description"), nil
	// The two dropped counts moved off metric_ingests onto the resources and
	// scopes rows it now references, so they resolve through the joins rather
	// than as columns on m.
	case "resource.droppedAttributesCount":
		return search.NativeInteger("r.dropped_attributes_count"), nil
	case "scope.droppedAttributesCount":
		return search.NativeInteger("sc.dropped_attributes_count"), nil
	default:
		col := util.CamelToSnake(name)
		if err := util.ValidateColumnName(col, metricColumns); err != nil {
			return search.ResolvedExpression{}, fmt.Errorf("metric field %q: %w: %w", name, err, ErrInvalidMetricQuery)
		}
		return search.Text("m." + col), nil
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
func mapMetricAttributeExpressions(field *search.FieldDefinition, query *search.Query, params *[]search.NamedParam) ([]search.ResolvedExpression, error) {
	kind, mode, err := search.AttributeKind(field.Type)
	if err != nil {
		return nil, err
	}

	keyParam := fmt.Sprintf("attr_key_%d", len(*params))
	*params = append(*params, search.NamedParam{Name: keyParam, Value: field.Name})
	kindParam := ""
	if kind != "" {
		kindParam = fmt.Sprintf("attr_kind_%d", len(*params))
		*params = append(*params, search.NamedParam{Name: kindParam, Value: kind})
	}
	kindPredicate := search.AttributeKindPredicate(kindParam)
	valueExpression := search.AttributeValueExpression(kind)
	if mode == search.OTelArrayOperand {
		var attributeIDs string
		switch field.AttributeScope {
		case "resource", "metric":
			attributeIDs = "r.attribute_ids"
		case "scope":
			attributeIDs = "sc.attribute_ids"
		case "datapoint":
			attributeIDs = "d.attribute_ids"
		case "exemplar":
			attributeIDs = "e.attribute_ids"
		case "metadata":
			attributeIDs = "m.metadata_ids"
		default:
			return nil, fmt.Errorf("unknown attribute scope %s: %w", field.AttributeScope, ErrInvalidMetricQuery)
		}
		predicate, err := search.JSONValueArrayPredicate(attributeIDs, keyParam, kindParam, query, params)
		if err != nil {
			return nil, err
		}
		switch field.AttributeScope {
		case "resource", "metric":
			predicate = fmt.Sprintf("m.resource_id in (select r.id from resources r where %s)", predicate)
		case "scope":
			predicate = fmt.Sprintf("m.scope_id in (select sc.id from scopes sc where %s)", predicate)
		case "datapoint":
			predicate = fmt.Sprintf("m.id in (select d.metric_ingest_id from datapoints d where %s)", predicate)
		case "exemplar":
			predicate = fmt.Sprintf("m.id in (select d.metric_ingest_id from exemplars e join datapoints d on d.id = e.datapoint_id where %s)", predicate)
		}
		return []search.ResolvedExpression{search.Complete(predicate)}, nil
	}

	switch field.AttributeScope {
	case "resource", "metric":
		return []search.ResolvedExpression{search.AttributeExpression(fmt.Sprintf(
			`m.resource_id in (select r.id from resources r, unnest(r.attribute_ids) t(aid)
				join attributes a on a.id = t.aid where a.key = %s%s and
				%s {COND})`,
			keyParam, kindPredicate, valueExpression), kind, mode)}, nil
	case "scope":
		return []search.ResolvedExpression{search.AttributeExpression(fmt.Sprintf(
			`m.scope_id in (select sc.id from scopes sc, unnest(sc.attribute_ids) t(aid)
				join attributes a on a.id = t.aid where a.key = %s%s and
				%s {COND})`,
			keyParam, kindPredicate, valueExpression), kind, mode)}, nil
	case "datapoint":
		return []search.ResolvedExpression{search.AttributeExpression(fmt.Sprintf(matchIngestByLabel,
			"datapoints d", "d.attribute_ids", keyParam, kindPredicate, valueExpression), kind, mode)}, nil
	case "exemplar":
		return []search.ResolvedExpression{search.AttributeExpression(fmt.Sprintf(matchIngestByLabel,
			"exemplars e join datapoints d on d.id = e.datapoint_id", "e.attribute_ids", keyParam, kindPredicate, valueExpression), kind, mode)}, nil
	case "metadata":
		return []search.ResolvedExpression{search.AttributeExpression(fmt.Sprintf(`exists(
			select 1 from unnest(m.metadata_ids) t(aid) join attributes a on a.id = t.aid
			where a.key = %s%s and %s {COND}
		)`, keyParam, kindPredicate, valueExpression), kind, mode)}, nil
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
					a.key {COND} OR a.value::varchar {COND} OR
					exists(select 1 from json_each(a.value, '$.value') j where j.value::varchar {COND})
				)
			)
		)`

func mapMetricGlobalExpressions() ([]search.ResolvedExpression, error) {
	return search.TextExpressions([]string{
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
				a.key {COND} OR a.value::varchar {COND} OR
				exists(select 1 from json_each(a.value, '$.value') j where j.value::varchar {COND})
		)
	)`,
	}), nil
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

func int32Value(v driver.Value) (int32, error) {
	switch n := v.(type) {
	case int32:
		return n, nil
	case int64:
		if n < math.MinInt32 || n > math.MaxInt32 {
			return 0, fmt.Errorf("INTEGER value out of int32 range: %d", n)
		}
		return int32(n), nil
	default:
		return 0, fmt.Errorf("expected INTEGER, got %T", v)
	}
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

// getMetricParams selects which shape of response the projection builds.
//
// The CTE definitions are identical either way; only the final json_object
// changes. DuckDB prunes whatever the projection does not read, so asking for
// less is not merely a smaller payload -- it is a smaller plan. Measured on a
// 21-series histogram: the full projection plans in 212ms and runs in 314ms,
// the aggregate-only one in 67ms and 150ms.
type getMetricParams struct {
	TimeFilter string
	// AggregateOnly emits just the cross-series aggregates, which is all
	// GetMetricAggregate returns. It drops `timeseries` -- the field that
	// carries the per-series pipelines and most of the planning cost.
	AggregateOnly bool

	// NoHistogramMerge and NoScalarPools drop a chain whose output is already
	// known to be empty for this metric's shape, so the planner never builds
	// it. scalar_dps admits Gauge and Sum only, so a histogram's pools are
	// always []; the histogram merge needs bucket vectors, so a scalar's
	// aggregate is always null.
	//
	// Only worth setting alongside AggregateOnly. In the full projection
	// `timeseries` reaches into both chains anyway -- measured, dropping
	// scalarAggregate there saved nothing -- so the pruning has no room to
	// work until the per-series fields are gone.
	NoHistogramMerge bool
	NoScalarPools    bool
}

// aggregateShapeFor decides which chains a metric's aggregate can possibly
// need. A primary-key lookup on metric_streams, measured at 0.09ms, against
// 120ms saved on a scalar metric.
func aggregateShapeFor(ctx context.Context, db *sql.DB, streamID string) getMetricParams {
	p := getMetricParams{AggregateOnly: true}
	var metricType string
	err := db.QueryRowContext(ctx,
		`select metric_type from metric_streams where id = ?::uuid`, streamID).Scan(&metricType)
	if err != nil {
		// Unknown shape: ask for both, which is what this call did before the
		// pruning existed. A stream id that does not resolve fails in the main
		// query with ErrStreamIDNotFound, and that is the error worth showing.
		return p
	}
	switch metricType {
	case "Histogram", "ExponentialHistogram":
		p.NoScalarPools = true
	case "Gauge", "Sum":
		p.NoHistogramMerge = true
	}
	return p
}

// searchSummariesParams are the fragments SearchSummaries assembles into
// queries/metrics/search_summaries.sql.
type searchSummariesParams struct {
	// CTEs is the search_params CTE holding the time bounds.
	CTEs string
	// From is the shared FROM/JOIN chain for metric search.
	From string
	// Where is the predicate, "true" when there are no criteria.
	Where          string
	DatapointWhere string
	CandidateOrder string
	CandidateLimit string
	SummaryOrder   string
	SummaryLimit   string
}
