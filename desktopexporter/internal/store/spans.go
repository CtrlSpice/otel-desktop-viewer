package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/marcboeker/go-duckdb/v2"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// flushIntervalSpans is how many spans to buffer before flushing appenders. Normalized schema keeps row size predictable.
const flushIntervalSpans = 50

// IngestSpans ingests trace spans from pdata into the spans, events, links, and attributes tables
func (s *Store) IngestSpans(ctx context.Context, traces ptrace.Traces) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf("failed to add spans: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tables := []string{"attributes", "events", "links", "spans"}
	appenders, err := NewAppenders(s.conn, tables)
	if err != nil {
		return err
	}
	defer CloseAppenders(appenders, tables)

	spanCount := 0
	for _, resourceSpan := range traces.ResourceSpans().All() {
		resource := resourceSpan.Resource()

		for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
			scope := scopeSpan.Scope()

			for _, span := range scopeSpan.Spans().All() {
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
				spanSearchText := strings.Join([]string{
					span.Name(),
					span.Kind().String(),
					span.Status().Code().String(),
					span.Status().Message(),
					span.TraceState().AsRaw(),
					scope.Name(),
					scope.Version(),
				}, " ")

				err := appenders["spans"].AppendRow(
					traceUUID,                         // TraceID UUID
					span.TraceState().AsRaw(),         // TraceState VARCHAR
					spanUUID,                          // SpanID UUID
					parentSpanUUID,                    // ParentSpanID UUID
					span.Name(),                       // Name VARCHAR
					span.Kind().String(),              // Kind VARCHAR
					int64(span.StartTimestamp()),      // StartTime BIGINT
					int64(span.EndTimestamp()),        // EndTime BIGINT
					resource.DroppedAttributesCount(), // ResourceDroppedAttributesCount UINTEGER
					scope.Name(),                      // ScopeName VARCHAR
					scope.Version(),                   // ScopeVersion VARCHAR
					scope.DroppedAttributesCount(),    // ScopeDroppedAttributesCount UINTEGER
					span.DroppedAttributesCount(),     // DroppedAttributesCount UINTEGER
					span.DroppedEventsCount(),         // DroppedEventsCount UINTEGER
					span.DroppedLinksCount(),          // DroppedLinksCount UINTEGER
					span.Status().Code().String(),     // StatusCode VARCHAR
					span.Status().Message(),           // StatusMessage VARCHAR
					spanSearchText,                    // SearchText VARCHAR
				)
				if err != nil {
					return fmt.Errorf("failed to append row: %w", err)
				}

				// Insert events into events table (generate UUID in Go so we can set event attributes)
				for _, event := range span.Events().All() {
					eventID := duckdb.UUID(uuid.New())
					err = appenders["events"].AppendRow(
						eventID,                        // ID UUID
						spanUUID,                       // SpanID UUID
						event.Name(),                   // Name VARCHAR
						int64(event.Timestamp()),       // Timestamp BIGINT
						event.DroppedAttributesCount(), // DroppedAttributesCount UINTEGER
						event.Name(),                   // SearchText VARCHAR
					)
					if err != nil {
						return fmt.Errorf("failed to append row: %w", err)
					}
					if err := IngestAttributes(appenders["attributes"],
						[]AttributeBatchItem{{Attrs: event.Attributes(), IDs: AttributeOwnerIDs{SpanID: &spanUUID, EventID: &eventID}, Scope: "event"}}); err != nil {
						return err
					}
				}

				// Insert links into links table (generate UUID in Go so we can set link attributes)
				for _, link := range span.Links().All() {
					linkID := duckdb.UUID(uuid.New())
					linkTraceUUID := duckdb.UUID(link.TraceID())
					linkSpanID := link.SpanID()
					var linkSpanPadded [16]byte
					copy(linkSpanPadded[8:], linkSpanID[:])
					linkSpanUUID := duckdb.UUID(linkSpanPadded)

					linkSearchText := strings.Join([]string{
						link.TraceID().String(),
						link.SpanID().String(),
						link.TraceState().AsRaw(),
					}, " ")

					err = appenders["links"].AppendRow(
						linkID,                        // ID UUID
						spanUUID,                      // SpanID UUID
						linkTraceUUID,                 // TraceID UUID
						linkSpanUUID,                  // LinkedSpanID UUID
						link.TraceState().AsRaw(),     // TraceState VARCHAR
						link.DroppedAttributesCount(), // DroppedAttributesCount UINTEGER
						linkSearchText,                // SearchText VARCHAR
					)
					if err != nil {
						return fmt.Errorf("failed to append row: %w", err)
					}
					if err := IngestAttributes(appenders["attributes"], []AttributeBatchItem{{Attrs: link.Attributes(), IDs: AttributeOwnerIDs{SpanID: &spanUUID, LinkID: &linkID}, Scope: "link"}}); err != nil {
						return err
					}
				}

				// Insert attributes: span + resource + scope (same SpanID, distinct Scope)
				spanIDs := AttributeOwnerIDs{SpanID: &spanUUID}
				if err := IngestAttributes(appenders["attributes"], []AttributeBatchItem{
					{Attrs: span.Attributes(), IDs: spanIDs, Scope: "span"},
					{Attrs: resource.Attributes(), IDs: spanIDs, Scope: "resource"},
					{Attrs: scope.Attributes(), IDs: spanIDs, Scope: "scope"},
				}); err != nil {
					return err
				}

				spanCount++
				// Flush periodically so appender buffers don't grow unbounded. Normalized schema keeps row size predictable.
				if spanCount%flushIntervalSpans == 0 {
					if err := FlushAppenders(appenders, tables); err != nil {
						return fmt.Errorf("failed to flush appender: %w", err)
					}
				}
			}
		}
	}

	return nil
}

func (s *Store) SearchTraces(ctx context.Context, startTime int64, endTime int64, query any) (json.RawMessage, error) {
	if err := s.checkConnection(); err != nil {
		return nil, fmt.Errorf("failed to search traces: %w", err)
	}

	// 1. Parse query tree
	var queryTree *QueryNode
	if query != nil {
		var err error
		queryTree, err = ParseQueryTree(query)
		if err != nil {
			return nil, fmt.Errorf("failed to parse query tree: %w", err)
		}
	}

	// 2. Build CTE, WHERE clause, and args using trace-specific SQL builder
	cteSQL, whereClause, args, err := BuildTraceSQL(queryTree, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to build trace SQL: %w", err)
	}

	// 3. Compose the full query from parts
	finalQuery := fmt.Sprintf(`%s
		SELECT CAST(COALESCE(to_json(list(json_object(
			'traceID',        sub.TraceID,
			'rootSpan',       CASE WHEN sub.service_name IS NOT NULL THEN json_object(
				'serviceName', sub.service_name,
				'name',        sub.root_name,
				'startTime',   sub.root_start_time,
				'endTime',     sub.root_end_time
			) END,
			'spanCount',      sub.span_count,
			'errorCount',     sub.error_count,
			'exceptionCount', sub.exception_count
		) ORDER BY
			COALESCE(sub.root_start_time, (SELECT MIN(s2.StartTime) FROM spans s2 WHERE s2.TraceID = sub.TraceID)) DESC
		)), '[]') AS VARCHAR) AS summaries
		FROM (
			SELECT DISTINCT ON (s.TraceID)
				s.TraceID,
				CASE WHEN s.ParentSpanID IS NULL THEN (
					SELECT a.Value FROM attributes a
					WHERE a.SpanID = s.SpanID AND a.Scope = 'resource' AND a.Key = 'service.name'
					LIMIT 1
				) END as service_name,
				CASE WHEN s.ParentSpanID IS NULL THEN s.Name END as root_name,
				CASE WHEN s.ParentSpanID IS NULL THEN s.StartTime END as root_start_time,
				CASE WHEN s.ParentSpanID IS NULL THEN s.EndTime END as root_end_time,
				COUNT(*) OVER (PARTITION BY s.TraceID) as span_count,
				COUNT(CASE WHEN s.StatusCode = 'ERROR' THEN 1 END) OVER (PARTITION BY s.TraceID) as error_count,
				COUNT(CASE WHEN EXISTS(
					SELECT 1 FROM attributes a
					WHERE a.SpanID = s.SpanID AND a.Scope = 'span' AND a.Key = 'exception.type'
				) THEN 1 END) OVER (PARTITION BY s.TraceID) as exception_count
			FROM spans s, search_params
			WHERE %s
			ORDER BY
				s.TraceID,
				CASE WHEN s.ParentSpanID IS NULL THEN 0 ELSE 1 END
		) sub`, cteSQL, whereClause)

	var raw []byte
	if err := s.db.QueryRowContext(ctx, finalQuery, args...).Scan(&raw); err != nil {
		return nil, fmt.Errorf("failed to search traces: %w", err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

func (s *Store) GetTrace(ctx context.Context, traceID string) (json.RawMessage, error) {
	query := `
		WITH RECURSIVE
		param(traceID) AS (VALUES (?)),

		-- 1. Depth-first span tree (only span-table columns)
		spans_tree AS (
			SELECT
				s.TraceID, s.TraceState, s.SpanID, s.ParentSpanID,
				s.Name, s.Kind, s.StartTime, s.EndTime,
				s.ResourceDroppedAttributesCount, s.ScopeName, s.ScopeVersion,
				s.ScopeDroppedAttributesCount, s.DroppedAttributesCount,
				s.DroppedEventsCount, s.DroppedLinksCount,
				s.StatusCode, s.StatusMessage,
				0 AS depth,
				ARRAY[ROW_NUMBER() OVER (ORDER BY
					CASE WHEN s.ParentSpanID IS NULL THEN 0 ELSE 1 END,
					s.StartTime
				)] AS sort_path
			FROM spans s, param p
			WHERE s.TraceID = p.traceID
			AND (s.ParentSpanID IS NULL OR s.ParentSpanID NOT IN (SELECT SpanID FROM spans WHERE TraceID = p.traceID))

			UNION ALL

			SELECT
				s.TraceID, s.TraceState, s.SpanID, s.ParentSpanID,
				s.Name, s.Kind, s.StartTime, s.EndTime,
				s.ResourceDroppedAttributesCount, s.ScopeName, s.ScopeVersion,
				s.ScopeDroppedAttributesCount, s.DroppedAttributesCount,
				s.DroppedEventsCount, s.DroppedLinksCount,
				s.StatusCode, s.StatusMessage,
				st.depth + 1,
				st.sort_path || ARRAY[ROW_NUMBER() OVER (
					PARTITION BY st.SpanID ORDER BY s.StartTime
				)] AS sort_path
			FROM spans s, param p
			JOIN spans_tree st ON s.ParentSpanID = st.SpanID AND s.TraceID = st.TraceID
			WHERE s.TraceID = p.traceID
		),

		-- 2. Attributes grouped by (SpanID, Scope) → one JSON object per group
		--    Covers scope = 'resource', 'scope', 'span' (EventID/LinkID are NULL)
		span_attributes AS (
			SELECT a.SpanID, a.Scope, json_group_array(json_object('key', a.Key, 'value', a.Value, 'type', a.Type::VARCHAR)) AS attributes
			FROM attributes a
			WHERE a.SpanID IN (SELECT SpanID FROM spans_tree)
				AND a.EventID IS NULL AND a.LinkID IS NULL
			GROUP BY a.SpanID, a.Scope
		),

		-- 3. Event attributes → one JSON object per EventID
		event_attributes AS (
			SELECT a.EventID,
				json_group_array(json_object('key', a.Key, 'value', a.Value, 'type', a.Type::VARCHAR)) AS attributes
			FROM attributes a
			WHERE a.EventID IS NOT NULL
				AND a.SpanID IN (SELECT SpanID FROM spans_tree)
			GROUP BY a.EventID
		),

		-- 4. Events with their attributes → one JSON array per SpanID
		event_data AS (
			SELECT e.SpanID,
				to_json(list(json_object(
					'name', e.Name,
					'timestamp', e.Timestamp,
					'droppedAttributesCount', e.DroppedAttributesCount,
					'attributes', COALESCE(ea.attributes, json('[]'))
				) ORDER BY e.Timestamp)) AS events
			FROM events e
			LEFT JOIN event_attributes ea ON e.ID = ea.EventID
			WHERE e.SpanID IN (SELECT SpanID FROM spans_tree)
			GROUP BY e.SpanID
		),

		-- 5. Link attributes → one JSON object per LinkID
		link_attributes AS (
			SELECT a.LinkID,
				json_group_array(json_object('key', a.Key, 'value', a.Value, 'type', a.Type::VARCHAR)) AS attributes
			FROM attributes a
			WHERE a.LinkID IS NOT NULL
				AND a.SpanID IN (SELECT SpanID FROM spans_tree)
			GROUP BY a.LinkID
		),

		-- 6. Links with their attributes → one JSON array per SpanID
		link_data AS (
			SELECT l.SpanID,
				json_group_array(json_object(
				'traceID', l.TraceID,
				'spanID', l.LinkedSpanID,
					'traceState', l.TraceState,
					'droppedAttributesCount', l.DroppedAttributesCount,
					'attributes', COALESCE(la.attributes, json('[]'))
				)) AS links
			FROM links l
			LEFT JOIN link_attributes la ON l.ID = la.LinkID
			WHERE l.SpanID IN (SELECT SpanID FROM spans_tree)
			GROUP BY l.SpanID
		),

		-- 7. Assemble each span as a JSON object (with depth), ordered depth-first
		ordered_spans AS (
			SELECT json_object(
				'spanData', json_object(
				'traceID',       st.TraceID,
				'traceState',    st.TraceState,
				'spanID',        st.SpanID,
				'parentSpanID',  st.ParentSpanID,
					'name',          st.Name,
					'kind',          st.Kind,
					'startTime',     st.StartTime,
					'endTime',       st.EndTime,
					'attributes',    COALESCE(sa_span.attributes, json('[]')),
					'events',        COALESCE(ed.events, json('[]')),
					'links',         COALESCE(ld.links, json('[]')),
					'resource', json_object(
						'attributes',             COALESCE(sa_res.attributes, json('[]')),
						'droppedAttributesCount', st.ResourceDroppedAttributesCount
					),
					'scope', json_object(
						'name',                   st.ScopeName,
						'version',                st.ScopeVersion,
						'attributes',             COALESCE(sa_scope.attributes, json('[]')),
						'droppedAttributesCount', st.ScopeDroppedAttributesCount
					),
					'droppedAttributesCount', st.DroppedAttributesCount,
					'droppedEventsCount',     st.DroppedEventsCount,
					'droppedLinksCount',      st.DroppedLinksCount,
					'statusCode',             st.StatusCode,
					'statusMessage',          st.StatusMessage
				),
				'depth', st.depth
			) AS span_json,
			st.sort_path
			FROM spans_tree st
			LEFT JOIN span_attributes sa_span  ON st.SpanID = sa_span.SpanID  AND sa_span.Scope  = 'span'
			LEFT JOIN span_attributes sa_res   ON st.SpanID = sa_res.SpanID   AND sa_res.Scope   = 'resource'
			LEFT JOIN span_attributes sa_scope ON st.SpanID = sa_scope.SpanID AND sa_scope.Scope  = 'scope'
			LEFT JOIN event_data ed       ON st.SpanID = ed.SpanID
			LEFT JOIN link_data  ld       ON st.SpanID = ld.SpanID
		)

		-- 8. Guard: return NULL if the trace doesn't exist
		-- 9. Wrap everything in {traceID, spans: [...]}
		SELECT CASE
			WHEN NOT EXISTS (SELECT 1 FROM spans WHERE TraceID = (SELECT traceID FROM param))
			THEN NULL
			ELSE CAST(json_object(
				'traceID', (SELECT traceID FROM param),
				'spans',   COALESCE(to_json(list(span_json ORDER BY sort_path)), json('[]'))
			) AS VARCHAR)
		END AS trace
		FROM ordered_spans
	`
	var raw []byte
	if err := s.db.QueryRowContext(ctx, query, traceID).Scan(&raw); err != nil {
		return nil, fmt.Errorf("failed to get trace: %w", err)
	}
	if raw == nil {
		return nil, fmt.Errorf("failed to get trace %s: %w", traceID, ErrTraceIDNotFound)
	}
	return json.RawMessage(raw), nil
}

// ClearTraces truncates the spans table and all child tables (events, links, and their attributes).
func (s *Store) ClearTraces(ctx context.Context) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf("failed to clear traces: %w", err)
	}

	childQueries := []string{
		`DELETE FROM attributes WHERE SpanID IS NOT NULL`,
		`TRUNCATE TABLE links`,
		`TRUNCATE TABLE events`,
		`TRUNCATE TABLE spans`,
	}
	for _, query := range childQueries {
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("failed to clear traces: %w", err)
		}
	}
	return nil
}

// DeleteSpansByTraceID deletes all spans for a specific trace, including child events, links, and attributes.
func (s *Store) DeleteSpansByTraceID(ctx context.Context, traceID string) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf("failed to delete spans by trace ID: %w", err)
	}

	childQueries := []string{
		`DELETE FROM attributes WHERE SpanID IN (SELECT SpanID FROM spans WHERE TraceID = ?)`,
		`DELETE FROM links WHERE SpanID IN (SELECT SpanID FROM spans WHERE TraceID = ?)`,
		`DELETE FROM events WHERE SpanID IN (SELECT SpanID FROM spans WHERE TraceID = ?)`,
		`DELETE FROM spans WHERE TraceID = ?`,
	}
	for _, query := range childQueries {
		if _, err := s.db.ExecContext(ctx, query, traceID); err != nil {
			return fmt.Errorf("failed to delete spans by trace ID: %w", err)
		}
	}

	return nil
}

// GetTraceAttributes discovers all attributes whose SpanID belongs to a span in the given time range.
// Returns a JSON array of objects { "name", "attributeScope", "type" } built by DuckDB.
func (s *Store) GetTraceAttributes(ctx context.Context, startTime, endTime int64) (json.RawMessage, error) {
	if err := s.checkConnection(); err != nil {
		return nil, fmt.Errorf("failed to get trace attributes: %w", err)
	}

	query := `
		SELECT CAST(to_json(list(json_object('name', sub.Key, 'attributeScope', sub.Scope, 'type', sub.Type::VARCHAR)
			ORDER BY sub.Key, sub.Scope)) AS VARCHAR) AS attributes
		FROM (
			SELECT DISTINCT a.Key, a.Scope, a.Type
			FROM attributes a
			INNER JOIN spans s ON a.SpanID = s.SpanID
			WHERE s.StartTime >= ? AND s.StartTime <= ?
		) sub
	`
	var raw []byte
	if err := s.db.QueryRowContext(ctx, query, startTime, endTime).Scan(&raw); err != nil {
		return nil, fmt.Errorf("failed to get trace attributes: %w", err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// DeleteSpanByID deletes a specific span by its ID, including child events, links, and attributes.
func (s *Store) DeleteSpanByID(ctx context.Context, spanID string) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf("failed to delete span by ID: %w", err)
	}

	childQueries := []string{
		`DELETE FROM attributes WHERE SpanID = ?`,
		`DELETE FROM links WHERE SpanID = ?`,
		`DELETE FROM events WHERE SpanID = ?`,
		`DELETE FROM spans WHERE SpanID = ?`,
	}
	for _, query := range childQueries {
		if _, err := s.db.ExecContext(ctx, query, spanID); err != nil {
			return fmt.Errorf("failed to delete span by ID: %w", err)
		}
	}

	return nil
}

// DeleteSpansByIDs deletes multiple spans by their IDs, including child events, links, and attributes.
func (s *Store) DeleteSpansByIDs(ctx context.Context, spanIDs []any) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf("failed to delete span by ID: %w", err)
	}

	if len(spanIDs) == 0 {
		return nil
	}

	placeholders := buildPlaceholders(len(spanIDs))
	childQueries := []string{
		fmt.Sprintf(`DELETE FROM attributes WHERE SpanID IN (%s)`, placeholders),
		fmt.Sprintf(`DELETE FROM links WHERE SpanID IN (%s)`, placeholders),
		fmt.Sprintf(`DELETE FROM events WHERE SpanID IN (%s)`, placeholders),
		fmt.Sprintf(`DELETE FROM spans WHERE SpanID IN (%s)`, placeholders),
	}
	for _, query := range childQueries {
		if _, err := s.db.ExecContext(ctx, query, spanIDs...); err != nil {
			return fmt.Errorf("failed to delete spans by ID: %w", err)
		}
	}

	return nil
}

// DeleteSpansByTraceIDs deletes all spans for multiple traces, including child events, links, and attributes.
func (s *Store) DeleteSpansByTraceIDs(ctx context.Context, traceIDs []any) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf("failed to delete spans by trace ID: %w", err)
	}

	if len(traceIDs) == 0 {
		return nil
	}

	placeholders := buildPlaceholders(len(traceIDs))
	childQueries := []string{
		fmt.Sprintf(`DELETE FROM attributes WHERE SpanID IN (SELECT SpanID FROM spans WHERE TraceID IN (%s))`, placeholders),
		fmt.Sprintf(`DELETE FROM links WHERE SpanID IN (SELECT SpanID FROM spans WHERE TraceID IN (%s))`, placeholders),
		fmt.Sprintf(`DELETE FROM events WHERE SpanID IN (SELECT SpanID FROM spans WHERE TraceID IN (%s))`, placeholders),
		fmt.Sprintf(`DELETE FROM spans WHERE TraceID IN (%s)`, placeholders),
	}
	for _, query := range childQueries {
		if _, err := s.db.ExecContext(ctx, query, traceIDs...); err != nil {
			return fmt.Errorf("failed to delete spans by trace ID: %w", err)
		}
	}

	return nil
}

// traceFieldMapper returns a FieldMapper for trace-specific field-to-SQL mapping.
// This is the bridge between the generic query tree walker and trace schema knowledge.
func traceFieldMapper() FieldMapper {
	return func(field *FieldDefinition) ([]string, error) {
		switch field.SearchScope {
		case "field":
			expr, err := mapTraceFieldExpression(field)
			if err != nil {
				return nil, err
			}
			return []string{expr}, nil
		case "attribute":
			return mapTraceAttributeExpressions(field)
		case "global":
			return mapTraceGlobalExpressions()
		default:
			return nil, fmt.Errorf("unknown search scope: %s", field.SearchScope)
		}
	}
}

// mapTraceFieldExpression maps a trace field-scope field to a SQL expression
func mapTraceFieldExpression(field *FieldDefinition) (string, error) {
	// Resource fields
	if resourceField, found := strings.CutPrefix(field.Name, "resource."); found {
		return "Resource" + strings.ToUpper(resourceField[:1]) + resourceField[1:], nil
	}

	// Scope fields
	if scopeField, found := strings.CutPrefix(field.Name, "scope."); found {
		return "Scope" + strings.ToUpper(scopeField[:1]) + scopeField[1:], nil
	}

	// Event column: event.name -> events.Name
	if col, found := strings.CutPrefix(field.Name, "event."); found {
		capitalized := strings.ToUpper(col[:1]) + col[1:]
		return fmt.Sprintf("EXISTS(SELECT 1 FROM events e WHERE e.SpanID = s.SpanID AND e.%s = ?)", capitalized), nil
	}

	// Link column: link.traceID -> links.TraceID
	if col, found := strings.CutPrefix(field.Name, "link."); found {
		capitalized := strings.ToUpper(col[:1]) + col[1:]
		return fmt.Sprintf("EXISTS(SELECT 1 FROM links l WHERE l.SpanID = s.SpanID AND l.%s = ?)", capitalized), nil
	}

	// Direct span column
	if len(field.Name) > 0 {
		return strings.ToUpper(field.Name[:1]) + field.Name[1:], nil
	}
	return field.Name, nil
}

// mapTraceAttributeExpressions maps trace attributes to SQL expressions
func mapTraceAttributeExpressions(field *FieldDefinition) ([]string, error) {
	switch field.AttributeScope {
	case "resource", "scope", "span":
		expr := fmt.Sprintf("(SELECT a.Value FROM attributes a WHERE a.SpanID = s.SpanID AND a.Scope = '%s' AND a.Key = '%s' LIMIT 1)", field.AttributeScope, field.Name)
		return []string{expr}, nil
	case "event":
		expr := fmt.Sprintf("EXISTS(SELECT 1 FROM events e JOIN attributes a ON a.EventID = e.ID WHERE e.SpanID = s.SpanID AND a.Scope = 'event' AND a.Key = '%s' AND a.Value = ?)", field.Name)
		return []string{expr}, nil
	case "link":
		expr := fmt.Sprintf("EXISTS(SELECT 1 FROM links l JOIN attributes a ON a.LinkID = l.ID WHERE l.SpanID = s.SpanID AND a.Scope = 'link' AND a.Key = '%s' AND a.Value = ?)", field.Name)
		return []string{expr}, nil
	default:
		return nil, fmt.Errorf("unknown attribute scope: %s", field.AttributeScope)
	}
}

// mapTraceGlobalExpressions returns all SQL expressions for a global search across traces.
// Each table has a SearchText column populated at ingest time; attributes are searched directly via Key/Value.
//
// The "= ?" placeholders are conventions: BuildOperatorCondition replaces "= ?" with the
// actual operator and a named CTE parameter (e.g. "LIKE value_0") based on the query's
// FieldOperator.
//
// See BuildOperatorCondition in query_tree.go.
func mapTraceGlobalExpressions() ([]string, error) {
	return []string{
		"s.SearchText = ?",
		"EXISTS(SELECT 1 FROM events e WHERE e.SpanID = s.SpanID AND e.SearchText = ?)",
		"EXISTS(SELECT 1 FROM links l WHERE l.SpanID = s.SpanID AND l.SearchText = ?)",
		"EXISTS(SELECT 1 FROM attributes a WHERE a.SpanID = s.SpanID AND (a.Key = ? OR a.Value = ?))",
	}, nil
}

// BuildTraceSQL converts a QueryNode into a parameterized CTE, WHERE clause, and args slice
// for trace queries.
func BuildTraceSQL(queryNode *QueryNode, startTime, endTime int64) (cteSQL string, whereSQL string, args []any, err error) {
	return BuildSearchSQL(queryNode, startTime, endTime, traceFieldMapper(), "StartTime >= time_start AND StartTime <= time_end")
}
