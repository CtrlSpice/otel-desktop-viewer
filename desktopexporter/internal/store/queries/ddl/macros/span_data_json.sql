-- span_data_json renders one span's payload for the wire.
--
-- Inputs are precomputed at owner, span, or trace granularity; this macro does
-- not fetch. The caller adds tree depth and search-match metadata.
create or replace macro span_data_json(
    ts, attrs, events, links, resource_seq, scope_seq, trace_start_ns
) as (
    json_object(
        -- traceID is stored once at the response root.
        'traceState', ts.trace_state,
        'spanID', span_id_wire(ts.span_id),
        'parentSpanID', case when ts.parent_span_id is not null then span_id_wire(ts.parent_span_id) end,
        'flags', ts.flags,
        'name', ts.name,
		-- kindCode is received int32; kind is a display label.
		'kindCode', ts.kind,
		'kind', case ts.kind
			when 0 then 'Unspecified'
			when 1 then 'Internal'
			when 2 then 'Server'
			when 3 then 'Client'
			when 4 then 'Producer'
			when 5 then 'Consumer'
			else 'Unknown (' || ts.kind::varchar || ')'
		end,
        -- Nanosecond offset from traceStart and duration from the span start.
        'start', (ts.start_time::hugeint - trace_start_ns::hugeint)::varchar,
        'dur', (ts.end_time::hugeint - ts.start_time::hugeint)::varchar,
        'attributes', coalesce(attrs, json('[]')),
        'events', coalesce(events, json('[]')),
        'links', coalesce(links, json('[]')),
        -- Sequence references index the top-level resource and scope maps.
        'r', resource_seq,
        's', scope_seq,
        'droppedAttributesCount', ts.dropped_attributes_count,
        'droppedEventsCount', ts.dropped_events_count,
        'droppedLinksCount', ts.dropped_links_count,
		-- statusCodeValue is received int32; statusCode is a display label.
		'statusCodeValue', ts.status_code,
		'statusCode', case ts.status_code
			when 0 then 'Unset'
			when 1 then 'Ok'
			when 2 then 'Error'
			else 'Unknown (' || ts.status_code::varchar || ')'
		end,
        'statusMessage', ts.status_message
    )
)
