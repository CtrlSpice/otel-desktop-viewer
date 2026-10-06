---
name: otel-desktop-viewer
description: "Use when a user asks to inspect telemetry already received by OTel Desktop Viewer, including recent spans, correlated logs, or typed span-attribute use, through its bounded read-only query client."
---

# Inspect OTel Desktop Viewer telemetry

## Connect to one viewer

Check that the installed build supports the query client:

```sh
otel-desktop-viewer --help
```

Continue only when the root help lists `query`. Otherwise, tell the user that
the installed build does not provide the query command and direct them to the
web UI.

Use one viewer process for the whole task, including follow-up questions:

1. Probe the configured HTTP endpoint with
   `otel-desktop-viewer query 'SHOW TABLES'`. The default endpoint is
   `http://localhost:8000`.
2. If the probe succeeds, reuse that existing viewer for every command. Treat
   it as user-owned.
3. If the probe fails, start at most one
   `otel-desktop-viewer --open-browser=false` foreground child for this task.
   Keep its process handle.
4. Require both that the child remains running and that the same `SHOW TABLES`
   probe succeeds before sending queries. If the child exits, report its error
   output and stop the workflow.
5. Keep the same child running for every command and follow-up question. At the
   end of the task, terminate and wait for that child only.

Never stop or restart an existing viewer. The caller owns the child handle; the
viewer does not return one.

The query command returns at most 25 rows and prints an aligned table. Use
`--endpoint` for another HTTP address, `--limit` for another row limit, or
`--json` for the result object.

## Inspect the installed schema

Check the installed build rather than assuming its schema or options:

```sh
otel-desktop-viewer query --help
otel-desktop-viewer query 'SHOW TABLES'
otel-desktop-viewer query 'DESCRIBE spans'
otel-desktop-viewer query 'DESCRIBE logs'
otel-desktop-viewer query 'DESCRIBE attributes'
otel-desktop-viewer query "SELECT function_name FROM duckdb_functions() WHERE function_type = 'macro' AND function_name IN ('span_id_wire', 'trace_id_wire') ORDER BY function_name"
```

The examples below use the current `spans`, `logs`, and `attributes` tables and
the registered `trace_id_wire` and `span_id_wire` macros.

## Query recent spans

Stored trace and span IDs require the `trace_id_wire` and `span_id_wire` macros
for display. `start_time` and `end_time` are stored unsigned integer nanoseconds
since the Unix epoch. `status_code` is the received OTel enum number. Any labels
or durations are derived values, not replacements for these stored fields.

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
ORDER BY start_time DESC"
```

## Query one trace and its logs

A span is identified by `(trace_id, span_id)`, not by `span_id` alone. Logs can
have a trace ID with a null `span_id` or with no matching stored span. Query logs
separately to preserve those trace-only and missing-span logs.
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

## Query recent span attribute use

`attributes` is a dictionary, not a table of telemetry rows. Spans own entries
through `spans.attribute_ids`. Each value uses canonical recursive tagged JSON
with the shape `{kind,value}`, and `value_kind` below reads that received type
tag. Received `int64` values are stored as decimal strings and must not be cast
to `DOUBLE`; native SQL integers remain integer JSON tokens in `--json` output.
Join the ownership list and count the owning span identities:

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
GROUP BY a.key, value_kind
ORDER BY owning_span_count DESC, a.key, value_kind"
```

This counts recent spans that own each key and received value kind. It does not
count resource, scope, event, link, log, datapoint, or exemplar ownership.

## Interpret results

- Treat no rows as a successful query result. Check the endpoint, time
  predicate, trace ID, and exact received attribute key before reporting that
  telemetry is absent.
- Treat a SQL error separately from no rows. Run `SHOW TABLES` and `DESCRIBE`
  against the same viewer, then correct the named table or column.
- The default limit is 25. The table output reports when a look-ahead row proves
  that more rows are available; exactly 25 rows alone does not prove truncation.
- Use `--json` when exact machine-readable values or column types matter. Counts
  and `effective_timestamp` are computed by these queries, not received OTel
  fields.
