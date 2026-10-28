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
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/nested"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/searchindex"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/CtrlSpice/otel-desktop-viewer/internal/discovery"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

var (
	ErrInvalidMetricQuery    = errors.New("invalid metric search query")
	ErrInvalidMetricLimit    = errors.New("invalid metric search limit")
	ErrMetricIDNotFound      = errors.New("metric ID not found")
	ErrUnsupportedMetricType = nested.ErrUnsupportedMetricType
	ErrMetricsStoreInternal  = errors.New("metrics store internal error")
)

// Ingest appends received metric records, points and exemplars atomically.
func Ingest(ctx context.Context, conn driver.Conn, m pmetric.Metrics) error {
	_, err := IngestReport(ctx, conn, m)
	return err
}

// IngestReport reports rejected Metrics while retaining accepted records.
func IngestReport(ctx context.Context, conn driver.Conn, m pmetric.Metrics) (rejected ingest.Rejected, err error) {
	defer func() {
		err = ingest.InterruptedContextError(ctx, err)
		if rejected.Count() == 0 {
			rejected = ingest.Rejected{}
		}
	}()
	return nested.IngestMetrics(ctx, conn, m)
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
	return nested.Mutate(ctx, db, `delete from metric_records`)
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
	return nested.Mutate(ctx, db, `delete from metric_records where id=?::uuid`, metricID)
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
