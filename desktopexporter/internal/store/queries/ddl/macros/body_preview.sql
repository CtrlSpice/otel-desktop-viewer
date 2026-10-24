-- Display text only. Keep numeric tokens as text and never reinterpret a string
-- body as structured JSON. The complete typed body remains available in getLog.
create or replace macro body_preview_item(body) as (
    case json_extract_string(body, '$.kind')
        when 'map' then 'object (' || cast(coalesce(json_array_length(body, '$.value'), 0) as varchar) || ' fields)'
        when 'array' then 'array (' || cast(coalesce(json_array_length(body, '$.value'), 0) as varchar) || ' items)'
        when 'empty' then 'empty'
        when 'bytes' then 'bytes (base64): ' || substring(json_extract_string(body, '$.value'), 1, 48) || case when length(json_extract_string(body, '$.value')) > 48 then '…' else '' end
        when 'string' then cast(json_quote(substring(regexp_replace(json_extract_string(body, '$.value'), '\s+', ' ', 'g'), 1, 48) || case when length(regexp_replace(json_extract_string(body, '$.value'), '\s+', ' ', 'g')) > 48 then '…' else '' end) as varchar)
        else coalesce(json_extract_string(body, '$.value'), 'empty')
    end
);

create or replace macro body_preview(body) as (
    case when body is null then null else
    case json_extract_string(body, '$.kind')
        when 'string' then substring(regexp_replace(json_extract_string(body, '$.value'), '\s+', ' ', 'g'), 1, case when length(regexp_replace(json_extract_string(body, '$.value'), '\s+', ' ', 'g')) > 300 then 299 else 300 end) || case when length(regexp_replace(json_extract_string(body, '$.value'), '\s+', ' ', 'g')) > 300 then '…' else '' end
        when 'map' then '{' || array_to_string(list_transform(list_slice(from_json(json_extract(body, '$.value'), '["JSON"]'), 1, 3), entry ->
            substring(json_extract_string(entry, '$.key'), 1, 32) || case when length(json_extract_string(entry, '$.key')) > 32 then '…' else '' end || ': ' || body_preview_item(json_extract(entry, '$.value'))), ', ')
            || case when json_array_length(body, '$.value') > 3 then ', … (+' || cast(json_array_length(body, '$.value') - 3 as varchar) || ' fields)' else '' end || '}'
        when 'array' then '[' || array_to_string(list_transform(list_slice(from_json(json_extract(body, '$.value'), '["JSON"]'), 1, 3), item -> body_preview_item(item)), ', ')
            || case when json_array_length(body, '$.value') > 3 then ', … (+' || cast(json_array_length(body, '$.value') - 3 as varchar) || ' items)' else '' end || ']'
        else body_preview_item(body)
    end end
)
