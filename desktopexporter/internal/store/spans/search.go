package spans

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/searchindex"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/CtrlSpice/otel-desktop-viewer/internal/discovery"
)

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

func buildTraceSQL(queryNode *search.QueryNode, timeRange timerange.TimeRange) (cteSQL string, eligibilityWhere string, queryWhere string, args []any, err error) {
	timeCondition, timeParams := search.TimePredicate("s.start_time", timeRange.Start, timeRange.End)
	timeCondition = strings.ReplaceAll(timeCondition, " AND ", " and ")
	cteSQL, eligibilityWhere, args, err = searchindex.BuildSearchSQL(queryNode, traceFieldMapper(), timeCondition, timeParams, "span", spanDocumentScope)
	if err != nil || queryNode == nil {
		return cteSQL, eligibilityWhere, "", args, err
	}
	// Keep time bindings in the parameter list so query references retain
	// the same positions as eligibilityWhere.
	_, queryWhere, _, err = searchindex.BuildSearchSQL(queryNode, traceFieldMapper(), "", timeParams, "span", spanDocumentScope)
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

	return searchindex.BuildSearchSQL(queryNode, traceFieldMapper(), "s.trace_id = try_cast($1 as uuid)", params, "span", spanDocumentScope)
}

const spanDocumentScope = "try_cast(split_part(search_record.root_key,':',1) as uuid)=s.trace_id and try_cast('0x'||split_part(search_record.root_key,':',2) as ubigint)=s.span_id"

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
