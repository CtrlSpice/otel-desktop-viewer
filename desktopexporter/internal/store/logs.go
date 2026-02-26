package store

import (
	"context"
	"encoding/hex"
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
				traceIDStr := ""
				if tid := log.TraceID(); !tid.IsEmpty() {
					traceIDStr = hex.EncodeToString(tid[:])
				}
				spanIDStr := ""
				if sid := log.SpanID(); !sid.IsEmpty() {
					spanIDStr = hex.EncodeToString(sid[:])
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
					traceIDStr,                        // TraceID VARCHAR
					spanIDStr,                         // SpanID VARCHAR
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

	// Log time: prefer Timestamp, fall back to ObservedTimestamp per OTLP
	logTimeExpr := `(CASE WHEN l.Timestamp IS NULL OR l.Timestamp = 0 THEN l.ObservedTimestamp ELSE l.Timestamp END)`
	whereWithTime := strings.ReplaceAll(whereClause, "l.log_time", logTimeExpr)
	finalQuery := fmt.Sprintf(`%s,
		filtered AS (
			SELECT l.* FROM logs l, search_params
			WHERE %s
		),
		log_attrs AS (
			SELECT a.LogID, a.Scope, json_group_array(json_object('key', a.Key, 'value', a.Value, 'type', a.Type::VARCHAR)) AS attrs
			FROM attributes a
			WHERE a.LogID IN (SELECT ID FROM filtered)
			GROUP BY a.LogID, a.Scope
		)
		SELECT CAST(COALESCE(to_json(list(json_object(
			'id', l.ID,
			'timestamp', l.Timestamp,
			'observedTimestamp', l.ObservedTimestamp,
			'traceID', l.TraceID,
			'spanID', l.SpanID,
			'severityText', l.SeverityText,
			'severityNumber', l.SeverityNumber,
			'body', l.Body,
			'bodyType', l.BodyType,
			'resource', json_object('attributes', COALESCE(res.attrs, json('[]')), 'droppedAttributesCount', l.ResourceDroppedAttributesCount),
			'scope', json_object('name', l.ScopeName, 'version', l.ScopeVersion, 'attributes', COALESCE(scope_attrs.attrs, json('[]')), 'droppedAttributesCount', l.ScopeDroppedAttributesCount),
			'droppedAttributesCount', l.DroppedAttributesCount,
			'flags', l.Flags,
			'eventName', l.EventName,
			'attributes', COALESCE(log_attrs.attrs, json('[]'))
		) ORDER BY COALESCE(l.Timestamp, l.ObservedTimestamp) DESC)), '[]') AS VARCHAR) AS logs
		FROM filtered l
		LEFT JOIN log_attrs res ON res.LogID = l.ID AND res.Scope = 'resource'
		LEFT JOIN log_attrs scope_attrs ON scope_attrs.LogID = l.ID AND scope_attrs.Scope = 'scope'
		LEFT JOIN log_attrs log_attrs ON log_attrs.LogID = l.ID AND log_attrs.Scope = 'log'`,
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
		`DELETE FROM attributes WHERE LogID IS NOT NULL`,
		`TRUNCATE TABLE logs`,
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
		`DELETE FROM attributes WHERE LogID = ?`,
		`DELETE FROM logs WHERE ID = ?`,
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
		fmt.Sprintf(`DELETE FROM attributes WHERE LogID IN (%s)`, placeholders),
		fmt.Sprintf(`DELETE FROM logs WHERE ID IN (%s)`, placeholders),
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
	// Direct log columns: TraceID, SpanID, SeverityText, SeverityNumber, Body, EventName, ScopeName, ScopeVersion, etc.
	name := field.Name
	if name == "" {
		return "", fmt.Errorf("empty field name")
	}
	// Map common names to column names (logs table uses PascalCase in schema)
	switch name {
	case "traceID", "traceId":
		return "l.TraceID", nil
	case "spanID", "spanId":
		return "l.SpanID", nil
	case "severityText":
		return "l.SeverityText", nil
	case "severityNumber":
		return "l.SeverityNumber", nil
	case "body":
		return "l.Body", nil
	case "eventName":
		return "l.EventName", nil
	case "scope.name":
		return "l.ScopeName", nil
	case "scope.version":
		return "l.ScopeVersion", nil
	default:
		cap := strings.ToUpper(name[:1]) + name[1:]
		return "l." + cap, nil
	}
}

func mapLogAttributeExpressions(field *FieldDefinition) ([]string, error) {
	switch field.AttributeScope {
	case "resource", "scope", "log":
		expr := fmt.Sprintf("(SELECT a.Value FROM attributes a WHERE a.LogID = l.ID AND a.Scope = '%s' AND a.Key = '%s' LIMIT 1)", field.AttributeScope, field.Name)
		return []string{expr}, nil
	default:
		return nil, fmt.Errorf("unknown attribute scope: %s", field.AttributeScope)
	}
}

func mapLogGlobalExpressions() ([]string, error) {
	return []string{
		"l.SearchText LIKE ?",
		"EXISTS(SELECT 1 FROM attributes a WHERE a.LogID = l.ID AND (a.Key LIKE ? OR a.Value LIKE ?))",
	}, nil
}

// BuildLogSQL converts a QueryNode into a parameterized CTE, WHERE clause, and args for log queries.
// The WHERE clause uses l.log_time so the caller can substitute the full (CASE WHEN ...) expression.
func BuildLogSQL(queryNode *QueryNode, startTime, endTime int64) (cteSQL string, whereSQL string, args []any, err error) {
	return BuildSearchSQL(queryNode, startTime, endTime, logFieldMapper(), "l.log_time >= time_start AND l.log_time <= time_end")
}
