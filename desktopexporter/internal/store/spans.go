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

	// 2. Build CTE, where clause, and args using trace-specific SQL builder
	cteSQL, whereClause, args, err := BuildTraceSQL(queryTree, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to build trace SQL: %w", err)
	}

	// 3. Compose the full query from parts
	finalQuery := fmt.Sprintf(`%s
		select cast(coalesce(to_json(list(json_object(
			'traceID',        sub.trace_id,
			'rootSpan',       case when sub.service_name is not null then json_object(
				'serviceName', sub.service_name,
				'name',        sub.root_name,
				'startTime',   sub.root_start_time,
				'endTime',     sub.root_end_time
			) end,
			'spanCount',      sub.span_count,
			'errorCount',     sub.error_count,
			'exceptionCount', sub.exception_count
		) order by
			coalesce(sub.root_start_time, (select min(s2.start_time) from spans s2 where s2.trace_id = sub.trace_id)) desc
		)), '[]') as varchar) as summaries
		from (
			select distinct on (s.trace_id)
				s.trace_id,
				case when s.parent_span_id is null then (
					select a.value from attributes a
					where a.span_id = s.span_id and a.scope = 'resource' and a.key = 'service.name'
 limit 1
				) end as service_name,
				case when s.parent_span_id is null then s.name end as root_name,
				case when s.parent_span_id is null then s.start_time end as root_start_time,
				case when s.parent_span_id is null then s.end_time end as root_end_time,
				count(*) over (partition by s.trace_id) as span_count,
				count(case when s.status_code = 'ERROR' then 1 end) over (partition by s.trace_id) as error_count,
				count(case when exists(
					select 1 from attributes a
					where a.span_id = s.span_id and a.scope = 'span' and a.key = 'exception.type'
				) then 1 end) over (partition by s.trace_id) as exception_count
			from spans s, search_params
			where %s
			order by
				s.trace_id,
				case when s.parent_span_id is null then 0 else 1 END
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
		with recursive
		param(trace_id) as (values (?)),

		-- 1. Depth-first span tree (only span-table columns)
		spans_tree as (
			select
				s.trace_id, s.trace_state, s.span_id, s.parent_span_id,
				s.name, s.kind, s.start_time, s.end_time,
				s.resource_dropped_attributes_count, s.scope_name, s.scope_version,
				s.scope_dropped_attributes_count, s.dropped_attributes_count,
				s.dropped_events_count, s.dropped_links_count,
				s.status_code, s.status_message,
				0 as depth,
				array[row_number() over (order by
					case when s.parent_span_id is null then 0 else 1 END,
					s.start_time
				)] as sort_path
			from spans s, param p
			where s.trace_id = p.trace_id
			and (s.parent_span_id is null or s.parent_span_id not in (select span_id from spans where trace_id = p.trace_id))

			union all

			select
				s.trace_id, s.trace_state, s.span_id, s.parent_span_id,
				s.name, s.kind, s.start_time, s.end_time,
				s.resource_dropped_attributes_count, s.scope_name, s.scope_version,
				s.scope_dropped_attributes_count, s.dropped_attributes_count,
				s.dropped_events_count, s.dropped_links_count,
				s.status_code, s.status_message,
				st.depth + 1,
				st.sort_path || array[row_number() over (
					partition by st.span_id order by s.start_time
				)] as sort_path
			from spans s, param p
			join spans_tree st on s.parent_span_id = st.span_id and s.trace_id = st.trace_id
			where s.trace_id = p.trace_id
		),

		-- 2. Attributes grouped by (span_id, scope) → one JSON object per group
		--    Covers scope = 'resource', 'scope', 'span' (event_id/link_id are NULL)
		span_attributes as (
			select a.span_id, a.scope, json_group_array(json_object('key', a.key, 'value', a.value, 'type', a.type::varchar)) as attributes
			from attributes a
			where a.span_id in (select span_id from spans_tree)
				and a.event_id is null and a.link_id is null
 group by  a.span_id, a.scope
		),

		-- 3. Event attributes → one JSON object per event_id
		event_attributes as (
			select a.event_id,
				json_group_array(json_object('key', a.key, 'value', a.value, 'type', a.type::varchar)) as attributes
			from attributes a
			where a.event_id is not null
				and a.span_id in (select span_id from spans_tree)
 group by  a.event_id
		),

		-- 4. Events with their attributes → one JSON array per span_id
		event_data as (
			select e.span_id,
				to_json(list(json_object(
					'name', e.name,
					'timestamp', e.timestamp,
					'droppedAttributesCount', e.dropped_attributes_count,
					'attributes', coalesce(ea.attributes, json('[]'))
				) order by e.timestamp)) as events
			from events e
 left join  event_attributes ea on e.id = ea.event_id
			where e.span_id in (select span_id from spans_tree)
 group by  e.span_id
		),

		-- 5. Link attributes → one JSON object per link_id
		link_attributes as (
			select a.link_id,
				json_group_array(json_object('key', a.key, 'value', a.value, 'type', a.type::varchar)) as attributes
			from attributes a
			where a.link_id is not null
				and a.span_id in (select span_id from spans_tree)
 group by  a.link_id
		),

		-- 6. Links with their attributes → one JSON array per span_id
		link_data as (
			select l.span_id,
				json_group_array(json_object(
				'traceID', l.trace_id,
				'spanID', l.linked_span_id,
					'traceState', l.trace_state,
					'droppedAttributesCount', l.dropped_attributes_count,
					'attributes', coalesce(la.attributes, json('[]'))
				)) as links
			from links l
 left join  link_attributes la on l.id = la.link_id
			where l.span_id in (select span_id from spans_tree)
 group by  l.span_id
		),

		-- 7. Assemble each span as a JSON object (with depth), ordered depth-first
		ordered_spans as (
			select json_object(
				'spanData', json_object(
				'traceID',       st.trace_id,
				'traceState',    st.trace_state,
				'spanID',        st.span_id,
				'parentSpanID',  st.parent_span_id,
					'name',          st.name,
					'kind',          st.kind,
					'startTime',     st.start_time,
					'endTime',       st.end_time,
					'attributes',    coalesce(sa_span.attributes, json('[]')),
					'events',        coalesce(ed.events, json('[]')),
					'links',         coalesce(ld.links, json('[]')),
					'resource', json_object(
						'attributes',             coalesce(sa_res.attributes, json('[]')),
						'droppedAttributesCount', st.resource_dropped_attributes_count
					),
					'scope', json_object(
						'name',                   st.scope_name,
						'version',                st.scope_version,
						'attributes',             coalesce(sa_scope.attributes, json('[]')),
						'droppedAttributesCount', st.scope_dropped_attributes_count
					),
					'droppedAttributesCount', st.dropped_attributes_count,
					'droppedEventsCount',     st.dropped_events_count,
					'droppedLinksCount',      st.dropped_links_count,
					'statusCode',             st.status_code,
					'statusMessage',          st.status_message
				),
				'depth', st.depth
			) as span_json,
			st.sort_path
			from spans_tree st
 left join  span_attributes sa_span  on st.span_id = sa_span.span_id  and sa_span.scope  = 'span'
 left join  span_attributes sa_res   on st.span_id = sa_res.span_id   and sa_res.scope   = 'resource'
 left join  span_attributes sa_scope on st.span_id = sa_scope.span_id and sa_scope.scope  = 'scope'
 left join  event_data ed       on st.span_id = ed.span_id
 left join  link_data  ld       on st.span_id = ld.span_id
		)

		-- 8. Guard: return NULL if the trace doesn't exist
		-- 9. Wrap everything in {traceID, spans: [...]}
		select case
			when not exists (select 1 from spans where trace_id = (select trace_id from param))
			then null
			else cast(json_object(
				'traceID', (select trace_id from param),
				'spans',   coalesce(to_json(list(span_json order by sort_path)), json('[]'))
			) as varchar)
		end as trace
		from ordered_spans
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
		`delete from attributes where span_id is not null`,
		`truncate table links`,
		`truncate table events`,
		`truncate table spans`,
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
		`delete from attributes where span_id in (select span_id from spans where trace_id = ?)`,
		`delete from links where span_id in (select span_id from spans where trace_id = ?)`,
		`delete from events where span_id in (select span_id from spans where trace_id = ?)`,
		`delete from spans where trace_id = ?`,
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
		select cast(to_json(list(json_object('name', sub.key, 'attributeScope', sub.scope, 'type', sub.type::varchar)
			order by sub.key, sub.scope)) as varchar) as attributes
		from (
			select distinct a.key, a.scope, a.type
			from attributes a
			inner join spans s on a.span_id = s.span_id
			where s.start_time >= ? and s.start_time <= ?
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
		`delete from attributes where span_id = ?`,
		`delete from links where span_id = ?`,
		`delete from events where span_id = ?`,
		`delete from spans where span_id = ?`,
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
		fmt.Sprintf(`delete from attributes where span_id in (%s)`, placeholders),
		fmt.Sprintf(`delete from links where span_id in (%s)`, placeholders),
		fmt.Sprintf(`delete from events where span_id in (%s)`, placeholders),
		fmt.Sprintf(`delete from spans where span_id in (%s)`, placeholders),
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
		fmt.Sprintf(`delete from attributes where span_id in (select span_id from spans where trace_id in (%s))`, placeholders),
		fmt.Sprintf(`delete from links where span_id in (select span_id from spans where trace_id in (%s))`, placeholders),
		fmt.Sprintf(`delete from events where span_id in (select span_id from spans where trace_id in (%s))`, placeholders),
		fmt.Sprintf(`delete from spans where trace_id in (%s)`, placeholders),
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

// mapTraceFieldExpression maps a trace field-scope field to a SQL expression (snake_case columns).
func mapTraceFieldExpression(field *FieldDefinition) (string, error) {
	// Resource fields (span table columns only)
	if resourceField, found := strings.CutPrefix(field.Name, "resource."); found {
		col := camelToSnake(resourceField)
		return "s." + col, nil
	}

	// Scope fields (span table columns: scope_name, scope_version, scope_dropped_attributes_count)
	if scopeField, found := strings.CutPrefix(field.Name, "scope."); found {
		col := "scope_" + camelToSnake(scopeField)
		return "s." + col, nil
	}

	// Event column: event.name -> e.name
	if col, found := strings.CutPrefix(field.Name, "event."); found {
		snake := camelToSnake(col)
		return fmt.Sprintf("exists(select 1 from events e where e.span_id = s.span_id and e.%s = ?)", snake), nil
	}

	// Link column: link.traceID -> l.trace_id
	if col, found := strings.CutPrefix(field.Name, "link."); found {
		snake := camelToSnake(col)
		return fmt.Sprintf("exists(select 1 from links l where l.span_id = s.span_id and l.%s = ?)", snake), nil
	}

	// Direct span column (snake_case)
	if len(field.Name) > 0 {
		return "s." + camelToSnake(field.Name), nil
	}
	return field.Name, nil
}

// mapTraceAttributeExpressions maps trace attributes to SQL expressions (snake_case columns).
func mapTraceAttributeExpressions(field *FieldDefinition) ([]string, error) {
	switch field.AttributeScope {
	case "resource", "scope", "span":
		expr := fmt.Sprintf("(select a.value from attributes a where a.span_id = s.span_id and a.scope = '%s' and a.key = '%s' limit 1)", field.AttributeScope, field.Name)
		return []string{expr}, nil
	case "event":
		expr := fmt.Sprintf("exists(select 1 from events e join attributes a on a.event_id = e.id where e.span_id = s.span_id and a.scope = 'event' and a.key = '%s' and a.value = ?)", field.Name)
		return []string{expr}, nil
	case "link":
		expr := fmt.Sprintf("exists(select 1 from links l join attributes a on a.link_id = l.id where l.span_id = s.span_id and a.scope = 'link' and a.key = '%s' and a.value = ?)", field.Name)
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
		"s.search_text = ?",
		"exists(select 1 from events e where e.span_id = s.span_id and e.search_text = ?)",
		"exists(select 1 from links l where l.span_id = s.span_id and l.search_text = ?)",
		"exists(select 1 from attributes a where a.span_id = s.span_id and (a.key = ? or a.value = ?))",
	}, nil
}

// BuildTraceSQL converts a QueryNode into a parameterized CTE, where clause, and args slice
// for trace queries.
func BuildTraceSQL(queryNode *QueryNode, startTime, endTime int64) (cteSQL string, whereSQL string, args []any, err error) {
	return BuildSearchSQL(queryNode, startTime, endTime, traceFieldMapper(), "s.start_time >= time_start and s.start_time <= time_end")
}
