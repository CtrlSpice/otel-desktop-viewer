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
// JSON object built entirely by DuckDB. sizeBytes and maxSizeBytes describe
// current storage usage and the retention cap (0 = retention disabled); they
// are measured by the caller because size lives outside the SQL schema
// (file stat or duckdb_memory, depending on mode).
func GetStats(ctx context.Context, db *sql.DB, sizeBytes int64, maxSizeBytes int64) (json.RawMessage, error) {
	query := `
		select cast(json_object(
			'storage', json_object(
				'sizeBytes',    ?::bigint,
				'maxSizeBytes', ?::bigint
			),
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
				'lastReceived', cast(max(start_time) as varchar)
			) from spans),
			'logs', (select json_object(
				'logCount',     count(*),
				'errorCount',   count(*) filter (where severity_number >= 17),
				'lastReceived', cast(coalesce(max(nullif(timestamp, 0)), max(observed_timestamp)) as varchar)
			) from logs),
			'metrics', (select json_object(
				-- metricCount is the number of distinct logical streams
				-- (one per name+unit+type+temporality+monotonic+scope+
				-- service tuple), so the frontend's "metrics" badge
				-- shows logical concepts rather than ingest batches.
				-- metric_ingests is the per-batch table; using its row
				-- count would inflate by the number of OTLP requests.
				'metricCount',    (select count(*) from metric_streams),
				'dataPointCount', count(*),
				-- lastReceived = latest datapoint timestamp observed
				-- (source recency), not collector wall-clock arrival.
				-- Mirrors traces/logs which also use source timestamps.
				'lastReceived',   cast(max(timestamp) as varchar)
			) from datapoints)
		) as varchar) as stats
	`

	var raw []byte
	if err := db.QueryRowContext(ctx, query, sizeBytes, maxSizeBytes).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetStats: %w: %w", ErrStatsInternal, err)
	}
	if raw == nil {
		return json.RawMessage("{}"), nil
	}
	return json.RawMessage(raw), nil
}
