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
	ErrSpansStoreInternal = errors.New("spans store internal error")
)

// flushIntervalSpans bounds how many spans accumulate in the appenders before
// they are pushed to DuckDB. It exists to cap memory on a pathological batch,
// not to make writes visible sooner -- nothing reads mid-batch, since ingest
// holds the store write lock throughout.
//
// Raised from 50 to 500 on measurement. Each flush is a cgo call into
// duckdb_appender_flush, and that call is roughly half of ingest time, so
// flushing more often than memory requires is pure overhead. Measured on
// 2000-span batches (Apple M4 Pro -- absolute figures are an upper bound, but
// the shape of the curve is what the choice rests on, and slower hardware moves
// the knee later, not earlier):
//
//	interval   50 -> 15.77 us/span
//	          100 -> 11.14
//	          250 ->  8.12
//	          500 ->  7.13   <- knee
//	         1000 ->  6.60
//	   close only ->  6.27
//
// Past 500 the remaining 14% buys unbounded appender memory, which is a bad
// trade for a desktop tool sharing RAM with the user's actual work.
const flushIntervalSpans = 500

// resourceServiceName extracts the service.name resource attribute as a
// plain string, returning "" when not present. This is the same logic
// used by the metrics package to denormalize service onto metric_streams,
// kept private here so spans doesn't grow a metrics dependency.
func resourceServiceName(attrs pcommon.Map) string {
	if v, ok := attrs.Get("service.name"); ok {
		return v.AsString()
	}
	return ""
}

// Ingest ingests trace spans from pdata into the spans, events, links, and
// attributes tables. The caller must hold any required lock on the connection.
//
// Two passes. The first walks the resource/scope hierarchy, hashing every
// attribute into a Dictionary and resolving each resource and scope to a
// content-derived id; it then writes the dictionary in three inserts. The
// second opens the appenders and walks again, appending owners with the id
// arrays the first pass produced.
//
// The split exists because the dictionary needs conflict handling and the
// appender has none: its whole surface is AppendRow/Flush/Clear/Close, and a
// duplicate errors at flush and takes the chunk with it. Ordering is safe
// because the inserts commit before any appender flush, and the appenders open
// after the resolve -- the same shape metrics.Ingest already used for its
// stream upsert.
func Ingest(ctx context.Context, conn driver.Conn, traces ptrace.Traces, flushed *ingest.FlushedIDs) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Pass 1: hash everything and resolve resource/scope identities.
	dict := ingest.NewDictionary(flushed)

	type scopeKey struct{ ri, si int }
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

	for ri, resourceSpan := range traces.ResourceSpans().All() {
		resourceIDs[ri] = dict.AddResource(resourceSpan.Resource())
		for si, scopeSpan := range resourceSpan.ScopeSpans().All() {
			scopeIDs[scopeKey{ri, si}] = dict.AddScope(scopeSpan.Scope())
			for _, span := range scopeSpan.Spans().All() {
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
		return fmt.Errorf("Ingest: %w: %w", ErrSpansStoreInternal, err)
	}

	// Pass 2: append the signal rows.
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

				traceUUID := duckdb.UUID(span.TraceID())

				spanID := span.SpanID()
				var spanPadded [16]byte
				copy(spanPadded[8:], spanID[:])
				spanUUID := duckdb.UUID(spanPadded)

				var parentSpanUUID *duckdb.UUID
				if pid := span.ParentSpanID(); !pid.IsEmpty() {
					var parentPadded [16]byte
					copy(parentPadded[8:], pid[:])
					u := duckdb.UUID(parentPadded)
					parentSpanUUID = &u
				}

				// Hashed in pass 1. NonNil because AttributeSet returns nil for
				// an empty map while the column is NOT NULL: an owner with no
				// attributes stores an empty array.
				spanAttrIDs := ingest.NonNil(spanAttrs[spanCur])
				spanCur++

				err := appenders["spans"].AppendRow(
					traceUUID,                     // TraceID UUID
					span.TraceState().AsRaw(),     // TraceState VARCHAR
					spanUUID,                      // SpanID UUID
					parentSpanUUID,                // ParentSpanID UUID
					span.Name(),                   // Name VARCHAR
					span.Kind().String(),          // Kind VARCHAR
					int64(span.StartTimestamp()),  // StartTime BIGINT
					int64(span.EndTimestamp()),    // EndTime BIGINT
					resourceID,                    // ResourceID UUID
					scopeID,                       // ScopeID UUID
					spanAttrIDs,                   // AttributeIDs UUID[]
					span.DroppedAttributesCount(), // DroppedAttributesCount UINTEGER
					span.DroppedEventsCount(),     // DroppedEventsCount UINTEGER
					span.DroppedLinksCount(),      // DroppedLinksCount UINTEGER
					span.Status().Code().String(), // StatusCode VARCHAR
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
						spanUUID,                       // SpanID UUID
						event.Name(),                   // Name VARCHAR
						int64(event.Timestamp()),       // Timestamp BIGINT
						eventAttrIDs,                   // AttributeIDs UUID[]
						event.DroppedAttributesCount(), // DroppedAttributesCount UINTEGER
					)
					if err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrSpansStoreInternal, err)
					}
				}

				for _, link := range span.Links().All() {
					linkTraceUUID := duckdb.UUID(link.TraceID())
					linkSpanID := link.SpanID()
					var linkSpanPadded [16]byte
					copy(linkSpanPadded[8:], linkSpanID[:])
					linkSpanUUID := duckdb.UUID(linkSpanPadded)

					linkAttrIDs := ingest.NonNil(linkAttrs[linkCur])
					linkCur++
					err = appenders["links"].AppendRow(
						duckdb.UUID(uuid.New()),       // ID UUID
						spanUUID,                      // SpanID UUID
						linkTraceUUID,                 // TraceID UUID
						linkSpanUUID,                  // LinkedSpanID UUID
						link.TraceState().AsRaw(),     // TraceState VARCHAR
						linkAttrIDs,                   // AttributeIDs UUID[]
						link.DroppedAttributesCount(), // DroppedAttributesCount UINTEGER
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
		return fmt.Errorf("Ingest: %w: pass mismatch (spans %d/%d, events %d/%d, links %d/%d)",
			ErrSpansStoreInternal, spanCur, len(spanAttrs),
			eventCur, len(eventAttrs), linkCur, len(linkAttrs))
	}

	return nil
}

// SearchTraces returns trace summaries in the time range matching the optional criteria.
func SearchTraces(ctx context.Context, db *sql.DB, startTime, endTime int64, criteria any) (json.RawMessage, error) {
	finalQuery, args, err := searchTracesSQL(startTime, endTime, criteria)
	if err != nil {
		return nil, err
	}

	var raw []byte
	if err := db.QueryRowContext(ctx, finalQuery, args...).Scan(&raw); err != nil {
		return nil, fmt.Errorf("SearchTraces: %w: %w", ErrSpansStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// searchTracesSQL renders the trace-summary query and its bound arguments.
// Split out for the same reason as searchSpansSQL: so a golden test can pin the
// rendered text without standing up a store.
func searchTracesSQL(startTime, endTime int64, criteria any) (string, []any, error) {
	var searchTree *search.QueryNode
	if criteria != nil {
		var err error
		searchTree, err = search.ParseQueryTree(criteria)
		if err != nil {
			return "", nil, fmt.Errorf("SearchTraces: %w: %w", ErrInvalidTraceQuery, err)
		}
	}

	cteSQL, whereClause, args, err := buildTraceSQL(searchTree, startTime, endTime)
	if err != nil {
		return "", nil, fmt.Errorf("SearchTraces: %w: %w", ErrInvalidTraceQuery, err)
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
	finalQuery, err := queries.Render(queries.SearchTraces, searchTracesParams{
		CTEs:  cteSQL,
		From:  spanSearchFrom,
		Where: whereClause,
	})
	if err != nil {
		return "", nil, fmt.Errorf("SearchTraces: %w: %w", ErrSpansStoreInternal, err)
	}

	return finalQuery, args, nil
}

// SearchSpans returns spans for a single trace, optionally filtered by search criteria.
// When criteria is nil, all spans for the trace are returned (replacing GetTrace).
// When criteria is provided, only matching spans are returned (replacing SearchTraceSpans).
func SearchSpans(ctx context.Context, db *sql.DB, traceID string, criteria any) (json.RawMessage, error) {
	query, args, err := searchSpansSQL(traceID, criteria)
	if err != nil {
		return nil, err
	}

	var raw []byte
	if err := db.QueryRowContext(ctx, query, args...).Scan(&raw); err != nil {
		return nil, fmt.Errorf("SearchSpans: %w: %w", ErrSpansStoreInternal, err)
	}
	if raw == nil {
		return nil, fmt.Errorf("SearchSpans: %w", ErrTraceIDNotFound)
	}
	return json.RawMessage(raw), nil
}

// searchSpansSQL renders the trace-fetch query and its bound arguments.
//
// Split out from SearchSpans so the SQL is reachable without a database. That
// is what lets a golden test assert the rendered text, which in turn is what
// makes moving these query bodies into .sql files provable as a no-op rather
// than merely believed to be one.
func searchSpansSQL(traceID string, criteria any) (string, []any, error) {
	var searchTree *search.QueryNode
	if criteria != nil {
		var err error
		searchTree, err = search.ParseQueryTree(criteria)
		if err != nil {
			return "", nil, fmt.Errorf("SearchSpans: %w: %w", ErrInvalidTraceQuery, err)
		}
	}

	cteSQL, whereClause, args, err := buildSpanSQL(searchTree, traceID)
	if err != nil {
		return "", nil, fmt.Errorf("SearchSpans: %w: %w", ErrInvalidTraceQuery, err)
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

	// The recursion carries only what the walk needs -- six narrow columns --
	// and the payload is joined back on once, in the `tree` CTE. Materialising
	// all 17 span columns through every recursion level was the single largest
	// avoidable cost here: copied at each depth, thrown away at every level but
	// the last.
	//
	// `tree` exists so the trace's span set is defined in exactly one place.
	// Without it the payload gets re-derived four separate times after the
	// recursion (span_attrs, resource_data, scope_data, and the final
	// projection), which is easy to get subtly out of step. Measured at no
	// performance difference -- DuckDB already materialises a recursive CTE once
	// and reuses it across references, and the re-derivations were PK-indexed
	// joins over a few thousand rows. Kept for the structure, not the speed.
	query, err := queries.Render(queries.SearchSpans, searchSpansParams{
		CTEs:        cteSQL,
		MatchedCTE:  matchedCTE,
		MatchedExpr: matchedExpr,
		MatchedJoin: matchedJoin,
	})
	if err != nil {
		return "", nil, fmt.Errorf("SearchSpans: %w: %w", ErrSpansStoreInternal, err)
	}

	return query, args, nil
}

// GetTraceAttributes returns every attribute name/scope/type this store knows
// about, for the search field dropdowns.
//
// The time range is accepted and ignored, and that is a deliberate behaviour
// change. These used to be an indexed join from attributes to spans over the
// window; with attributes deduped into a dictionary, honouring the window would
// mean unnesting every span's array and joining back -- on the interactive path
// that populates a dropdown. The dictionary is small and already bounded by
// retention, so it answers directly. The semantics become "keys this store
// knows about" rather than "keys in this window", which is the better answer
// for a dropdown anyway.
//
// This is why scope is part of dictionary identity: without it, attributeScope
// could not be produced without the unnest this exists to avoid.
func GetTraceAttributes(ctx context.Context, db *sql.DB, startTime, endTime int64) (json.RawMessage, error) {
	return traceAttributeKeys(ctx, db)
}

// GetAttributesByTraceID returns the same store-wide key list as
// GetTraceAttributes. Narrowing to one trace would cost the unnest described
// there; both callers populate the same dropdown.
func GetAttributesByTraceID(ctx context.Context, db *sql.DB, traceID string) (json.RawMessage, error) {
	return traceAttributeKeys(ctx, db)
}

func traceAttributeKeys(ctx context.Context, db *sql.DB) (json.RawMessage, error) {
	query := `
		select cast(to_json(list(attribute_def_json(sub.key, sub.scope, sub.type)
			order by sub.key, sub.scope)) as varchar) as attributes
		from (
			select distinct a.key, a.scope, a.type
			from attributes a
			where a.scope in ('resource', 'scope', 'span', 'event', 'link')
		) sub
	`
	var raw []byte
	if err := db.QueryRowContext(ctx, query).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetTraceAttributes: %w: %w", ErrSpansStoreInternal, err)
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

// DeleteSpansByIDs deletes multiple spans by their IDs.
func DeleteSpansByIDs(ctx context.Context, db *sql.DB, spanIDs []any) error {
	if len(spanIDs) == 0 {
		return nil
	}
	// One bound list per statement, rather than one placeholder per id: the
	// SQL is static and cannot disagree with the argument count.
	ids := util.ToStringList(spanIDs)
	childQueries := []string{
		`delete from links where span_id in (select id from uuid_list(?))`,
		`delete from events where span_id in (select id from uuid_list(?))`,
		`delete from spans where span_id in (select id from uuid_list(?))`,
	}
	for _, q := range childQueries {
		if _, err := db.ExecContext(ctx, q, ids); err != nil {
			return fmt.Errorf("DeleteSpansByIDs: %w: %w", ErrSpansStoreInternal, err)
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
		`delete from links where span_id in (select span_id from spans where trace_id in (select id from uuid_list(?)))`,
		`delete from events where span_id in (select span_id from spans where trace_id in (select id from uuid_list(?)))`,
		`delete from spans where trace_id in (select id from uuid_list(?))`,
	}
	for _, q := range childQueries {
		if _, err := db.ExecContext(ctx, q, ids); err != nil {
			return fmt.Errorf("DeleteSpansByTraceIDs: %w: %w", ErrSpansStoreInternal, err)
		}
	}
	return nil
}

func buildTraceSQL(queryNode *search.QueryNode, startTime, endTime int64) (cteSQL string, whereSQL string, args []any, err error) {
	return search.BuildSearchSQL(queryNode, startTime, endTime, traceFieldMapper(), "s.start_time >= time_start and s.start_time <= time_end")
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
// resource.* and scope.* search fields now resolve through them rather than
// through denormalized columns on spans; both are inner joins, since
// resource_id and scope_id are NOT NULL.
const spanSearchFrom = `from search_params, spans s
		join resources r on r.id = s.resource_id
		join scopes sc on sc.id = s.scope_id`

var spanColumns = map[string]struct{}{
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

var linkColumns = map[string]struct{}{
	"id":                       {},
	"span_id":                  {},
	"trace_id":                 {},
	"linked_span_id":           {},
	"trace_state":              {},
	"dropped_attributes_count": {},
}

func traceFieldMapper() search.FieldMapper {
	return func(field *search.FieldDefinition, query *search.Query, params *[]search.NamedParam) ([]string, error) {
		switch field.SearchScope {
		case "field":
			expr, err := mapTraceFieldExpression(field)
			if err != nil {
				return nil, err
			}
			return []string{expr}, nil
		case "attribute":
			return mapTraceAttributeExpressions(field, query, params)
		case "global":
			return mapTraceGlobalExpressions()
		default:
			return nil, fmt.Errorf("unknown search scope %s: %w", field.SearchScope, ErrInvalidTraceQuery)
		}
	}
}

func mapTraceFieldExpression(field *search.FieldDefinition) (string, error) {
	if resourceField, found := strings.CutPrefix(field.Name, "resource."); found {
		col := util.CamelToSnake(resourceField)
		if err := util.ValidateColumnName(col, resourceColumns); err != nil {
			return "", fmt.Errorf("trace field %q: %w: %w", field.Name, err, ErrInvalidTraceQuery)
		}
		return "r." + col, nil
	}
	if scopeField, found := strings.CutPrefix(field.Name, "scope."); found {
		col := util.CamelToSnake(scopeField)
		if err := util.ValidateColumnName(col, scopeColumns); err != nil {
			return "", fmt.Errorf("trace field %q: %w: %w", field.Name, err, ErrInvalidTraceQuery)
		}
		return "sc." + col, nil
	}
	if col, found := strings.CutPrefix(field.Name, "event."); found {
		snake := util.CamelToSnake(col)
		if err := util.ValidateColumnName(snake, eventColumns); err != nil {
			return "", fmt.Errorf("event field %q: %w: %w", field.Name, err, ErrInvalidTraceQuery)
		}
		return fmt.Sprintf("exists(select 1 from events e where e.span_id = s.span_id and e.%s {COND})", snake), nil
	}
	if col, found := strings.CutPrefix(field.Name, "link."); found {
		snake := util.CamelToSnake(col)
		// The served link JSON calls the linked target "spanID" (OTLP wire
		// shape); the owning span is implicit in where the link appears.
		// Alias the wire name to the linked_span_id column so searching
		// what the UI displays actually matches.
		if snake == "span_id" {
			snake = "linked_span_id"
		}
		if err := util.ValidateColumnName(snake, linkColumns); err != nil {
			return "", fmt.Errorf("link field %q: %w: %w", field.Name, err, ErrInvalidTraceQuery)
		}
		colExpr := "l." + snake
		// Compare IDs in wire form: the stored uuid is zero-padded for span
		// IDs, and raw uuid-column comparisons error on malformed input.
		switch snake {
		case "linked_span_id":
			colExpr = "right(replace(l." + snake + "::varchar, '-', ''), 16)"
		case "trace_id":
			colExpr = "replace(l.trace_id::varchar, '-', '')"
		}
		return fmt.Sprintf("exists(select 1 from links l where l.span_id = s.span_id and %s {COND})", colExpr), nil
	}
	if field.Name == "duration" {
		return "(s.end_time - s.start_time)", nil
	}
	if field.Name == "spanID" || field.Name == "parentSpanID" {
		col := util.CamelToSnake(field.Name)
		return "right(replace(s." + col + "::varchar, '-', ''), 16)", nil
	}
	if field.Name == "traceID" {
		// Wire-form comparison, same reasoning as the link.traceID branch.
		return "replace(s.trace_id::varchar, '-', '')", nil
	}
	if len(field.Name) > 0 {
		col := util.CamelToSnake(field.Name)
		if err := util.ValidateColumnName(col, spanColumns); err != nil {
			return "", fmt.Errorf("trace field %q: %w: %w", field.Name, err, ErrInvalidTraceQuery)
		}
		return "s." + col, nil
	}
	return field.Name, nil
}

// mapTraceAttributeExpressions turns "attribute foo.bar, in scope X" into a
// predicate.
//
// The scope parameter the old form carried is gone: scope is implied by which
// owner's array is unnested, and an owner's array only ever holds ids of its own
// kind. One param per field instead of two.
//
// Event and link attributes need an EXISTS, because the question is whether
// *any* event or link on the span matches. A span's own attributes are on its
// own row, so those stay a plain attr_value lookup.//
// Resource and scope predicates are **hoisted into the owner table** rather
// than evaluated per row. The obvious form, `attr_value(r.attribute_ids, k)
// {COND}`, is correlated: it unnests and joins the dictionary once per span.
// Measured on 200k spans over 24 resources, exact match on service.name:
//
//	attr_value(r.attribute_ids, k) per span      60.59 ms
//	resource_id in (select ... from resources)    2.14 ms
//
// 28x, because the subquery runs once over ~24 rows and the outer predicate is
// then an indexed equality on resource_id (idx_spans_resource). {COND} is
// embedded so it lands inside the subquery, where the small scan is.
func mapTraceAttributeExpressions(field *search.FieldDefinition, query *search.Query, params *[]search.NamedParam) ([]string, error) {
	// Fast path first: an equality test on a string attribute is a membership
	// test against an id we can compute here. IDProbe returns "" for anything
	// it cannot answer exactly, which falls through to the value comparison.
	switch field.AttributeScope {
	case "resource":
		if p := ingest.IDProbe("attribute_ids", field, query, ingest.ScopeResource); p != "" {
			return []string{search.Complete("s.resource_id in (select id from resources where " + p + ")")}, nil
		}
	case "scope":
		if p := ingest.IDProbe("attribute_ids", field, query, ingest.ScopeScope); p != "" {
			return []string{search.Complete("s.scope_id in (select id from scopes where " + p + ")")}, nil
		}
	case "span":
		if p := ingest.IDProbe("s.attribute_ids", field, query, ingest.ScopeSpan); p != "" {
			return []string{search.Complete(p)}, nil
		}
	case "event":
		if p := ingest.IDProbe("e.attribute_ids", field, query, ingest.ScopeEvent); p != "" {
			return []string{search.Complete(
				"exists(select 1 from events e where e.span_id = s.span_id and " + p + ")")}, nil
		}
	case "link":
		if p := ingest.IDProbe("l.attribute_ids", field, query, ingest.ScopeLink); p != "" {
			return []string{search.Complete(
				"exists(select 1 from links l where l.span_id = s.span_id and " + p + ")")}, nil
		}
	}

	keyParam := fmt.Sprintf("attr_key_%d", len(*params))
	*params = append(*params, search.NamedParam{Name: keyParam, Value: field.Name})

	switch field.AttributeScope {
	case "resource":
		return []string{fmt.Sprintf(
			"s.resource_id in (select id from resources where attr_value(attribute_ids, %s) {COND})",
			keyParam)}, nil
	case "scope":
		return []string{fmt.Sprintf(
			"s.scope_id in (select id from scopes where attr_value(attribute_ids, %s) {COND})",
			keyParam)}, nil
	case "span":
		return []string{fmt.Sprintf("attr_value(s.attribute_ids, %s)", keyParam)}, nil
	case "event":
		return []string{fmt.Sprintf(`exists(
			select 1 from events e, unnest(e.attribute_ids) as t(aid)
			join attributes a on a.id = t.aid
			where e.span_id = s.span_id and a.key = %s and a.value {COND}
		)`, keyParam)}, nil
	case "link":
		return []string{fmt.Sprintf(`exists(
			select 1 from links l, unnest(l.attribute_ids) as t(aid)
			join attributes a on a.id = t.aid
			where l.span_id = s.span_id and a.key = %s and a.value {COND}
		)`, keyParam)}, nil
	default:
		return nil, fmt.Errorf("unknown attribute scope %s: %w", field.AttributeScope, ErrInvalidTraceQuery)
	}
}

func mapTraceGlobalExpressions() ([]string, error) {
	return []string{
		"replace(s.trace_id::varchar, '-', '') {COND}",
		"right(replace(s.span_id::varchar, '-', ''), 16) {COND}",
		"right(replace(s.parent_span_id::varchar, '-', ''), 16) {COND}",
		"CAST(s.name AS VARCHAR) {COND}",
		"CAST(s.kind AS VARCHAR) {COND}",
		"CAST(s.status_code AS VARCHAR) {COND}",
		"CAST(s.status_message AS VARCHAR) {COND}",
		"CAST(s.trace_state AS VARCHAR) {COND}",
		"CAST(sc.name AS VARCHAR) {COND}",
		"CAST(sc.version AS VARCHAR) {COND}",
		"exists(select 1 from events e where e.span_id = s.span_id and CAST(e.name AS VARCHAR) {COND})",
		"exists(select 1 from links l where l.span_id = s.span_id and (replace(l.trace_id::varchar, '-', '') {COND} or CAST(l.trace_state AS VARCHAR) {COND} or right(replace(l.linked_span_id::varchar, '-', ''), 16) {COND}))",
		// Free-text search over attributes now takes three clauses where it used
		// to take one. Under the old schema every attribute a span could reach
		// -- its own, its resource's, its scope's, and every event's and link's
		// -- carried that span_id, so a single `where a.span_id = s.span_id`
		// covered all five. The dictionary keeps them in five separate arrays on
		// four tables, so the coverage has to be spelled out.
		//
		// Resource, scope and span attributes are on the span's own row, so
		// concatenating those three arrays is one unnest. Events and links are
		// separate rows and need their own EXISTS.
		matchAnyAttribute("unnest(s.attribute_ids || r.attribute_ids || sc.attribute_ids) as t(aid)", ""),
		matchAnyAttribute("events e, unnest(e.attribute_ids) as t(aid)", "e.span_id = s.span_id and "),
		matchAnyAttribute("links l, unnest(l.attribute_ids) as t(aid)", "l.span_id = s.span_id and "),
	}, nil
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
			a.key {COND} or a.value {COND} or
			(a.type = 'string[]' AND list_contains(TRY_CAST(a.value AS VARCHAR[]), CAST({RAW} AS VARCHAR))) OR
			(a.type = 'int64[]' AND list_contains(TRY_CAST(a.value AS BIGINT[]), TRY_CAST({RAW} AS BIGINT))) OR
			(a.type = 'float64[]' AND list_contains(TRY_CAST(a.value AS DOUBLE[]), TRY_CAST({RAW} AS DOUBLE))) OR
			(a.type = 'boolean[]' AND list_contains(TRY_CAST(a.value AS BOOLEAN[]), TRY_CAST({RAW} AS BOOLEAN)))
		)
	)`, from, scope)
}
