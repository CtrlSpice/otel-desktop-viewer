package metrics

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

// SearchSummaries returns lightweight per-Metric summaries for the drawer
// cards and CLI: identity fields, seriesCount, lastValue (Gauge/Sum),
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
	return summariesSQL(timeRange, criteria, options, false)
}

func summariesSQL(timeRange timerange.TimeRange, criteria any, options search.ResultOptions, native bool) (string, []any, error) {
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
	render := queries.Render
	if native {
		render = queries.RenderArrow
	}
	query, err := render(queries.SearchMetricSummaries, searchSummariesParams{
		CTEs:           strings.TrimPrefix(cteSQL, "with "),
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

// GetFieldValueCompletions returns distinct values of one completable metric column
// matching term. Same contract as spans.GetFieldValueCompletions; alphabetical rather
// than by frequency, because metrics has no row count to rank by and
// Metric names read best sorted.
func GetFieldValueCompletions(ctx context.Context, db *sql.DB, field, term string, limit int64) (json.RawMessage, error) {
	query, args, err := searchindex.FieldValuesSQL("metrics", field, term, limit)
	if err != nil {
		return nil, fmt.Errorf("GetFieldValueCompletions: %w: field %q has no value completion", ErrInvalidMetricQuery, field)
	}
	raw, err := searchindex.FieldValuesJSON(ctx, db, query, args)
	if err != nil {
		return nil, fmt.Errorf("GetFieldValueCompletions: %w: %w", ErrMetricsStoreInternal, err)
	}
	return raw, nil
}

// GetMetricAttributeDefinitions returns every metric-side attribute name/scope/type this
// store knows about. See the note on spans.GetTraceAttributeDefinitions.
//
// The definition set includes Metric metadata, datapoint labels, exemplar
// attributes, and the owned Resource and Scope maps.
func GetMetricAttributeDefinitions(ctx context.Context, db *sql.DB) (json.RawMessage, error) {
	raw, err := searchindex.Definitions(ctx, db, "metric")
	if err != nil {
		return nil, fmt.Errorf("GetMetricAttributeDefinitions: %w: %w", ErrMetricsStoreInternal, err)
	}
	return raw, nil
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

// metricColumns lists field names the search expression syntax can
// reference. Metric fields resolve through metrics (s).
var metricColumns = map[string]struct{}{
	"id":          {},
	"description": {},
}

func metricFieldMapper() search.FieldMapper {
	return search.Resolver("metric", map[string]search.ValueOwner{
		"metric":    {Documents: map[string]string{"metadata": "mr.metadata"}, Correlation: "metric_records mr where mr.id=s.id"},
		"resource":  {Documents: map[string]string{"attributes": "mr.resource.attributes"}, Correlation: "metric_records mr where mr.id=s.id"},
		"scope":     {Documents: map[string]string{"attributes": "mr.scope.attributes"}, Correlation: "metric_records mr where mr.id=s.id"},
		"datapoint": {Documents: map[string]string{"attributes": "d.attributes"}, Correlation: "metric_datapoints d where d.metric_id = s.id"},
		"exemplar":  {Documents: map[string]string{"attributes": "e.attributes"}, Correlation: "exemplars e join metric_datapoints d on d.id = e.metric_datapoint_id where d.metric_id = s.id"},
	}, func(f *search.FieldDefinition) (search.ResolvedExpression, error) {
		e, err := mapMetricFieldExpression(f)
		e.SQL = strings.ReplaceAll(e.SQL, "s.", "mr.")
		return e, err
	}, search.RejectUnindexedText)
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
