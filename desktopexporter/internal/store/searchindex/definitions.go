package searchindex

import (
	"context"
	"database/sql"
	"encoding/json"
)

func definitionRows(signal, traceID string) (string, []any, error) {
	owners := discoveryOwners("$1")
	order := discoveryOrder
	if signal != "span" {
		order = "key,attribute_scope,kind,path"
	}
	if traceID != "" {
		// Child owners use the enclosing span's root key. Shared resource/scope
		// owners are included only through references from this trace.
		owners = `with trace_owners as materialized (
 select * from search_owners where signal=$1 and try_cast(split_part(root_key,':',1) as uuid)=$6::uuid
), discovery_context_refs as (
 select distinct unnest([resource_ref,scope_ref]) ref from trace_owners
), discovery_owners as materialized (
 select id,record_key from trace_owners union
 select o.id,o.record_key from search_owners o join discovery_context_refs r using(ref) where o.signal=''
) `
		order = "key,owner,kind,path"
	}
	q, args, err := discoverySQL(DiscoveryOptions{Signal: signal}, true, owners)
	if err != nil {
		return "", nil, err
	}
	if traceID != "" {
		args = append(args, traceID)
	}
	return `select *,row_number() over(order by ` + order + `) as ordinal from (` + q + `) definitions`, args, nil
}

func DefinitionsJSON(ctx context.Context, db *sql.DB, signal, traceID string) (json.RawMessage, error) {
	q, args, err := definitionRows(signal, traceID)
	if err != nil {
		return nil, err
	}
	var raw []byte
	err = db.QueryRowContext(ctx, `select coalesce(to_json(list(json_object(
 'name',key,'attributeScope',attribute_scope,'owner',owner,'path',path::JSON,'type',kind)
 order by ordinal)),json('[]'))::varchar from (`+q+`) definitions`, args...).Scan(&raw)
	return json.RawMessage(raw), err
}

func DefinitionsArrowSQL(signal, traceID string) (string, []any, error) {
	q, args, err := definitionRows(signal, traceID)
	if err != nil {
		return "", nil, err
	}
	return `select key as name,attribute_scope as attributeScope,owner,` + nativePath + ` as path,kind as type from (` + q + `) definitions order by ordinal`, args, nil
}
