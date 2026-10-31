package nested

import (
	"context"
	"database/sql"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/searchindex"
	"strconv"
)

// Mutate performs owned-row deletion and catalogue cleanup atomically. Children
// in native lists and their catalogue identities disappear in the same commit.
func Mutate(ctx context.Context, db *sql.DB, query string, args ...any) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return err
	}
	if err = CollectMissing(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func CollectMissing(ctx context.Context, exec searchindex.Executor) error {
	for _, q := range []string{
		`delete from search_owners where signal='metric' and (
 (owner='metric' and record_key not in(select record_id::varchar from metric_records)) or
 (owner='datapoint' and record_key not in(select id::varchar from metric_datapoints)) or
 (owner='exemplar' and record_key not in(select id::varchar from exemplars)))`,
		`delete from search_owners where signal='span' and (
 (owner='span' and root_key not in(select replace(trace_id::varchar,'-','')||':'||span_id_wire(span_id) from spans)) or
 (owner='event' and record_key not in(select id::varchar from events)) or
 (owner='link' and record_key not in(select id::varchar from links)))`,
	} {
		if _, err := exec.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return searchindex.CollectGarbage(ctx, exec)
}

func PrunePoints(ctx context.Context, db *sql.DB, cutoff uint64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `create or replace temp table pruned_metric_records as
 select record_id,list_filter(datapoints,lambda p:p.timestamp > $1::ubigint) as remaining
 from metric_records
 where point_time_min<=$1::ubigint`, strconv.FormatUint(cutoff, 10)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `update metric_records r set datapoints=p.remaining,
 point_time_min=list_min(list_transform(p.remaining,lambda d:d.timestamp)),
 point_time_max=list_max(list_transform(p.remaining,lambda d:d.timestamp))
 from pruned_metric_records p where r.record_id=p.record_id`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `delete from metric_records where record_id in(select record_id from pruned_metric_records where len(remaining)=0)`); err != nil {
		return err
	}
	if err = CollectMissing(ctx, tx); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `drop table pruned_metric_records`); err != nil {
		return err
	}
	return tx.Commit()
}
