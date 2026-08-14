-- link_json renders one span link for the wire.
--
-- traceID and spanID go out in OTLP wire form (dash-less lowercase hex) via
-- trace_id_wire / span_id_wire; linked_span_id is the span pointed *at*, as
-- distinct from the owning span_id.
create or replace macro link_json(l, attrs) as (
    json_object(
        'traceID', trace_id_wire(l.trace_id),
        'spanID', span_id_wire(l.linked_span_id),
        'traceState', l.trace_state,
        'droppedAttributesCount', l.dropped_attributes_count,
        'attributes', coalesce(attrs, json('[]'))
    )
)
