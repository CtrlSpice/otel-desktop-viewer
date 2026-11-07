// Package searchindex maintains a derived structured discovery/search catalogue.
// Owned native telemetry remains canonical. No extension, token index, cache or
// reconstruction of telemetry from catalogue rows is involved.
package searchindex

import (
	"context"
	"database/sql"
)

type Executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// CollectGarbage is part of deletion/replacement transactions, never a read
// cache. Context objects survive only while a live record references them.
func CollectGarbage(ctx context.Context, exec Executor) error {
	for _, q := range []string{
		`delete from search_owners where signal='experiment' and owner='experiment' and root_key not in(select id from eval_experiments)`,
		`delete from search_owners c where signal='' and not exists(select 1 from search_owners r where c.ref=r.resource_ref or c.ref=r.scope_ref)`,
		`delete from search_service_resources r where not exists(select 1 from search_owners o where o.owner='resource' and o.record_key=r.resource_id::varchar)`,
		`delete from search_services s where not exists(select 1 from search_service_resources r where r.service_ref=s.ref)`,
		`delete from search_postings where owner_id not in(select id from search_owners)`,
		`delete from search_paths where id not in(select path_id from search_postings)`,
		`delete from search_values where id not in(select value_id from search_postings)`,
	} {
		if _, err := exec.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}
