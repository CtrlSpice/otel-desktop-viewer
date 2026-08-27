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
				-- Counted off the denormalized column rather than resolved
				-- through resource_id: same value by construction (both are
				-- written from the same service.name attribute at ingest), and
				-- this is a single column scan instead of a join plus an array
				-- unnest. The empty string is the "no service.name" marker, so
				-- it is excluded rather than counted as a service.
				'serviceCount', (select count(distinct service_name) from spans where service_name <> ''),
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
			) from datapoints),
			-- Telemetry the store would not write. Empty in the ordinary case,
			-- so the home page shows the section only when there is something
			-- to say. Ordered by recency.
			'rejections', coalesce((select to_json(list(json_object(
				'signal',      signal,
				'kind',        kind,
				'occurrences', occurrences,
				'samples',     samples,
				'firstSeen',   cast(first_seen as varchar),
				'lastSeen',    cast(last_seen as varchar)
			) order by last_seen desc)) from ingest_rejections), json('[]'))
		) as varchar) as stats
	`

	var raw []byte
	if err := db.QueryRowContext(ctx, query, sizeBytes, maxSizeBytes).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetStats: %w: %w", ErrStatsInternal, err)
	}
	// The projection is a json_object of scalar subqueries over aggregates,
	// which always yields one non-null row -- a nil scan means the query
	// itself misbehaved. Surfacing "{}" here would violate the wire contract
	// (frontend wire-types.ts declares all three signal blocks required).
	if raw == nil {
		return nil, fmt.Errorf("GetStats: %w: query returned no row", ErrStatsInternal)
	}
	return json.RawMessage(raw), nil
}
