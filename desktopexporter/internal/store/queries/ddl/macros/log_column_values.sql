-- Project one selected field/attribute. Empty lists mean absent, not an empty
-- received value. Attribute occurrences retain their owner-array order and types.
-- Timestamps use decimal strings (uint64), IDs use the shared wire encoders.
create or replace macro log_column_values(l, r, sc, owner, kind, key) as (
    case when kind = 'attribute' then coalesce((
        select to_json(list(a.value order by t.ordinality))
        from unnest(case owner
            when 'log' then l.attribute_ids
            when 'resource' then r.attribute_ids
            when 'scope' then sc.attribute_ids
        end) with ordinality t(aid, ordinality)
        join attributes a on a.id = t.aid
        where a.key = key
    ), '[]'::json)
    else (
        select case when v is null then '[]'::json else json_array(v) end
        from (select case owner
            when 'log' then case key
                when 'timeUnixNano' then json_object('kind', 'string', 'value', l.timestamp::varchar)
                when 'observedTimeUnixNano' then json_object('kind', 'string', 'value', l.observed_timestamp::varchar)
                when 'severityText' then json_object('kind', 'string', 'value', l.severity_text)
                when 'severityNumber' then json_object('kind', 'int64', 'value', l.severity_number::varchar)
                when 'traceId' then case when l.trace_id is not null then json_object('kind', 'string', 'value', trace_id_wire(l.trace_id)) end
                when 'spanId' then case when l.span_id is not null then json_object('kind', 'string', 'value', span_id_wire(l.span_id)) end
                when 'eventName' then json_object('kind', 'string', 'value', l.event_name)
                when 'body' then l.body
                when 'flags' then json_object('kind', 'int64', 'value', l.flags::varchar)
                when 'droppedAttributesCount' then json_object('kind', 'int64', 'value', l.dropped_attributes_count::varchar)
            end
            when 'resource' then case key
                when 'schemaUrl' then json_object('kind', 'string', 'value', r.schema_url)
                when 'droppedAttributesCount' then json_object('kind', 'int64', 'value', r.dropped_attributes_count::varchar)
            end
            when 'scope' then case key
                when 'name' then json_object('kind', 'string', 'value', sc.name)
                when 'version' then json_object('kind', 'string', 'value', sc.version)
                when 'schemaUrl' then json_object('kind', 'string', 'value', sc.schema_url)
                when 'droppedAttributesCount' then json_object('kind', 'int64', 'value', sc.dropped_attributes_count::varchar)
            end
        end as v)
    ) end
)
