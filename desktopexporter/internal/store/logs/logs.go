package logs

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowingest"
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
	"go.opentelemetry.io/collector/pdata/plog"
)

// resourceServiceName extracts the service.name resource attribute as a
// plain string, returning "" when not present. Mirrors the same helper
// in spans / metrics; kept private here so logs doesn't grow a cross-
// package dependency just for one attribute lookup.
func resourceServiceName(attrs pcommon.Map) string {
	if v, ok := util.LastValue(attrs, "service.name"); ok {
		return v.AsString()
	}
	return ""
}

var (
	ErrInvalidLogQuery   = errors.New("invalid log search query")
	ErrInvalidLogLimit   = errors.New("invalid log search limit")
	ErrLogsStoreInternal = errors.New("logs store internal error")
	ErrLogRefNotFound    = errors.New("log ID not found")
)

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Ingest ingests log records from pdata into the logs table.
// The caller must hold any required lock on the connection.
//
// Each log carries its body, attributes, Resource and Scope in one append.
// Ingest is IngestReport for callers with nowhere to put the report. Records
// the store refused are still skipped rather than failing the batch; this form
// just cannot say how many there were.
func Ingest(ctx context.Context, conn driver.Conn, logs plog.Logs) error {
	_, err := IngestReport(ctx, conn, logs)
	return err
}

// IngestReport ingests a batch and reports the records it could not write. A
// non-empty Rejected is not a failure: the batch landed, minus those rows.
func IngestReport(ctx context.Context, conn driver.Conn, logs plog.Logs) (_ ingest.Rejected, err error) {
	defer func() { err = ingest.InterruptedContextError(ctx, err) }()

	if err := ctx.Err(); err != nil {
		return ingest.Rejected{}, err
	}

	return ingest.BisectingWrite(ctx, logs.LogRecordCount(), nil, func(lo, hi int) error {
		return ingest.InTransaction(ctx, conn, func() error {
			return appendPass(ctx, conn, logs,
				func(ordinal int) bool { return ordinal >= lo && ordinal < hi })
		})
	})
}

// appendPass writes the records keep selects, by ordinal. Skipped records
// still advance logCur, which is consumed by position.
func appendPass(
	ctx context.Context,
	conn driver.Conn,
	logs plog.Logs,
	keep func(ordinal int) bool,
) (err error) {
	tables := []string{"logs"}
	appenders, err := arrowingest.NewAppenders(ctx, conn, tables)
	if err != nil {
		return fmt.Errorf("Ingest: %w: %w", ErrLogsStoreInternal, err)
	}
	var catalogue searchindex.Batch
	defer func() {
		if err != nil {
			for _, appender := range appenders {
				appender.Discard()
			}
			return
		}
		err = errors.Join(err, arrowingest.CloseAppenders(appenders, tables))
		if err == nil {
			if indexErr := catalogue.Write(ctx, conn); indexErr != nil {
				err = fmt.Errorf("%w: %w", ingest.ErrNotRowFault, indexErr)
			}
		}
	}()

	logCur := 0
	for _, resourceLogs := range logs.ResourceLogs().All() {
		resource := resourceLogs.Resource()
		resourceInput, err := ingest.ResourceInput(resource, resourceLogs.SchemaUrl())
		if err != nil {
			return err
		}
		serviceName := resourceServiceName(resource.Attributes())

		for _, scopeLogs := range resourceLogs.ScopeLogs().All() {
			scopeInput, err := ingest.ScopeInput(scopeLogs.Scope(), scopeLogs.SchemaUrl())
			if err != nil {
				return err
			}

			appenders["logs"].SetContext(resourceInput, scopeInput)
			for _, log := range scopeLogs.LogRecords().All() {
				if err := ctx.Err(); err != nil {
					return err
				}

				if !keep(logCur) {
					logCur++
					continue
				}

				var traceUUID *duckdb.UUID
				if tid := log.TraceID(); !tid.IsEmpty() {
					u := duckdb.UUID(tid)
					traceUUID = &u
				}
				var spanID driver.Value
				if sid := log.SpanID(); !sid.IsEmpty() {
					spanID = util.SpanIDUint64(sid)
				}

				body, err := ingest.ValueBytes(log.Body())
				if err != nil {
					return fmt.Errorf("Ingest: %w: %w", ErrLogsStoreInternal, err)
				}
				attributes, err := ingest.AttributeBytes(log.Attributes())
				if err != nil {
					return err
				}
				logCur++

				id := uuid.New()
				err = appenders["logs"].AppendLog(log, id, body, attributes, serviceName)
				if err != nil {
					return fmt.Errorf("Ingest: %w: %w", ErrLogsStoreInternal, err)
				}
				resourceRef, scopeRef, err := catalogue.Contexts(resourceInput, scopeInput)
				if err != nil {
					return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
				}
				o := searchindex.Object{Ref: "log:log:" + id.String(), Signal: "log", Owner: "log", RootKey: id.String(), RecordKey: id.String(), ResourceRef: resourceRef, ScopeRef: scopeRef}
				if err := o.AddNative("attributes", attributes); err != nil {
					return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
				}
				if err := o.AddNative("body", body); err != nil {
					return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
				}
				var traceWireID, spanWireID any
				if traceUUID != nil {
					traceWireID = log.TraceID().String()
				}
				if spanID != nil {
					spanWireID = log.SpanID().String()
				}
				if err := o.AddStrings(map[string]any{"traceID": traceWireID, "spanID": spanWireID, "severityText": log.SeverityText(), "severityNumber": fmt.Sprint(int32(log.SeverityNumber())), "eventName": log.EventName()}); err != nil {
					return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
				}
				catalogue.Add(o)

			}
		}
	}

	return nil
}

// fieldValueQueries names the log columns whose values the search box may
// complete, keyed by the search grammar's field name. An allowlist of whole
// baked queries rather than an identifier spliced into one: a field name from
// the wire never becomes SQL.
var fieldValueQueries = map[string]string{
	"eventName": `
		select cast(coalesce(
			to_json(list(sub.v order by sub.cnt desc, sub.v)),
			to_json([])) as varchar)
		from (
			select event_name as v, count(*) as cnt
			from logs
			where event_name is not null and event_name <> ''
				and event_name ilike '%' || ? || '%' escape '\'
			group by event_name
			order by cnt desc, event_name
			limit ?
		) sub
	`,
}

// GetFieldValueCompletions returns distinct values of one completable log column
// matching term, most frequent first. Same contract as spans.GetFieldValueCompletions.
func GetFieldValueCompletions(ctx context.Context, db *sql.DB, field, term string, limit int64) (json.RawMessage, error) {
	query, ok := fieldValueQueries[field]
	if !ok {
		return nil, fmt.Errorf("GetFieldValueCompletions: %w: field %q has no value completion", ErrInvalidLogQuery, field)
	}
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(term)
	var raw []byte
	if err := db.QueryRowContext(ctx, query, escaped, limit).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetFieldValueCompletions: %w: %w", ErrLogsStoreInternal, err)
	}
	return json.RawMessage(raw), nil
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
		if len(page.Columns) > 0 && native {
			nativeColumns, args, err = arrowColumns(page.Columns, args)
			if err != nil {
				return nil, err
			}
		} else if len(page.Columns) > 0 {
			columns, err := json.Marshal(page.Columns)
			if err != nil {
				return nil, err
			}
			args = append(args, string(columns))
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

// Get returns the full LogData for a single log identified by its
// tool-minted UUID. Used by the log-detail pane after a user clicks
// a card from Search results. Returns ErrLogRefNotFound when no log
// matches.
func Get(ctx context.Context, db *sql.DB, logRef string) (json.RawMessage, error) {
	// attrs_json resolves each of the row's three attribute arrays in place.
	query, err := queries.Render(queries.GetLog, nil)
	if err != nil {
		return nil, err
	}

	var raw []byte
	if err := db.QueryRowContext(ctx, query, logRef).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("Get: %w", ErrLogRefNotFound)
		}
		return nil, fmt.Errorf("Get: %w: %w", ErrLogsStoreInternal, err)
	}
	if raw == nil {
		return nil, fmt.Errorf("Get: %w", ErrLogRefNotFound)
	}
	return json.RawMessage(raw), nil
}

// GetLogOTLP returns the stored log identified by logRef as one standard OTLP
// JSON document. Callers must transport the returned bytes unchanged: parsing
// and re-encoding in JavaScript loses negative zero.
func GetLogOTLP(ctx context.Context, db *sql.DB, logRef string) (json.RawMessage, error) {
	query, err := queries.Render(queries.GetLogOTLP, nil)
	if err != nil {
		return nil, fmt.Errorf("GetLogOTLP: %w: %w", ErrLogsStoreInternal, err)
	}

	var raw []byte
	if err := db.QueryRowContext(ctx, query, logRef).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetLogOTLP: %w", ErrLogRefNotFound)
		}
		return nil, fmt.Errorf("GetLogOTLP: %w: %w", ErrLogsStoreInternal, err)
	}
	if raw == nil {
		return nil, fmt.Errorf("GetLogOTLP: %w", ErrLogRefNotFound)
	}
	return json.RawMessage(raw), nil
}

// GetTraceLogSummaries returns every lightweight log summary carrying traceID. The
// trace boundary is the complete correlation boundary: timestamps only order
// rows, and nullable or dangling span IDs remain in the result for callers to
// classify. Full body and attribute detail remains owned by Get.
func GetTraceLogSummaries(ctx context.Context, db queryRower, traceID string) (json.RawMessage, error) {
	query, args, err := getTraceLogSummariesSQL(traceID)
	if err != nil {
		return nil, err
	}

	var raw []byte
	if err := db.QueryRowContext(ctx, query, args...).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetTraceLogSummaries: %w: %w", ErrLogsStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// GetSpanLogs returns every full log row associated with one exact composite
// span identity. The result is complete and uses one bulk query.
func GetSpanLogs(ctx context.Context, db queryRower, traceID string, spanID uint64) (json.RawMessage, error) {
	query, err := queries.Render(queries.GetSpanLogs, nil)
	if err != nil {
		return nil, fmt.Errorf("GetSpanLogs: %w: %w", ErrLogsStoreInternal, err)
	}
	var raw []byte
	if err := db.QueryRowContext(ctx, query, traceID, []uint64{spanID}).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetSpanLogs: %w: %w", ErrLogsStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

func getTraceLogSummariesSQL(traceID string) (string, []any, error) {
	query, err := queries.Render(queries.GetTraceLogSummaries, nil)
	if err != nil {
		return "", nil, err
	}
	return query, []any{traceID}, nil
}

// GetLogAttributeDefinitions returns every log-side attribute name/scope/type this store
// knows about, collected from the owned maps.
func GetLogAttributeDefinitions(ctx context.Context, db *sql.DB) (json.RawMessage, error) {
	query, err := queries.Render(queries.GetLogAttributeDefinitions, nil)
	if err != nil {
		return nil, err
	}

	var raw []byte
	if err := db.QueryRowContext(ctx, query).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetLogAttributeDefinitions: %w: %w", ErrLogsStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// Clear deletes logs and their search documents.
func Clear(ctx context.Context, db *sql.DB) error {
	if err := ingest.DeleteLogs(ctx, db, `truncate table logs`); err != nil {
		return fmt.Errorf("Clear: %w: %w", ErrLogsStoreInternal, err)
	}
	return nil
}

// DeleteLogsByRefs deletes multiple logs by their IDs.
func DeleteLogsByRefs(ctx context.Context, db *sql.DB, logRefs []any) error {
	if len(logRefs) == 0 {
		return nil
	}
	ids := util.ToStringList(logRefs)
	if err := ingest.DeleteLogs(ctx, db, `delete from logs where id in (select id from uuid_list(?))`, ids); err != nil {
		return fmt.Errorf("DeleteLogsByRefs: %w: %w", ErrLogsStoreInternal, err)
	}
	return nil
}

func buildLogSQL(queryNode *search.QueryNode, timeRange timerange.TimeRange) (cteSQL string, whereSQL string, args []any, err error) {
	timeCondition, timeParams := search.TimePredicate("l.log_time", timeRange.Start, timeRange.End)
	return searchindex.BuildSearchSQL(queryNode, logFieldMapper(), timeCondition, timeParams, "log", "try_cast(search_record.root_key as uuid)=l.id")
}

// MatchingRowsSQL shares log search's exact predicate and timestamp fallback.
func MatchingRowsSQL(queryNode *search.QueryNode, timeRange timerange.TimeRange) (string, []any, error) {
	cte, where, args, err := buildLogSQL(queryNode, timeRange)
	where = strings.ReplaceAll(where, "l.log_time", "coalesce(nullif(l.timestamp, 0), l.observed_timestamp)")
	return cte + " SELECT l.* " + logSearchFrom + " WHERE " + where, args, err
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
