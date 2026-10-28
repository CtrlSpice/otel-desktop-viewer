select coalesce(to_json(list(json_object(
    'name', key, 'attributeScope', attribute_scope, 'owner', owner,
    'path', path, 'type', kind) order by key, attribute_scope, kind)),json('[]'))::varchar
from (
    select distinct p.key,p.attribute_scope,p.owner,p.path,p.kind
    from search_paths p join search_postings l on l.path_id=p.id join search_owners o on o.id=l.owner_id
    where p.is_attribute and (o.signal='metric' or (o.signal='' and exists(
        select 1 from search_owners r where r.signal='metric' and (r.resource_ref=o.ref or r.scope_ref=o.ref))))
) definitions
