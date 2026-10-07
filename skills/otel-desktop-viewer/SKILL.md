---
name: otel-desktop-viewer
description: "Use when a user asks to inspect traces, spans, logs, Metrics, or stored OpenTelemetry values already received by OTel Desktop Viewer through its read-only CLI."
---

# Inspect OTel Desktop Viewer telemetry

## Connect to one viewer

Check that the installed build supports the required read-only command:

```sh
otel-desktop-viewer --help
```

Continue only if listed; otherwise report the missing command and direct the user
to the web UI.

Use one viewer throughout the task and follow-ups:

1. Probe the configured endpoint with `otel-desktop-viewer query 'SHOW TABLES'`;
   the default is `http://localhost:8000`.
2. On success, reuse that user-owned viewer for commands.
3. Only when the default-endpoint probe fails, start at most one foreground
   `otel-desktop-viewer --open-browser=false` child and retain its process handle.
   For a failed non-default endpoint, report unavailability and do not start a
   viewer at the default.
4. Before queries, require child liveness and the same successful probe. If it
   exits, report its error output and stop; never replace it.
5. Keep it for all follow-ups. Afterwards, terminate and wait for only that
   owned child.

Never stop or restart an existing viewer. The caller retains the child handle;
the viewer does not return one.

## Choose the narrowest command

Choose the narrowest command for the question:

| Question | Command |
| --- | --- |
| Which traces arrived? | `traces` |
| What spans and trace-linked logs belong to one trace? | `trace <trace-id>` |
| What exact data belongs to one span? | `span <span-id>` or `span <trace-id> <span-id>` |
| Which logs arrived? | `logs` |
| Which Metrics arrived? | `metrics` |
| Which attribute keys and kinds occur? | `attributes keys` |
| What are one key's common typed values and frequencies? | `attributes values <key>` |
| What custom aggregation or stored field is needed? | `query <sql>` |

Use `--json` for machine consumption or when exact values, JSON nulls, objects,
or column types matter; use table output for human inspection.

## Search traces, logs, and Metrics

Summary commands default to the last hour and at most 25 rows:

```sh
otel-desktop-viewer traces --service checkout --since 30m
otel-desktop-viewer logs --service checkout --since 30m
otel-desktop-viewer metrics --service checkout --since 30m --json
```

All accept `--endpoint`, `--service`, `--since`, `--start`, `--end`, `--limit`,
and `--json`. `--start` and `--end` are inclusive RFC3339 timestamps accepting
nanoseconds. An explicit `--since` is incompatible with `--start` or `--end`.

`traces` returns compact summaries. With `--service`, the count covers matching
spans, although the trace can contain other services.

`logs` returns compact summaries. `logRef` is database-local, not a received OTel
field.

`metrics --json` includes `metricRef`, the exact UUID text from `metrics.id`.
This viewer-generated reference is valid only in that database; OTLP does not
provide it. Pass it unchanged without parsing. The Metric table omits it.

## Inspect one trace or span

Use a 32-character hexadecimal trace ID from `traces`; a dashed UUID also works:

```sh
otel-desktop-viewer trace 4bf92f3577b34da6a3ce929d0e0e4736
otel-desktop-viewer trace 4bf92f35-77b3-4da6-a3ce-929d0e0e4736 --json
```

`trace` returns every compact stored span and log carrying that trace ID. Trace
start is the minimum received span start; duration is the maximum received span
end minus it. Span offsets and durations derive from received nanoseconds as
exact decimal strings. A log uses received timestamp, or received observed
timestamp when timestamp is zero. Displayed severity uses received text, or a
label from the received enum. The body is a preview; use `span` or `query` for
complete stored log data.

A span ID can occur in multiple traces. The command never chooses: it returns
one exact span, not found, or bounded summaries of every match.

```sh
otel-desktop-viewer span 000000000000002a
otel-desktop-viewer span 4bf92f3577b34da6a3ce929d0e0e4736 000000000000002a --json
```

After ambiguity, use the qualified form. It returns the full span; typed
resource, scope, span, event, and link attributes; and logs with both IDs.
Those logs are exact span correlations. Use `trace` or `query` for trace-only
logs. `--limit` affects only ambiguous standalone-ID summaries.

## Run custom read-only SQL

`query` runs one read-only DuckDB statement against the viewer, returning at
most 25 rows in aligned columns by default. Use `--endpoint` for another address,
`--limit` for another limit, or `--json` for the result object.

## Inspect the installed schema

```sh
otel-desktop-viewer query --help
otel-desktop-viewer query 'SHOW TABLES'
otel-desktop-viewer query 'DESCRIBE spans'
otel-desktop-viewer query 'DESCRIBE logs'
otel-desktop-viewer query 'DESCRIBE metrics'
otel-desktop-viewer query 'DESCRIBE metric_series'
otel-desktop-viewer query 'DESCRIBE metric_datapoints'
otel-desktop-viewer query 'DESCRIBE attributes'
otel-desktop-viewer query "SELECT function_name FROM duckdb_functions() WHERE function_type = 'macro' AND function_name IN ('span_id_wire', 'trace_id_wire') ORDER BY function_name"
```

The examples use schema 21 tables `spans`, `logs`, `metrics`,
`metric_series`, `metric_datapoints`, and `attributes`, plus registered
`trace_id_wire` and `span_id_wire` macros. Inspect the installed schema before
adapting SQL.

## Query recent spans

Use `trace_id_wire` and `span_id_wire` to display stored IDs. `start_time` and
`end_time` are stored unsigned integer nanoseconds since Unix epoch;
`status_code` is the received OTel enum number. Derived labels and durations do
not replace them.

```sh
otel-desktop-viewer query "
SELECT
  trace_id_wire(trace_id) AS trace_id,
  span_id_wire(span_id) AS span_id,
  service_name,
  name,
  start_time,
  end_time,
  status_code
FROM spans
WHERE start_time >= epoch_ns(current_timestamp - INTERVAL '1 hour')
  AND start_time <= epoch_ns(current_timestamp)
ORDER BY start_time DESC"
```

## Query one trace and its logs

A span identity is `(trace_id, span_id)`, not `span_id` alone. Logs may have a
trace ID with null `span_id` or no stored span; query them separately to keep
trace-only and missing-span logs. `effective_timestamp` derives from received
`timestamp` and `observed_timestamp`. Received `severity_number` is the OTel enum
and `severity_text` is its received text. Log `body` is canonical recursive
tagged JSON shaped `{kind,value}`.

Replace the example with a returned 32-character trace ID.

```sh
otel-desktop-viewer query "
SELECT
  trace_id_wire(trace_id) AS trace_id,
  span_id_wire(span_id) AS span_id,
  CASE WHEN parent_span_id IS NULL THEN NULL
       ELSE span_id_wire(parent_span_id) END AS parent_span_id,
  service_name,
  name,
  start_time,
  end_time
FROM spans
WHERE trace_id = '4bf92f3577b34da6a3ce929d0e0e4736'::UUID
ORDER BY start_time, span_id"

otel-desktop-viewer query "
SELECT
  id,
  trace_id_wire(trace_id) AS trace_id,
  CASE WHEN span_id IS NULL THEN NULL ELSE span_id_wire(span_id) END AS span_id,
  coalesce(nullif(timestamp, 0), observed_timestamp) AS effective_timestamp,
  severity_number,
  severity_text,
  body
FROM logs
WHERE trace_id = '4bf92f3577b34da6a3ce929d0e0e4736'::UUID
ORDER BY effective_timestamp, id"
```

## Query recent Metric datapoints

`metrics.id`, `metric_series.id`, and `metric_datapoints.id` are viewer-assigned
database-local references. `metric_datapoints.metric_id` links the exact Metric;
`series_id` links one Metric and datapoint-attribute set. They preserve
associations but are not received OTel fields.

`timestamp` and `start_time` are received unsigned integer nanoseconds.
`value_type` keeps received number types: `Int` uses signed `int_value`; `Double`
uses `double_value`. Never coalesce or cast them to one floating-point value.
Histogram `sum`, `min`, and `max` preserve presence: null is absent; zero is
present zero. `aggregation_temporality` is the received signed OTel enum for Sum,
Histogram, and ExponentialHistogram. Gauge stores a non-applicable zero, so
interpret it only with `metric_type`.

```sh
otel-desktop-viewer query --json "
SELECT
  m.id::VARCHAR AS metric_ref,
  ms.id::VARCHAR AS series_ref,
  m.name,
  m.metric_type,
  m.aggregation_temporality,
  m.is_monotonic,
  d.timestamp,
  d.start_time,
  d.value_type,
  d.int_value,
  d.double_value,
  d.count,
  d.sum,
  d.min,
  d.max
FROM metrics AS m
JOIN metric_series AS ms ON ms.metric_id = m.id
JOIN metric_datapoints AS d
  ON d.metric_id = m.id AND d.series_id = ms.id
WHERE d.timestamp >= epoch_ns(current_timestamp - INTERVAL '1 hour')
  AND d.timestamp <= epoch_ns(current_timestamp)
ORDER BY d.timestamp DESC, m.name, series_ref"
```

These are stored fields, not Metric-summary display values. `metric_ref` and
`series_ref` are computed text projections of stored UUIDs. The one-hour
predicate is query scope, not received.

## Discover attributes and inspect matching records

```sh
otel-desktop-viewer attributes keys --json
otel-desktop-viewer attributes values http.request.method --limit 10 --json
otel-desktop-viewer attributes keys --signal logs --owner-type log --json
otel-desktop-viewer attributes values region --signal metrics --owner-type resource --json
```

Both default to the last hour and 25 rows. They accept `--endpoint`, `--service`,
`--since`, `--start`, `--end`, `--limit`, and `--json`, with the same time rules as
the summary commands. Defaults remain `--signal traces --owner-type span`;
select both flags when changing to logs or Metric datapoints. Keys return
distinct key/kind pairs with actual `foundOn` locations.

| Signal | Supported owners | Counted records | Time filter |
| --- | --- | --- | --- |
| `traces` | `span`, `event`, `link`, `resource`, `scope` | Distinct `(trace_id, span_id)` pairs | Event timestamp for `event`; span start otherwise |
| `logs` | `log`, `resource`, `scope` | Distinct log records | Received timestamp; observed timestamp when timestamp is zero |
| `metrics` | `datapoint`, `exemplar`, `metadata`, `resource`, `scope` | Distinct datapoints | Exemplar timestamp for `exemplar`; datapoint timestamp otherwise |

Resource/scope attributes stay on those owners. Frequencies count referencing
records: 100 spans sharing a west resource and one referencing an east resource
give counts 100 and 1, not one resource each. A histogram datapoint counts once,
not by its observations or buckets. `--service` matches the counted record's
stored resource-derived `service_name`
projection. Events and links count their owning spans; exemplars count their
owning datapoints; metadata counts the Metric's datapoints. Repeated child values
count a parent once per value. Event/exemplar time bounds apply to their own
received timestamps, even when the parent is outside the window. Only in-window
events/exemplars contribute values and parent counts; their denominator is distinct
parents with an in-window event/exemplar carrying the key. Zero timestamps are
compared as zero, with no parent-time fallback. Link targets and exemplar trace/span
correlations do not determine ownership. Metric metadata/resource/scope lookup
follows current stored associations,
not a reconstruction of their history at each datapoint time.

Values return complete tagged values, each with its own `foundOn`, exact `count`
and `denominator`, and computed `relativeFrequency`. Count is the number of
distinct matching records associated with that value; denominator is the number
of matching records whose selected owner carries the key, before limiting values. Their ratio is a
unitless DuckDB `DOUBLE`; the displayed percentage is that ratio times 100.
A record associated with multiple values counts once per value but once in the
denominator, so percentages can sum above 100%. Results include `truncated`.
Table output shows value, kind, count and percentage without bars.

Use `query` to find records for a selected typed value, then `trace` or `span`
to inspect their returned IDs. Keep the endpoint, service and time bounds the
same across discovery and lookup. For example, this follows the string `POST`
in `checkout` during one fixed hour:

```sh
otel-desktop-viewer attributes values http.request.method --service checkout --start 2026-10-02T08:00:00Z --end 2026-10-02T09:00:00Z --json
otel-desktop-viewer query --json "
SELECT trace_id_wire(s.trace_id) AS trace_id,
       span_id_wire(s.span_id) AS span_id, s.service_name, s.name
FROM spans s
WHERE s.service_name = 'checkout'
  AND s.start_time >= 1790928000000000000::UBIGINT
  AND s.start_time <= 1790931600000000000::UBIGINT
  AND EXISTS (
    SELECT 1 FROM attributes a
    WHERE a.key = 'http.request.method'
      AND json_extract_string(a.value, '$.kind') = 'string'
      AND json_extract_string(a.value, '$.value') = 'POST'
      AND list_contains(s.attribute_ids, a.id)
  )
ORDER BY s.start_time DESC, s.trace_id, s.span_id"
```

Adapt the key, received kind and value together. An `int64` payload is an exact
decimal string, not a `DOUBLE`. Double SQL single quotes inside key or string
value literals. Use the existing `trace` and trace-qualified `span` forms above;
there is no separate attribute-record lookup command.

## Custom span attribute SQL

`attributes` is a canonical `{kind,value}` dictionary. Owner arrays such as
`spans.attribute_ids` store its database-local IDs. For example:

```json
{"kind":"int64","value":"9007199254740993"}
```

For exact equality, first find the matching dictionary `id`, then filter owners
with `list_contains(attribute_ids, '<id>'::UUID)`. Use unnest/join for counts,
ranges, partial matches, or values without one exact ID.

Below, `value_kind` is `int64` for the example. The decimal string stays exact;
never cast it to `DOUBLE`. Native SQL integers remain integer JSON tokens in
`--json`. Join ownership to count span identities:

```sh
otel-desktop-viewer query "
SELECT
  a.key,
  json_extract_string(a.value, '$.kind') AS value_kind,
  count(DISTINCT struct_pack(trace_id := s.trace_id, span_id := s.span_id))
    AS owning_span_count
FROM spans AS s
CROSS JOIN unnest(s.attribute_ids) AS owned(attribute_id)
JOIN attributes AS a ON a.id = owned.attribute_id
WHERE s.start_time >= epoch_ns(current_timestamp - INTERVAL '1 hour')
  AND s.start_time <= epoch_ns(current_timestamp)
GROUP BY a.key, value_kind
ORDER BY owning_span_count DESC, a.key, value_kind"
```

This counts recent spans owning each key and received value kind, excluding
resource, scope, event, link, log, datapoint, and exemplar owners.

For common values, replace `http.request.method`. The denominator is one-hour
spans owning that key, not all recent spans or dictionary rows. Counts are exact;
derived `relative_frequency` uses `DOUBLE`. Keep `value_kind` with complete
`tagged_value` to preserve received kind and representation.

```sh
otel-desktop-viewer query --limit 10 "
WITH owned_values AS (
  SELECT
    s.trace_id,
    s.span_id,
    json_extract_string(a.value, '$.kind') AS value_kind,
    a.value AS tagged_value
  FROM spans AS s
  CROSS JOIN unnest(s.attribute_ids) AS owned(attribute_id)
  JOIN attributes AS a ON a.id = owned.attribute_id
  WHERE s.start_time >= epoch_ns(current_timestamp - INTERVAL '1 hour')
    AND s.start_time <= epoch_ns(current_timestamp)
    AND a.key = 'http.request.method'
),
value_counts AS (
  SELECT
    value_kind,
    tagged_value,
    count(DISTINCT struct_pack(trace_id := trace_id, span_id := span_id))
      AS owning_span_count
  FROM owned_values
  GROUP BY value_kind, tagged_value
),
denominator AS (
  SELECT count(DISTINCT struct_pack(trace_id := trace_id, span_id := span_id))
    AS owning_span_count
  FROM owned_values
)
SELECT
  value_kind,
  tagged_value,
  value_counts.owning_span_count,
  value_counts.owning_span_count::DOUBLE /
    nullif(denominator.owning_span_count, 0)::DOUBLE AS relative_frequency
FROM value_counts
CROSS JOIN denominator
ORDER BY value_counts.owning_span_count DESC, value_kind, tagged_value::VARCHAR
LIMIT 10"
```

## Interpret results

- No rows is success. Before reporting absence, verify endpoint, time, service,
  trace ID, exact key, and signal export.
- For SQL errors, run `SHOW TABLES` and `DESCRIBE` on the same viewer, then fix the
  named table or column.
- A table result is truncated only when a look-ahead row proves more rows exist;
  exactly 25 rows does not prove it.
- Keep received, stored, computed, and display values distinct. Durations,
  effective timestamps, severity labels, counts, relative frequencies, and UUID
  reference text are not received OTel fields.
