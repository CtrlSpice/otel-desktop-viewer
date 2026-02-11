package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"encoding/hex"

	"github.com/google/uuid"
	"github.com/marcboeker/go-duckdb/v2"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// flushIntervalSpans is how many spans to buffer before flushing appenders. Normalized schema keeps row size predictable.
const flushIntervalSpans = 100

// IngestSpans ingests trace spans from pdata into the spans, events, links, and attributes tables
func (s *Store) IngestSpans(ctx context.Context, traces ptrace.Traces) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf(ErrAddSpans, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	appenders := struct {
		attributes *duckdb.Appender
		events     *duckdb.Appender
		links      *duckdb.Appender
		spans      *duckdb.Appender
	}{}

	tables := []string{"attributes", "events", "links", "spans"}
	dests := []**duckdb.Appender{&appenders.attributes, &appenders.events, &appenders.links, &appenders.spans}
	for i, table := range tables {
		a, err := duckdb.NewAppender(s.conn, "", "", table)
		if err != nil {
			return fmt.Errorf(ErrCreateAppender, err)
		}
		*dests[i] = a
		defer a.Close()
	}

	spanCount := 0
	for _, resourceSpan := range traces.ResourceSpans().All() {
		resource := resourceSpan.Resource()

		for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
			scope := scopeSpan.Scope()

			for _, span := range scopeSpan.Spans().All() {
				traceID := span.TraceID()
				traceIDStr := hex.EncodeToString(traceID[:])

				spanID := span.SpanID()
				spanIDStr := hex.EncodeToString(spanID[:])

				parentSpanID := span.ParentSpanID()
				parentSpanIDStr := ""
				if !parentSpanID.IsEmpty() {
					parentSpanIDStr = hex.EncodeToString(parentSpanID[:])
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

				err := appenders.spans.AppendRow(
					traceIDStr,                        // TraceID VARCHAR
					span.TraceState().AsRaw(),         // TraceState VARCHAR
					spanIDStr,                         // SpanID VARCHAR
					parentSpanIDStr,                   // ParentSpanID VARCHAR
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
					return fmt.Errorf(ErrAppendRow, err)
				}

				// Insert events into events table (generate UUID in Go so we can set event attributes)
				for _, event := range span.Events().All() {
					eventID := uuid.New()
					err = appenders.events.AppendRow(
						eventID.String(),               // ID UUID
						spanIDStr,                      // SpanID VARCHAR
						event.Name(),                   // Name VARCHAR
						int64(event.Timestamp()),       // Timestamp BIGINT
						event.DroppedAttributesCount(), // DroppedAttributesCount UINTEGER
						event.Name(),                   // SearchText VARCHAR
					)
					if err != nil {
						return fmt.Errorf(ErrAppendRow, err)
					}
					if err := IngestAttributes(appenders.attributes,
						[]AttributeBatchItem{{Attrs: event.Attributes(), IDs: AttributeOwnerIDs{SpanID: spanIDStr, EventID: &eventID}, Scope: "event"}}); err != nil {
						return err
					}
				}

				// Insert links into links table (generate UUID in Go so we can set link attributes)
				for _, link := range span.Links().All() {
					linkID := uuid.New()
					linkTraceID := link.TraceID()
					linkTraceIDStr := hex.EncodeToString(linkTraceID[:])
					linkSpanID := link.SpanID()
					linkSpanIDStr := hex.EncodeToString(linkSpanID[:])

					linkSearchText := strings.Join([]string{
						linkTraceIDStr,
						linkSpanIDStr,
						link.TraceState().AsRaw(),
					}, " ")

					err = appenders.links.AppendRow(
						linkID.String(),               // ID UUID
						spanIDStr,                     // SpanID VARCHAR
						linkTraceIDStr,                // TraceID VARCHAR
						linkSpanIDStr,                 // LinkedSpanID VARCHAR
						link.TraceState().AsRaw(),     // TraceState VARCHAR
						link.DroppedAttributesCount(), // DroppedAttributesCount UINTEGER
						linkSearchText,                // SearchText VARCHAR
					)
					if err != nil {
						return fmt.Errorf(ErrAppendRow, err)
					}
					if err := IngestAttributes(appenders.attributes, []AttributeBatchItem{{Attrs: link.Attributes(), IDs: AttributeOwnerIDs{SpanID: spanIDStr, LinkID: &linkID}, Scope: "link"}}); err != nil {
						return err
					}
				}

				// Insert attributes: span + resource + scope (same SpanID, distinct Scope)
				spanIDs := AttributeOwnerIDs{SpanID: spanIDStr}
				if err := IngestAttributes(appenders.attributes, []AttributeBatchItem{
					{Attrs: span.Attributes(), IDs: spanIDs, Scope: "span"},
					{Attrs: resource.Attributes(), IDs: spanIDs, Scope: "resource"},
					{Attrs: scope.Attributes(), IDs: spanIDs, Scope: "scope"},
				}); err != nil {
					return err
				}

				spanCount++
				// Flush periodically so appender buffers don't grow unbounded. Normalized schema keeps row size predictable.
				if spanCount%flushIntervalSpans == 0 {
					if err := appenders.spans.Flush(); err != nil {
						return fmt.Errorf(ErrFlushAppender, err)
					}
					if err := appenders.events.Flush(); err != nil {
						return fmt.Errorf(ErrFlushAppender, err)
					}
					if err := appenders.links.Flush(); err != nil {
						return fmt.Errorf(ErrFlushAppender, err)
					}
					if err := appenders.attributes.Flush(); err != nil {
						return fmt.Errorf(ErrFlushAppender, err)
					}
				}
			}
		}
	}

	return nil
}

func (s *Store) SearchTraces(ctx context.Context, startTime int64, endTime int64, query any) (json.RawMessage, error) {
	if err := s.checkConnection(); err != nil {
		return nil, fmt.Errorf(ErrSearchTraces, err)
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
		SELECT COALESCE(json_group_array(json_object(
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
		)), '[]') AS summaries
		FROM (
			SELECT DISTINCT ON (s.TraceID)
				s.TraceID,
				CASE WHEN s.ParentSpanID = '' THEN (
					SELECT a.Value FROM attributes a
					WHERE a.SpanID = s.SpanID AND a.Scope = 'resource' AND a.Key = 'service.name'
					LIMIT 1
				) END as service_name,
				CASE WHEN s.ParentSpanID = '' THEN s.Name END as root_name,
				CASE WHEN s.ParentSpanID = '' THEN s.StartTime END as root_start_time,
				CASE WHEN s.ParentSpanID = '' THEN s.EndTime END as root_end_time,
				COUNT(*) OVER (PARTITION BY s.TraceID) as span_count,
				COUNT(CASE WHEN s.StatusCode = 'ERROR' THEN 1 END) OVER (PARTITION BY s.TraceID) as error_count,
				COUNT(CASE WHEN EXISTS(
					SELECT 1 FROM attributes a
					WHERE a.SpanID = s.SpanID AND a.Scope = 'span' AND a.Key = 'exception.type'
				) THEN 1 END) OVER (PARTITION BY s.TraceID) as exception_count
			FROM spans s, search_params
			WHERE %s
			ORDER BY
				COALESCE(
					MIN(CASE WHEN s.ParentSpanID = '' THEN s.StartTime END) OVER (PARTITION BY s.TraceID),
					MIN(s.StartTime) OVER (PARTITION BY s.TraceID)
				) DESC,
				s.TraceID,
				CASE WHEN s.ParentSpanID = '' THEN 0 ELSE 1 END
		) sub`, cteSQL, whereClause)

	var raw []byte
	if err := s.db.QueryRowContext(ctx, finalQuery, args...).Scan(&raw); err != nil {
		return nil, fmt.Errorf(ErrSearchTraces, err)
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
					CASE WHEN s.ParentSpanID IS NULL OR s.ParentSpanID = '' THEN 0 ELSE 1 END,
					s.StartTime
				)] AS sort_path
			FROM spans s, param p
			WHERE s.TraceID = p.traceID
			AND s.ParentSpanID NOT IN (SELECT SpanID FROM spans WHERE TraceID = p.traceID)

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
				json_group_array(json_object(
					'name', e.Name,
					'timestamp', e.Timestamp,
					'droppedAttributesCount', e.DroppedAttributesCount,
					'attributes', COALESCE(ea.attributes, json('[]'))
				)) AS events
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
					'traceID', encode(l.TraceID, 'hex'),
					'spanID', encode(l.LinkedSpanID, 'hex'),
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
					'traceID',       encode(st.TraceID, 'hex'),
					'traceState',    st.TraceState,
					'spanID',        encode(st.SpanID, 'hex'),
					'parentSpanID',  encode(st.ParentSpanID, 'hex'),
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
			) AS span_json
			FROM spans_tree st
			LEFT JOIN span_attributes sa_span  ON st.SpanID = sa_span.SpanID  AND sa_span.Scope  = 'span'
			LEFT JOIN span_attributes sa_res   ON st.SpanID = sa_res.SpanID   AND sa_res.Scope   = 'resource'
			LEFT JOIN span_attributes sa_scope ON st.SpanID = sa_scope.SpanID AND sa_scope.Scope  = 'scope'
			LEFT JOIN event_data ed       ON st.SpanID = ed.SpanID
			LEFT JOIN link_data  ld       ON st.SpanID = ld.SpanID
			ORDER BY st.sort_path
		)

		-- 8. Wrap everything in {traceID, spans: [...]}
		SELECT json_object(
			'traceID', (SELECT encode(traceID, 'hex') FROM param),
			'spans',   COALESCE(json_group_array(span_json), json('[]'))
		) AS trace
		FROM ordered_spans
	`
	var raw []byte
	if err := s.db.QueryRowContext(ctx, query, traceID).Scan(&raw); err != nil {
		return nil, fmt.Errorf("failed to get trace: %w", err)
	}
	if raw == nil {
		return nil, fmt.Errorf(ErrGetTrace, traceID, ErrTraceIDNotFound)
	}
	return json.RawMessage(raw), nil
}

// ClearTraces truncates the spans table.
func (s *Store) ClearTraces(ctx context.Context) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf(ErrClearTraces, err)
	}

	if _, err := s.db.ExecContext(ctx, TruncateSpans); err != nil {
		return fmt.Errorf(ErrClearTraces, err)
	}
	return nil
}

// DeleteSpansByTraceID deletes all spans for a specific trace.
func (s *Store) DeleteSpansByTraceID(ctx context.Context, traceID string) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf(ErrDeleteSpansByTraceID, err)
	}

	_, err := s.db.ExecContext(ctx, DeleteSpansByTraceID, traceID)
	if err != nil {
		return fmt.Errorf(ErrDeleteSpansByTraceID, err)
	}

	return nil
}

// GetTraceAttributes discovers all attributes whose SpanID belongs to a span in the given time range.
// Returns a JSON array of objects { "name", "attributeScope", "type" } built by DuckDB.
func (s *Store) GetTraceAttributes(ctx context.Context, startTime, endTime int64) (json.RawMessage, error) {
	if err := s.checkConnection(); err != nil {
		return nil, fmt.Errorf(ErrGetTraceAttributes, err)
	}

	query := `
		SELECT json_group_array(json_object('name', sub.Key, 'attributeScope', sub.Scope, 'type', sub.Type::VARCHAR)) AS attributes
		FROM (
			SELECT DISTINCT a.Key, a.Scope, a.Type
			FROM attributes a
			INNER JOIN spans s ON a.SpanID = s.SpanID
			WHERE s.StartTime >= ? AND s.StartTime <= ?
			ORDER BY a.Key, a.Scope
		) sub
	`
	var raw []byte
	if err := s.db.QueryRowContext(ctx, query, startTime, endTime).Scan(&raw); err != nil {
		return nil, fmt.Errorf(ErrGetTraceAttributes, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// DeleteSpanByID deletes a specific span by its ID.
func (s *Store) DeleteSpanByID(ctx context.Context, spanID string) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf(ErrDeleteSpanByID, err)
	}

	_, err := s.db.ExecContext(ctx, `DELETE FROM spans WHERE SpanID = ?`, spanID)
	if err != nil {
		return fmt.Errorf(ErrDeleteSpanByID, err)
	}

	return nil
}

// DeleteSpansByIDs deletes multiple spans by their IDs.
func (s *Store) DeleteSpansByIDs(ctx context.Context, spanIDs []any) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf(ErrDeleteSpanByID, err)
	}

	if len(spanIDs) == 0 {
		return nil // Nothing to delete
	}

	placeholders := buildPlaceholders(len(spanIDs))
	query := fmt.Sprintf(`DELETE FROM spans WHERE SpanID IN (%s)`, placeholders)

	_, err := s.db.ExecContext(ctx, query, spanIDs...)
	if err != nil {
		return fmt.Errorf(ErrDeleteSpanByID, err)
	}

	return nil
}

// DeleteSpansByTraceIDs deletes all spans for multiple traces.
func (s *Store) DeleteSpansByTraceIDs(ctx context.Context, traceIDs []any) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf(ErrDeleteSpansByTraceID, err)
	}

	if len(traceIDs) == 0 {
		return nil // Nothing to delete
	}

	placeholders := buildPlaceholders(len(traceIDs))
	query := fmt.Sprintf(`DELETE FROM spans WHERE TraceID IN (%s)`, placeholders)

	_, err := s.db.ExecContext(ctx, query, traceIDs...)
	if err != nil {
		return fmt.Errorf(ErrDeleteSpansByTraceID, err)
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
func mapTraceGlobalExpressions() ([]string, error) {
	return []string{
		"s.SearchText LIKE ?",
		"EXISTS(SELECT 1 FROM events e WHERE e.SpanID = s.SpanID AND e.SearchText LIKE ?)",
		"EXISTS(SELECT 1 FROM links l WHERE l.SpanID = s.SpanID AND l.SearchText LIKE ?)",
		"EXISTS(SELECT 1 FROM attributes a WHERE a.SpanID = s.SpanID AND (a.Key LIKE ? OR a.Value LIKE ?))",
	}, nil
}

// mapCommonFields handles resource and scope field mapping (shared across signals)
func mapCommonFields(fieldName string) (string, bool) {

	return "", false
}

// BuildTraceSQL converts a QueryNode into a parameterized CTE, WHERE clause, and args slice
// for trace queries. This is the trace-specific entry point that provides schema knowledge
// to the generic query tree walker.
func BuildTraceSQL(queryNode *QueryNode, startTime, endTime int64) (cteSQL string, whereSQL string, args []any, err error) {
	namedArgs := make(map[string]any)

	// Always add time parameters
	namedArgs["time_start"] = startTime
	namedArgs["time_end"] = endTime

	// Walk the query tree with the trace field mapper
	var conditions []string
	if queryNode != nil {
		if err := BuildConditions(queryNode, &conditions, &namedArgs, traceFieldMapper()); err != nil {
			return "", "", nil, err
		}
	}

	// Build WHERE clause
	if len(conditions) > 0 {
		whereSQL = "(" + strings.Join(conditions, " ") + ") AND StartTime >= time_start AND StartTime <= time_end"
	} else {
		whereSQL = "StartTime >= time_start AND StartTime <= time_end"
	}

	// Convert namedArgs to ordered args slice
	args = make([]any, len(namedArgs))
	paramNames := make([]string, len(namedArgs))

	// Time parameters first (deterministic order)
	args[0] = namedArgs["time_start"]
	paramNames[0] = "time_start"
	args[1] = namedArgs["time_end"]
	paramNames[1] = "time_end"

	// User parameters sorted alphabetically
	userParamIndex := 2
	var userParamNames []string
	for name := range namedArgs {
		if name != "time_start" && name != "time_end" {
			userParamNames = append(userParamNames, name)
		}
	}
	sort.Strings(userParamNames)
	for _, name := range userParamNames {
		args[userParamIndex] = namedArgs[name]
		paramNames[userParamIndex] = name
		userParamIndex++
	}

	// Build CTE
	var cteParams []string
	for _, name := range paramNames {
		cteParams = append(cteParams, fmt.Sprintf("? as %s", name))
	}
	cteSQL = fmt.Sprintf("WITH search_params AS (SELECT %s)", strings.Join(cteParams, ", "))

	return cteSQL, whereSQL, args, nil
}
