package logs

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
	"go.opentelemetry.io/collector/pdata/plog"
)

// resourceServiceName extracts the service.name resource attribute as a
// plain string, returning "" when not present. Mirrors the same helper
// in spans / metrics; kept private here so logs doesn't grow a cross-
// package dependency just for one attribute lookup.
func resourceServiceName(attrs pcommon.Map) string {
	if v, ok := attrs.Get("service.name"); ok {
		return v.AsString()
	}
	return ""
}

var (
	ErrInvalidLogQuery   = errors.New("invalid log search query")
	ErrLogsStoreInternal = errors.New("logs store internal error")
	ErrLogIDNotFound     = errors.New("log ID not found")
)

// flushIntervalLogs bounds how many log records accumulate in the appenders before
// they are pushed to DuckDB. It exists to cap memory on a pathological batch,
// not to make writes visible sooner -- nothing reads mid-batch, since ingest
// holds the store write lock throughout.
//
// Raised from 100 to 500 on measurement. Each flush is a cgo call into
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
const flushIntervalLogs = 500

// Ingest ingests log records from pdata into the logs table.
// The caller must hold any required lock on the connection.
//
// Two passes, matching spans.Ingest: hash and resolve resource/scope
// identities, flush the dictionary, then append the log rows with the id
// arrays. See spans.Ingest for why the dictionary cannot ride the appender.
func Ingest(ctx context.Context, conn driver.Conn, logs plog.Logs, flushed *ingest.FlushedIDs) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Pass 1: dictionary.
	dict := ingest.NewDictionary(flushed)
	type scopeKey struct{ ri, si int }
	resourceIDs := map[int]duckdb.UUID{}
	scopeIDs := map[scopeKey]duckdb.UUID{}

	// AddAttributes already returns the array its owner stores; pass 2 reads it
	// back instead of hashing the same attributes again. Consumed by position:
	// both passes walk identical loops, and the cursor is checked against the
	// length at the end so a desynchronising edit fails loudly.
	var logAttrs [][]duckdb.UUID

	for ri, resourceLogs := range logs.ResourceLogs().All() {
		resourceIDs[ri] = dict.AddResource(resourceLogs.Resource())
		for si, scopeLogs := range resourceLogs.ScopeLogs().All() {
			scopeIDs[scopeKey{ri, si}] = dict.AddScope(scopeLogs.Scope())
			for _, log := range scopeLogs.LogRecords().All() {
				logAttrs = append(logAttrs, dict.AddAttributes(log.Attributes(), ingest.ScopeLog))
			}
		}
	}

	if err := dict.Flush(ctx, conn); err != nil {
		return fmt.Errorf("Ingest: %w: %w", ErrLogsStoreInternal, err)
	}

	// Pass 2: append.
	tables := []string{"logs"}
	appenders, err := ingest.NewAppenders(conn, tables)
	if err != nil {
		return fmt.Errorf("Ingest: %w: %w", ErrLogsStoreInternal, err)
	}
	defer func() {
		err = errors.Join(err, ingest.CloseAppenders(appenders, tables))
	}()

	logCount := 0
	logCur := 0
	for ri, resourceLogs := range logs.ResourceLogs().All() {
		resource := resourceLogs.Resource()
		resourceID := resourceIDs[ri]
		// Denormalize service.name onto every log row in this resource; the
		// resource's attribute row, reached through resource_id, is still the
		// source of truth. See spans.Ingest for the same pattern.
		serviceName := resourceServiceName(resource.Attributes())

		for si, scopeLogs := range resourceLogs.ScopeLogs().All() {
			scopeID := scopeIDs[scopeKey{ri, si}]

			for _, log := range scopeLogs.LogRecords().All() {
				if err := ctx.Err(); err != nil {
					return err
				}

				var traceUUID *duckdb.UUID
				if tid := log.TraceID(); !tid.IsEmpty() {
					u := duckdb.UUID(tid)
					traceUUID = &u
				}
				var spanUUID *duckdb.UUID
				if sid := log.SpanID(); !sid.IsEmpty() {
					var padded [16]byte
					copy(padded[8:], sid[:])
					u := duckdb.UUID(padded)
					spanUUID = &u
				}

				bodyValue, bodyType := util.ValueToStringAndType(log.Body())
				// Hashed in pass 1; read back by position.
				logAttrIDs := logAttrs[logCur]
				logCur++

				err := appenders["logs"].AppendRow(
					duckdb.UUID(uuid.New()),        // ID UUID
					int64(log.Timestamp()),         // Timestamp BIGINT
					int64(log.ObservedTimestamp()), // ObservedTimestamp BIGINT
					traceUUID,                      // TraceID UUID
					spanUUID,                       // SpanID UUID
					log.SeverityText(),             // SeverityText VARCHAR
					int32(log.SeverityNumber()),    // SeverityNumber INTEGER
					bodyValue,                      // Body VARCHAR
					bodyType,                       // BodyType VARCHAR
					resourceID,                     // ResourceID UUID
					scopeID,                        // ScopeID UUID
					ingest.NonNil(logAttrIDs),      // AttributeIDs UUID[]
					log.DroppedAttributesCount(),   // DroppedAttributesCount UINTEGER
					uint32(log.Flags()),            // Flags UINTEGER
					log.EventName(),                // EventName VARCHAR
					serviceName,                    // ServiceName VARCHAR (NOT NULL, '' = unknown)
				)
				if err != nil {
					return fmt.Errorf("Ingest: %w: %w", ErrLogsStoreInternal, err)
				}

				logCount++
				if logCount%flushIntervalLogs == 0 {
					if err := ingest.FlushAppenders(appenders, tables); err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrLogsStoreInternal, err)
					}
				}
			}
		}
	}

	// The two passes must have visited the same logs; a divergence would pair
	// each record past that point with another record's attributes, silently.
	if logCur != len(logAttrs) {
		return fmt.Errorf("Ingest: %w: pass mismatch (logs %d/%d)",
			ErrLogsStoreInternal, logCur, len(logAttrs))
	}

	return nil
}

// bodyPreviewLen is the max character count for the server-truncated
// body preview returned by Search. Callers that need the full body
// must fetch the log via Get.
const bodyPreviewLen = 200

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
// `bodyPreview` is server-truncated to bodyPreviewLen characters.
func Search(ctx context.Context, db *sql.DB, startTime, endTime int64, criteria any) (json.RawMessage, error) {
	var searchTree *search.QueryNode
	if criteria != nil {
		var err error
		searchTree, err = search.ParseQueryTree(criteria)
		if err != nil {
			return nil, fmt.Errorf("Search: %w: %w", ErrInvalidLogQuery, err)
		}
	}

	cteSQL, whereClause, args, err := buildLogSQL(searchTree, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("Search: %w: %w", ErrInvalidLogQuery, err)
	}

	logTimeExpr := `(case when l.timestamp is null or l.timestamp = 0 then l.observed_timestamp else l.timestamp end)`
	whereWithTime := strings.ReplaceAll(whereClause, "l.log_time", logTimeExpr)
	finalQuery := fmt.Sprintf(`%s,
		filtered as (
			select l.* %s
			where %s
		)
		select cast(coalesce(to_json(list(json_object(
			'id',             l.id,
			'timestamp',      cast(coalesce(nullif(l.timestamp, 0), l.observed_timestamp) as varchar),
			'severityText',   l.severity_text,
			'severityNumber', l.severity_number,
			'serviceName',    l.service_name,
			'bodyPreview',    substring(l.body, 1, %d)
		) order by coalesce(nullif(l.timestamp, 0), l.observed_timestamp) desc)), '[]') as varchar) as logs
		from filtered l`,
		cteSQL,
		logSearchFrom,
		whereWithTime,
		bodyPreviewLen,
	)

	var raw []byte
	if err := db.QueryRowContext(ctx, finalQuery, args...).Scan(&raw); err != nil {
		return nil, fmt.Errorf("Search: %w: %w", ErrLogsStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// Get returns the full LogData for a single log identified by its
// tool-minted UUID. Used by the log-detail pane after a user clicks
// a card from Search results. Returns ErrLogIDNotFound when no log
// matches.
func Get(ctx context.Context, db *sql.DB, logID string) (json.RawMessage, error) {
	// One row, three attribute arrays, no CTEs: attrs_json resolves each array
	// against the dictionary in place. This replaced a log_attrs CTE that
	// grouped the log's attribute rows by scope and was left-joined back three
	// times under different aliases.
	query, err := queries.Render(queries.GetLog, nil)
	if err != nil {
		return nil, err
	}

	var raw []byte
	if err := db.QueryRowContext(ctx, query, logID).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("Get: %w", ErrLogIDNotFound)
		}
		return nil, fmt.Errorf("Get: %w: %w", ErrLogsStoreInternal, err)
	}
	if raw == nil {
		return nil, fmt.Errorf("Get: %w", ErrLogIDNotFound)
	}
	return json.RawMessage(raw), nil
}

// GetLogAttributes returns every log-side attribute name/scope/type this store
// knows about. The time range is accepted and ignored -- see the note on
// spans.GetTraceAttributes for why the dictionary answers this directly instead
// of unnesting every log in the window.
func GetLogAttributes(ctx context.Context, db *sql.DB, startTime, endTime int64) (json.RawMessage, error) {
	query, err := queries.Render(queries.GetLogAttributes, nil)
	if err != nil {
		return nil, err
	}

	var raw []byte
	if err := db.QueryRowContext(ctx, query).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetLogAttributes: %w: %w", ErrLogsStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// Clear truncates the logs table and all child attributes.
func Clear(ctx context.Context, db *sql.DB) error {
	// Attribute, resource and scope rows are shared across signals, so this
	// cannot know whether the ones it just abandoned are still in use.
	// ingest.SweepOrphans collects them; the store runs it once for all three
	// signals rather than three times here.
	childQueries := []string{
		`truncate table logs`,
	}
	for _, q := range childQueries {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("Clear: %w: %w", ErrLogsStoreInternal, err)
		}
	}
	return nil
}

// DeleteLogsByIDs deletes multiple logs by their IDs.
func DeleteLogsByIDs(ctx context.Context, db *sql.DB, logIDs []any) error {
	if len(logIDs) == 0 {
		return nil
	}
	ids := util.ToStringList(logIDs)
	childQueries := []string{
		`delete from logs where id in (select id from uuid_list(?))`,
	}
	for _, q := range childQueries {
		if _, err := db.ExecContext(ctx, q, ids); err != nil {
			return fmt.Errorf("DeleteLogsByIDs: %w: %w", ErrLogsStoreInternal, err)
		}
	}
	return nil
}

func buildLogSQL(queryNode *search.QueryNode, startTime, endTime int64) (cteSQL string, whereSQL string, args []any, err error) {
	return search.BuildSearchSQL(queryNode, startTime, endTime, logFieldMapper(), "l.log_time >= time_start AND l.log_time <= time_end")
}

// logSearchFrom is the FROM clause log search predicates are written against.
// Mirrors spans.spanSearchFrom: resources and scopes are joined in
// unconditionally so resource.* and scope.* fields have somewhere to resolve.
const logSearchFrom = `from search_params, logs l
			join resources r on r.id = l.resource_id
			join scopes sc on sc.id = l.scope_id`

var logColumns = map[string]struct{}{
	"id":                       {},
	"timestamp":                {},
	"observed_timestamp":       {},
	"trace_id":                 {},
	"span_id":                  {},
	"severity_text":            {},
	"severity_number":          {},
	"body":                     {},
	"body_type":                {},
	"service_name":             {},
	"dropped_attributes_count": {},
	"flags":                    {},
	"event_name":               {},
}

func logFieldMapper() search.FieldMapper {
	return func(field *search.FieldDefinition, query *search.Query, params *[]search.NamedParam) ([]string, error) {
		switch field.SearchScope {
		case "field":
			expr, err := mapLogFieldExpression(field)
			if err != nil {
				return nil, err
			}
			return []string{expr}, nil
		case "attribute":
			return mapLogAttributeExpressions(field, query, params)
		case "global":
			return mapLogGlobalExpressions()
		default:
			return nil, fmt.Errorf("unknown search scope %s: %w", field.SearchScope, ErrInvalidLogQuery)
		}
	}
}

func mapLogFieldExpression(field *search.FieldDefinition) (string, error) {
	name := field.Name
	if name == "" {
		return "", fmt.Errorf("empty field name: %w", ErrInvalidLogQuery)
	}
	switch name {
	case "traceID", "traceId":
		// Compare in wire form (dash-less 32-hex) like the global mapper:
		// a raw uuid-column comparison errors on malformed input and LIKE
		// operators would match against the dashed internal form. Values
		// are dash-stripped and lowercased by the search package.
		return "replace(l.trace_id::varchar, '-', '')", nil
	case "spanID", "spanId":
		// Span IDs are served as 16-char hex (OTLP wire form), which does
		// NOT cast to uuid. Convert the column to wire form instead, same
		// as the trace-search spanID branch, so pasted IDs compare as
		// strings and typos match nothing rather than erroring.
		return "right(replace(l.span_id::varchar, '-', ''), 16)", nil
	case "severityText":
		return "l.severity_text", nil
	case "severityNumber":
		return "l.severity_number", nil
	case "body":
		return "l.body", nil
	case "eventName":
		return "l.event_name", nil
	case "scope.name":
		return "sc.name", nil
	case "scope.version":
		return "sc.version", nil
	case "resource.droppedAttributesCount":
		return "r.dropped_attributes_count", nil
	case "scope.droppedAttributesCount":
		return "sc.dropped_attributes_count", nil
	default:
		col := util.CamelToSnake(name)
		if err := util.ValidateColumnName(col, logColumns); err != nil {
			return "", fmt.Errorf("log field %q: %w: %w", name, err, ErrInvalidLogQuery)
		}
		return "l." + col, nil
	}
}

// mapLogAttributeExpressions resolves an attribute by key against whichever
// array its scope names. The scope parameter the old form carried is gone:
// scope is implied by which array is searched.
//
// Resource and scope predicates are hoisted into the owner table -- see
// spans.mapTraceAttributeExpressions for the measurement that motivates it.
func mapLogAttributeExpressions(field *search.FieldDefinition, query *search.Query, params *[]search.NamedParam) ([]string, error) {
	// Fast path: equality on a string attribute is a membership test against a
	// content-derived id. IDProbe returns "" for anything it cannot answer
	// exactly, falling through to the value comparison below.
	switch field.AttributeScope {
	case "resource":
		if p := ingest.IDProbe("attribute_ids", field, query, ingest.ScopeResource); p != "" {
			return []string{search.Complete("l.resource_id in (select id from resources where " + p + ")")}, nil
		}
	case "scope":
		if p := ingest.IDProbe("attribute_ids", field, query, ingest.ScopeScope); p != "" {
			return []string{search.Complete("l.scope_id in (select id from scopes where " + p + ")")}, nil
		}
	case "log":
		if p := ingest.IDProbe("l.attribute_ids", field, query, ingest.ScopeLog); p != "" {
			return []string{search.Complete(p)}, nil
		}
	}

	keyParam := fmt.Sprintf("attr_key_%d", len(*params))
	*params = append(*params, search.NamedParam{Name: keyParam, Value: field.Name})

	switch field.AttributeScope {
	case "resource":
		return []string{fmt.Sprintf(
			"l.resource_id in (select id from resources where attr_value(attribute_ids, %s) {COND})",
			keyParam)}, nil
	case "scope":
		return []string{fmt.Sprintf(
			"l.scope_id in (select id from scopes where attr_value(attribute_ids, %s) {COND})",
			keyParam)}, nil
	case "log":
		return []string{fmt.Sprintf("attr_value(l.attribute_ids, %s)", keyParam)}, nil
	default:
		return nil, fmt.Errorf("unknown attribute scope %s: %w", field.AttributeScope, ErrInvalidLogQuery)
	}
}

func mapLogGlobalExpressions() ([]string, error) {
	return []string{
		"replace(l.trace_id::varchar, '-', '') {COND}",
		"right(replace(l.span_id::varchar, '-', ''), 16) {COND}",
		"CAST(l.body AS VARCHAR) {COND}",
		"CAST(l.severity_text AS VARCHAR) {COND}",
		"CAST(l.severity_number AS VARCHAR) {COND}",
		"CAST(l.event_name AS VARCHAR) {COND}",
		"CAST(sc.name AS VARCHAR) {COND}",
		"CAST(sc.version AS VARCHAR) {COND}",
		// The log's own attributes plus its resource's and scope's -- the three
		// sets that used to share one log_id in the attributes table.
		`EXISTS(
			SELECT 1
			FROM unnest(l.attribute_ids || r.attribute_ids || sc.attribute_ids) AS t(aid)
			JOIN attributes a ON a.id = t.aid
			WHERE (
				a.key {COND} OR a.value {COND} OR
				(a.type = 'string[]' AND list_contains(TRY_CAST(a.value AS VARCHAR[]), CAST({RAW} AS VARCHAR))) OR
				(a.type = 'int64[]' AND list_contains(TRY_CAST(a.value AS BIGINT[]), TRY_CAST({RAW} AS BIGINT))) OR
				(a.type = 'float64[]' AND list_contains(TRY_CAST(a.value AS DOUBLE[]), TRY_CAST({RAW} AS DOUBLE))) OR
				(a.type = 'boolean[]' AND list_contains(TRY_CAST(a.value AS BOOLEAN[]), TRY_CAST({RAW} AS BOOLEAN)))
			)
		)`,
	}, nil
}
