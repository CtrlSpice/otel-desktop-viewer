package logs

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

// GetFieldValueCompletions returns distinct values of one completable log column
// matching term, most frequent first. Same contract as spans.GetFieldValueCompletions.
func GetFieldValueCompletions(ctx context.Context, db *sql.DB, field, term string, limit int64) (json.RawMessage, error) {
	query, args, err := searchindex.FieldValuesSQL("logs", field, term, limit)
	if err != nil {
		return nil, fmt.Errorf("GetFieldValueCompletions: %w: field %q has no value completion", ErrInvalidLogQuery, field)
	}
	raw, err := searchindex.FieldValuesJSON(ctx, db, query, args)
	if err != nil {
		return nil, fmt.Errorf("GetFieldValueCompletions: %w: %w", ErrLogsStoreInternal, err)
	}
	return raw, nil
}

// Search returns log summaries in the time range matching the optional
// criteria. Each row is a lightweight projection -- enough to render
// the log card without shipping the full body or attribute set. Use
// Get(id) to fetch full LogData for a single log on demand.
//
// `id` is included in the wire payload because the UI needs a handle
// for keying, selection, and detail fetches, but it's tool-minted
// scaffolding (OTLP logs are anonymous) and must never be rendered to
// users.
//
// `bodyPreview` is server-truncated by the body_preview macro.
func Search(ctx context.Context, db *sql.DB, timeRange timerange.TimeRange, criteria any) (json.RawMessage, error) {
	return searchLogSummaries(ctx, db, timeRange, criteria, search.ResultOptions{})
}

// SearchWithLimit returns at most limit log summaries.
func SearchWithLimit(ctx context.Context, db *sql.DB, timeRange timerange.TimeRange, criteria any, limit int64) (json.RawMessage, error) {
	return searchLogSummaries(ctx, db, timeRange, criteria, search.ResultOptions{Limit: &limit})
}

func SearchSummariesWithOptions(ctx context.Context, db *sql.DB, timeRange timerange.TimeRange, criteria any, options search.ResultOptions) (json.RawMessage, error) {
	return searchLogSummaries(ctx, db, timeRange, criteria, options)
}

func searchLogSummaries(ctx context.Context, db *sql.DB, timeRange timerange.TimeRange, criteria any, options search.ResultOptions) (json.RawMessage, error) {
	return searchLogResults(ctx, db, timeRange, criteria, options, nil, nil)
}

// PageOptions bounds a page inside the sorted, optionally limited search result.
type PageOptions struct {
	Size    int64
	Offset  int64
	Columns []Column
}

// SearchPage computes the count and page in one database statement.
func SearchPage(ctx context.Context, db *sql.DB, timeRange timerange.TimeRange, criteria any, options search.ResultOptions, page PageOptions) (json.RawMessage, error) {
	if page.Size < 1 || page.Size > 250 || page.Offset < 0 {
		return nil, fmt.Errorf("page size must be 1..250 and offset must be non-negative: %w", ErrInvalidLogLimit)
	}
	if err := ValidateColumns(page.Columns); err != nil {
		return nil, err
	}
	return searchLogResults(ctx, db, timeRange, criteria, options, &page, nil)
}

// GetErrorNavigation reports neighbours in the complete sorted and limited result,
// not only the currently loaded page. Offsets let the UI load the target page.
func GetErrorNavigation(ctx context.Context, db *sql.DB, timeRange timerange.TimeRange, criteria any, options search.ResultOptions, selectedID string) (json.RawMessage, error) {
	return searchLogResults(ctx, db, timeRange, criteria, options, nil, &selectedID)
}

func searchLogResults(ctx context.Context, db *sql.DB, timeRange timerange.TimeRange, criteria any, options search.ResultOptions, page *PageOptions, selectedID *string) (json.RawMessage, error) {
	query, err := logResultsSQL(timeRange, criteria, options, page, selectedID, false)
	if err != nil {
		return nil, err
	}
	var raw []byte
	if err := db.QueryRowContext(ctx, query.sql, query.args...).Scan(&raw); err != nil {
		return nil, fmt.Errorf("Search: %w: %w", ErrLogsStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

type logQuery struct {
	sql  string
	args []any
}

func logResultsSQL(timeRange timerange.TimeRange, criteria any, options search.ResultOptions, page *PageOptions, selectedID *string, native bool) (*logQuery, error) {
	var searchTree *search.QueryNode
	if criteria != nil {
		var err error
		searchTree, err = search.ParseQueryTree(criteria)
		if err != nil {
			return nil, fmt.Errorf("Search: %w: %w", ErrInvalidLogQuery, err)
		}
	}

	cteSQL, whereClause, args, err := buildLogSQL(searchTree, timeRange)
	if err != nil {
		return nil, fmt.Errorf("Search: %w: %w", ErrInvalidLogQuery, err)
	}

	logTimeExpr := `(case when l.timestamp is null or l.timestamp = 0 then l.observed_timestamp else l.timestamp end)`
	whereWithTime := strings.ReplaceAll(whereClause, "l.log_time", logTimeExpr)
	orderBy, err := logSummaryOrderBy(options.Sort)
	if err != nil {
		return nil, err
	}
	limitClause := ""
	if options.Limit != nil {
		if *options.Limit < 1 {
			return nil, fmt.Errorf("Search: limit must be positive: %w", ErrInvalidLogLimit)
		}
		limitClause = "\n\t\t\tlimit ?"
		args = append(args, *options.Limit)
	}
	queryName := queries.SearchLogSummaries
	nativeColumns := ""
	if page != nil {
		queryName = queries.SearchLogPage
		args = append(args, page.Size, page.Offset)
		if len(page.Columns) > 0 {
			nativeColumns, args, err = columnPreviews(page.Columns, args)
			if err != nil {
				return nil, err
			}
		}
	}
	if selectedID != nil {
		queryName = queries.GetLogErrorNavigation
		args = append(args, *selectedID)
	}
	render := queries.Render
	if native {
		render = queries.RenderArrow
	}
	finalQuery, err := render(queryName, searchLogSummariesParams{
		CTEs:          cteSQL,
		From:          logSearchFrom,
		Where:         whereWithTime,
		Order:         orderBy,
		Limit:         limitClause,
		Columns:       page != nil && len(page.Columns) > 0,
		NativeColumns: nativeColumns,
	})
	if err != nil {
		return nil, err
	}

	return &logQuery{sql: finalQuery, args: args}, nil
}

func logSummaryOrderBy(sortOption *search.Sort) (string, error) {
	if sortOption == nil {
		return "coalesce(nullif(l.timestamp, 0), l.observed_timestamp) desc, l.id asc", nil
	}
	expressions := map[string]string{
		"timestamp": "coalesce(nullif(l.timestamp, 0), l.observed_timestamp)",
		"severity":  "coalesce(l.severity_number, 0)",
		"service":   "coalesce(l.service_name, '')",
		"name":      "coalesce(l.event_name, '')",
		"body":      "coalesce(body_preview(otel_value_json(l.body)), '')",
	}
	expression, ok := expressions[sortOption.Field]
	if !ok {
		return "", fmt.Errorf("unsupported log sort field %q: %w", sortOption.Field, search.ErrInvalidSort)
	}
	direction, err := search.SortDirectionSQL(sortOption.Direction)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s %s, l.id asc", expression, direction), nil
}

// GetLogAttributeDefinitions returns every log-side attribute name/scope/type this store
// knows about, collected from the owned maps.
func GetLogAttributeDefinitions(ctx context.Context, db *sql.DB) (json.RawMessage, error) {
	raw, err := searchindex.Definitions(ctx, db, "log")
	if err != nil {
		return nil, fmt.Errorf("GetLogAttributeDefinitions: %w: %w", ErrLogsStoreInternal, err)
	}
	return raw, nil
}

func buildLogSQL(queryNode *search.QueryNode, timeRange timerange.TimeRange) (cteSQL string, whereSQL string, args []any, err error) {
	timeCondition, timeParams := search.TimePredicate("l.log_time", timeRange.Start, timeRange.End)
	return searchindex.BuildSearchSQL(queryNode, logFieldMapper(), timeCondition, timeParams, "log", "try_cast(search_record.root_key as uuid)=l.id")
}

// MatchingRowsSQL shares log search's exact predicate and timestamp fallback.
func MatchingRowsSQL(queryNode *search.QueryNode, timeRange timerange.TimeRange) (string, []any, error) {
	cte, where, args, err := buildLogSQL(queryNode, timeRange)
	where = strings.ReplaceAll(where, "l.log_time", "coalesce(nullif(l.timestamp, 0), l.observed_timestamp)")
	return cte + " select l.* " + logSearchFrom + " where " + where, args, err
}

// logSearchFrom is the FROM clause log search predicates are written against.
const logSearchFrom = `from logs l`

var logColumns = map[string]struct{}{
	"id":                       {},
	"timestamp":                {},
	"observed_timestamp":       {},
	"trace_id":                 {},
	"span_id":                  {},
	"severity_text":            {},
	"severity_number":          {},
	"body":                     {},
	"service_name":             {},
	"dropped_attributes_count": {},
	"flags":                    {},
	"event_name":               {},
}

func logFieldMapper() search.FieldMapper {
	return search.Resolver("log", map[string]search.ValueOwner{
		"log":      {Documents: map[string]string{"attributes": "l.attributes", "body": "l.body"}},
		"resource": {Documents: map[string]string{"attributes": "l.resource.attributes"}},
		"scope":    {Documents: map[string]string{"attributes": "l.scope.attributes"}},
	}, mapLogFieldExpression, search.RejectUnindexedText)
}

func mapLogFieldExpression(field *search.FieldDefinition) (search.ResolvedExpression, error) {
	if field.Name == "serviceIdentity" || field.Name == "resource.serviceIdentity" {
		return search.Text(discovery.ServiceIdentity("l.resource")), nil
	}
	name := field.Name
	if name == "" {
		return search.ResolvedExpression{}, fmt.Errorf("empty field name: %w", ErrInvalidLogQuery)
	}
	switch name {
	case "traceID", "traceId":
		// Compare in wire form (dash-less 32-hex) like the global mapper:
		// a raw uuid-column comparison errors on malformed input and LIKE
		// operators would match against the dashed internal form. Values
		// are dash-stripped and lowercased by the search package.
		return search.WireID("replace(l.trace_id::varchar, '-', '')"), nil
	case "spanID", "spanId":
		// Span IDs are served as 16-char hex (OTLP wire form), which does
		// NOT cast to uuid. Convert the column to wire form instead, same
		// as the trace-search spanID branch, so pasted IDs compare as
		// strings and typos match nothing rather than erroring.
		return search.WireID("span_id_wire(l.span_id)"), nil
	case "severityText":
		return search.Text("l.severity_text"), nil
	case "severityNumber":
		return search.NativeInteger("l.severity_number"), nil
	case "timestamp":
		return search.Timestamp("l.timestamp"), nil
	case "observedTimestamp":
		return search.Timestamp("l.observed_timestamp"), nil
	case "droppedAttributesCount":
		return search.NativeInteger("l.dropped_attributes_count"), nil
	case "flags":
		return search.NativeInteger("l.flags"), nil
	case "body":
		return search.Text("otel_search_text(l.body)"), nil
	case "eventName":
		return search.Text("l.event_name"), nil
	case "scope.name":
		return search.Text("l.scope.name"), nil
	case "scope.version":
		return search.Text("l.scope.version"), nil
	case "resource.droppedAttributesCount":
		return search.NativeInteger("l.resource.dropped_attributes_count"), nil
	case "scope.droppedAttributesCount":
		return search.NativeInteger("l.scope.dropped_attributes_count"), nil
	default:
		col := util.CamelToSnake(name)
		if err := util.ValidateColumnName(col, logColumns); err != nil {
			return search.ResolvedExpression{}, fmt.Errorf("log field %q: %w: %w", name, err, ErrInvalidLogQuery)
		}
		return search.Text("l." + col), nil
	}
}

// searchLogSummariesParams are the fragments Search assembles into
// queries/logs/search_log_summaries.sql.
//
// SQL fragments and projection switches; values travel as bound arguments.
type searchLogSummariesParams struct {
	CTEs          string
	From          string
	Where         string
	Order         string
	Limit         string
	Columns       bool
	NativeColumns string
}
