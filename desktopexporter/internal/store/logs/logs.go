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
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/google/uuid"
	"github.com/duckdb/duckdb-go/v2"
	"go.opentelemetry.io/collector/pdata/plog"
)

var (
	ErrInvalidLogQuery   = errors.New("invalid log search query")
	ErrLogsStoreInternal = errors.New("logs store internal error")
	ErrLogIDNotFound     = errors.New("log ID not found")
)

const flushIntervalLogs = 100

// Ingest ingests log records from pdata into the logs table.
// The caller must hold any required lock on the connection.
func Ingest(ctx context.Context, conn driver.Conn, logs plog.Logs) (err error) {
	tables := []string{"attributes", "logs"}
	appenders, err := ingest.NewAppenders(conn, tables)
	if err != nil {
		return fmt.Errorf("Ingest: %w: %w", ErrLogsStoreInternal, err)
	}
	defer func() {
		err = errors.Join(err, ingest.CloseAppenders(appenders, tables))
	}()

	logCount := 0
	for _, resourceLogs := range logs.ResourceLogs().All() {
		resource := resourceLogs.Resource()

		for _, scopeLogs := range resourceLogs.ScopeLogs().All() {
			scope := scopeLogs.Scope()

			for _, log := range scopeLogs.LogRecords().All() {
				logID := duckdb.UUID(uuid.New())
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

				err := appenders["logs"].AppendRow(
					logID,                             // ID UUID
					int64(log.Timestamp()),            // Timestamp BIGINT
					int64(log.ObservedTimestamp()),    // ObservedTimestamp BIGINT
					traceUUID,                         // TraceID UUID
					spanUUID,                          // SpanID UUID
					log.SeverityText(),                // SeverityText VARCHAR
					int32(log.SeverityNumber()),       // SeverityNumber INTEGER
					bodyValue,                         // Body VARCHAR
					bodyType,                          // BodyType VARCHAR
					resource.DroppedAttributesCount(), // ResourceDroppedAttributesCount UINTEGER
					scope.Name(),                      // ScopeName VARCHAR
					scope.Version(),                   // ScopeVersion VARCHAR
					scope.DroppedAttributesCount(),    // ScopeDroppedAttributesCount UINTEGER
					log.DroppedAttributesCount(),      // DroppedAttributesCount UINTEGER
					uint32(log.Flags()),               // Flags UINTEGER
					log.EventName(),                   // EventName VARCHAR
				)
				if err != nil {
					return fmt.Errorf("Ingest: %w: %w", ErrLogsStoreInternal, err)
				}

				ownerIDs := ingest.AttributeOwnerIDs{LogID: &logID}
				if err := ingest.IngestAttributes(appenders["attributes"], []ingest.AttributeBatchItem{
					{Attrs: resource.Attributes(), IDs: ownerIDs, Scope: "resource"},
					{Attrs: scope.Attributes(), IDs: ownerIDs, Scope: "scope"},
					{Attrs: log.Attributes(), IDs: ownerIDs, Scope: "log"},
				}); err != nil {
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

	return nil
}

// Search returns logs in the time range matching the optional criteria.
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
			select l.* from logs l, search_params
			where %s
		),
		log_attrs as (
			select a.log_id, a.scope, json_group_array(json_object('key', a.key, 'value', a.value, 'type', a.type::varchar)) as attrs
			from attributes a
			where a.log_id in (select id from filtered)
			group by a.log_id, a.scope
		)
		select cast(coalesce(to_json(list(json_object(
			'id', l.id,
			'timestamp', l.timestamp,
			'observedTimestamp', l.observed_timestamp,
			'traceID', replace(l.trace_id::varchar, '-', ''),
			'spanID', right(replace(l.span_id::varchar, '-', ''), 16),
			'severityText', l.severity_text,
			'severityNumber', l.severity_number,
			'body', l.body,
			'bodyType', l.body_type,
			'resource', json_object('attributes', coalesce(res.attrs, json('[]')), 'droppedAttributesCount', l.resource_dropped_attributes_count),
			'scope', json_object('name', l.scope_name, 'version', l.scope_version, 'attributes', coalesce(scope_attrs.attrs, json('[]')), 'droppedAttributesCount', l.scope_dropped_attributes_count),
			'droppedAttributesCount', l.dropped_attributes_count,
			'flags', l.flags,
			'eventName', l.event_name,
			'attributes', coalesce(log_attrs.attrs, json('[]'))
		) order by coalesce(nullif(l.timestamp, 0), l.observed_timestamp) desc)), '[]') as varchar) as logs
		from filtered l
		left join log_attrs res on res.log_id = l.id and res.scope = 'resource'
		left join log_attrs scope_attrs on scope_attrs.log_id = l.id and scope_attrs.scope = 'scope'
		left join log_attrs log_attrs on log_attrs.log_id = l.id and log_attrs.scope = 'log'`,
		cteSQL,
		whereWithTime,
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

// GetLogAttributes returns a JSON array of attribute names/scopes/types for logs in the time range.
func GetLogAttributes(ctx context.Context, db *sql.DB, startTime, endTime int64) (json.RawMessage, error) {
	query := `
		select cast(to_json(list(json_object('name', sub.key, 'attributeScope', sub.scope, 'type', sub.type::varchar)
			order by sub.key, sub.scope)) as varchar) as attributes
		from (
			select distinct a.key, a.scope, a.type
			from attributes a
			inner join logs l on a.log_id = l.id
			where coalesce(nullif(l.timestamp, 0), l.observed_timestamp) >= ?
			  and coalesce(nullif(l.timestamp, 0), l.observed_timestamp) <= ?
		) sub
	`
	var raw []byte
	if err := db.QueryRowContext(ctx, query, startTime, endTime).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetLogAttributes: %w: %w", ErrLogsStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// Clear truncates the logs table and all child attributes.
func Clear(ctx context.Context, db *sql.DB) error {
	childQueries := []string{
		`delete from attributes where log_id is not null`,
		`truncate table logs`,
	}
	for _, q := range childQueries {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("Clear: %w: %w", ErrLogsStoreInternal, err)
		}
	}
	return nil
}

// DeleteLogByID deletes a specific log by its ID.
func DeleteLogByID(ctx context.Context, db *sql.DB, logID string) error {
	childQueries := []string{
		`delete from attributes where log_id = ?`,
		`delete from logs where id = ?`,
	}
	for _, q := range childQueries {
		if _, err := db.ExecContext(ctx, q, logID); err != nil {
			return fmt.Errorf("DeleteLogByID: %w: %w", ErrLogsStoreInternal, err)
		}
	}
	return nil
}

// DeleteLogsByIDs deletes multiple logs by their IDs.
func DeleteLogsByIDs(ctx context.Context, db *sql.DB, logIDs []any) error {
	if len(logIDs) == 0 {
		return nil
	}
	placeholders := util.BuildPlaceholders(len(logIDs))
	childQueries := []string{
		fmt.Sprintf(`delete from attributes where log_id in (%s)`, placeholders),
		fmt.Sprintf(`delete from logs where id in (%s)`, placeholders),
	}
	for _, q := range childQueries {
		if _, err := db.ExecContext(ctx, q, logIDs...); err != nil {
			return fmt.Errorf("DeleteLogsByIDs: %w: %w", ErrLogsStoreInternal, err)
		}
	}
	return nil
}

func buildLogSQL(queryNode *search.QueryNode, startTime, endTime int64) (cteSQL string, whereSQL string, args []any, err error) {
	return search.BuildSearchSQL(queryNode, startTime, endTime, logFieldMapper(), "l.log_time >= time_start AND l.log_time <= time_end")
}

var logColumns = map[string]struct{}{
	"id":                                {},
	"timestamp":                         {},
	"observed_timestamp":                {},
	"trace_id":                          {},
	"span_id":                           {},
	"severity_text":                     {},
	"severity_number":                   {},
	"body":                              {},
	"body_type":                         {},
	"resource_dropped_attributes_count": {},
	"scope_name":                        {},
	"scope_version":                     {},
	"scope_dropped_attributes_count":    {},
	"dropped_attributes_count":          {},
	"flags":                             {},
	"event_name":                        {},
}

func logFieldMapper() search.FieldMapper {
	return func(field *search.FieldDefinition, params *[]search.NamedParam) ([]string, error) {
		switch field.SearchScope {
		case "field":
			expr, err := mapLogFieldExpression(field)
			if err != nil {
				return nil, err
			}
			return []string{expr}, nil
		case "attribute":
			return mapLogAttributeExpressions(field, params)
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
		return "l.trace_id", nil
	case "spanID", "spanId":
		return "l.span_id", nil
	case "severityText":
		return "l.severity_text", nil
	case "severityNumber":
		return "l.severity_number", nil
	case "body":
		return "l.body", nil
	case "eventName":
		return "l.event_name", nil
	case "scope.name":
		return "l.scope_name", nil
	case "scope.version":
		return "l.scope_version", nil
	default:
		col := util.CamelToSnake(name)
		if err := util.ValidateColumnName(col, logColumns); err != nil {
			return "", fmt.Errorf("log field %q: %w: %w", name, err, ErrInvalidLogQuery)
		}
		return "l." + col, nil
	}
}

func mapLogAttributeExpressions(field *search.FieldDefinition, params *[]search.NamedParam) ([]string, error) {
	idx := len(*params)
	scopeParam := fmt.Sprintf("attr_scope_%d", idx)
	keyParam := fmt.Sprintf("attr_key_%d", idx+1)
	*params = append(*params,
		search.NamedParam{Name: scopeParam, Value: field.AttributeScope},
		search.NamedParam{Name: keyParam, Value: field.Name},
	)

	switch field.AttributeScope {
	case "resource", "scope", "log":
		expr := fmt.Sprintf("(SELECT a.value FROM attributes a WHERE a.log_id = l.id AND a.scope = %s AND a.key = %s LIMIT 1)", scopeParam, keyParam)
		return []string{expr}, nil
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
		"CAST(l.scope_name AS VARCHAR) {COND}",
		"CAST(l.scope_version AS VARCHAR) {COND}",
		`EXISTS(
			SELECT 1
			FROM attributes a
			WHERE a.log_id = l.id AND (
				a.key {COND} OR a.value {COND} OR
				(a.type = 'string[]' AND list_contains(CAST(a.value AS VARCHAR[]), CAST({RAW} AS VARCHAR))) OR
				(a.type = 'int64[]' AND list_contains(CAST(a.value AS BIGINT[]), TRY_CAST({RAW} AS BIGINT))) OR
				(a.type = 'float64[]' AND list_contains(CAST(a.value AS DOUBLE[]), TRY_CAST({RAW} AS DOUBLE))) OR
				(a.type = 'boolean[]' AND list_contains(CAST(a.value AS BOOLEAN[]), TRY_CAST({RAW} AS BOOLEAN)))
			)
		)`,
	}, nil
}
