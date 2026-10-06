---
name: otel-desktop-viewer-usage
description: "Inspect traces, correlated logs, and attribute use in a running OTel Desktop Viewer with bounded read-only SQL."
---

# OTel Desktop Viewer usage

Use this skill to answer focused questions about telemetry already received by a
local viewer. It is not an instrumentation recipe or an audit framework.

## Prerequisite

This skill requires a build where `otel-desktop-viewer --help` lists `query`.
If it does not, use the web UI instead of guessing a query command or RPC call.

The viewer is a foreground server. The `query`, `traces`, `logs`, and `metrics`
commands are clients and require that server to remain running. A person can
keep it running in another terminal:

```sh
otel-desktop-viewer --open-browser=false
```

For agent or automated use:

1. Try the `otel-desktop-viewer query 'SHOW TABLES'` probe below against the
   configured viewer HTTP endpoint and reuse a compatible viewer that is already
   running. The default endpoint is `http://localhost:8000`.
2. If none is available, start `otel-desktop-viewer --open-browser=false` as a
   managed, nonblocking child process and retain its process handle.
3. During startup, require both that the child remains running and that the same
   `SHOW TABLES` probe succeeds. If the child exits, surface its error output and
   stop; do not reuse or stop another process listening at the endpoint.
4. Once both checks pass, run `query`, `traces`, `logs`, or `metrics` commands.
5. In a `finally` or equivalent cleanup step, terminate and wait for only the
   child process started in step 2.

Do not stop or restart an existing viewer owned by the user. Starting the viewer
does not return a PID or process object; the caller's process tool must retain
the child handle. There is no detach, status, stop, daemon, REPL, or direct
database mode, and client commands do not launch the viewer automatically.

The query command defaults to `http://localhost:8000`, returns at most 25 rows,
and prints an aligned table. Use `--endpoint` for a viewer at another HTTP
address, `--limit` for another row limit, or `--json` for the result object.

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

## Recent spans

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

`start_time` and `end_time` are stored unsigned integer nanoseconds since the
Unix epoch. `status_code` is the received OTel enum number. Labels and durations
would be computed values, not replacements for these stored values.

## One trace and its logs

Replace the example ID with a 32-character trace ID returned above. Query logs
separately so trace-only logs and logs whose span is absent remain visible.

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

A span is identified by `(trace_id, span_id)`; do not join on `span_id` alone.
A null log `span_id` is a valid trace-only association. `effective_timestamp` is
computed from the received timestamps. `severity_number` is the received enum
number, while `severity_text` is the received text.

## Recent span attribute use

`attributes` is a dictionary of distinct key and canonical typed-value pairs;
its row count is not a telemetry count. Join each span's `attribute_ids` and
count the owning span identities:

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
Attribute values and log bodies are stored as recursive `{kind,value}` JSON.
Received `int64` attribute values are decimal strings inside that tagged value;
do not cast them to `DOUBLE`. Native SQL integer results remain integer tokens
in `--json` output.

## Interpret and verify

- No rows is a successful result. Check the endpoint, time predicate, trace ID,
  and exact received attribute key before concluding that telemetry is absent.
- A SQL error is different from no rows. Run `SHOW TABLES` and `DESCRIBE` against
  the same viewer build, then correct the named table or column.
- The default limit is 25. The table output reports when a look-ahead row proves
  that more rows are available; exactly 25 rows alone does not prove truncation.
- Use `--json` when exact machine-readable values or column types matter. Counts
  and `effective_timestamp` are computed by these queries, not received OTel
  fields.
- Verify an investigation by narrowing its time range or ID, checking the owner
  count rather than dictionary rows, and confirming any truncation notice before
  reporting the result.
