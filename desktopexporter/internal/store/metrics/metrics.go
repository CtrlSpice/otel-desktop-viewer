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
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/searchindex"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/CtrlSpice/otel-desktop-viewer/internal/discovery"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

var (
	ErrInvalidMetricQuery    = errors.New("invalid metric search query")
	ErrInvalidMetricLimit    = errors.New("invalid metric search limit")
	ErrMetricIDNotFound      = errors.New("metric ID not found")
	ErrUnsupportedMetricType = errors.New("unsupported metric type")
	ErrMetricsStoreInternal  = errors.New("metrics store internal error")
)

// flushIntervalMetrics counts metrics, not datapoints. One metric may contain
// thousands of datapoints.
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

// Ingest writes the metric data in m to the metrics, metric_series,
// metric_datapoints, and exemplars tables, including their owned values. The
// caller must hold any required lock on the connection.
//
// Ingest runs in two passes:
//
//  1. First pass collects every distinct (resource, scope, metric)
//     identity in the request and upserts them into metrics,
//     resolving each to its UUID. This is the only round-trip-per-batch
//     step; the appender path that follows is constant per identity.
//  2. Second pass walks the same hierarchy again, this time writing the
//     datapoints and exemplars with their attributes for each Metric.
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
	var identities []metricIdentity
	identityIndexes := make(map[metricLookupKey]int)
	var metricIdentityIndexes []int
	var seriesRows []seriesRow
	preRejected := make(map[int]error)
	cleanupArmed := false
	metricOrdinal := 0

	// Pass 1: collect every distinct identity in this OTLP request, plus
	// per-identity service_name (denormalized onto metrics). We
	// build the identity list eagerly so resolution sees the whole batch
	// and can resolve everything in two round trips.
	// Only histogram bounds use shared payload storage. Attribute hashes are
	// temporary series identity inputs; the maps themselves live on their owners.
	bounds := ingest.NewBoundsBatch(flushed)
	resourceIDs := map[int]duckdb.UUID{}
	resourcePayloadIDs := map[int]duckdb.UUID{}
	scopeIDs := map[scopeKey]duckdb.UUID{}
	// Prepare each received group's bytes once. Keep positional context even
	// when several received groups resolve to the same Metric identity.
	resourceInputs := map[int]map[string]any{}
	scopeInputs := map[scopeKey]map[string]any{}

	// One id array per datapoint, in walk order, handed to collectSeries below.
	var dpAttrIDs [][]duckdb.UUID

	for ri, resourceMetric := range m.ResourceMetrics().All() {
		resource := resourceMetric.Resource()
		serviceName := serviceNameFromAttrs(resource.Attributes())
		resourceInput, err := ingest.ResourceInput(resource, resourceMetric.SchemaUrl())
		if err != nil {
			return ingest.Rejected{}, err
		}
		resourceInputs[ri] = resourceInput
		resourceIDs[ri] = resourceInput["id"].(duckdb.UUID)
		resourcePayloadIDs[ri] = resourceInput["payload_id"].(duckdb.UUID)
		for si, scopeMetric := range resourceMetric.ScopeMetrics().All() {
			scope := scopeMetric.Scope()
			key := scopeKey{ri, si}
			scopeInput, err := ingest.ScopeInput(scope, scopeMetric.SchemaUrl())
			if err != nil {
				return ingest.Rejected{}, err
			}
			scopeInputs[key] = scopeInput
			scopeIDs[key] = scopeInput["id"].(duckdb.UUID)
			for _, metric := range scopeMetric.Metrics().All() {
				ordinal := metricOrdinal
				metricOrdinal++
				if err := ctx.Err(); err != nil {
					return ingest.Rejected{}, err
				}
				if !supportedReceivedMetricType(metric.Type().String()) {
					preRejected[ordinal] = fmt.Errorf("%w: %s", ErrUnsupportedMetricType, metric.Type())
				}
				metadata, err := ingest.AttributeBytes(metric.Metadata())
				if err != nil {
					return ingest.Rejected{}, err
				}
				identity := metricIdentityFromMetric(
					metric, resourceIDs[ri], resourcePayloadIDs[ri], scopeIDs[key])
				identity.Description = metric.Description()
				identity.Metadata = metadata
				identity.Resource, identity.Scope = resourceInput, scopeInput
				identity.ServiceName = serviceName
				identityKey := identity.lookupKey()
				index, exists := identityIndexes[identityKey]
				if exists {
					identities[index] = identity
				} else {
					index = len(identities)
					identityIndexes[identityKey] = index
					identities = append(identities, identity)
				}
				metricIdentityIndexes = append(metricIdentityIndexes, index)
				dpAttrIDs, err = collectDatapointIdentities(bounds, metric, dpAttrIDs)
				if err != nil {
					return ingest.Rejected{}, err
				}
			}
		}
	}
	if len(identities) == 0 {
		return ingest.Rejected{}, nil
	}
	if err := ctx.Err(); err != nil {
		return ingest.Rejected{}, err
	}
	// Find or insert metrics by their exact stored keys. Unique indexes
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
			cleanupCtx := context.WithoutCancel(ctx)
			err = errors.Join(err, ingest.InTransaction(cleanupCtx, conn, func() error {
				if err := cleanupProvisionalIdentities(cleanupCtx, dconn, prepareArg, flushed, identities, seriesRows); err != nil {
					return err
				}
				return searchindex.RemoveMissingMetricDocuments(cleanupCtx, conn)
			}))
		}
	}()

	// Shared histogram bounds must exist before the datapoints that reference
	// them. Owned attributes need no separate rows or dictionary flush.
	cleanupArmed = true
	if err := bounds.Flush(ctx, conn); err != nil {
		return ingest.Rejected{}, fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
	}
	if err := ingest.InTransaction(ctx, conn, func() error {
		if err := resolveMetricIDs(ctx, dconn, prepareArg, identities); err != nil {
			return err
		}
		var catalogue searchindex.Batch
		for _, identity := range identities {
			if !identity.DescriptorChanged {
				continue
			}
			if err := catalogue.Metric(identity.ID, identity.Metadata, identity.Resource, identity.Scope, identity.Name, identity.Description, identity.Unit); err != nil {
				return err
			}
		}
		return catalogue.Write(ctx, conn)
	}); err != nil {
		return ingest.Rejected{}, err
	}

	// Pass 2: open the appenders and walk the request again, writing
	// datapoints. We resolve each metric's metric_id by
	// matching its exact identity in the resolved batch.

	// Resolve every datapoint's series before any of them are appended.
	//
	// This has to sit here specifically: a series id needs the Metric id, which
	// is only known after the upsert round trip above, and datapoints.series_id
	// is a foreign key, so the rows must be committed before the appender
	// flushes. Between the two is the only place it fits.
	//
	// The walk consumes the attribute hashes the first pass derived, and carries
	// them forward again so pass 2 reads them by position too. Neither this walk
	// nor pass 2 hashes a label set: datapoints are the highest-volume path in
	// the store, and one derivation each is all they get.
	dpIdents, collectedSeries, err := collectSeries(ctx, m, identities, identityIndexes,
		resourceIDs, resourcePayloadIDs, scopeIDs, dpAttrIDs)
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
	return ingest.BisectingWrite(ctx, countMetrics(m), preRejected, func(lo, hi int) error {
		err := ingest.InTransaction(ctx, conn, func() error {
			return appendPass(ctx, conn, m, identities, identityIndexes,
				resourceInputs, scopeInputs, dpIdents,
				func(ordinal int) bool {
					_, rejected := preRejected[ordinal]
					return !rejected && ordinal >= lo && ordinal < hi
				})
		})
		if err == nil {
			// A committed empty Metric has no series to protect it during cleanup.
			for ordinal := lo; ordinal < hi; ordinal++ {
				if _, rejected := preRejected[ordinal]; !rejected {
					identities[metricIdentityIndexes[ordinal]].Accepted = true
				}
			}
		}
		return err
	})
}

// appendPass writes the metrics keep selects, by ordinal. Skipped metrics still
// step dpCur over their datapoints, since dpIdents is consumed by position.
func appendPass(
	ctx context.Context,
	conn driver.Conn,
	m pmetric.Metrics,
	metricIDs []metricIdentity,
	metricIndexes map[metricLookupKey]int,
	resourceInputs map[int]map[string]any,
	scopeInputs map[scopeKey]map[string]any,
	dpIdents []dpIdentity,
	keep func(ordinal int) bool,
) (err error) {
	tables := []string{"exemplars", "metric_datapoints"}
	dpCur := 0

	appenders, err := ingest.NewAppenders(conn, tables)
	if err != nil {
		return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
	}
	var catalogue searchindex.Batch
	defer func() {
		err = errors.Join(err, ingest.CloseAppenders(appenders, tables))
		if err == nil {
			if indexErr := catalogue.Write(ctx, conn); indexErr != nil {
				err = fmt.Errorf("%w: %w", ingest.ErrNotRowFault, indexErr)
			}
		}
	}()

	metricCount := 0
	metricOrdinal := 0
	for ri, resourceMetric := range m.ResourceMetrics().All() {
		resourceInput := resourceInputs[ri]
		resourceID := resourceInput["id"].(duckdb.UUID)
		resourcePayloadID := resourceInput["payload_id"].(duckdb.UUID)
		for si, scopeMetric := range resourceMetric.ScopeMetrics().All() {
			key := scopeKey{ri, si}
			scopeInput := scopeInputs[key]
			scopeID := scopeInput["id"].(duckdb.UUID)
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

				identity := metricIdentityFromMetric(metric, resourceID, resourcePayloadID, scopeID)
				metricIndex, ok := metricIndexes[identity.lookupKey()]
				if !ok {
					return fmt.Errorf("Ingest: %w: Metric id missing for identity %+v", ErrMetricsStoreInternal, identity)
				}
				metricID := metricIDs[metricIndex].ID

				switch metric.Type() {
				case pmetric.MetricTypeGauge:
					if err := ingestGaugeDatapoints(appenders, metricID, metric.Gauge().DataPoints(), dpIdents, &dpCur, resourceInput, scopeInput, &catalogue); err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
					}
				case pmetric.MetricTypeSum:
					if err := ingestSumDatapoints(appenders, metricID, metric.Sum().DataPoints(), dpIdents, &dpCur, resourceInput, scopeInput, &catalogue); err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
					}
				case pmetric.MetricTypeHistogram:
					if err := ingestHistogramDatapoints(appenders, metricID, metric.Histogram().DataPoints(), dpIdents, &dpCur, resourceInput, scopeInput, &catalogue); err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
					}
				case pmetric.MetricTypeExponentialHistogram:
					if err := ingestExponentialHistogramDatapoints(appenders, metricID, metric.ExponentialHistogram().DataPoints(), dpIdents, &dpCur, resourceInput, scopeInput, &catalogue); err != nil {
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

// collectDatapointIdentities computes series identity inputs in datapoint walk order
// and collects explicit histogram bounds before opening datapoint appenders.
func collectDatapointIdentities(bounds *ingest.BoundsBatch, metric pmetric.Metric, out [][]duckdb.UUID) ([][]duckdb.UUID, error) {
	// Bounds must land before the datapoint appenders because a foreign key
	// enforces their relationship. Pass 2 recomputes their deterministic IDs.
	if metric.Type() == pmetric.MetricTypeHistogram {
		for _, dp := range metric.Histogram().DataPoints().All() {
			bounds.AddBounds(dp.ExplicitBounds().AsRaw())
		}
	}
	var identityErr error
	eachDatapoint(metric, func(attrs pcommon.Map, _ pmetric.ExemplarSlice) {
		if identityErr != nil {
			return
		}
		ids, err := ingest.AttributeIdentities(attrs)
		if err != nil {
			identityErr = err
			return
		}
		out = append(out, ids)
	})
	return out, identityErr
}

// dpIdentity is what pass 1 works out for one datapoint and pass 2 writes.
type dpIdentity struct {
	id          duckdb.UUID
	series      duckdb.UUID
	seriesIndex int
	attrs       []byte
}

// seriesRow is a metric_series row awaiting insert.
type seriesRow struct {
	id             duckdb.UUID
	metric         duckdb.UUID
	attrs          []duckdb.UUID
	payload        []byte
	existed        bool
	existenceKnown bool
}

func seriesKey(row seriesRow) string {
	// UUID strings have a fixed width, so concatenation is an exact in-memory
	// lookup key. It is not stored or exposed as the series ID.
	var key strings.Builder
	key.Grow(36 * (len(row.attrs) + 1))
	key.WriteString(ingest.FormatUUID(row.metric))
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
	metricIDs []metricIdentity,
	metricIndexes map[metricLookupKey]int,
	resourceIDs map[int]duckdb.UUID,
	resourcePayloadIDs map[int]duckdb.UUID,
	scopeIDs map[scopeKey]duckdb.UUID,
	dpAttrIDs [][]duckdb.UUID,
) ([]dpIdentity, []seriesRow, error) {
	idents := make([]dpIdentity, 0, len(dpAttrIDs))
	var rows []seriesRow
	rowIndexes := make(map[string]int)
	cur := 0

	for ri, resourceMetric := range m.ResourceMetrics().All() {
		for si, scopeMetric := range resourceMetric.ScopeMetrics().All() {
			key := scopeKey{ri, si}
			for _, metric := range scopeMetric.Metrics().All() {
				if err := ctx.Err(); err != nil {
					return nil, nil, err
				}
				identity := metricIdentityFromMetric(
					metric, resourceIDs[ri], resourcePayloadIDs[ri], scopeIDs[key])
				metricIndex, ok := metricIndexes[identity.lookupKey()]
				if !ok {
					return nil, nil, fmt.Errorf("collectSeries: %w: Metric id missing for identity %+v",
						ErrMetricsStoreInternal, identity)
				}
				metricID := metricIDs[metricIndex].ID
				var overrun bool
				var attributeErr error
				eachDatapoint(metric, func(attributes pcommon.Map, _ pmetric.ExemplarSlice) {
					if cur >= len(dpAttrIDs) {
						overrun = true
						return
					}
					ids := dpAttrIDs[cur]
					cur++
					payload, err := ingest.AttributeBytes(attributes)
					if err != nil {
						attributeErr = err
						return
					}
					row := seriesRow{metric: metricID, attrs: ids, payload: payload}
					rowKey := seriesKey(row)
					rowIndex, ok := rowIndexes[rowKey]
					if !ok {
						rowIndex = len(rows)
						rows = append(rows, row)
						rowIndexes[rowKey] = rowIndex
					}
					idents = append(idents, dpIdentity{seriesIndex: rowIndex, attrs: payload})
				})
				if attributeErr != nil {
					return nil, nil, attributeErr
				}
				if overrun {
					return nil, nil, fmt.Errorf("collectSeries: %w: more datapoints than the identity pass saw (%d)",
						ErrMetricsStoreInternal, len(dpAttrIDs))
				}
			}
		}
	}
	if cur != len(dpAttrIDs) {
		return nil, nil, fmt.Errorf("collectSeries: %w: identity pass mismatch (%d/%d)",
			ErrMetricsStoreInternal, cur, len(dpAttrIDs))
	}
	return idents, rows, nil
}

// eachDatapoint visits a metric's datapoints in the order the ingest* helpers
// write them. Every walk goes through this, so they cannot drift apart -- which
// matters because the identity pass and collectSeries pair up by position,
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
// The appender has no conflict handling, and a series recurs on every batch from
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
	// Series identities and owned label sets travel as bound typed arrays.
	ordinals := make([]int32, 0, len(rows))
	metricIDs := make([]string, 0, len(rows))
	attrs := make([]string, 0, len(rows))
	for i, r := range rows {
		ordinals = append(ordinals, int32(i))
		metricIDs = append(metricIDs, ingest.FormatUUID(r.metric))
		attrs = append(attrs, strings.Join(uuidStrings(r.attrs), ","))
	}

	selectArgs, err := appendNamedValues(nil, prepareArg, ordinals, metricIDs, attrs)
	if err != nil {
		return fmt.Errorf("Ingest: %w: prep series select: %w", ErrMetricsStoreInternal, err)
	}
	const resolve = `select w.ordinal::bigint, s.id::varchar from metric_series s join (
		select unnest(?::integer[]) as ordinal,
		       unnest(?::varchar[])::uuid as metric_id,
		       unnest(?::varchar[]) as attribute_key
	) w on s.metric_id = w.metric_id and s.attribute_key = w.attribute_key`
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
	a, err := ingest.NewTypedQueryAppender(dconn, ingest.OwnedAppendQuery("metric_series", []string{"attributes"})+`
		on conflict (metric_id, attribute_key) do nothing`,
		`select * replace(null::blob as attributes) from metric_series where false`)
	if err != nil {
		return err
	}
	for i, row := range rows {
		if err := ctx.Err(); err != nil {
			return errors.Join(err, a.Close())
		}
		if err := a.AppendRow(duckdb.UUID(uuid.New()), row.metric, attrs[i], row.payload); err != nil {
			return errors.Join(err, a.Close())
		}
	}
	if err := a.Close(); err != nil {
		return err
	}
	if err := resolveRows(false, true); err != nil {
		return err
	}
	return nil
}

// metricIdentity is the complete stored identity tuple for one OTel Metric.
type metricIdentity struct {
	DescriptorChanged      bool
	ID                     duckdb.UUID
	Existed                bool
	ExistenceKnown         bool
	Accepted               bool
	ResourceID             duckdb.UUID
	ResourcePayloadID      duckdb.UUID
	ScopeID                duckdb.UUID
	Name                   string
	Description            string
	Unit                   string
	Metadata               []byte
	Resource               map[string]any
	Scope                  map[string]any
	MetricType             string
	AggregationTemporality int32
	IsMonotonic            string
	ServiceName            string
}

type metricLookupKey struct {
	resourcePayloadID      duckdb.UUID
	scopeID                duckdb.UUID
	name                   string
	unit                   string
	metricType             string
	aggregationTemporality int32
	isMonotonic            string
}

func (s metricIdentity) lookupKey() metricLookupKey {
	return metricLookupKey{
		resourcePayloadID:      s.ResourcePayloadID,
		scopeID:                s.ScopeID,
		name:                   s.Name,
		unit:                   s.Unit,
		metricType:             s.MetricType,
		aggregationTemporality: s.AggregationTemporality,
		isMonotonic:            s.IsMonotonic,
	}
}

func resolveMetricIDs(
	ctx context.Context,
	dconn *duckdb.Conn,
	prepareArg func(any) (driver.Value, error),
	identities []metricIdentity,
) error {
	ordinals := make([]int32, 0, len(identities))
	resourcePayloadIDs := make([]string, 0, len(identities))
	scopeIDs := make([]string, 0, len(identities))
	names := make([]string, 0, len(identities))
	units := make([]string, 0, len(identities))
	types := make([]string, 0, len(identities))
	temporalities := make([]int32, 0, len(identities))
	monotonics := make([]bool, 0, len(identities))
	for i, identity := range identities {
		identities[i].DescriptorChanged = true
		if err := ctx.Err(); err != nil {
			return err
		}
		ordinals = append(ordinals, int32(i))
		resourcePayloadIDs = append(resourcePayloadIDs, ingest.FormatUUID(identity.ResourcePayloadID))
		scopeIDs = append(scopeIDs, ingest.FormatUUID(identity.ScopeID))
		names = append(names, identity.Name)
		units = append(units, identity.Unit)
		types = append(types, identity.MetricType)
		temporalities = append(temporalities, identity.AggregationTemporality)
		monotonics = append(monotonics, isMonotonicToBool(identity.IsMonotonic))
	}

	keyArgs := []any{resourcePayloadIDs, scopeIDs, names, units, types, temporalities, monotonics}
	selectArgs, err := appendNamedValues(nil, prepareArg, append([]any{ordinals}, keyArgs...)...)
	if err != nil {
		return fmt.Errorf("Ingest: %w: prep Metric select: %w", ErrMetricsStoreInternal, err)
	}
	const selectSQL = `select w.ordinal::bigint, s.id::varchar,s.description,s.resource_id::varchar,
	 variant_to_parquet_variant(variant_normalize(s.metadata))::struct(metadata blob,value blob)
	 from metrics s join (
		select unnest(?::integer[]) as ordinal,
		       unnest(?::varchar[])::uuid as resource_payload_id,
		       unnest(?::varchar[])::uuid as scope_id,
		       unnest(?::varchar[]) as name, unnest(?::varchar[]) as unit,
		       unnest(?::varchar[]) as metric_type, unnest(?::integer[]) as aggregation_temporality,
		       unnest(?::boolean[]) as is_monotonic
	) w on s.resource_payload_id = w.resource_payload_id and s.scope_id = w.scope_id and s.name = w.name
	   and s.unit = w.unit and s.metric_type = w.metric_type
	   and s.aggregation_temporality = w.aggregation_temporality
	   and s.is_monotonic = w.is_monotonic`
	resolveRows := func(markExisting, requireAll bool) error {
		rows, err := dconn.QueryContext(ctx, selectSQL, selectArgs)
		if err != nil {
			return fmt.Errorf("Ingest: %w: Metric select: %w", ErrMetricsStoreInternal, err)
		}
		resolved := make([]bool, len(identities))
		defer rows.Close()
		for {
			dest := make([]driver.Value, 5)
			err := rows.Next(dest)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return fmt.Errorf("Ingest: %w: Metric select: %w", ErrMetricsStoreInternal, err)
			}
			ordinal := int(dest[0].(int64))
			if resolved[ordinal] {
				return fmt.Errorf("Ingest: %w: duplicate exact Metric key", ErrMetricsStoreInternal)
			}
			parsed, err := uuid.Parse(dest[1].(string))
			if err != nil {
				return fmt.Errorf("Ingest: %w: parse Metric ID: %w", ErrMetricsStoreInternal, err)
			}
			identities[ordinal].ID = duckdb.UUID(parsed)
			identities[ordinal].Existed = identities[ordinal].Existed || markExisting
			if markExisting {
				identity := &identities[ordinal]
				if dest[2] == identity.Description && dest[3] == ingest.FormatUUID(identity.ResourceID) {
					parts := dest[4].(map[string]any)
					equal, err := searchindex.NativeMatches(identity.Metadata, parts["metadata"].([]byte), parts["value"].([]byte))
					if err != nil {
						return err
					}
					identity.DescriptorChanged = !equal
				}
			}
			resolved[ordinal] = true
		}
		if requireAll && slices.Contains(resolved, false) {
			return fmt.Errorf("Ingest: %w: Metric ID not resolved", ErrMetricsStoreInternal)
		}
		return nil
	}
	if err := resolveRows(true, false); err != nil {
		return err
	}
	for i := range identities {
		identities[i].ExistenceKnown = true
	}
	a, err := ingest.NewTypedQueryAppender(dconn, ingest.OwnedAppendQuery("metrics", []string{"metadata", "resource.attributes", "scope.attributes"})+`
		on conflict (resource_payload_id, scope_id, name, unit, metric_type, aggregation_temporality, is_monotonic)
		do update set resource_id = excluded.resource_id, resource = excluded.resource,
			scope = excluded.scope, description = excluded.description,
			metadata = excluded.metadata, service_name = excluded.service_name`,
		`select * replace(null::blob as metadata,
			null::otel_resource_input as resource, null::otel_scope_input as scope) from metrics where false`)
	if err != nil {
		return err
	}
	for _, identity := range identities {
		if err := ctx.Err(); err != nil {
			return errors.Join(err, a.Close())
		}
		if err := a.AppendRow(duckdb.UUID(uuid.New()), identity.ResourceID, identity.ResourcePayloadID,
			identity.ScopeID, identity.Name, identity.Description, identity.Unit, identity.Metadata,
			identity.MetricType, identity.AggregationTemporality, isMonotonicToBool(identity.IsMonotonic),
			identity.ServiceName, identity.Resource, identity.Scope); err != nil {
			return errors.Join(err, a.Close())
		}
	}
	if err := a.Close(); err != nil {
		return err
	}
	return resolveRows(false, true)
}

func cleanupProvisionalIdentities(
	ctx context.Context,
	dconn *duckdb.Conn,
	prepareArg func(any) (driver.Value, error),
	flushed *ingest.FlushedIDs,
	identities []metricIdentity,
	seriesRows []seriesRow,
) error {
	var cleanupErr error
	for _, row := range seriesRows {
		if !row.existenceKnown || row.existed {
			continue
		}
		args, err := appendNamedValues(nil, prepareArg,
			ingest.FormatUUID(row.metric), strings.Join(uuidStrings(row.attrs), ","))
		if err != nil {
			cleanupErr = errors.Join(cleanupErr,
				fmt.Errorf("cleanupProvisionalIdentities: %w: %w", ErrMetricsStoreInternal, err))
			continue
		}
		_, err = dconn.ExecContext(ctx, `delete from metric_series s
			where s.metric_id = ?::uuid
			  and s.attribute_key = ?
			  and not exists (select 1 from metric_datapoints d where d.series_id = s.id)`, args)
		if err != nil {
			cleanupErr = errors.Join(cleanupErr,
				fmt.Errorf("cleanupProvisionalIdentities: %w: %w", ErrMetricsStoreInternal, err))
		}
	}
	for _, identity := range identities {
		if !identity.ExistenceKnown || identity.Existed || identity.Accepted {
			continue
		}
		args, err := appendNamedValues(nil, prepareArg,
			ingest.FormatUUID(identity.ResourcePayloadID), ingest.FormatUUID(identity.ScopeID),
			identity.Name, identity.Unit, identity.MetricType,
			identity.AggregationTemporality, isMonotonicToBool(identity.IsMonotonic))
		if err != nil {
			cleanupErr = errors.Join(cleanupErr,
				fmt.Errorf("cleanupProvisionalIdentities: %w: %w", ErrMetricsStoreInternal, err))
			continue
		}
		_, err = dconn.ExecContext(ctx, `delete from metrics s
			where s.resource_payload_id = ?::uuid and s.scope_id = ?::uuid
			  and s.name = ? and s.unit = ? and s.metric_type = ?
			  and s.aggregation_temporality = ? and s.is_monotonic = ?
			  and not exists (select 1 from metric_series ms where ms.metric_id = s.id)`, args)
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
	if v, ok := util.LastValue(attrs, "service.name"); ok {
		return v.AsString()
	}
	return ""
}

// metricIdentityFromMetric extracts the exact identity tuple from one Metric.
// aggregation_temporality remains its received signed int32 code; metric type
// distinguishes non-applicable stored placeholders from received values.
func metricIdentityFromMetric(
	metric pmetric.Metric,
	resourceID, resourcePayloadID, scopeID duckdb.UUID,
) metricIdentity {
	id := metricIdentity{
		ResourceID: resourceID, ResourcePayloadID: resourcePayloadID,
		ScopeID: scopeID,
		Name:    metric.Name(), Unit: metric.Unit(), MetricType: metric.Type().String(),
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
// datapoint, plus their attributes.
func ingestExemplars(appenders map[string]*duckdb.Appender, datapointID duckdb.UUID, exemplars pmetric.ExemplarSlice, metricID duckdb.UUID, catalogue *searchindex.Batch) error {
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
		attributes, err := ingest.AttributeBytes(ex.FilteredAttributes())
		if err != nil {
			return err
		}
		doubleVal, intVal := exemplarValue(ex)
		if err := appenders["exemplars"].AppendRow(
			exemplarID, datapointID, uint64(ex.Timestamp()), doubleVal, intVal, traceUUID, spanID,
			attributes,
		); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		if err := catalogue.Exemplar(metricID, datapointID, exemplarID, attributes); err != nil {
			return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
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

func ingestGaugeDatapoints(appenders map[string]*duckdb.Appender, metricID duckdb.UUID, dps pmetric.NumberDataPointSlice, idents []dpIdentity, cur *int, resource, scope map[string]any, catalogue *searchindex.Batch) error {
	for _, dp := range dps.All() {
		doubleVal, intVal, valType := numberDataPointValue(dp)
		datapointID := duckdb.UUID(uuid.New())
		idents[*cur].id = datapointID
		ident := idents[*cur]
		*cur++
		if err := appenders["metric_datapoints"].AppendRow(
			datapointID, metricID, ident.series, uint64(dp.Timestamp()), uint64(dp.StartTimestamp()), uint32(dp.Flags()),
			doubleVal, intVal, valType, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			ident.attrs, resource, scope,
		); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		if err := catalogue.Datapoint(metricID, datapointID, ident.attrs, resource, scope); err != nil {
			return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
		}
		if err := ingestExemplars(appenders, datapointID, dp.Exemplars(), metricID, catalogue); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
	}
	return nil
}

// Sum, Histogram, and ExponentialHistogram share the same datapoint walk.
// Their appenders differ only in the columns they populate.

func ingestSumDatapoints(appenders map[string]*duckdb.Appender, metricID duckdb.UUID, dps pmetric.NumberDataPointSlice, idents []dpIdentity, cur *int, resource, scope map[string]any, catalogue *searchindex.Batch) error {
	for _, dp := range dps.All() {
		doubleVal, intVal, valType := numberDataPointValue(dp)
		datapointID := duckdb.UUID(uuid.New())
		idents[*cur].id = datapointID
		ident := idents[*cur]
		*cur++
		if err := appenders["metric_datapoints"].AppendRow(
			datapointID, metricID, ident.series, uint64(dp.Timestamp()), uint64(dp.StartTimestamp()), uint32(dp.Flags()),
			doubleVal, intVal, valType,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			ident.attrs, resource, scope,
		); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		if err := catalogue.Datapoint(metricID, datapointID, ident.attrs, resource, scope); err != nil {
			return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
		}
		if err := ingestExemplars(appenders, datapointID, dp.Exemplars(), metricID, catalogue); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
	}
	return nil
}

func ingestHistogramDatapoints(appenders map[string]*duckdb.Appender, metricID duckdb.UUID, dps pmetric.HistogramDataPointSlice, idents []dpIdentity, cur *int, resource, scope map[string]any, catalogue *searchindex.Batch) error {
	for _, dp := range dps.All() {
		datapointID := duckdb.UUID(uuid.New())
		idents[*cur].id = datapointID
		ident := idents[*cur]
		*cur++
		if err := appenders["metric_datapoints"].AppendRow(
			datapointID, metricID, ident.series, uint64(dp.Timestamp()), uint64(dp.StartTimestamp()), uint32(dp.Flags()),
			nil, nil, nil,
			dp.Count(), optionalFloat64(dp.HasSum(), dp.Sum()), optionalFloat64(dp.HasMin(), dp.Min()), optionalFloat64(dp.HasMax(), dp.Max()), dp.BucketCounts().AsRaw(), ingest.BoundsID(dp.ExplicitBounds().AsRaw()),
			nil, nil, nil, nil, nil, nil, nil,
			ident.attrs, resource, scope,
		); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		if err := catalogue.Datapoint(metricID, datapointID, ident.attrs, resource, scope); err != nil {
			return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
		}
		if err := ingestExemplars(appenders, datapointID, dp.Exemplars(), metricID, catalogue); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
	}
	return nil
}

func ingestExponentialHistogramDatapoints(appenders map[string]*duckdb.Appender, metricID duckdb.UUID, dps pmetric.ExponentialHistogramDataPointSlice, idents []dpIdentity, cur *int, resource, scope map[string]any, catalogue *searchindex.Batch) error {
	for _, dp := range dps.All() {
		pos, neg := dp.Positive(), dp.Negative()
		datapointID := duckdb.UUID(uuid.New())
		idents[*cur].id = datapointID
		ident := idents[*cur]
		*cur++
		if err := appenders["metric_datapoints"].AppendRow(
			datapointID, metricID, ident.series, uint64(dp.Timestamp()), uint64(dp.StartTimestamp()), uint32(dp.Flags()),
			nil, nil, nil,
			dp.Count(), optionalFloat64(dp.HasSum(), dp.Sum()), optionalFloat64(dp.HasMin(), dp.Min()), optionalFloat64(dp.HasMax(), dp.Max()), nil, nil,
			dp.Scale(), dp.ZeroCount(), dp.ZeroThreshold(), pos.Offset(), pos.BucketCounts().AsRaw(), neg.Offset(), neg.BucketCounts().AsRaw(),
			ident.attrs, resource, scope,
		); err != nil {
			return fmt.Errorf("Ingest: %w: %w", ErrMetricsStoreInternal, err)
		}
		if err := catalogue.Datapoint(metricID, datapointID, ident.attrs, resource, scope); err != nil {
			return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
		}
		if err := ingestExemplars(appenders, datapointID, dp.Exemplars(), metricID, catalogue); err != nil {
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

// SearchSummaries returns lightweight per-Metric summaries for the drawer
// cards: identity fields, description, seriesCount, lastValue (Gauge/Sum),
// and lastSeen. One row per metrics row that has at least one
// in-range datapoint and matches the optional search criteria.
//
// Filtering and summary aggregation run over matched Metrics and their
// in-range datapoints.
func SearchSummaries(ctx context.Context, db *sql.DB, timeRange timerange.TimeRange, criteria any) (json.RawMessage, error) {
	return searchSummaries(ctx, db, timeRange, criteria, search.ResultOptions{})
}

// SearchSummariesWithLimit returns at most limit Metric summaries.
func SearchSummariesWithLimit(ctx context.Context, db *sql.DB, timeRange timerange.TimeRange, criteria any, limit int64) (json.RawMessage, error) {
	return searchSummaries(ctx, db, timeRange, criteria, search.ResultOptions{Limit: &limit})
}

func SearchSummariesWithOptions(ctx context.Context, db *sql.DB, timeRange timerange.TimeRange, criteria any, options search.ResultOptions) (json.RawMessage, error) {
	return searchSummaries(ctx, db, timeRange, criteria, options)
}

func searchSummaries(ctx context.Context, db *sql.DB, timeRange timerange.TimeRange, criteria any, options search.ResultOptions) (json.RawMessage, error) {
	query, args, err := searchSummariesSQL(timeRange, criteria, options)
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

// searchSummariesSQL builds the query and bound arguments without database I/O.
// Validation, filtering, sorting, and limit placement depend only on the inputs.
func searchSummariesSQL(timeRange timerange.TimeRange, criteria any, options search.ResultOptions) (string, []any, error) {
	var searchTree *search.QueryNode
	if criteria != nil {
		var err error
		searchTree, err = search.ParseQueryTree(criteria)
		if err != nil {
			return "", nil, fmt.Errorf("SearchSummaries: %w: %w", ErrInvalidMetricQuery, err)
		}
	}
	cteSQL, whereClause, args, err := buildMetricSQL(searchTree, timeRange)
	if err != nil {
		return "", nil, fmt.Errorf("SearchSummaries: %w: %w", ErrInvalidMetricQuery, err)
	}

	candidateOrder, summaryOrder, earlyLimit, err := metricSummaryOrderBy(options.Sort)
	if err != nil {
		return "", nil, err
	}
	candidateLimit := ""
	summaryLimit := ""
	if options.Limit != nil {
		if *options.Limit < 1 {
			return "", nil, fmt.Errorf("SearchSummaries: limit must be positive: %w", ErrInvalidMetricLimit)
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
		return "", nil, err
	}
	return query, args, nil
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
		"lastSeen":       {candidate: "mldp.last_dp_ts", summary: "last_dp_ts", early: true},
		"name":           {candidate: "fm.name", summary: "name", early: true},
		"metricType":     {candidate: "fm.metric_type", summary: "metric_type", early: true},
		"serviceName":    {candidate: "fm.service_name", summary: "service_name", early: true},
		"description":    {summary: "coalesce(description, '')"},
		"dataPointCount": {summary: "coalesce(datapoint_count, 0)"},
		"seriesCount":    {summary: "coalesce(series_count, 0)"},
	}[field]
	if !ok {
		return "", "", false, fmt.Errorf("unsupported metric sort field %q: %w", field, search.ErrInvalidSort)
	}
	if spec.early {
		candidateOrder = fmt.Sprintf("%s %s nulls last, fm.id asc", spec.candidate, direction)
	}
	summaryOrder = fmt.Sprintf("%s %s nulls last, id asc", spec.summary, direction)
	return candidateOrder, summaryOrder, spec.early, nil
}

// fieldValueQueries names the metric columns whose values the search box may
// complete, keyed by the search grammar's field name. An allowlist of whole
// baked queries rather than an identifier spliced into one: a field name from
// the wire never becomes SQL. metrics holds one row per logical Metric,
// so these scan a tiny table, not the datapoints.
var fieldValueQueries = map[string]string{
	"name": `
		select cast(coalesce(to_json(list(sub.v order by sub.v)), to_json([])) as varchar)
		from (
			select distinct name as v
			from metrics
			where name ilike '%' || ? || '%' escape '\'
			order by name
			limit ?
		) sub
	`,
	"unit": `
		select cast(coalesce(to_json(list(sub.v order by sub.v)), to_json([])) as varchar)
		from (
			select distinct unit as v
			from metrics
			where unit <> '' and unit ilike '%' || ? || '%' escape '\'
			order by unit
			limit ?
		) sub
	`,
}

// GetFieldValueCompletions returns distinct values of one completable metric column
// matching term. Same contract as spans.GetFieldValueCompletions; alphabetical rather
// than by frequency, because metrics has no row count to rank by and
// Metric names read best sorted.
func GetFieldValueCompletions(ctx context.Context, db *sql.DB, field, term string, limit int64) (json.RawMessage, error) {
	query, ok := fieldValueQueries[field]
	if !ok {
		return nil, fmt.Errorf("GetFieldValueCompletions: %w: field %q has no value completion", ErrInvalidMetricQuery, field)
	}
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(term)
	var raw []byte
	if err := db.QueryRowContext(ctx, query, escaped, limit).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetFieldValueCompletions: %w: %w", ErrMetricsStoreInternal, err)
	}
	return json.RawMessage(raw), nil
}

// GetMetricView returns the chart/UI projection for a Metric in the time window.
// An unknown metricID returns ErrMetricIDNotFound; a known Metric with no
// datapoints in the window returns valid MetricViewData with an empty timeseries
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
// narrows nothing else: the All line keeps folding every series in the Metric,
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
func GetMetricView(ctx context.Context, db *sql.DB, metricID string, timeRange timerange.TimeRange, targetBuckets int64, seriesIDs []string, quantiles []float64, tzOffsetNs int64, viewBuckets int64, sparklineBuckets int64, selectedSeriesIDs []string, tzName string, datapointSeriesIDs []string, datapointSeriesLimit int64) (json.RawMessage, error) {
	return getMetricView(ctx, db, metricViewShapeFor(ctx, db, metricID), metricID, timeRange, targetBuckets, seriesIDs, quantiles, tzOffsetNs, viewBuckets, sparklineBuckets, selectedSeriesIDs, tzName, datapointSeriesIDs, datapointSeriesLimit)
}

// GetMetric returns the exact stored Metric identity and a catalogue of its
// generated database-local series. Datapoint counts and first/last datapoint
// timestamps are computed from all retained points; no datapoints are returned.
func GetMetric(ctx context.Context, db *sql.DB, metricID string) (json.RawMessage, error) {
	return getReceivedMetric(ctx, db, queries.GetMetric, "GetMetric", metricID)
}

// GetMetricSeries returns one exact series' retained received datapoints in the
// requested timestamp window.
func GetMetricSeries(ctx context.Context, db *sql.DB, metricID, seriesID string, timeRange timerange.TimeRange) (json.RawMessage, error) {
	metricType, err := receivedMetricType(ctx, db, "GetMetricSeries", metricID)
	if err != nil {
		return nil, err
	}
	query, err := queries.Render(queries.GetMetricSeries, nil)
	if err != nil {
		return nil, fmt.Errorf("GetMetricSeries: %w: %w", ErrMetricsStoreInternal, err)
	}
	var startTime, endTime any
	if timeRange.Start != nil {
		startTime = []uint64{*timeRange.Start}
	}
	if timeRange.End != nil {
		endTime = []uint64{*timeRange.End}
	}
	var queriedMetricType string
	var raw []byte
	if err := db.QueryRowContext(ctx, query, metricID, seriesID, startTime, endTime).Scan(&queriedMetricType, &raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetMetricSeries: %w", ErrMetricIDNotFound)
		}
		return nil, fmt.Errorf("GetMetricSeries: %w: %w", ErrMetricsStoreInternal, err)
	}
	if queriedMetricType != metricType {
		return nil, fmt.Errorf("GetMetricSeries: %w: Metric type changed during query", ErrMetricsStoreInternal)
	}
	if raw == nil || string(raw) == "null" {
		return nil, fmt.Errorf("GetMetricSeries: %w: query returned null", ErrMetricsStoreInternal)
	}
	return json.RawMessage(raw), nil
}

func receivedMetricType(ctx context.Context, db *sql.DB, operation, metricID string) (string, error) {
	var metricType string
	if err := db.QueryRowContext(ctx,
		`select metric_type from metrics where id = ?::uuid`, metricID).Scan(&metricType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%s: %w", operation, ErrMetricIDNotFound)
		}
		return "", fmt.Errorf("%s: %w: %w", operation, ErrMetricsStoreInternal, err)
	}
	if !supportedReceivedMetricType(metricType) {
		return "", fmt.Errorf("%s: %w: %s", operation, ErrUnsupportedMetricType, metricType)
	}
	return metricType, nil
}

func getReceivedMetric(ctx context.Context, db *sql.DB, queryName queries.Name, operation, metricID string) (json.RawMessage, error) {
	query, err := queries.Render(queryName, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %w", operation, ErrMetricsStoreInternal, err)
	}
	var metricType string
	var raw []byte
	if err := db.QueryRowContext(ctx, query, metricID).Scan(&metricType, &raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%s: %w", operation, ErrMetricIDNotFound)
		}
		return nil, fmt.Errorf("%s: %w: %w", operation, ErrMetricsStoreInternal, err)
	}
	if !supportedReceivedMetricType(metricType) {
		return nil, fmt.Errorf("%s: %w: %s", operation, ErrUnsupportedMetricType, metricType)
	}
	if raw == nil || string(raw) == "null" {
		return nil, fmt.Errorf("%s: %w: query returned null", operation, ErrMetricsStoreInternal)
	}
	return json.RawMessage(raw), nil
}

func supportedReceivedMetricType(metricType string) bool {
	switch metricType {
	case "Gauge", "Sum", "Histogram", "ExponentialHistogram":
		return true
	default:
		return false
	}
}

// GetMetricOTLP returns a Metric as a standard OTLP JSON object. Callers
// must transport the returned bytes unchanged because re-encoding can lose -0.0.
func GetMetricOTLP(ctx context.Context, db *sql.DB, metricID string) (json.RawMessage, error) {
	return GetMetricSelectionOTLP(ctx, db, metricID, "", "", true)
}

// GetMetricSelectionOTLP exports only the exact selected stored series or datapoint.
// Empty selectors export the whole retained Metric. Selection never uses chart reductions.
// When includeDatapoints is false, it omits dataPoints and skips their reconstruction.
func GetMetricSelectionOTLP(ctx context.Context, db *sql.DB, metricID, seriesID, datapointID string, includeDatapoints bool) (json.RawMessage, error) {
	query, err := queries.Render(queries.GetMetricOTLP, nil)
	if err != nil {
		return nil, fmt.Errorf("GetMetricOTLP: %w: %w", ErrMetricsStoreInternal, err)
	}

	var metricType string
	var raw []byte
	if err := db.QueryRowContext(ctx, query, seriesID, datapointID, includeDatapoints, metricID).Scan(&metricType, &raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetMetricOTLP: %w", ErrMetricIDNotFound)
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

// getMetricView runs the chart query in whichever shape params asks for. Both shapes take
// the same arguments and the same CTEs; only the projection differs.
func getMetricView(ctx context.Context, db *sql.DB, params getMetricViewParams, metricID string, timeRange timerange.TimeRange, targetBuckets int64, seriesIDs []string, quantiles []float64, tzOffsetNs int64, viewBuckets int64, sparklineBuckets int64, selectedSeriesIDs []string, tzName string, datapointSeriesIDs []string, datapointSeriesLimit int64) (json.RawMessage, error) {
	// Deduplicate the quantile list, keeping first-occurrence order.
	//
	// DuckDB map keys must be unique. Preserve first-occurrence request order.
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

	// Every view filters the owned datapoints by metric_id.
	params.HasStart = timeRange.Start != nil
	params.HasEnd = timeRange.End != nil

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
	query, args, err := queries.RenderBound(queries.GetMetricView, params, map[string]any{
		"metric_id": metricID, "time_start": startTime, "time_end": endTime,
		"target_buckets": targetBuckets, "series_ids": seriesArg, "quantiles": quantiles,
		"tz_offset_ns": tzOffsetNs, "view_buckets": viewBuckets, "sparkline_buckets": sparklineBuckets,
		"selected_series_ids": selectedArg, "tz_name": tzArg, "datapoint_series_ids": datapointArg,
		"datapoint_series_limit": datapointSeriesLimit, "contribution_id": params.ContributionID,
	})
	if err != nil {
		return nil, err
	}
	if err := db.QueryRowContext(ctx, query, args...).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetMetricView: %w", ErrMetricIDNotFound)
		}
		return nil, fmt.Errorf("GetMetricView: %w: %w", ErrMetricsStoreInternal, err)
	}
	// The projection is a non-null json_object, so a null here means the
	// query itself misbehaved -- an internal anomaly, not a missing Metric.
	if raw == nil || string(raw) == "null" {
		return nil, fmt.Errorf("GetMetricView: %w: query returned null", ErrMetricsStoreInternal)
	}
	return json.RawMessage(raw), nil
}

// GetMetricAggregateView returns only the cross-series aggregate: the selected
// series merged into one histogram per time bucket.
//
// The aggregate is fetched separately because it changes with legend selection
// while per-series data does not.
//
// A histogram merges the selected series into one histogram per bucket, so the
// caller narrows with seriesIDs and the merge sees only those.
//
// A scalar needs both selectedSeriesIDs and the complete metric so it can
// compute Selected and All without narrowing the latter.
func GetMetricAggregateView(ctx context.Context, db *sql.DB, metricID string, timeRange timerange.TimeRange, targetBuckets int64, seriesIDs []string, quantiles []float64, tzOffsetNs int64, viewBuckets int64, selectedSeriesIDs []string, tzName string, contributionID ...string) (json.RawMessage, error) {
	// The aggregate projection lets DuckDB prune per-series CTEs.
	shape := metricViewShapeFor(ctx, db, metricID)
	shape.AggregateOnly = true
	if len(contributionID) > 0 {
		shape.ContributionID = contributionID[0]
	}
	raw, err := getMetricView(ctx, db, shape,
		metricID, timeRange, targetBuckets, seriesIDs, quantiles,
		tzOffsetNs, viewBuckets, 0, selectedSeriesIDs, tzName,
		[]string{}, 0)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// GetMetricAttributeDefinitions returns every metric-side attribute name/scope/type this
// store knows about. See the note on spans.GetTraceAttributeDefinitions.
//
// The definition set includes Metric metadata, datapoint labels, exemplar
// attributes, and the owned Resource and Scope maps.
func GetMetricAttributeDefinitions(ctx context.Context, db *sql.DB) (json.RawMessage, error) {
	query, err := queries.Render(queries.GetMetricAttributeDefinitions, nil)
	if err != nil {
		return nil, err
	}

	var raw []byte
	if err := db.QueryRowContext(ctx, query).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetMetricAttributeDefinitions: %w: %w", ErrMetricsStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// Clear removes every Metric, series, datapoint, and exemplar. DuckDB does not
// cascade foreign keys, so rows are deleted explicitly from child to parent.
func Clear(ctx context.Context, db *sql.DB) error {
	// Histogram bounds remain until the store sweeps unreferenced vectors.
	for _, q := range [][2]string{
		{`delete from exemplars`, `delete from search_owners where signal='metric' and owner='exemplar'`},
		{`delete from metric_datapoints`, `delete from search_owners where signal='metric' and owner='datapoint'`},
		{`delete from metric_series`, ""},
		{`delete from metrics`, `delete from search_owners where signal='metric'`},
	} {
		if err := searchindex.DeleteRows(ctx, db, q[0], q[1]); err != nil {
			return fmt.Errorf("Clear: %w: %w", ErrMetricsStoreInternal, err)
		}
	}
	return nil
}

// DeleteMetric removes a Metric, its series, datapoints, and exemplars.
// ingest.SweepOrphans collects unreferenced histogram bounds separately.
//
// We still can't wrap this in a transaction: DuckDB issue #13819 still
// fires "phantom" FK violations for in-tx cascades. The pinned-conn
// auto-commit pattern works around it. A retry of DeleteMetric
// completes any remaining Metric-table cascade; orphaned bounds
// are collected by a later SweepOrphans call after Clear or during retention.
//
// Returns nil if the Metric does not exist (idempotent delete).
func DeleteMetric(ctx context.Context, db *sql.DB, metricID string) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("DeleteMetric: %w: acquire conn: %w", ErrMetricsStoreInternal, err)
	}
	defer conn.Close()

	// Each statement names the doomed Metric in its own WHERE clause so
	// they're independent at the FK layer. Order: leaves first.
	for _, q := range [][2]string{
		{`delete from exemplars where metric_datapoint_id in (
			select id from metric_datapoints where metric_id = ?::uuid
		)`, `delete from search_owners where signal='metric' and owner='exemplar' and root_key=(?::uuid)::varchar`},
		{`delete from metric_datapoints where metric_id = ?::uuid`, `delete from search_owners where signal='metric' and owner='datapoint' and root_key=(?::uuid)::varchar`},
		{`delete from metric_series where metric_id = ?::uuid`, ""},
		{`delete from metrics where id = ?::uuid`, `delete from search_owners where signal='metric' and root_key=(?::uuid)::varchar`},
	} {
		if err := searchindex.DeleteRows(ctx, conn, q[0], q[1], metricID); err != nil {
			return fmt.Errorf("DeleteMetric: %w: %w", ErrMetricsStoreInternal, err)
		}
	}
	return nil
}

// buildMetricSQL builds the WHERE clause for the metric Search query.
func buildMetricSQL(queryNode *search.QueryNode, timeRange timerange.TimeRange) (cteSQL string, whereSQL string, args []any, err error) {
	dpCondition, timeParams := search.TimePredicate("d.timestamp", timeRange.Start, timeRange.End)
	timeCondition := "exists (select 1 from metric_datapoints d where d.metric_id = s.id"
	if dpCondition != "" {
		timeCondition += " and " + dpCondition
	}
	timeCondition += ")"
	return searchindex.BuildSearchSQL(queryNode, metricFieldMapper(), timeCondition, timeParams, "metric", "try_cast(search_record.root_key as uuid)=s.id")
}

// MatchingRowsSQL uses the same metric identity and datapoint eligibility as search.
func MatchingRowsSQL(queryNode *search.QueryNode, timeRange timerange.TimeRange) (string, []any, error) {
	cte, where, args, err := buildMetricSQL(queryNode, timeRange)
	return cte + " SELECT s.* " + metricSearchFrom + " WHERE " + where, args, err
}

func metricDatapointWhere(timeRange timerange.TimeRange) string {
	predicate, _ := search.TimePredicate("d.timestamp", timeRange.Start, timeRange.End)
	if predicate == "" {
		return ""
	}
	return "where " + predicate
}

// metricColumns lists field names the search expression syntax can
// reference. Metric fields resolve through metrics (s).
var metricColumns = map[string]struct{}{
	"id":          {},
	"description": {},
}

func metricFieldMapper() search.FieldMapper {
	return search.Resolver("metric", map[string]search.ValueOwner{
		"metric":    {Documents: map[string]string{"metadata": "s.metadata"}},
		"resource":  {Documents: map[string]string{"attributes": "s.resource.attributes"}},
		"scope":     {Documents: map[string]string{"attributes": "s.scope.attributes"}},
		"datapoint": {Documents: map[string]string{"attributes": "d.attributes"}, Correlation: "metric_datapoints d where d.metric_id = s.id"},
		"exemplar":  {Documents: map[string]string{"attributes": "e.attributes"}, Correlation: "exemplars e join metric_datapoints d on d.id = e.metric_datapoint_id where d.metric_id = s.id"},
	}, mapMetricFieldExpression, search.RejectUnindexedText)
}

// metricSearchFrom is the FROM clause metric search predicates are written
// against. Mirrors spans.spanSearchFrom and logs.logSearchFrom.
const metricSearchFrom = `from metrics s`

func mapMetricFieldExpression(field *search.FieldDefinition) (search.ResolvedExpression, error) {
	if field.Name == "serviceIdentity" || field.Name == "resource.serviceIdentity" {
		return search.Text(discovery.ServiceIdentity("s.resource")), nil
	}
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
		return search.Text("s.scope.name"), nil
	case "scope.version":
		return search.Text("s.scope.version"), nil
	case "description":
		return search.Text("s.description"), nil
	case "resource.droppedAttributesCount":
		return search.NativeInteger("s.resource.dropped_attributes_count"), nil
	case "scope.droppedAttributesCount":
		return search.NativeInteger("s.scope.dropped_attributes_count"), nil
	default:
		col := util.CamelToSnake(name)
		if err := util.ValidateColumnName(col, metricColumns); err != nil {
			return search.ResolvedExpression{}, fmt.Errorf("metric field %q: %w: %w", name, err, ErrInvalidMetricQuery)
		}
		return search.Text("s." + col), nil
	}
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

// decodeMetricID normalizes a UUID coming back from the driver.Conn
// QueryContext path into a duckdb.UUID.
func decodeMetricID(v driver.Value) (duckdb.UUID, error) {
	switch t := v.(type) {
	case duckdb.UUID:
		return t, nil
	case [16]byte:
		return duckdb.UUID(t), nil
	case []byte:
		if len(t) != 16 {
			return duckdb.UUID{}, fmt.Errorf("decodeMetricID: expected 16 bytes, got %d", len(t))
		}
		var u duckdb.UUID
		copy(u[:], t)
		return u, nil
	case string:
		parsed, err := uuid.Parse(t)
		if err != nil {
			return duckdb.UUID{}, fmt.Errorf("decodeMetricID: parse %q: %w", t, err)
		}
		return duckdb.UUID(parsed), nil
	default:
		return duckdb.UUID{}, fmt.Errorf("decodeMetricID: unsupported value type %T", v)
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

// getMetricViewParams selects which shape of response the projection builds.
//
// The CTE definitions are shared; DuckDB prunes branches unused by the selected
// final projection.
type getMetricViewParams struct {
	HasStart bool
	HasEnd   bool
	// ContributionID selects interval activity after differencing, never before.
	ContributionID string
	// AggregateOnly emits just the cross-series aggregates, which is all
	// GetMetricAggregateView returns. It drops `timeseries` -- the field that
	// carries the per-series pipelines and most of the planning cost.
	AggregateOnly bool

	// NoHistogramMerge and NoScalarPools drop a chain whose output is already
	// known to be empty for this metric's shape, so the planner never builds
	// it. scalar_dps admits Gauge and Sum only, so a histogram's pools are
	// always []; the histogram merge needs bucket vectors, so a scalar's
	// aggregate is always null.
	//
	// Scalar detail also omits histogram reduction and quantile branches before
	// binding; an empty runtime result would still incur their planning cost.
	NoHistogramMerge bool
	NoScalarPools    bool
	// Explicit histograms have no exponential scale/offset alignment. Delta
	// histograms already contain interval activity and need no predecessor scan.
	ExplicitHistogram bool
	DeltaHistogram    bool
}

// metricViewShapeFor selects the query chains required by the metric type.
func metricViewShapeFor(ctx context.Context, db *sql.DB, metricID string) getMetricViewParams {
	p := getMetricViewParams{}
	var metricType string
	var temporality int
	err := db.QueryRowContext(ctx,
		`select metric_type, aggregation_temporality from metrics where id = ?::uuid`, metricID).Scan(&metricType, &temporality)
	if err != nil {
		// An unresolved Metric id is reported by the main query, so retain both
		// chains and let it return ErrMetricIDNotFound.
		return p
	}
	switch metricType {
	case "Histogram", "ExponentialHistogram":
		p.NoScalarPools = true
		p.ExplicitHistogram = metricType == "Histogram"
		p.DeltaHistogram = temporality == 1
	case "Gauge", "Sum":
		p.NoHistogramMerge = true
	}
	return p
}

// searchSummariesParams are the fragments SearchSummaries assembles into
// queries/metrics/search_summaries.sql.
type searchSummariesParams struct {
	// CTEs contains optional search graph definitions.
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
