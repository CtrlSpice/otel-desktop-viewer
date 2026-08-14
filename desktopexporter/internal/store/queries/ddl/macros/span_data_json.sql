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
        'name', ts.name,
        'kind', ts.kind,
        -- Offset from traceStart, and duration from the span's own start.
        -- Deliberately not two offsets: an end offset inherits the trace's full
        -- magnitude however brief the span, while a duration stays small. It is
        -- also what the waterfall wants -- a bar is a position and a width, so
        -- the client stops subtracting on every render.
        'start', ts.start_time - trace_start_ns,
        'dur', ts.end_time - ts.start_time,
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
        'statusCode', ts.status_code,
        'statusMessage', ts.status_message
    )
)
