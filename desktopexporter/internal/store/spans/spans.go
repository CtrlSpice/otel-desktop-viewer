package spans

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
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Sentinel errors for use with errors.Is.
var (
	ErrTraceIDNotFound    = errors.New("trace ID not found")
	ErrSpanIDNotFound     = errors.New("span ID not found")
	ErrInvalidSpanID      = errors.New("invalid span ID")
	ErrInvalidTraceQuery  = errors.New("invalid trace search query")
	ErrSpansStoreInternal = errors.New("spans store internal error")
)

const flushIntervalSpans = 50

// Ingest ingests trace spans from pdata into the spans, events, links, and attributes tables.
// The caller must hold any required lock on the connection.
func Ingest(ctx context.Context, conn driver.Conn, traces ptrace.Traces) error {
	tables := []string{"attributes", "events", "links", "spans"}
	appenders, err := ingest.NewAppenders(conn, tables)
	if err != nil {
		return err
	}
	defer ingest.CloseAppenders(appenders, tables)

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
					return fmt.Errorf("Ingest: %w: %w", ErrSpansStoreInternal, err)
				}

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
						return fmt.Errorf("Ingest: %w: %w", ErrSpansStoreInternal, err)
					}
					if err := ingest.IngestAttributes(appenders["attributes"],
						[]ingest.AttributeBatchItem{{Attrs: event.Attributes(), IDs: ingest.AttributeOwnerIDs{SpanID: &spanUUID, EventID: &eventID}, Scope: "event"}}); err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrSpansStoreInternal, err)
					}
				}

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
						return fmt.Errorf("Ingest: %w: %w", ErrSpansStoreInternal, err)
					}
					if err := ingest.IngestAttributes(appenders["attributes"], []ingest.AttributeBatchItem{{Attrs: link.Attributes(), IDs: ingest.AttributeOwnerIDs{SpanID: &spanUUID, LinkID: &linkID}, Scope: "link"}}); err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrSpansStoreInternal, err)
					}
				}

				spanIDs := ingest.AttributeOwnerIDs{SpanID: &spanUUID}
				if err := ingest.IngestAttributes(appenders["attributes"], []ingest.AttributeBatchItem{
					{Attrs: span.Attributes(), IDs: spanIDs, Scope: "span"},
					{Attrs: resource.Attributes(), IDs: spanIDs, Scope: "resource"},
					{Attrs: scope.Attributes(), IDs: spanIDs, Scope: "scope"},
				}); err != nil {
					return fmt.Errorf("Ingest: %w: %w", ErrSpansStoreInternal, err)
				}

				spanCount++
				if spanCount%flushIntervalSpans == 0 {
					if err := ingest.FlushAppenders(appenders, tables); err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrSpansStoreInternal, err)
					}
				}
			}
		}
	}

	return nil
}

// SearchTraces returns trace summaries in the time range matching the optional criteria.
// A separate SearchTraceSpans (span-level results for a trace) may be added later.
func SearchTraces(ctx context.Context, db *sql.DB, startTime, endTime int64, criteria any) (json.RawMessage, error) {
	var searchTree *search.QueryNode
	if criteria != nil {
		var err error
		searchTree, err = search.ParseQueryTree(criteria)
		if err != nil {
			return nil, fmt.Errorf("SearchTraces: %w: %w", ErrInvalidTraceQuery, err)
		}
	}

	cteSQL, whereClause, args, err := buildTraceSQL(searchTree, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("SearchTraces: %w: %w", ErrInvalidTraceQuery, err)
	}

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
				case when s.parent_span_id is null then 0 else 1 end
		) sub`, cteSQL, whereClause)

	var raw []byte
	if err := db.QueryRowContext(ctx, finalQuery, args...).Scan(&raw); err != nil {
		return nil, fmt.Errorf("SearchTraces: %w: %w", ErrSpansStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// GetTrace returns a single trace by ID as JSON, or ErrTraceIDNotFound if not found.
// traceID is passed as a string; DuckDB casts it to UUID (accepts both hyphenated and 32-char hex).
func GetTrace(ctx context.Context, db *sql.DB, traceID string) (json.RawMessage, error) {
	query := `
		with recursive
		param(trace_id) as (values (?)),

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

		span_attributes as (
			select a.span_id, a.scope,
				json_group_array(json_object('key', a.key, 'value', a.value, 'type', a.type::varchar)) as attributes
			from attributes a
			where a.span_id in (select span_id from spans_tree)
				and a.event_id is null and a.link_id is null
			group by a.span_id, a.scope
		),

		event_attributes as (
			select a.event_id,
				json_group_array(json_object('key', a.key, 'value', a.value, 'type', a.type::varchar)) as attributes
			from attributes a
			where a.event_id is not null
				and a.span_id in (select span_id from spans_tree)
			group by a.event_id
		),

		event_data as (
			select e.span_id,
				to_json(list(json_object(
					'name', e.name,
					'timestamp', e.timestamp,
					'droppedAttributesCount', e.dropped_attributes_count,
					'attributes', coalesce(ea.attributes, json('[]'))
				) order by e.timestamp)) as events
			from events e
			left join event_attributes ea on e.id = ea.event_id
			where e.span_id in (select span_id from spans_tree)
			group by e.span_id
		),

		link_attributes as (
			select a.link_id,
				json_group_array(json_object('key', a.key, 'value', a.value, 'type', a.type::varchar)) as attributes
			from attributes a
			where a.link_id is not null
				and a.span_id in (select span_id from spans_tree)
			group by a.link_id
		),

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
			left join link_attributes la on l.id = la.link_id
			where l.span_id in (select span_id from spans_tree)
			group by l.span_id
		),

		ordered_spans as (
			select json_object(
					'spanData', json_object(
						'traceID', st.trace_id,
						'traceState', st.trace_state,
						'spanID', st.span_id,
						'parentSpanID', st.parent_span_id,
						'name', st.name,
						'kind', st.kind,
						'startTime', st.start_time,
						'endTime', st.end_time,
						'attributes', coalesce(sa_span.attributes, json('[]')),
						'events', coalesce(ed.events, json('[]')),
						'links', coalesce(ld.links, json('[]')),
						'resource', json_object(
							'attributes', coalesce(sa_res.attributes, json('[]')),
							'droppedAttributesCount', st.resource_dropped_attributes_count
						),
						'scope', json_object(
							'name', st.scope_name,
							'version', st.scope_version,
							'attributes', coalesce(sa_scope.attributes, json('[]')),
							'droppedAttributesCount', st.scope_dropped_attributes_count
						),
						'droppedAttributesCount', st.dropped_attributes_count,
						'droppedEventsCount', st.dropped_events_count,
						'droppedLinksCount', st.dropped_links_count,
						'statusCode', st.status_code,
						'statusMessage', st.status_message
					),
					'depth', st.depth
				) as span_json,
				st.sort_path
			from spans_tree st
			left join span_attributes sa_span on st.span_id = sa_span.span_id and sa_span.scope = 'span'
			left join span_attributes sa_res on st.span_id = sa_res.span_id and sa_res.scope = 'resource'
			left join span_attributes sa_scope on st.span_id = sa_scope.span_id and sa_scope.scope = 'scope'
			left join event_data ed on st.span_id = ed.span_id
			left join link_data ld on st.span_id = ld.span_id
		)

		select case
			when not exists (select 1 from spans where trace_id = (select trace_id from param))
				then null
			else cast(json_object(
				'traceID', (select trace_id from param),
				'spans', coalesce(to_json(list(span_json order by sort_path)), json('[]'))
			) as varchar)
		end as trace
		from ordered_spans
	`
	var raw []byte
	if err := db.QueryRowContext(ctx, query, traceID).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetTrace: %w: %w", ErrSpansStoreInternal, err)
	}
	if raw == nil {
		return nil, fmt.Errorf("GetTrace: %w", ErrTraceIDNotFound)
	}
	return json.RawMessage(raw), nil
}

// GetTraceAttributes returns a JSON array of attribute names/scopes/types for spans in the time range.
func GetTraceAttributes(ctx context.Context, db *sql.DB, startTime, endTime int64) (json.RawMessage, error) {
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
	if err := db.QueryRowContext(ctx, query, startTime, endTime).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetTraceAttributes: %w: %w", ErrSpansStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// Clear truncates the spans table and all child tables (events, links, and their attributes).
func Clear(ctx context.Context, db *sql.DB) error {
	childQueries := []string{
		`delete from attributes where span_id is not null`,
		`truncate table links`,
		`truncate table events`,
		`truncate table spans`,
	}
	for _, q := range childQueries {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("Clear: %w: %w", ErrSpansStoreInternal, err)
		}
	}
	return nil
}

// DeleteSpansByTraceID deletes all spans for a specific trace.
func DeleteSpansByTraceID(ctx context.Context, db *sql.DB, traceID string) error {
	childQueries := []string{
		`delete from attributes where span_id in (select span_id from spans where trace_id = ?)`,
		`delete from links where span_id in (select span_id from spans where trace_id = ?)`,
		`delete from events where span_id in (select span_id from spans where trace_id = ?)`,
		`delete from spans where trace_id = ?`,
	}
	for _, q := range childQueries {
		if _, err := db.ExecContext(ctx, q, traceID); err != nil {
			return fmt.Errorf("DeleteSpansByTraceID: %w: %w", ErrSpansStoreInternal, err)
		}
	}
	return nil
}

// DeleteSpanByID deletes a specific span by its ID.
func DeleteSpanByID(ctx context.Context, db *sql.DB, spanID string) error {
	childQueries := []string{
		`delete from attributes where span_id = ?`,
		`delete from links where span_id = ?`,
		`delete from events where span_id = ?`,
		`delete from spans where span_id = ?`,
	}
	for _, q := range childQueries {
		if _, err := db.ExecContext(ctx, q, spanID); err != nil {
			return fmt.Errorf("DeleteSpanByID: %w: %w", ErrSpansStoreInternal, err)
		}
	}
	return nil
}

// DeleteSpansByIDs deletes multiple spans by their IDs.
func DeleteSpansByIDs(ctx context.Context, db *sql.DB, spanIDs []any) error {
	if len(spanIDs) == 0 {
		return nil
	}
	placeholders := util.BuildPlaceholders(len(spanIDs))
	childQueries := []string{
		fmt.Sprintf(`delete from attributes where span_id in (%s)`, placeholders),
		fmt.Sprintf(`delete from links where span_id in (%s)`, placeholders),
		fmt.Sprintf(`delete from events where span_id in (%s)`, placeholders),
		fmt.Sprintf(`delete from spans where span_id in (%s)`, placeholders),
	}
	for _, q := range childQueries {
		if _, err := db.ExecContext(ctx, q, spanIDs...); err != nil {
			return fmt.Errorf("DeleteSpansByIDs: %w: %w", ErrSpansStoreInternal, err)
		}
	}
	return nil
}

// DeleteSpansByTraceIDs deletes all spans for multiple traces.
func DeleteSpansByTraceIDs(ctx context.Context, db *sql.DB, traceIDs []any) error {
	if len(traceIDs) == 0 {
		return nil
	}
	placeholders := util.BuildPlaceholders(len(traceIDs))
	childQueries := []string{
		fmt.Sprintf(`delete from attributes where span_id in (select span_id from spans where trace_id in (%s))`, placeholders),
		fmt.Sprintf(`delete from links where span_id in (select span_id from spans where trace_id in (%s))`, placeholders),
		fmt.Sprintf(`delete from events where span_id in (select span_id from spans where trace_id in (%s))`, placeholders),
		fmt.Sprintf(`delete from spans where trace_id in (%s)`, placeholders),
	}
	for _, q := range childQueries {
		if _, err := db.ExecContext(ctx, q, traceIDs...); err != nil {
			return fmt.Errorf("DeleteSpansByTraceIDs: %w: %w", ErrSpansStoreInternal, err)
		}
	}
	return nil
}

func buildTraceSQL(queryNode *search.QueryNode, startTime, endTime int64) (cteSQL string, whereSQL string, args []any, err error) {
	return search.BuildSearchSQL(queryNode, startTime, endTime, traceFieldMapper(), "s.start_time >= time_start and s.start_time <= time_end")
}

func traceFieldMapper() search.FieldMapper {
	return func(field *search.FieldDefinition) ([]string, error) {
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
			return nil, fmt.Errorf("unknown search scope %s: %w", field.SearchScope, ErrInvalidTraceQuery)
		}
	}
}

func mapTraceFieldExpression(field *search.FieldDefinition) (string, error) {
	if resourceField, found := strings.CutPrefix(field.Name, "resource."); found {
		return "s." + util.CamelToSnake(resourceField), nil
	}
	if scopeField, found := strings.CutPrefix(field.Name, "scope."); found {
		return "s.scope_" + util.CamelToSnake(scopeField), nil
	}
	if col, found := strings.CutPrefix(field.Name, "event."); found {
		snake := util.CamelToSnake(col)
		return fmt.Sprintf("exists(select 1 from events e where e.span_id = s.span_id and e.%s = ?)", snake), nil
	}
	if col, found := strings.CutPrefix(field.Name, "link."); found {
		snake := util.CamelToSnake(col)
		return fmt.Sprintf("exists(select 1 from links l where l.span_id = s.span_id and l.%s = ?)", snake), nil
	}
	if len(field.Name) > 0 {
		return "s." + util.CamelToSnake(field.Name), nil
	}
	return field.Name, nil
}

func mapTraceAttributeExpressions(field *search.FieldDefinition) ([]string, error) {
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
		return nil, fmt.Errorf("unknown attribute scope %s: %w", field.AttributeScope, ErrInvalidTraceQuery)
	}
}

func mapTraceGlobalExpressions() ([]string, error) {
	return []string{
		"s.search_text = ?",
		"exists(select 1 from events e where e.span_id = s.span_id and e.search_text = ?)",
		"exists(select 1 from links l where l.span_id = s.span_id and l.search_text = ?)",
		"exists(select 1 from attributes a where a.span_id = s.span_id and (a.key = ? or a.value = ?))",
	}, nil
}

// normalizeSpanID converts a 16-char OTel span ID to 32-char zero-padded hex
// so it matches the UUID format used during ingest (8 zero bytes + 8 span ID bytes).
// If the input is already 32 chars (with or without hyphens), it's returned as-is after stripping hyphens.
var _ = normalizeSpanID // keep for upcoming span-lookup queries

func normalizeSpanID(s string) (string, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "-", "")
	switch len(s) {
	case 16:
		return "0000000000000000" + s, nil
	case 32:
		return s, nil
	default:
		return "", fmt.Errorf("%w: invalid length %d (expected 16 or 32 hex chars)", ErrInvalidSpanID, len(s))
	}
}
