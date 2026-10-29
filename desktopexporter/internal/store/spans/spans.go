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
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/nested"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/searchindex"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/CtrlSpice/otel-desktop-viewer/internal/discovery"
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

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// resourceServiceName extracts the service.name resource attribute as a
// plain string, returning "" when not present. This is the same logic
// used by the metrics package to denormalize service onto metrics,
// kept private here so spans doesn't grow a metrics dependency.
func resourceServiceName(attrs pcommon.Map) string {
	if v, ok := util.LastValue(attrs, "service.name"); ok {
		return v.AsString()
	}
	return ""
}

// Ingest is IngestReport for callers with nowhere to put the report. Refused
// rows are still skipped rather than failing the batch.
func Ingest(ctx context.Context, conn driver.Conn, traces ptrace.Traces) error {
	_, err := IngestReport(ctx, conn, traces)
	return err
}

// IngestReport ingests trace spans into the spans, events and links
// tables, reporting the spans it could not write. A non-empty Rejected is not a
// failure: the batch landed without them. The caller must hold any required
// lock on the connection.
//
// Check span identities before appending self-contained payloads. Bisection
// isolates bad rows without separating owners from their attributes.
func IngestReport(ctx context.Context, conn driver.Conn, traces ptrace.Traces) (_ ingest.Rejected, err error) {
	defer func() { err = ingest.InterruptedContextError(ctx, err) }()

	if err := ctx.Err(); err != nil {
		return ingest.Rejected{}, err
	}

	// Span identities in walk order, so dedupe below can speak in the same
	// ordinals the append pass and bisection do.
	var spanKeys []spanKey

	for _, resourceSpan := range traces.ResourceSpans().All() {
		for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
			for _, span := range scopeSpan.Spans().All() {
				spanKeys = append(spanKeys, spanKey{
					trace: duckdb.UUID(span.TraceID()),
					span:  util.SpanIDUint64(span.SpanID()),
				})
			}
		}
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
	rejected, err := appendSpansBisecting(ctx, conn, traces, skip)
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
// occurrences of one id stay separable.
func appendPass(
	ctx context.Context,
	conn driver.Conn,
	traces ptrace.Traces,
	keep func(ordinal int) bool,
) (err error) {
	a := nested.NewSpanAppender(ctx, conn)
	var catalogue searchindex.Batch
	defer func() {
		if err != nil {
			a.Discard()
			return
		}
		err = errors.Join(err, a.Close())
		if err == nil {
			if indexErr := catalogue.Write(ctx, conn); indexErr != nil {
				err = fmt.Errorf("%w: %w", ingest.ErrNotRowFault, indexErr)
			}
		}
	}()

	var spanCur int
	for _, resourceSpan := range traces.ResourceSpans().All() {
		resource := resourceSpan.Resource()
		resourceInput, err := ingest.ResourceInput(resource, resourceSpan.SchemaUrl())
		if err != nil {
			return err
		}
		serviceName := resourceServiceName(resource.Attributes())

		for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
			scopeInput, err := ingest.ScopeInput(scopeSpan.Scope(), scopeSpan.SchemaUrl())
			if err != nil {
				return err
			}

			a.SetContext(resourceInput, scopeInput)
			for _, span := range scopeSpan.Spans().All() {
				if err := ctx.Err(); err != nil {
					return err
				}

				if !keep(spanCur) {
					spanCur++
					continue
				}

				traceUUID := duckdb.UUID(span.TraceID())

				spanID := util.SpanIDUint64(span.SpanID())
				rootKey := fmt.Sprintf("%s:%016x", span.TraceID().String(), spanID)

				var parentSpanID driver.Value
				if pid := span.ParentSpanID(); !pid.IsEmpty() {
					parentSpanID = util.SpanIDUint64(pid)
				}

				attributes, err := ingest.AttributeBytes(span.Attributes())
				if err != nil {
					return err
				}
				spanCur++

				err = a.AppendSpan(span, attributes, serviceName)
				if err != nil {
					return fmt.Errorf("Ingest: %w: %w", ErrSpansStoreInternal, err)
				}
				resourceRef, scopeRef, err := catalogue.Contexts(resourceInput, scopeInput)
				if err != nil {
					return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
				}
				parent := searchindex.Object{Ref: "span:span:" + rootKey, Signal: "span", Owner: "span", RootKey: rootKey, RecordKey: rootKey, ResourceRef: resourceRef, ScopeRef: scopeRef}
				if err := parent.AddNative("attributes", attributes); err != nil {
					return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
				}
				kind, status := span.Kind().String(), span.Status().Code().String()
				if int32(span.Kind()) < 0 || int32(span.Kind()) > 5 {
					kind = fmt.Sprintf("Unknown (%d)", span.Kind())
				}
				if int32(span.Status().Code()) < 0 || int32(span.Status().Code()) > 2 {
					status = fmt.Sprintf("Unknown (%d)", span.Status().Code())
				}
				var parentWireID any
				if parentSpanID != nil {
					parentWireID = span.ParentSpanID().String()
				}
				if err := parent.AddStrings(map[string]any{"traceID": span.TraceID().String(), "spanID": span.SpanID().String(), "parentSpanID": parentWireID, "name": span.Name(), "statusMessage": span.Status().Message(), "traceState": span.TraceState().AsRaw(), "kind": kind, "status": status}); err != nil {
					return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
				}
				catalogue.Add(parent)

				for ordinal, event := range span.Events().All() {
					eventID := uuid.New()
					attributes, err := ingest.AttributeBytes(event.Attributes())
					if err != nil {
						return err
					}
					err = a.AppendEvent(event, eventID, traceUUID, spanID, ordinal, attributes)
					if err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrSpansStoreInternal, err)
					}
					child := searchindex.Object{Ref: "span:event:" + eventID.String(), Signal: "span", Owner: "event", RootKey: rootKey, RecordKey: eventID.String(), ParentRef: parent.Ref}
					if err := child.AddNative("attributes", attributes); err != nil {
						return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
					}
					if err := child.AddStrings(map[string]any{"name": event.Name()}); err != nil {
						return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
					}
					catalogue.Add(child)
				}

				for ordinal, link := range span.Links().All() {
					linkID := uuid.New()
					var linkTraceUUID driver.Value
					if tid := link.TraceID(); !tid.IsEmpty() {
						linkTraceUUID = duckdb.UUID(tid)
					}
					var linkSpanID driver.Value
					if sid := link.SpanID(); !sid.IsEmpty() {
						linkSpanID = util.SpanIDUint64(sid)
					}

					attributes, err := ingest.AttributeBytes(link.Attributes())
					if err != nil {
						return err
					}
					err = a.AppendLink(link, linkID, traceUUID, spanID, ordinal, attributes)
					if err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrSpansStoreInternal, err)
					}
					child := searchindex.Object{Ref: "span:link:" + linkID.String(), Signal: "span", Owner: "link", RootKey: rootKey, RecordKey: linkID.String(), ParentRef: parent.Ref}
					if err := child.AddNative("attributes", attributes); err != nil {
						return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
					}
					var traceWireID, spanWireID any
					if linkTraceUUID != nil {
						traceWireID = link.TraceID().String()
					}
					if linkSpanID != nil {
						spanWireID = link.SpanID().String()
					}
					if err := child.AddStrings(map[string]any{"traceID": traceWireID, "spanID": spanWireID, "traceState": link.TraceState().AsRaw()}); err != nil {
						return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
					}
					catalogue.Add(child)
				}
			}
		}
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
	return traceSummariesSQL(timeRange, criteria, options, false)
}

func traceSummariesSQL(timeRange timerange.TimeRange, criteria any, options search.ResultOptions, native bool) (string, []any, error) {
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
		identity := `json_object(
				'traceID', replace(trace_id::varchar, '-', ''),
				'spanID', span_id_wire(span_id)
			)`
		if native {
			identity = `struct_pack(traceID := trace_id_wire(trace_id), spanID := span_id_wire(span_id))`
		}
		matchCTEs = fmt.Sprintf(`
		query_matches as materialized (
			select distinct s.trace_id, s.span_id, s.start_time
			%s
			join selected_trace_ids selected on selected.trace_id = s.trace_id
			where %s
		),
		matched_span_lists as (
			select trace_id as matched_trace_id, list(%s order by start_time, span_id) as matched_spans
			from query_matches
			group by trace_id
		),`, spanSearchFrom, queryWhere, identity)
		matchJoin = "\n\t\tjoin matched_span_lists matches on matches.matched_trace_id = sub.trace_id"
		matchProjection = ",\n\t\t\t'matchedSpans', to_json(matches.matched_spans)"
	}

	render := queries.Render
	if native {
		render = queries.RenderArrow
	}
	finalQuery, err := render(queries.SearchTraceSummaries, searchTraceSummariesParams{
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

// GetTraceView returns a trace with matching spans annotated when criteria is
// provided. Optional spanIDs restrict payloads to those exact stored IDs and
// compress display depth through available ancestry; received parent IDs remain intact.
//
// The ordinary tree walk reports unreachable spans. A non-zero count triggers
// the cycle-aware query, which chooses entry points and tracks ancestry.
func GetTraceView(ctx context.Context, db *sql.DB, traceID string, criteria any, spanIDs ...uint64) (json.RawMessage, error) {
	query, args, err := renderSpansQuery(queries.GetTraceView, traceID, criteria, spanIDs...)
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

	// Re-run ancestry with the cycle-aware walk, retaining the same payload selection.
	salvaged, err := salvageSpans(ctx, db, traceID, criteria, spanIDs...)
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
	return GetTraceSelectionOTLP(ctx, db, traceID, "")
}

// GetTraceSelectionOTLP keeps resource/scope wrappers and filters by exact span identity.
// spanID is decimal unsigned text, or empty for the complete retained trace.
func GetTraceSelectionOTLP(ctx context.Context, db *sql.DB, traceID, spanID string) (json.RawMessage, error) {
	query, err := queries.Render(queries.GetTraceOTLP, nil)
	if err != nil {
		return nil, fmt.Errorf("GetTraceOTLP: %w: %w", ErrSpansStoreInternal, err)
	}

	var raw []byte
	if err := db.QueryRowContext(ctx, query, traceID, spanID, spanID).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetTraceOTLP: %w", ErrTraceIDNotFound)
		}
		return nil, fmt.Errorf("GetTraceOTLP: %w: %w", ErrSpansStoreInternal, err)
	}
	return json.RawMessage(raw), nil
}

// salvageSpans runs the cycle-aware variant of the trace fetch.
func salvageSpans(ctx context.Context, db *sql.DB, traceID string, criteria any, spanIDs ...uint64) (json.RawMessage, error) {
	query, args, err := renderSpansQuery(queries.SalvageSpans, traceID, criteria, spanIDs...)
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
func renderSpansQuery(name queries.Name, traceID string, criteria any, spanIDs ...uint64) (string, []any, error) {
	return spansQuery(name, traceID, criteria, false, spanIDs...)
}

func spansQuery(name queries.Name, traceID string, criteria any, native bool, spanIDs ...uint64) (string, []any, error) {
	return traceQuery(name, traceID, criteria, native, false, spanIDs...)
}

func traceQuery(name queries.Name, traceID string, criteria any, native, lightweight bool, spanIDs ...uint64) (string, []any, error) {
	return traceQueryColumns(name, traceID, criteria, native, lightweight, nil, spanIDs...)
}

func traceQueryColumns(name queries.Name, traceID string, criteria any, native, lightweight bool, columns []WaterfallColumn, spanIDs ...uint64) (string, []any, error) {
	var searchTree *search.QueryNode
	if criteria != nil {
		var err error
		searchTree, err = search.ParseQueryTree(criteria)
		if err != nil {
			return "", nil, fmt.Errorf("GetTraceView: %w: %w", ErrInvalidTraceQuery, err)
		}
	}

	_, whereClause, args, err := buildSpanSQL(searchTree, traceID)
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
	selectedIDs := ""
	if len(spanIDs) > 0 {
		selectedIDs = fmt.Sprintf("$%d::ubigint[]", len(args)+1)
		args = append(args, spanIDs)
	}
	render := queries.Render
	if native {
		render = queries.RenderArrow
	}
	previews, args, err := waterfallColumnExpressions(columns, args, "ts")
	if err != nil {
		return "", nil, err
	}
	query, err := render(name, getTraceViewParams{
		SelectedIDs:    selectedIDs,
		MatchedCTE:     matchedCTE,
		MatchedExpr:    matchedExpr,
		MatchedJoin:    matchedJoin,
		Selected:       len(spanIDs) > 0,
		Salvage:        name == queries.SalvageSpans,
		Lightweight:    lightweight,
		ColumnPreviews: previews,
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
	query, args, err := searchindex.FieldValuesSQL("traces", field, term, limit)
	if err != nil {
		return nil, fmt.Errorf("GetFieldValueCompletions: %w: field %q has no value completion", ErrInvalidTraceQuery, field)
	}
	raw, err := searchindex.FieldValuesJSON(ctx, db, query, args)
	if err != nil {
		return nil, fmt.Errorf("GetFieldValueCompletions: %w: %w", ErrSpansStoreInternal, err)
	}
	return raw, nil
}

func GetTraceAttributeDefinitionsByTraceID(ctx context.Context, db *sql.DB, traceID string) (json.RawMessage, error) {
	raw, err := searchindex.DefinitionsJSON(ctx, db, "span", traceID)
	if err != nil {
		return nil, fmt.Errorf("GetTraceAttributeDefinitionsByTraceID: %w: %w", ErrSpansStoreInternal, err)
	}
	return raw, nil
}

func traceAttributeKeys(ctx context.Context, db *sql.DB) (json.RawMessage, error) {
	return searchindex.Definitions(ctx, db, "span")
}

// Clear deletes owning span rows and their complete search catalogue.
func Clear(ctx context.Context, db *sql.DB) error {
	return nested.Mutate(ctx, db, `delete from spans`)
}

// DeleteSpansByTraceIDs deletes all spans for multiple traces.
func DeleteSpansByTraceIDs(ctx context.Context, db *sql.DB, traceIDs []any) error {
	return nested.Mutate(ctx, db, `delete from spans where trace_id in(select id from uuid_list(?))`, util.ToStringList(traceIDs))
}

func buildTraceSQL(queryNode *search.QueryNode, timeRange timerange.TimeRange) (cteSQL string, eligibilityWhere string, queryWhere string, args []any, err error) {
	timeCondition, timeParams := search.TimePredicate("s.start_time", timeRange.Start, timeRange.End)
	timeCondition = strings.ReplaceAll(timeCondition, " AND ", " and ")
	const documentScope = "try_cast(split_part(search_record.root_key,':',1) as uuid)=s.trace_id and try_cast('0x'||split_part(search_record.root_key,':',2) as ubigint)=s.span_id"
	cteSQL, eligibilityWhere, args, err = searchindex.BuildSearchSQL(queryNode, traceFieldMapper(), timeCondition, timeParams, "span", documentScope)
	if err != nil || queryNode == nil {
		return cteSQL, eligibilityWhere, "", args, err
	}
	// Keep time bindings in the parameter list so query references retain
	// the same positions as eligibilityWhere.
	_, queryWhere, _, err = searchindex.BuildSearchSQL(queryNode, traceFieldMapper(), "", timeParams, "span", documentScope)
	return cteSQL, eligibilityWhere, queryWhere, args, err
}

// MatchingRowsSQL shares the trace search predicate with cross-signal discovery.
// It returns matching spans, rather than expanding them to their complete traces.
func MatchingRowsSQL(queryNode *search.QueryNode, timeRange timerange.TimeRange) (string, []any, error) {
	cte, where, _, args, err := buildTraceSQL(queryNode, timeRange)
	return cte + " SELECT s.* " + spanSearchFrom + " WHERE " + where, args, err
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
		whereSQL = "s.trace_id = try_cast($1 as uuid) AND (" + strings.Join(conditions, " ") + ")"
	} else {
		whereSQL = "s.trace_id = try_cast($1 as uuid)"
	}

	return "", whereSQL, search.Arguments(params), nil
}

// spanSearchFrom is the FROM clause every span search predicate is written
// against. Resource and scope fields live on each span.
const spanSearchFrom = `from spans s`

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
	return search.Resolver("span", map[string]search.ValueOwner{
		"span":     {Documents: map[string]string{"attributes": "s.attributes"}},
		"resource": {Documents: map[string]string{"attributes": "s.resource.attributes"}},
		"scope":    {Documents: map[string]string{"attributes": "s.scope.attributes"}},
		"event":    {Documents: map[string]string{"attributes": "e.attributes"}, Correlation: "events e where " + eventOwner},
		"link":     {Documents: map[string]string{"attributes": "l.attributes"}, Correlation: "links l where " + linkOwner},
	}, mapTraceFieldExpression, search.RejectUnindexedText)
}

func mapTraceFieldExpression(field *search.FieldDefinition) (search.ResolvedExpression, error) {
	if field.Name == "serviceIdentity" || field.Name == "resource.serviceIdentity" {
		return search.Text(discovery.ServiceIdentity("s.resource")), nil
	}
	if resourceField, found := strings.CutPrefix(field.Name, "resource."); found {
		col := util.CamelToSnake(resourceField)
		if err := util.ValidateColumnName(col, resourceColumns); err != nil {
			return search.ResolvedExpression{}, fmt.Errorf("trace field %q: %w: %w", field.Name, err, ErrInvalidTraceQuery)
		}
		if resourceField == "droppedAttributesCount" {
			return search.NativeInteger("s.resource." + col), nil
		}
		return search.Text("s.resource." + col), nil
	}
	if scopeField, found := strings.CutPrefix(field.Name, "scope."); found {
		col := util.CamelToSnake(scopeField)
		if err := util.ValidateColumnName(col, scopeColumns); err != nil {
			return search.ResolvedExpression{}, fmt.Errorf("trace field %q: %w: %w", field.Name, err, ErrInvalidTraceQuery)
		}
		if scopeField == "droppedAttributesCount" {
			return search.NativeInteger("s.scope." + col), nil
		}
		return search.Text("s.scope." + col), nil
	}
	if col, found := strings.CutPrefix(field.Name, "event."); found {
		snake := util.CamelToSnake(col)
		if err := util.ValidateColumnName(snake, eventColumns); err != nil {
			return search.ResolvedExpression{}, fmt.Errorf("event field %q: %w: %w", field.Name, err, ErrInvalidTraceQuery)
		}
		expr := "e." + snake
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
		expr := colExpr
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
