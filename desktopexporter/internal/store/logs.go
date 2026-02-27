package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/marcboeker/go-duckdb/v2"
	"go.opentelemetry.io/collector/pdata/plog"
)

// IngestLogs ingests log records from pdata into the logs table.
func (s *Store) IngestLogs(ctx context.Context, logs plog.Logs) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf("failed to add logs: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tables := []string{"attributes", "logs"}
	appenders, err := NewAppenders(s.conn, tables)
	if err != nil {
		return err
	}
	defer CloseAppenders(appenders, tables)

	const flushIntervalLogs = 100
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

				bodyValue, bodyType := ValueToStringAndType(log.Body())
				logSearchText := strings.Join([]string{
					bodyValue,
					log.SeverityText(),
					log.EventName(),
					scope.Name(),
					scope.Version(),
				}, " ")

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
					logSearchText,                     // SearchText VARCHAR
				)
				if err != nil {
					return fmt.Errorf("failed to append row: %w", err)
				}

				ownerIDs := AttributeOwnerIDs{LogID: &logID}
				if err := IngestAttributes(appenders["attributes"], []AttributeBatchItem{
					{Attrs: resource.Attributes(), IDs: ownerIDs, Scope: "resource"},
					{Attrs: scope.Attributes(), IDs: ownerIDs, Scope: "scope"},
					{Attrs: log.Attributes(), IDs: ownerIDs, Scope: "log"},
				}); err != nil {
					return err
				}

				logCount++
				if logCount%flushIntervalLogs == 0 {
					if err := FlushAppenders(appenders, tables); err != nil {
						return fmt.Errorf("failed to flush appender: %w", err)
					}
				}
			}
		}
	}
	return nil
}

// SearchLogs returns logs in the time range matching the optional query tree, as a JSON array of log objects.
func (s *Store) SearchLogs(ctx context.Context, startTime, endTime int64, query any) (json.RawMessage, error) {
	if err := s.checkConnection(); err != nil {
		return nil, fmt.Errorf("failed to search logs: %w", err)
	}

	var queryTree *QueryNode
	if query != nil {
		var err error
		queryTree, err = ParseQueryTree(query)
		if err != nil {
			return nil, fmt.Errorf("failed to parse query tree: %w", err)
		}
	}

	cteSQL, whereClause, args, err := BuildLogSQL(queryTree, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to build log SQL: %w", err)
	}

	// Log time: prefer timestamp, fall back to observed_timestamp per OTLP
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
			'traceID', l.trace_id,
			'spanID', l.span_id,
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
	if err := s.db.QueryRowContext(ctx, finalQuery, args...).Scan(&raw); err != nil {
		return nil, fmt.Errorf("failed to search logs: %w", err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// ClearLogs truncates the logs table and all child attributes.
func (s *Store) ClearLogs(ctx context.Context) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf("failed to clear logs: %w", err)
	}

	childQueries := []string{
		`delete from attributes where log_id is not null`,
		`truncate table logs`,
	}
	for _, query := range childQueries {
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("failed to clear logs: %w", err)
		}
	}
	return nil
}

// DeleteLogByID deletes a specific log by its ID, including child attributes.
func (s *Store) DeleteLogByID(ctx context.Context, logID string) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf("failed to delete log by ID: %w", err)
	}

	childQueries := []string{
		`delete from attributes where log_id = ?`,
		`delete from logs where id = ?`,
	}
	for _, query := range childQueries {
		if _, err := s.db.ExecContext(ctx, query, logID); err != nil {
			return fmt.Errorf("failed to delete log by ID: %w", err)
		}
	}

	return nil
}

// DeleteLogsByIDs deletes multiple logs by their IDs, including child attributes.
func (s *Store) DeleteLogsByIDs(ctx context.Context, logIDs []any) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf("failed to delete logs by ID: %w", err)
	}

	if len(logIDs) == 0 {
		return nil
	}

	placeholders := buildPlaceholders(len(logIDs))
	childQueries := []string{
		fmt.Sprintf(`delete from attributes where log_id in (%s)`, placeholders),
		fmt.Sprintf(`delete from logs where id in (%s)`, placeholders),
	}
	for _, query := range childQueries {
		if _, err := s.db.ExecContext(ctx, query, logIDs...); err != nil {
			return fmt.Errorf("failed to delete logs by ID: %w", err)
		}
	}

	return nil
}

// logFieldMapper returns a FieldMapper for log-specific field-to-SQL mapping.
func logFieldMapper() FieldMapper {
	return func(field *FieldDefinition) ([]string, error) {
		switch field.SearchScope {
		case "field":
			expr, err := mapLogFieldExpression(field)
			if err != nil {
				return nil, err
			}
			return []string{expr}, nil
		case "attribute":
			return mapLogAttributeExpressions(field)
		case "global":
			return mapLogGlobalExpressions()
		default:
			return nil, fmt.Errorf("unknown search scope: %s", field.SearchScope)
		}
	}
}

func mapLogFieldExpression(field *FieldDefinition) (string, error) {
	// Direct log columns (snake_case in schema)
	name := field.Name
	if name == "" {
		return "", fmt.Errorf("empty field name")
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
		return "l." + camelToSnake(name), nil
	}
}

func mapLogAttributeExpressions(field *FieldDefinition) ([]string, error) {
	switch field.AttributeScope {
	case "resource", "scope", "log":
		expr := fmt.Sprintf("(SELECT a.value FROM attributes a WHERE a.log_id = l.id AND a.scope = '%s' AND a.key = '%s' LIMIT 1)", field.AttributeScope, field.Name)
		return []string{expr}, nil
	default:
		return nil, fmt.Errorf("unknown attribute scope: %s", field.AttributeScope)
	}
}

// mapLogGlobalExpressions returns all SQL expressions for a global search across logs.
//
// The "= ?" placeholders are conventions: BuildOperatorCondition replaces "= ?" with the
// actual operator and a named CTE parameter (e.g. "LIKE value_0") based on the query's
// FieldOperator.
//
// See BuildOperatorCondition in query_tree.go.
func mapLogGlobalExpressions() ([]string, error) {
	return []string{
		"l.search_text = ?",
		"EXISTS(SELECT 1 FROM attributes a WHERE a.log_id = l.id AND (a.key = ? OR a.value = ?))",
	}, nil
}

// BuildLogSQL converts a QueryNode into a parameterized CTE, WHERE clause, and args for log queries.
// The WHERE clause uses l.log_time so the caller can substitute the full (CASE WHEN ...) expression.
func BuildLogSQL(queryNode *QueryNode, startTime, endTime int64) (cteSQL string, whereSQL string, args []any, err error) {
	return BuildSearchSQL(queryNode, startTime, endTime, logFieldMapper(), "l.log_time >= time_start AND l.log_time <= time_end")
}
