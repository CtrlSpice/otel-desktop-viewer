package searchindex

import "context"

// Accepted native telemetry owners are new, and Resource/Scope objects are
// immutable content identities. RETURNING supplies only newly inserted owners,
// so an existing shared context does not acquire duplicate entry links. All
// inserts belong to the enclosing native mutation transaction.
// Stock DuckDB 2 exposes a completed modifying CTE's rows to dependent scans.
// The scalar completion dependencies order dictionary inserts before ID lookup;
// no assumption is made about the order of independent CTEs.
func writeNewOwners(ctx context.Context, exec Executor, owners [][]string, paths, values map[string][]string, refs, pathRefs, valueRefs []string) error {
	args := make([]any, 0, 23)
	for _, column := range owners {
		args = append(args, column)
	}
	columns := make([][]string, 8)
	for identity, row := range paths {
		columns[0] = append(columns[0], identity)
		for i, value := range row {
			columns[i+1] = append(columns[i+1], value)
		}
	}
	for _, column := range columns {
		args = append(args, column)
	}
	columns = make([][]string, 3)
	for identity, row := range values {
		columns[0] = append(columns[0], identity)
		columns[1] = append(columns[1], row[0])
		columns[2] = append(columns[2], row[1])
	}
	for _, column := range columns {
		args = append(args, column)
	}
	args = append(args, refs, pathRefs, valueRefs)
	_, err := exec.ExecContext(ctx, `with
 incoming_owners as materialized(select unnest(?::varchar[]) ref,unnest(?::varchar[]) signal,unnest(?::varchar[]) owner,
 unnest(?::varchar[]) root_key,unnest(?::varchar[]) record_key,unnest(?::varchar[]) parent_ref,
 unnest(?::varchar[]) resource_ref,unnest(?::varchar[]) scope_ref,unnest(?::varchar[]) fingerprint),
 added_owners as materialized(insert into search_owners(ref,signal,owner,root_key,record_key,parent_ref,resource_ref,scope_ref,fingerprint)
 select ref,signal,owner,root_key,record_key,nullif(parent_ref,''),nullif(resource_ref,''),nullif(scope_ref,''),fingerprint from incoming_owners
 on conflict(ref) do nothing returning id,ref),
 incoming_paths as materialized(select unnest(?::varchar[]) identity,unnest(?::varchar[]) owner,unnest(?::varchar[])::json path,
 unnest(?::varchar[]) kind,unnest(?::varchar[]) key,unnest(?::varchar[]) key_text,unnest(?::varchar[]) attribute_scope,unnest(?::varchar[])::boolean is_attribute),
 added_paths as materialized(insert into search_paths(identity,owner,path,kind,key,key_text,attribute_scope,is_attribute)
 select i.* from incoming_paths i where not exists(select 1 from search_paths p where p.identity=i.identity) returning id,identity),
 path_ids as materialized(select p.id,p.identity from search_paths p join incoming_paths i using(identity)
 cross join(select count(*) added from added_paths) completed where completed.added>=0),
 incoming_values as materialized(select unnest(?::varchar[]) identity,unnest(?::varchar[]) kind,unnest(?::varchar[]) value),
 added_values as materialized(insert into search_values(identity,kind,value)
 select i.* from incoming_values i where not exists(select 1 from search_values v where v.identity=i.identity) returning id,identity),
 value_ids as materialized(select v.id,v.identity from search_values v join incoming_values i using(identity)
 cross join(select count(*) added from added_values) completed where completed.added>=0)
 insert into search_postings select distinct o.id,p.id,v.id
 from(select unnest(?::varchar[]) ref,unnest(?::varchar[]) path,unnest(?::varchar[]) value) i
 join added_owners o using(ref) join path_ids p on p.identity=i.path join value_ids v on v.identity=i.value`, args...)
	return err
}
