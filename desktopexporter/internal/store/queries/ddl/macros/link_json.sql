-- link_json renders one span link for the wire.
--
-- traceID and spanID go out in OTLP wire form (dash-less lowercase hex) via
-- trace_id_wire / span_id_wire; linked_trace_id / linked_span_id are the context pointed *at*, as
-- distinct from the owning trace_id / span_id.
create or replace macro link_json(l, attrs) as (
    json_object(
        'traceID', trace_id_wire(l.linked_trace_id),
        'spanID', span_id_wire(l.linked_span_id),
        'traceState', l.trace_state,
        'droppedAttributesCount', l.dropped_attributes_count,
        'flags', l.flags,
        'attributes', coalesce(attrs, json('[]'))
    )
)
