package stats

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrStatsInternal = errors.New("stats store internal error")

// GetTraceSpanCount returns the total number of spans for a given trace.
func GetTraceSpanCount(ctx context.Context, db *sql.DB, traceID string) (int64, error) {
	var count int64
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM spans WHERE trace_id = ?`, traceID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("GetTraceSpanCount: %w: %w", ErrStatsInternal, err)
	}
	return count, nil
}

// GetStats returns aggregate counts across all telemetry signals as a single
// JSON object built entirely by DuckDB.
func GetStats(ctx context.Context, db *sql.DB) (json.RawMessage, error) {
	query := `
		select cast(json_object(
			'traces', (select json_object(
				'traceCount',   count(distinct trace_id),
				'spanCount',    count(*),
				'serviceCount', (
					select count(distinct a.value)
					from attributes a
					inner join spans s2 on a.span_id = s2.span_id
					where a.scope = 'resource' and a.key = 'service.name'
						and a.event_id is null and a.link_id is null
				),
				'errorCount',   count(*) filter (where status_code = 'Error'),
				'lastReceived', max(start_time)
			) from spans),
			'logs', (select json_object(
				'logCount',     count(*),
				'errorCount',   count(*) filter (where severity_number >= 17),
				'lastReceived', coalesce(max(nullif(timestamp, 0)), max(observed_timestamp))
			) from logs),
			'metrics', (select json_object(
				'metricCount',    (select count(*) from metrics),
				'dataPointCount', count(*),
				'lastReceived',   (select max(received) from metrics)
			) from datapoints)
		) as varchar) as stats
	`

	var raw []byte
	if err := db.QueryRowContext(ctx, query).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetStats: %w: %w", ErrStatsInternal, err)
	}
	if raw == nil {
		return json.RawMessage("{}"), nil
	}
	return json.RawMessage(raw), nil
}
