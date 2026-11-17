-- Read-time presentation metadata, never received link attributes. A target
-- must match both IDs; missing or invalid targets remain in the result.
-- Omit owner_span_id to resolve the links for a whole trace in one query.
create or replace macro span_link_summaries(owner_trace_id, owner_span_id := null) as table (
    select l.*,
        json_object(
            'target', case when target.span_id is not null then json_object(
                'name', coalesce(target.name, ''),
                'serviceName', target.service_name
            ) else null end
        ) as summary
    from links l
    left join spans target on target.trace_id = l.linked_trace_id and target.span_id = l.linked_span_id
    where l.trace_id = owner_trace_id
        and (owner_span_id is null or l.span_id = owner_span_id)
)
