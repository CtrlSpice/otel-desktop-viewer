-- span_data_json renders one span's payload for the wire.
--
-- Everything it needs is passed in, because every argument has already been
-- computed once at the right granularity: attributes resolved in one pass per
-- owner kind, events and links grouped per span, resource and scope as short
-- seq keys from maps built per distinct owner, and the trace baseline as a
-- single scalar. The macro shapes; it does not fetch.
--
-- depth and matched stay in the caller: one is tree metadata and the other is
-- the search annotation, and both belong to the envelope around this object
-- rather than to the span itself.
create or replace macro span_data_json(
    ts, attrs, events, links, resource_seq, scope_seq, trace_start_ns
) as (
    json_object(
        -- No traceID: it is at the response root, and a single-trace response
        -- repeated it 32 bytes per span.
        'traceState', ts.trace_state,
        'spanID', span_id_wire(ts.span_id),
        'parentSpanID', case when ts.parent_span_id is not null then span_id_wire(ts.parent_span_id) end,
        'flags', ts.flags,
        'name', ts.name,
		-- kindCode is the authoritative received int32. kind is derived for
		-- display and preserves the UI's established labels.
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
        -- Offset from traceStart, and duration from the span's own start.
        -- Deliberately not two offsets: an end offset inherits the trace's full
        -- magnitude however brief the span, while a duration stays small. It is
        -- also what the waterfall wants -- a bar is a position and a width, so
        -- the client stops subtracting on every render.
        'start', (ts.start_time::hugeint - trace_start_ns::hugeint)::varchar,
        'dur', (ts.end_time::hugeint - ts.start_time::hugeint)::varchar,
        'attributes', coalesce(attrs, json('[]')),
        'events', coalesce(events, json('[]')),
        'links', coalesce(links, json('[]')),
        -- References into the top-level maps. seq rather than the uuid: two
        -- 36-char ids per span is ~413KB on the reference trace, a small
        -- integer ~57KB. The uuids are storage identity and have no business
        -- on the wire.
        'r', resource_seq,
        's', scope_seq,
        'droppedAttributesCount', ts.dropped_attributes_count,
        'droppedEventsCount', ts.dropped_events_count,
        'droppedLinksCount', ts.dropped_links_count,
		-- statusCodeValue is the authoritative received int32. statusCode is
		-- the separately derived label retained for existing UI consumers.
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
