package spans

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Sentinel errors for use with errors.Is.
var (
	ErrTraceIDNotFound    = errors.New("trace ID not found")
	ErrInvalidTraceQuery  = errors.New("invalid trace search query")
	ErrInvalidTraceLimit  = errors.New("invalid trace search limit")
	ErrInvalidSpanID      = errors.New("span ID is empty")
	ErrSpansStoreInternal = errors.New("spans store internal error")
)

// scopeKey identifies a scope by position: the ri'th resource's si'th scope.
type scopeKey struct{ ri, si int }

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// flushIntervalSpans bounds how many spans accumulate in the appenders before
// they are pushed to DuckDB, limiting memory used by large batches.
const flushIntervalSpans = 500

// resourceServiceName extracts the service.name resource attribute as a
// plain string, returning "" when not present. This is the same logic
// used by the metrics package to denormalize service onto metrics,
// kept private here so spans doesn't grow a metrics dependency.
func resourceServiceName(attrs pcommon.Map) string {
	if v, ok := attrs.Get("service.name"); ok {
		return v.AsString()
	}
	return ""
}

// Ingest is IngestReport for callers with nowhere to put the report. Refused
// rows are still skipped rather than failing the batch.
func Ingest(ctx context.Context, conn driver.Conn, traces ptrace.Traces, flushed *ingest.FlushedIDs) error {
	_, err := IngestReport(ctx, conn, traces, flushed)
	return err
}

// IngestReport ingests trace spans into the spans, events, links and attributes
// tables, reporting the spans it could not write. A non-empty Rejected is not a
// failure: the batch landed without them. The caller must hold any required
// lock on the connection.
//
// Two passes. The first hashes every attribute into a Dictionary, resolves each
// resource and scope to a content-derived id, and writes the dictionary in
// three inserts. The second appends the owner rows with the id arrays the first
// produced. They are split because the dictionary needs conflict handling and
// the appender has none.
func IngestReport(ctx context.Context, conn driver.Conn, traces ptrace.Traces, flushed *ingest.FlushedIDs) (_ ingest.Rejected, err error) {
	defer func() { err = ingest.InterruptedContextError(ctx, err) }()

	if err := ctx.Err(); err != nil {
		return ingest.Rejected{}, err
	}

	// Pass 1: hash everything and resolve resource/scope identities.
	dict := ingest.NewDictionary(flushed)

	resourceIDs := map[int]duckdb.UUID{}
	scopeIDs := map[scopeKey]duckdb.UUID{}

	// AddAttributes already returns the array its owner should store, so pass 1
	// records it and pass 2 reads it back rather than hashing the same
	// attributes a second time.
	//
	// Consumed by position: both passes walk the identical nested loops in the
	// identical order, so the Nth span visited in pass 2 is the Nth entry here.
	// The cursors are checked against the slice lengths when the walk finishes,
	// so a future edit that desynchronises the two passes fails loudly instead
	// of silently pairing a span with another span's attributes.
	var spanAttrs, eventAttrs, linkAttrs [][]duckdb.UUID
	// Span identities in walk order, so dedupe below can speak in the same
	// ordinals the append pass and bisection do.
	var spanKeys []spanKey

	for ri, resourceSpan := range traces.ResourceSpans().All() {
		resourceIDs[ri] = dict.AddResource(resourceSpan.Resource(), resourceSpan.SchemaUrl()).ID
		for si, scopeSpan := range resourceSpan.ScopeSpans().All() {
			scopeIDs[scopeKey{ri, si}] = dict.AddScope(scopeSpan.Scope(), scopeSpan.SchemaUrl())
			for _, span := range scopeSpan.Spans().All() {
				spanKeys = append(spanKeys, spanKey{
					trace: duckdb.UUID(span.TraceID()),
					span:  util.SpanIDUint64(span.SpanID()),
				})
				spanAttrs = append(spanAttrs, dict.AddAttributes(span.Attributes(), ingest.ScopeSpan))
				for _, event := range span.Events().All() {
					eventAttrs = append(eventAttrs, dict.AddAttributes(event.Attributes(), ingest.ScopeEvent))
				}
				for _, link := range span.Links().All() {
					linkAttrs = append(linkAttrs, dict.AddAttributes(link.Attributes(), ingest.ScopeLink))
				}
			}
		}
	}

	if err := dict.Flush(ctx, conn); err != nil {
		return ingest.Rejected{}, fmt.Errorf("Ingest: %w: %w", ErrSpansStoreInternal, err)
	}

	// Skip ids the store already holds before writing rather than after
	// failing: bisection would find them, but one transaction at a time.
	skip, err := skipAlreadyStored(ctx, conn, spanKeys)
	if err != nil {
		return ingest.Rejected{}, err
	}
	for ordinal, key := range spanKeys {
		if key.span == 0 {
			skip[ordinal] = ErrInvalidSpanID
		}
	}

	// Pass 2: append, retrying in halves so a bad row costs only itself. The
	// skipped ordinals go in as pre-rejected rather than being tallied on
	// afterwards, so every rejection is built in one place and in walk order.
	rejected, err := appendSpansBisecting(ctx, conn, traces, resourceIDs, scopeIDs,
		spanAttrs, eventAttrs, linkAttrs, skip)
	if err != nil {
		return rejected, err
	}

	// Recorded after the appends commit, never inside them: a rejection
	// describes a batch that succeeded, and rolling it back with a failed
	// attempt would lose the only trace that anything was refused.
	if err := recordSpanRejections(ctx, conn, rejected, spanKeys); err != nil {
		// The telemetry landed. Failing the batch because we could not write
		// a note about it would turn a report into an outage.
		return rejected, nil
	}
	return rejected, nil
}

// appendPass writes the spans keep selects, by ordinal rather than id so two
// occurrences of one id stay separable. Skipped spans still advance all three
// cursors, which are consumed by position.
func appendPass(
	ctx context.Context,
	conn driver.Conn,
	traces ptrace.Traces,
	resourceIDs map[int]duckdb.UUID,
	scopeIDs map[scopeKey]duckdb.UUID,
	spanAttrs, eventAttrs, linkAttrs [][]duckdb.UUID,
	keep func(ordinal int) bool,
) (err error) {
	tables := []string{"events", "links", "spans"}
	appenders, err := ingest.NewAppenders(conn, tables)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, ingest.CloseAppenders(appenders, tables))
	}()

	spanCount := 0
	var spanCur, eventCur, linkCur int
	for ri, resourceSpan := range traces.ResourceSpans().All() {
		resource := resourceSpan.Resource()
		resourceID := resourceIDs[ri]
		// Denormalize service.name onto every span row in this resource.
		// Source of truth is the resource's attribute row, reached through
		// resource_id; this column is the index target for "filter spans by
		// service", which is the hottest filter in span search.
		serviceName := resourceServiceName(resource.Attributes())

		for si, scopeSpan := range resourceSpan.ScopeSpans().All() {
			scopeID := scopeIDs[scopeKey{ri, si}]

			for _, span := range scopeSpan.Spans().All() {
				if err := ctx.Err(); err != nil {
					return err
				}

				if !keep(spanCur) {
					spanCur++
					eventCur += span.Events().Len()
					linkCur += span.Links().Len()
					continue
				}

				traceUUID := duckdb.UUID(span.TraceID())

				spanID := util.SpanIDUint64(span.SpanID())

				var parentSpanID driver.Value
				if pid := span.ParentSpanID(); !pid.IsEmpty() {
					parentSpanID = util.SpanIDUint64(pid)
				}

				// Hashed in pass 1. NonNil because AttributeSet returns nil for
				// an empty map while the column is NOT NULL: an owner with no
				// attributes stores an empty array.
				spanAttrIDs := ingest.NonNil(spanAttrs[spanCur])
				spanCur++

				err := appenders["spans"].AppendRow(
					traceUUID,                     // TraceID UUID
					span.TraceState().AsRaw(),     // TraceState VARCHAR
					spanID,                        // SpanID UBIGINT
					parentSpanID,                  // ParentSpanID UBIGINT or NULL
					uint32(span.Flags()),          // Flags UINTEGER
					span.Name(),                   // Name VARCHAR
					int32(span.Kind()),            // Kind INTEGER (received enum code)
					uint64(span.StartTimestamp()), // StartTime UBIGINT
					uint64(span.EndTimestamp()),   // EndTime UBIGINT
					resourceID,                    // ResourceID UUID
					scopeID,                       // ScopeID UUID
					spanAttrIDs,                   // AttributeIDs UUID[]
					span.DroppedAttributesCount(), // DroppedAttributesCount UINTEGER
					span.DroppedEventsCount(),     // DroppedEventsCount UINTEGER
					span.DroppedLinksCount(),      // DroppedLinksCount UINTEGER
					int32(span.Status().Code()),   // StatusCode INTEGER (received enum code)
					span.Status().Message(),       // StatusMessage VARCHAR
					serviceName,                   // ServiceName VARCHAR (NOT NULL, '' = unknown)
				)
				if err != nil {
					return fmt.Errorf("Ingest: %w: %w", ErrSpansStoreInternal, err)
				}

				for _, event := range span.Events().All() {
					eventAttrIDs := ingest.NonNil(eventAttrs[eventCur])
					eventCur++
					err = appenders["events"].AppendRow(
						duckdb.UUID(uuid.New()),        // ID UUID
						traceUUID,                      // TraceID UUID
						spanID,                         // SpanID UBIGINT
						event.Name(),                   // Name VARCHAR
						uint64(event.Timestamp()),      // Timestamp UBIGINT
						eventAttrIDs,                   // AttributeIDs UUID[]
						event.DroppedAttributesCount(), // DroppedAttributesCount UINTEGER
					)
					if err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrSpansStoreInternal, err)
					}
				}

				for _, link := range span.Links().All() {
					var linkTraceUUID driver.Value
					if tid := link.TraceID(); !tid.IsEmpty() {
						linkTraceUUID = duckdb.UUID(tid)
					}
					var linkSpanID driver.Value
					if sid := link.SpanID(); !sid.IsEmpty() {
						linkSpanID = util.SpanIDUint64(sid)
					}

					linkAttrIDs := ingest.NonNil(linkAttrs[linkCur])
					linkCur++
					err = appenders["links"].AppendRow(
						duckdb.UUID(uuid.New()),       // ID UUID
						traceUUID,                     // TraceID UUID (owner)
						spanID,                        // SpanID UBIGINT (owner)
						linkTraceUUID,                 // LinkedTraceID UUID or NULL
						linkSpanID,                    // LinkedSpanID UBIGINT or NULL
						link.TraceState().AsRaw(),     // TraceState VARCHAR
						linkAttrIDs,                   // AttributeIDs UUID[]
						link.DroppedAttributesCount(), // DroppedAttributesCount UINTEGER
						uint32(link.Flags()),          // Flags UINTEGER
					)
					if err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrSpansStoreInternal, err)
					}
				}

				spanCount++
				if spanCount%flushIntervalSpans == 0 {
					if err := ingest.FlushAppenders(appenders, tables); err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrSpansStoreInternal, err)
					}
				}
			}
		}
	}

	// The two passes must have visited exactly the same owners. If they ever
	// diverge, every span past the divergence point would silently receive some
	// other span's attributes -- corruption with no error, which is why this is
	// checked rather than assumed.
	if spanCur != len(spanAttrs) || eventCur != len(eventAttrs) || linkCur != len(linkAttrs) {
		// ErrNotRowFault: our bug, not a row's, so bisection must not search.
		return fmt.Errorf("Ingest: %w: %w: pass mismatch (spans %d/%d, events %d/%d, links %d/%d)",
			ErrSpansStoreInternal, ingest.ErrNotRowFault, spanCur, len(spanAttrs),
			eventCur, len(eventAttrs), linkCur, len(linkAttrs))
	}

	return nil
}

// SearchTraceSummaries returns trace summaries in the time range matching the optional criteria.
func SearchTraceSummaries(ctx context.Context, db *sql.DB, timeRange timerange.TimeRange, criteria any) (json.RawMessage, error) {
	return searchTraceSummaries(ctx, db, timeRange, criteria, search.ResultOptions{})
}

// SearchTraceSummariesWithLimit returns at most limit trace summaries.
func SearchTraceSummariesWithLimit(ctx context.Context, db *sql.DB, timeRange timerange.TimeRange, criteria any, limit int64) (json.RawMessage, error) {
	return searchTraceSummaries(ctx, db, timeRange, criteria, search.ResultOptions{Limit: &limit})
}

func SearchTraceSummariesWithOptions(ctx context.Context, db *sql.DB, timeRange timerange.TimeRange, criteria any, options search.ResultOptions) (json.RawMessage, error) {
	return searchTraceSummaries(ctx, db, timeRange, criteria, options)
}

func searchTraceSummaries(ctx context.Context, db *sql.DB, timeRange timerange.TimeRange, criteria any, options search.ResultOptions) (json.RawMessage, error) {
	finalQuery, args, err := searchTraceSummariesSQL(timeRange, criteria, options)
	if err != nil {
		return nil, err
	}

	var raw []byte
	if err := db.QueryRowContext(ctx, finalQuery, args...).Scan(&raw); err != nil {
		return nil, fmt.Errorf("SearchTraceSummaries: %w: %w", ErrSpansStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// searchTraceSummariesSQL renders the trace-summary query and its bound arguments.
// Split out for the same reason as getTraceViewSQL: so a golden test can pin the
// rendered text without standing up a store.
func searchTraceSummariesSQL(timeRange timerange.TimeRange, criteria any, options search.ResultOptions) (string, []any, error) {
	var searchTree *search.QueryNode
	if criteria != nil {
		var err error
		searchTree, err = search.ParseQueryTree(criteria)
		if err != nil {
			return "", nil, fmt.Errorf("SearchTraceSummaries: %w: %w", ErrInvalidTraceQuery, err)
		}
	}

	cteSQL, eligibilityWhere, queryWhere, args, err := buildTraceSQL(searchTree, timeRange)
	if err != nil {
		return "", nil, fmt.Errorf("SearchTraceSummaries: %w: %w", ErrInvalidTraceQuery, err)
	}

	// service_name comes from spans.service_name (denormalized at
	// ingest from the service.name resource attribute) rather than
	// resolving it through resource_id; same value, no join and no array
	// unnest on a per-trace aggregate.
	//
	// `hasRootSpan` makes the orphaned-trace state explicit so consumers
	// don't have to infer it from a null rootSpan. rootSpan carries only
	// serviceName + name (the root span's timing is not useful for
	// summary display -- trace-level startTime and durationNs are
	// computed from the min/max across ALL spans). startTime and
	// durationNs are precomputed from span bounds so the summary always
	// reflects wall-clock coverage.
	orderBy, err := traceSummaryOrderBy(options.Sort)
	if err != nil {
		return "", nil, err
	}
	limitClause := ""
	if options.Limit != nil {
		if *options.Limit < 1 {
			return "", nil, fmt.Errorf("SearchTraceSummaries: limit must be positive: %w", ErrInvalidTraceLimit)
		}
		limitClause = "\n\t\t\tlimit ?"
		args = append(args, *options.Limit)
	}

	matchCTEs := ""
	matchJoin := ""
	matchProjection := ""
	if searchTree != nil {
		matchCTEs = fmt.Sprintf(`
		query_matches as materialized (
			select distinct s.trace_id, s.span_id, s.start_time
			%s
			join selected_trace_ids selected on selected.trace_id = s.trace_id
			where %s
		),
		matched_span_lists as (
			select trace_id as matched_trace_id, list(json_object(
				'traceID', replace(trace_id::varchar, '-', ''),
				'spanID', span_id_wire(span_id)
			) order by start_time, span_id) as matched_spans
			from query_matches
			group by trace_id
		),`, spanSearchFrom, queryWhere)
		matchJoin = "\n\t\tjoin matched_span_lists matches on matches.matched_trace_id = sub.trace_id"
		matchProjection = ",\n\t\t\t'matchedSpans', to_json(matches.matched_spans)"
	}

	finalQuery, err := queries.Render(queries.SearchTraceSummaries, searchTraceSummariesParams{
		CTEs:             cteSQL,
		From:             spanSearchFrom,
		EligibilityWhere: eligibilityWhere,
		MatchCTEs:        matchCTEs,
		MatchJoin:        matchJoin,
		MatchProjection:  matchProjection,
		Order:            orderBy,
		Limit:            limitClause,
	})
	if err != nil {
		return "", nil, fmt.Errorf("SearchTraceSummaries: %w: %w", ErrSpansStoreInternal, err)
	}

	return finalQuery, args, nil
}

func traceSummaryOrderBy(sortOption *search.Sort) (string, error) {
	if sortOption == nil {
		return "trace_start_time desc, trace_id asc", nil
	}
	expressions := map[string]string{
		"serviceName":  "coalesce(service_name, '')",
		"rootSpanName": "coalesce(root_name, '')",
		"startTime":    "trace_start_time",
		"duration":     "(trace_end_time::hugeint - trace_start_time::hugeint)",
		"spanCount":    "span_count",
		"errorCount":   "error_count",
	}
	expression, ok := expressions[sortOption.Field]
	if !ok {
		return "", fmt.Errorf("unsupported trace sort field %q: %w", sortOption.Field, search.ErrInvalidSort)
	}
	direction, err := search.SortDirectionSQL(sortOption.Direction)
	if err != nil {
		return "", err
	}
	nulls := "nulls last"
	if sortOption.Field == "duration" && direction == "desc" {
		// compareByOptionalBigintField in frontend/src/utils/compare.ts puts
		// missing values after defined ones before descending reverses the order.
		nulls = "nulls first"
	}
	return fmt.Sprintf("%s %s %s, trace_id asc", expression, direction, nulls), nil
}

// GetTraceView returns a whole trace with matching spans annotated when criteria
// is provided.
//
// The ordinary tree walk reports unreachable spans. A non-zero count triggers
// the cycle-aware query, which chooses entry points and tracks ancestry.
func GetTraceView(ctx context.Context, db *sql.DB, traceID string, criteria any) (json.RawMessage, error) {
	query, args, err := getTraceViewSQL(traceID, criteria)
	if err != nil {
		return nil, err
	}

	var raw []byte
	var unplaced int64
	if err := db.QueryRowContext(ctx, query, args...).Scan(&raw, &unplaced); err != nil {
		return nil, fmt.Errorf("GetTraceView: %w: %w", ErrSpansStoreInternal, err)
	}
	if raw == nil {
		return nil, fmt.Errorf("GetTraceView: %w", ErrTraceIDNotFound)
	}
	if unplaced == 0 {
		return json.RawMessage(raw), nil
	}

	// Re-run the whole trace with the cycle-aware walk.
	salvaged, err := salvageSpans(ctx, db, traceID, criteria)
	if err != nil {
		// The ordinary result is still correct as far as it goes, and short a
		// few spans is better than an error page.
		return json.RawMessage(raw), nil
	}
	return salvaged, nil
}

// GetTraceOverview returns the compact, untruncated overview for one trace. The query
// computes exact nanosecond strings and reads every span in one operation.
func GetTraceOverview(ctx context.Context, db queryRower, traceID string) (json.RawMessage, error) {
	query, err := queries.Render(queries.GetTraceOverview, nil)
	if err != nil {
		return nil, fmt.Errorf("GetTraceOverview: %w: %w", ErrSpansStoreInternal, err)
	}
	var raw []byte
	if err := db.QueryRowContext(ctx, query, traceID).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetTraceOverview: %w", ErrTraceIDNotFound)
		}
		return nil, fmt.Errorf("GetTraceOverview: %w: %w", ErrSpansStoreInternal, err)
	}
	return json.RawMessage(raw), nil
}

// GetSpanSummaries returns at most limit stable summary rows for one span ID,
// together with the exact number of matching composite identities.
func GetSpanSummaries(ctx context.Context, db queryRower, spanID uint64, limit int64) (json.RawMessage, int64, error) {
	query, err := queries.Render(queries.GetSpanSummaries, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("GetSpanSummaries: %w: %w", ErrSpansStoreInternal, err)
	}
	var raw []byte
	var matchCount int64
	if err := db.QueryRowContext(ctx, query, []uint64{spanID}, limit).Scan(&raw, &matchCount); err != nil {
		return nil, 0, fmt.Errorf("GetSpanSummaries: %w: %w", ErrSpansStoreInternal, err)
	}
	if raw == nil {
		raw = []byte("[]")
	}
	return json.RawMessage(raw), matchCount, nil
}

// GetSpan returns full stored detail for one composite trace and span identity.
// A missing exact pair returns a nil result without an error.
func GetSpan(ctx context.Context, db queryRower, traceID string, spanID uint64) (json.RawMessage, error) {
	query, err := queries.Render(queries.GetSpan, nil)
	if err != nil {
		return nil, fmt.Errorf("GetSpan: %w: %w", ErrSpansStoreInternal, err)
	}
	var raw []byte
	if err := db.QueryRowContext(ctx, query, traceID, []uint64{spanID}).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetSpan: %w: %w", ErrSpansStoreInternal, err)
	}
	if raw == nil {
		return nil, nil
	}
	return json.RawMessage(raw), nil
}

// GetTraceOTLP returns every stored span for traceID as one standard OTLP JSON
// document. The SQL owns reconstruction so callers receive the stored signal,
// not the search and display projection. Callers must transport the returned
// bytes unchanged: parsing and re-encoding in JavaScript loses negative zero.
func GetTraceOTLP(ctx context.Context, db *sql.DB, traceID string) (json.RawMessage, error) {
	query, err := queries.Render(queries.GetTraceOTLP, nil)
	if err != nil {
		return nil, fmt.Errorf("GetTraceOTLP: %w: %w", ErrSpansStoreInternal, err)
	}

	var raw []byte
	if err := db.QueryRowContext(ctx, query, traceID).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetTraceOTLP: %w", ErrTraceIDNotFound)
		}
		return nil, fmt.Errorf("GetTraceOTLP: %w: %w", ErrSpansStoreInternal, err)
	}
	return json.RawMessage(raw), nil
}

// salvageSpans runs the cycle-aware variant of the trace fetch.
func salvageSpans(ctx context.Context, db *sql.DB, traceID string, criteria any) (json.RawMessage, error) {
	query, args, err := renderSpansQuery(queries.SalvageSpans, traceID, criteria)
	if err != nil {
		return nil, err
	}
	var raw []byte
	var unplaced int64
	if err := db.QueryRowContext(ctx, query, args...).Scan(&raw, &unplaced); err != nil {
		return nil, fmt.Errorf("salvageSpans: %w: %w", ErrSpansStoreInternal, err)
	}
	if raw == nil {
		return nil, fmt.Errorf("salvageSpans: %w", ErrTraceIDNotFound)
	}
	return json.RawMessage(raw), nil
}

// getTraceViewSQL renders the trace-fetch query and its bound arguments for
// execution and golden tests.
func getTraceViewSQL(traceID string, criteria any) (string, []any, error) {
	return renderSpansQuery(queries.GetTraceView, traceID, criteria)
}

// renderSpansQuery renders either trace-fetch query. Both take identical
// parameters and differ only in whether they carry the salvage walk, so the
// search-predicate plumbing is shared rather than duplicated.
func renderSpansQuery(name queries.Name, traceID string, criteria any) (string, []any, error) {
	var searchTree *search.QueryNode
	if criteria != nil {
		var err error
		searchTree, err = search.ParseQueryTree(criteria)
		if err != nil {
			return "", nil, fmt.Errorf("GetTraceView: %w: %w", ErrInvalidTraceQuery, err)
		}
	}

	cteSQL, whereClause, args, err := buildSpanSQL(searchTree, traceID)
	if err != nil {
		return "", nil, fmt.Errorf("GetTraceView: %w: %w", ErrInvalidTraceQuery, err)
	}

	// The recursive CTE always walks the full trace tree (filtered by trace_id only)
	// to compute depth and sort_path. When search criteria are present, a matched_spans
	// CTE identifies matching span IDs via LEFT JOIN so the full tree is always returned
	// with a per-span 'matched' boolean annotation.
	matchedCTE := ""
	matchedJoin := ""
	matchedExpr := "true"
	if searchTree != nil {
		matchedCTE = fmt.Sprintf(`,
		matched_spans as (
			select s.span_id
			%s
			where %s
		)`, spanSearchFrom, whereClause)
		matchedJoin = "left join matched_spans ms on ts.span_id = ms.span_id"
		matchedExpr = "case when ms.span_id is not null then true else false end"
	}

	// The recursion carries only traversal columns. The tree CTE joins the
	// payload once and defines one span set for every downstream projection.
	query, err := queries.Render(name, getTraceViewParams{
		CTEs:        cteSQL,
		MatchedCTE:  matchedCTE,
		MatchedExpr: matchedExpr,
		MatchedJoin: matchedJoin,
	})
	if err != nil {
		return "", nil, fmt.Errorf("GetTraceView: %w: %w", ErrSpansStoreInternal, err)
	}

	return query, args, nil
}

// GetTraceAttributeDefinitions returns attribute definitions across retained traces.
func GetTraceAttributeDefinitions(ctx context.Context, db *sql.DB) (json.RawMessage, error) {
	return traceAttributeKeys(ctx, db)
}

// fieldValueQueries names the span columns whose values the search box may
// complete, keyed by the search grammar's field name. An allowlist of whole
// baked queries rather than an identifier spliced into one: a field name from
// the wire never becomes SQL.
var fieldValueQueries = map[string]string{
	"name": `
		select cast(coalesce(
			to_json(list(sub.v order by sub.cnt desc, sub.v)),
			to_json([])) as varchar)
		from (
			select name as v, count(*) as cnt
			from spans
			where name <> '' and name ilike '%' || ? || '%' escape '\'
			group by name
			order by cnt desc, name
			limit ?
		) sub
	`,
}

// GetFieldValueCompletions returns distinct values of one completable span column
// matching term, most frequent first, as a JSON string array. Backs value
// completion in the search box.
//
// term is a substring match, case-insensitive, with LIKE wildcards escaped so
// a literal % or _ in the term matches itself. An empty term returns the top
// values by frequency, which is what the empty value position wants. limit is
// clamped by the caller. A field not in the allowlist is an error rather than
// an empty result, so a typo in a caller shows up as one.
func GetFieldValueCompletions(ctx context.Context, db *sql.DB, field, term string, limit int64) (json.RawMessage, error) {
	query, ok := fieldValueQueries[field]
	if !ok {
		return nil, fmt.Errorf("GetFieldValueCompletions: %w: field %q has no value completion", ErrInvalidTraceQuery, field)
	}
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(term)
	var raw []byte
	if err := db.QueryRowContext(ctx, query, escaped, limit).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetFieldValueCompletions: %w: %w", ErrSpansStoreInternal, err)
	}
	return json.RawMessage(raw), nil
}

func GetTraceAttributeDefinitionsByTraceID(ctx context.Context, db *sql.DB, traceID string) (json.RawMessage, error) {
	query := `
		select cast(to_json(list(json_object('name', sub.key, 'attributeScope', sub.scope,
			'type', sub.type) order by sub.key, sub.scope, sub.type)) as varchar) as attributes
		from (
			select distinct a.key, 'resource' as scope, json_extract_string(a.value, '$.kind') as type from spans s join resources r on r.id = s.resource_id, unnest(r.attribute_ids) t(aid) join attributes a on a.id = t.aid where s.trace_id = ?::uuid
			union select distinct a.key, 'scope', json_extract_string(a.value, '$.kind') from spans s join scopes sc on sc.id = s.scope_id, unnest(sc.attribute_ids) t(aid) join attributes a on a.id = t.aid where s.trace_id = ?::uuid
			union select distinct a.key, 'span', json_extract_string(a.value, '$.kind') from spans s, unnest(s.attribute_ids) t(aid) join attributes a on a.id = t.aid where s.trace_id = ?::uuid
			union select distinct a.key, 'event', json_extract_string(a.value, '$.kind') from events e, unnest(e.attribute_ids) t(aid) join attributes a on a.id = t.aid where e.trace_id = ?::uuid
			union select distinct a.key, 'link', json_extract_string(a.value, '$.kind') from links l, unnest(l.attribute_ids) t(aid) join attributes a on a.id = t.aid where l.trace_id = ?::uuid
		) sub
	`
	var raw []byte
	if err := db.QueryRowContext(ctx, query, traceID, traceID, traceID, traceID, traceID).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetTraceAttributeDefinitionsByTraceID: %w: %w", ErrSpansStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

func traceAttributeKeys(ctx context.Context, db *sql.DB) (json.RawMessage, error) {
	query := `
		select cast(to_json(list(json_object('name', sub.key, 'attributeScope', sub.scope,
			'type', sub.type) order by sub.key, sub.scope, sub.type)) as varchar) as attributes
		from (
			select distinct a.key, 'resource' as scope, json_extract_string(a.value, '$.kind') as type from spans s join resources r on r.id = s.resource_id, unnest(r.attribute_ids) t(aid) join attributes a on a.id = t.aid
			union select distinct a.key, 'scope', json_extract_string(a.value, '$.kind') from spans s join scopes sc on sc.id = s.scope_id, unnest(sc.attribute_ids) t(aid) join attributes a on a.id = t.aid
			union select distinct a.key, 'span', json_extract_string(a.value, '$.kind') from spans s, unnest(s.attribute_ids) t(aid) join attributes a on a.id = t.aid
			union select distinct a.key, 'event', json_extract_string(a.value, '$.kind') from events e, unnest(e.attribute_ids) t(aid) join attributes a on a.id = t.aid
			union select distinct a.key, 'link', json_extract_string(a.value, '$.kind') from links l, unnest(l.attribute_ids) t(aid) join attributes a on a.id = t.aid
		) sub
	`
	var raw []byte
	if err := db.QueryRowContext(ctx, query).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetTraceAttributeDefinitions: %w: %w", ErrSpansStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// Clear truncates the spans table and all child tables (events, links, and their attributes).
func Clear(ctx context.Context, db *sql.DB) error {
	// Attribute, resource and scope rows are not deleted here: they are shared
	// with logs and metrics, so "was it only ours?" is not a question this
	// function can answer. ingest.SweepOrphans collects whatever these deletes
	// orphaned, and the caller runs it once for all three signals.
	childQueries := []string{
		`truncate table links`,
		`truncate table events`,
		`truncate table spans`,
	}
	for _, q := range childQueries {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("Clear: %w: %w", ErrSpansStoreInternal, err)
		}
	}
	return nil
}

// DeleteSpansByTraceIDs deletes all spans for multiple traces.
func DeleteSpansByTraceIDs(ctx context.Context, db *sql.DB, traceIDs []any) error {
	if len(traceIDs) == 0 {
		return nil
	}
	ids := util.ToStringList(traceIDs)
	childQueries := []string{
		`delete from links where trace_id in (select id from uuid_list(?))`,
		`delete from events where trace_id in (select id from uuid_list(?))`,
		`delete from spans where trace_id in (select id from uuid_list(?))`,
	}
	for _, q := range childQueries {
		if _, err := db.ExecContext(ctx, q, ids); err != nil {
			return fmt.Errorf("DeleteSpansByTraceIDs: %w: %w", ErrSpansStoreInternal, err)
		}
	}
	return nil
}

func buildTraceSQL(queryNode *search.QueryNode, timeRange timerange.TimeRange) (cteSQL string, eligibilityWhere string, queryWhere string, args []any, err error) {
	timeCondition, timeParams := search.TimePredicate("s.start_time", timeRange.Start, timeRange.End)
	timeCondition = strings.ReplaceAll(timeCondition, " AND ", " and ")
	cteSQL, eligibilityWhere, args, err = search.BuildSearchSQL(queryNode, traceFieldMapper(), timeCondition, timeParams)
	if err != nil || queryNode == nil {
		return cteSQL, eligibilityWhere, "", args, err
	}
	// Keep the time parameters in the discarded CTE so generated query
	// parameter names retain the same indexes as eligibilityWhere.
	_, queryWhere, _, err = search.BuildSearchSQL(queryNode, traceFieldMapper(), "", timeParams)
	return cteSQL, eligibilityWhere, queryWhere, args, err
}

// Two idioms compare a trace id in this file, and they are not
// interchangeable. Which one applies depends on the operation:
//
//   - Exact lookup -- "give me this trace" -- compares uuid to uuid, here.
//     The caller has already normalised the id (normalizeUUID in the RPC
//     handler), so the value is well-formed, and uuid equality is what
//     idx_spans_traceid can actually serve.
//
//   - Fuzzy search -- "find traces whose id contains abc" -- compares text to
//     text, in mapTraceFieldExpression. It has to: LIKE against a uuid column
//     would match the dashed internal rendering rather than the wire form the
//     UI displays, and a half-typed id must match nothing rather than abort the
//     query. That predicate is deliberately not indexable.
//
// try_cast rather than a plain cast keeps the second property in the first
// idiom: a malformed id yields NULL, the comparison matches nothing, and the
// caller gets ErrTraceIDNotFound -- the same outcome as before, instead of a
// query error surfacing as an internal failure.
func buildSpanSQL(queryNode *search.QueryNode, traceID string) (cteSQL string, whereSQL string, args []any, err error) {
	params := []search.NamedParam{
		{Name: "trace_id", Value: traceID},
	}

	var conditions []string
	if queryNode != nil {
		if err := search.BuildConditions(queryNode, &conditions, &params, traceFieldMapper()); err != nil {
			return "", "", nil, err
		}
	}

	if len(conditions) > 0 {
		whereSQL = "s.trace_id = search_params.trace_id AND (" + strings.Join(conditions, " ") + ")"
	} else {
		whereSQL = "s.trace_id = search_params.trace_id"
	}

	args = make([]any, len(params))
	cteParams := make([]string, len(params))
	for i, p := range params {
		args[i] = p.Value
		if p.Name == "trace_id" {
			// Typed in the CTE so every downstream reference is uuid-to-uuid.
			// Left as a bare parameter, the CTE column is VARCHAR and each use
			// re-derives the comparison's cast direction from context, which is
			// exactly the kind of thing that is fine until it isn't.
			cteParams[i] = "try_cast(? as uuid) as trace_id"
			continue
		}
		cteParams[i] = fmt.Sprintf("? as %s", p.Name)
	}
	cteSQL = fmt.Sprintf("search_params as (select %s)", strings.Join(cteParams, ", "))
	return cteSQL, whereSQL, args, nil
}

// spanSearchFrom is the FROM clause every span search predicate is written
// against. resources and scopes are joined in unconditionally because
// resource.* and scope.* search fields resolve through them. Both are inner
// joins because resource_id and scope_id are NOT NULL.
const spanSearchFrom = `from search_params, spans s
		join resources r on r.id = s.resource_id
		join scopes sc on sc.id = s.scope_id`

var spanColumns = map[string]struct{}{
	"flags":                    {},
	"trace_id":                 {},
	"trace_state":              {},
	"span_id":                  {},
	"parent_span_id":           {},
	"name":                     {},
	"kind":                     {},
	"start_time":               {},
	"end_time":                 {},
	"service_name":             {},
	"dropped_attributes_count": {},
	"dropped_events_count":     {},
	"dropped_links_count":      {},
	"status_code":              {},
	"status_message":           {},
}

// Columns reachable on the joined resources / scopes rows. resource.* and
// scope.* search fields validate against these instead of spanColumns.
var resourceColumns = map[string]struct{}{
	"dropped_attributes_count": {},
}

var scopeColumns = map[string]struct{}{
	"name":                     {},
	"version":                  {},
	"dropped_attributes_count": {},
}

var eventColumns = map[string]struct{}{
	"id":                       {},
	"span_id":                  {},
	"name":                     {},
	"timestamp":                {},
	"dropped_attributes_count": {},
}

// A child reaches its owning span by the pair that identifies one. Matching on
// span_id alone would pull in another trace's events wherever two traces happen
// to share a span id, which OTLP permits because it is unique only within a trace.
const (
	eventOwner = "e.trace_id = s.trace_id and e.span_id = s.span_id"
	linkOwner  = "l.trace_id = s.trace_id and l.span_id = s.span_id"
)

var linkColumns = map[string]struct{}{
	"flags":                    {},
	"id":                       {},
	"span_id":                  {},
	"linked_trace_id":          {},
	"linked_span_id":           {},
	"trace_state":              {},
	"dropped_attributes_count": {},
}

func traceFieldMapper() search.FieldMapper {
	return func(field *search.FieldDefinition, query *search.Query, params *[]search.NamedParam) ([]search.ResolvedExpression, error) {
		switch field.SearchScope {
		case "field":
			expr, err := mapTraceFieldExpression(field)
			if err != nil {
				return nil, err
			}
			return []search.ResolvedExpression{expr}, nil
		case "attribute":
			return mapTraceAttributeExpressions(field, query, params)
		case "global":
			return mapTraceGlobalExpressions()
		default:
			return nil, fmt.Errorf("unknown search scope %s: %w", field.SearchScope, ErrInvalidTraceQuery)
		}
	}
}

func mapTraceFieldExpression(field *search.FieldDefinition) (search.ResolvedExpression, error) {
	if resourceField, found := strings.CutPrefix(field.Name, "resource."); found {
		col := util.CamelToSnake(resourceField)
		if err := util.ValidateColumnName(col, resourceColumns); err != nil {
			return search.ResolvedExpression{}, fmt.Errorf("trace field %q: %w: %w", field.Name, err, ErrInvalidTraceQuery)
		}
		if resourceField == "droppedAttributesCount" {
			return search.NativeInteger("r." + col), nil
		}
		return search.Text("r." + col), nil
	}
	if scopeField, found := strings.CutPrefix(field.Name, "scope."); found {
		col := util.CamelToSnake(scopeField)
		if err := util.ValidateColumnName(col, scopeColumns); err != nil {
			return search.ResolvedExpression{}, fmt.Errorf("trace field %q: %w: %w", field.Name, err, ErrInvalidTraceQuery)
		}
		if scopeField == "droppedAttributesCount" {
			return search.NativeInteger("sc." + col), nil
		}
		return search.Text("sc." + col), nil
	}
	if col, found := strings.CutPrefix(field.Name, "event."); found {
		snake := util.CamelToSnake(col)
		if err := util.ValidateColumnName(snake, eventColumns); err != nil {
			return search.ResolvedExpression{}, fmt.Errorf("event field %q: %w: %w", field.Name, err, ErrInvalidTraceQuery)
		}
		expr := fmt.Sprintf("exists(select 1 from events e where %s and e.%s {COND})", eventOwner, snake)
		if col == "timestamp" {
			return search.Timestamp(expr), nil
		}
		if col == "droppedAttributesCount" {
			return search.NativeInteger(expr), nil
		}
		return search.Text(expr), nil
	}
	if col, found := strings.CutPrefix(field.Name, "link."); found {
		snake := util.CamelToSnake(col)
		// The served link JSON calls the linked target "spanID" and
		// "traceID" (OTLP wire shape); the owning span is implicit in where
		// the link appears. Alias both wire names to the linked_ columns so
		// searching what the UI displays actually matches.
		switch snake {
		case "span_id":
			snake = "linked_span_id"
		case "trace_id":
			snake = "linked_trace_id"
		}
		if err := util.ValidateColumnName(snake, linkColumns); err != nil {
			return search.ResolvedExpression{}, fmt.Errorf("link field %q: %w: %w", field.Name, err, ErrInvalidTraceQuery)
		}
		colExpr := "l." + snake
		// Compare IDs in wire form so malformed input matches nothing instead
		// of being cast to the native integer type.
		switch snake {
		case "linked_span_id":
			colExpr = "span_id_wire(l." + snake + ")"
		case "linked_trace_id":
			colExpr = "replace(l.linked_trace_id::varchar, '-', '')"
		}
		expr := fmt.Sprintf("exists(select 1 from links l where %s and %s {COND})", linkOwner, colExpr)
		if col == "flags" || col == "droppedAttributesCount" {
			return search.NativeInteger(expr), nil
		}
		if snake == "linked_span_id" || snake == "linked_trace_id" {
			return search.WireID(expr), nil
		}
		return search.Text(expr), nil
	}
	if field.Name == "duration" {
		return search.Duration("(s.end_time::hugeint - s.start_time::hugeint)"), nil
	}
	if field.Name == "spanID" || field.Name == "parentSpanID" {
		col := util.CamelToSnake(field.Name)
		return search.WireID("span_id_wire(s." + col + ")"), nil
	}
	if field.Name == "traceID" {
		// Wire-form comparison, same reasoning as the link.traceID branch.
		return search.WireID("replace(s.trace_id::varchar, '-', '')"), nil
	}
	// Keep established readable enum fields searchable while the database owns
	// the received numbers. The explicit numeric fields make unknown codes
	// queryable without teaching the generic search layer about OTel enums.
	switch field.Name {
	case "kind":
		return search.Text(`case s.kind
			when 0 then 'Unspecified' when 1 then 'Internal' when 2 then 'Server'
			when 3 then 'Client' when 4 then 'Producer' when 5 then 'Consumer'
			else 'Unknown (' || s.kind::varchar || ')' end`), nil
	case "statusCode":
		return search.Text(`case s.status_code
			when 0 then 'Unset' when 1 then 'Ok' when 2 then 'Error'
			else 'Unknown (' || s.status_code::varchar || ')' end`), nil
	case "kindCode":
		return search.NativeInteger("s.kind"), nil
	case "statusCodeValue":
		return search.NativeInteger("s.status_code"), nil
	}
	if len(field.Name) > 0 {
		col := util.CamelToSnake(field.Name)
		if err := util.ValidateColumnName(col, spanColumns); err != nil {
			return search.ResolvedExpression{}, fmt.Errorf("trace field %q: %w: %w", field.Name, err, ErrInvalidTraceQuery)
		}
		expr := "s." + col
		switch field.Name {
		case "startTime", "endTime":
			return search.Timestamp(expr), nil
		case "flags", "droppedAttributesCount", "droppedEventsCount", "droppedLinksCount":
			return search.NativeInteger(expr), nil
		default:
			return search.Text(expr), nil
		}
	}
	return search.Text(field.Name), nil
}

// mapTraceAttributeExpressions resolves an attribute against the owner array
// implied by its scope.
//
// Event and link attributes need an EXISTS, because the question is whether
// *any* event or link on the span matches. A span's own attributes are on its
// own row, so those stay a plain attr_value lookup.//
// Resource and scope predicates run against their owner tables, then use the
// resulting IDs to filter spans.
func mapTraceAttributeExpressions(field *search.FieldDefinition, query *search.Query, params *[]search.NamedParam) ([]search.ResolvedExpression, error) {
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
		case "resource":
			attributeIDs = "r.attribute_ids"
		case "scope":
			attributeIDs = "sc.attribute_ids"
		case "span":
			attributeIDs = "s.attribute_ids"
		case "event":
			attributeIDs = "e.attribute_ids"
		case "link":
			attributeIDs = "l.attribute_ids"
		default:
			return nil, fmt.Errorf("unknown attribute scope %s: %w", field.AttributeScope, ErrInvalidTraceQuery)
		}
		predicate, err := search.JSONValueArrayPredicate(attributeIDs, keyParam, kindParam, query, params)
		if err != nil {
			return nil, err
		}
		switch field.AttributeScope {
		case "resource":
			predicate = fmt.Sprintf("s.resource_id in (select r.id from resources r where %s)", predicate)
		case "scope":
			predicate = fmt.Sprintf("s.scope_id in (select sc.id from scopes sc where %s)", predicate)
		case "event":
			predicate = fmt.Sprintf("exists(select 1 from events e where e.trace_id = s.trace_id and e.span_id = s.span_id and %s)", predicate)
		case "link":
			predicate = fmt.Sprintf("exists(select 1 from links l where l.trace_id = s.trace_id and l.span_id = s.span_id and %s)", predicate)
		}
		return []search.ResolvedExpression{search.Complete(predicate)}, nil
	}

	switch field.AttributeScope {
	case "resource":
		return []search.ResolvedExpression{search.AttributeExpression(fmt.Sprintf(
			` s.resource_id in (select r.id from resources r, unnest(r.attribute_ids) t(aid)
				join attributes a on a.id = t.aid where a.key = %s%s and
				%s {COND})`,
			keyParam, kindPredicate, valueExpression), kind, mode)}, nil
	case "scope":
		return []search.ResolvedExpression{search.AttributeExpression(fmt.Sprintf(
			` s.scope_id in (select sc.id from scopes sc, unnest(sc.attribute_ids) t(aid)
				join attributes a on a.id = t.aid where a.key = %s%s and
				%s {COND})`,
			keyParam, kindPredicate, valueExpression), kind, mode)}, nil
	case "span":
		return []search.ResolvedExpression{search.AttributeExpression(fmt.Sprintf(`exists(
			select 1 from unnest(s.attribute_ids) t(aid) join attributes a on a.id = t.aid
			where a.key = %s%s and %s {COND}
		)`, keyParam, kindPredicate, valueExpression), kind, mode)}, nil
	case "event":
		return []search.ResolvedExpression{search.AttributeExpression(fmt.Sprintf(`exists(
			select 1 from events e, unnest(e.attribute_ids) as t(aid)
			join attributes a on a.id = t.aid
			where e.trace_id = s.trace_id and e.span_id = s.span_id and a.key = %s%s and %s {COND}
		)`, keyParam, kindPredicate, valueExpression), kind, mode)}, nil
	case "link":
		return []search.ResolvedExpression{search.AttributeExpression(fmt.Sprintf(`exists(
			select 1 from links l, unnest(l.attribute_ids) as t(aid)
			join attributes a on a.id = t.aid
			where l.trace_id = s.trace_id and l.span_id = s.span_id and a.key = %s%s and %s {COND}
		)`, keyParam, kindPredicate, valueExpression), kind, mode)}, nil
	default:
		return nil, fmt.Errorf("unknown attribute scope %s: %w", field.AttributeScope, ErrInvalidTraceQuery)
	}
}

func mapTraceGlobalExpressions() ([]search.ResolvedExpression, error) {
	return search.TextExpressions([]string{
		"replace(s.trace_id::varchar, '-', '') {COND}",
		"span_id_wire(s.span_id) {COND}",
		"span_id_wire(s.parent_span_id) {COND}",
		"CAST(s.name AS VARCHAR) {COND}",
		`CAST(case s.kind
			when 0 then 'Unspecified' when 1 then 'Internal' when 2 then 'Server'
			when 3 then 'Client' when 4 then 'Producer' when 5 then 'Consumer'
			else 'Unknown (' || s.kind::varchar || ')' end AS VARCHAR) {COND}`,
		`CAST(case s.status_code
			when 0 then 'Unset' when 1 then 'Ok' when 2 then 'Error'
			else 'Unknown (' || s.status_code::varchar || ')' end AS VARCHAR) {COND}`,
		"CAST(s.status_message AS VARCHAR) {COND}",
		"CAST(s.trace_state AS VARCHAR) {COND}",
		"CAST(sc.name AS VARCHAR) {COND}",
		"CAST(sc.version AS VARCHAR) {COND}",
		"exists(select 1 from events e where " + eventOwner + " and CAST(e.name AS VARCHAR) {COND})",
		"exists(select 1 from links l where " + linkOwner + " and (replace(l.linked_trace_id::varchar, '-', '') {COND} or CAST(l.trace_state AS VARCHAR) {COND} or span_id_wire(l.linked_span_id) {COND}))",
		// Resource, scope and span attributes are on the span's own row, so
		// concatenating those three arrays is one unnest. Events and links are
		// separate rows and need their own EXISTS.
		matchAnyAttribute("unnest(s.attribute_ids || r.attribute_ids || sc.attribute_ids) as t(aid)", ""),
		matchAnyAttribute("events e, unnest(e.attribute_ids) as t(aid)", eventOwner+" and "),
		matchAnyAttribute("links l, unnest(l.attribute_ids) as t(aid)", linkOwner+" and "),
	}), nil
}

// matchAnyAttribute builds an EXISTS that resolves an owner's attribute id
// array against the dictionary and tests every stored form against the free-text
// term: key, value, and element-wise for the four array types.
//
// from is the source that produces t(aid); scope narrows that source to the
// current span and is empty when the array is already on the span's own row.
func matchAnyAttribute(from, scope string) string {
	return fmt.Sprintf(`exists(
		select 1
		from %s
		join attributes a on a.id = t.aid
		where %s(
			a.key {COND} or a.value::varchar {COND} or
			exists(select 1 from json_each(a.value, '$.value') j where j.value::varchar {COND})
		)
	)`, from, scope)
}
