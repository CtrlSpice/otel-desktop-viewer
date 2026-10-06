---
name: otel-desktop-viewer
description: "Use when a user asks to inspect traces, spans, logs, Metrics, or stored OpenTelemetry values already received by OTel Desktop Viewer through its read-only CLI."
---

# Inspect OTel Desktop Viewer telemetry

## Connect to one viewer

Confirm that the installed build supports the required read-only command:

```sh
otel-desktop-viewer --help
```

Continue only if the root help lists the required command. Otherwise, tell the
user that the installed build does not provide it and direct them to the web UI.

Use one viewer process throughout the task and any follow-up questions:

1. Probe the configured HTTP endpoint with
   `otel-desktop-viewer query 'SHOW TABLES'`. The default endpoint is
   `http://localhost:8000`.
2. If the probe succeeds, treat that viewer as user-owned and reuse it for every
   command.
3. If the probe fails at the default endpoint, start at most one
   `otel-desktop-viewer --open-browser=false` foreground child for this task.
   Keep its process handle. If a configured non-default endpoint fails, report
   it as unavailable; do not start an unrelated viewer at the default endpoint.
4. Before sending queries, require the child to remain running and the same
   `SHOW TABLES` probe to succeed. If the child exits, report its error output
   and stop the workflow.
5. Keep the child running for every command and follow-up question. At the end
   of the task, terminate and wait for that child only.

Never stop or restart an existing viewer. The caller owns the child handle; the
viewer does not return one.

## Choose the narrowest command

Use the narrowest purpose-built command that answers the question:

| Question | Command |
| --- | --- |
| Which traces arrived? | `traces` |
| What spans and trace-linked logs belong to one trace? | `trace <trace-id>` |
| What exact data belongs to one span? | `span <span-id>` or `span <trace-id> <span-id>` |
| Which logs arrived? | `logs` |
| Which Metrics arrived? | `metrics` |
| What custom aggregation or stored field is needed? | `query <sql>` |

Use `--json` when another command consumes the result or exact JSON numbers,
nulls, and object fields matter. Use the default table output for human
inspection.

## Search traces, logs, and Metrics

The summary commands search the last hour and return at most 25 rows by default:

```sh
otel-desktop-viewer traces --service checkout --since 30m
otel-desktop-viewer logs --service checkout --since 30m
otel-desktop-viewer metrics --service checkout --since 30m --json
```

All three commands accept `--endpoint`, `--service`, `--since`, `--start`,
`--end`, `--limit`, and `--json`. `--start` and `--end` accept inclusive RFC3339
timestamps with nanoseconds. Do not combine an explicitly supplied `--since`
with `--start` or `--end`.

An empty result is successful. Before reporting telemetry as absent, check the
endpoint, time window, service name, and whether the producer exported that
signal.

`traces` returns compact trace summaries. With `--service`, each summary also
reports how many spans matched that service; the trace can still contain spans
from other services.

`logs` returns compact log summaries. `logRef` is a database-local reference,
not a received OTel field.

`metrics --json` includes `metricRef`, the exact UUID text from `metrics.id`.
The viewer generates this database-local reference; OTLP does not provide it.
It is valid only for that database. Pass it unchanged and do not parse it. The
human-readable Metric table omits it.

## Inspect one trace or span

Use a 32-character hexadecimal trace ID returned by `traces`. A dashed UUID is
also accepted:

```sh
otel-desktop-viewer trace 4bf92f3577b34da6a3ce929d0e0e4736
otel-desktop-viewer trace 4bf92f35-77b3-4da6-a3ce-929d0e0e4736 --json
```

`trace` returns every compact stored span and every log carrying that trace ID.
Trace start is the minimum received span start timestamp. Trace duration is the
maximum received span end timestamp minus that start. Span offsets and durations
are computed from received nanosecond timestamps and returned as exact decimal
strings. A log timestamp uses its received timestamp unless that value is zero,
when it uses the received observed timestamp. Displayed severity uses received
text when present; otherwise, it uses a label derived from the received enum
number. The body is a compact preview; use `span` or `query` for complete stored
log data.

A span ID alone can occur in more than one trace. The command never chooses
between traces: it returns one exact span, a not-found result, or bounded
summaries of every match.

```sh
otel-desktop-viewer span 000000000000002a
otel-desktop-viewer span 4bf92f3577b34da6a3ce929d0e0e4736 000000000000002a --json
```

After an ambiguous result, use the qualified form. It returns the full span;
typed resource, scope, span, event, and link attributes; and logs carrying both
that trace ID and span ID. Those logs are exact span correlations. Use `trace`
or `query` for trace-only logs. `--limit` changes only the number of summaries
returned for an ambiguous standalone span ID.

## Run custom read-only SQL

`query` runs one read-only DuckDB statement against the existing viewer. By
default, it returns at most 25 rows in aligned columns. Use `--endpoint` for
another HTTP address, `--limit` for another row limit, or `--json` for the result
object.

## Inspect the installed schema

Inspect the installed build instead of assuming its schema or options:

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

The examples below use schema 21 with `spans`, `logs`, `metrics`,
`metric_series`, `metric_datapoints`, and `attributes`, plus the registered
`trace_id_wire` and `span_id_wire` macros. Inspect the installed schema before
adapting them.

## Query recent spans

Display stored trace and span IDs with the `trace_id_wire` and `span_id_wire`
macros. `start_time` and `end_time` are stored unsigned integer nanoseconds since
the Unix epoch. `status_code` is the received OTel enum number. Labels and
durations are derived values, not replacements for these stored fields.

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

A span is identified by `(trace_id, span_id)`, not by `span_id` alone. Logs can
have a trace ID with a null `span_id` or no matching stored span. Query logs
separately to preserve trace-only and missing-span logs.
`effective_timestamp` is derived from the received `timestamp` and
`observed_timestamp`. `severity_number` and `severity_text` are received fields;
the number is the OTel enum value. Log `body` uses canonical recursive tagged
JSON with the shape `{kind,value}`.

Replace the example ID with a 32-character trace ID returned above.

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

`metrics.id`, `metric_series.id`, and `metric_datapoints.id` are viewer-assigned,
database-local references. `metric_datapoints.metric_id` links a datapoint to its
exact Metric. `series_id` links it to one Metric and datapoint-attribute set.
These references preserve associations but are not received OTel fields.

`timestamp` and `start_time` are received unsigned integer nanoseconds.
`value_type` distinguishes received number datapoints: `Int` uses signed
`int_value`, while `Double` uses `double_value`. Do not coalesce or cast them to
one floating-point value. Histogram fields preserve presence: a null `sum`,
`min`, or `max` means absent, while zero means present with value zero.
`aggregation_temporality` is the received signed OTel enum number for Sum,
Histogram, and ExponentialHistogram. Gauge stores zero as a non-applicable
placeholder; use `metric_type` before interpreting it.

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

The query returns stored datapoint fields, not display values from the Metric
summary command. `metric_ref` and `series_ref` are string projections of stored
UUID references. The one-hour predicate defines the query scope; it is not a
received field.

## Query recent span attribute use

`attributes` is a dictionary, not a table of telemetry rows. Spans own its
entries through `spans.attribute_ids`. Each value uses canonical recursive tagged
JSON with the shape `{kind,value}`. For example:

```json
{"kind":"int64","value":"9007199254740993"}
```

For this value, `json_extract_string(a.value, '$.kind')` below returns `int64`
as `value_kind`. The decimal string preserves exactness; never cast it to
`DOUBLE`. Native SQL integers remain integer JSON tokens in `--json` output.
Join the ownership list to count owning span identities:

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

This counts recent spans that own each key and received value kind. It excludes
resource, scope, event, link, log, datapoint, and exemplar ownership.

To inspect common values for one exact span-attribute key, replace
`http.request.method` below. The one-hour predicate defines the telemetry scope.
The denominator is the number of spans in that scope that own the key, not all
recent spans or dictionary rows. `owning_span_count` is exact;
`relative_frequency` is the derived count divided by that denominator and uses
`DOUBLE`. Keeping `value_kind` with the complete `tagged_value` preserves the
received value kind and representation.

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

- Treat no rows as a successful query result. Before reporting telemetry as
  absent, check the endpoint, time predicate, trace ID, and exact received
  attribute key.
- Treat a SQL error separately from no rows. Run `SHOW TABLES` and `DESCRIBE`
  against the same viewer, then correct the named table or column.
- The default limit is 25. Table output reports when a look-ahead row proves
  more rows are available; exactly 25 rows alone does not prove truncation.
- Use `--json` when exact machine-readable values or column types matter. Counts
  and `effective_timestamp` are computed by these queries, not received OTel
  fields.
- Keep received, computed, and display values separate. Do not present durations,
  effective timestamps, severity labels, summary counts, relative frequencies,
  or UUID reference text as received OTel fields.
