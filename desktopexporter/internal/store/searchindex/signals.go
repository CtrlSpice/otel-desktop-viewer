package searchindex

import (
	"context"
	"database/sql/driver"
)

func RemoveMissingMetricDocuments(ctx context.Context, conn driver.Conn) error {
	exec := connection{conn}
	if _, err := exec.ExecContext(ctx, `delete from search_owners where signal='metric' and (
	 root_key not in(select id::varchar from metrics)
	 or(owner='datapoint' and record_key not in(select id::varchar from metric_datapoints))
	 or(owner='exemplar' and record_key not in(select id::varchar from exemplars)))`); err != nil {
		return err
	}
	return CollectGarbage(ctx, exec)
}
