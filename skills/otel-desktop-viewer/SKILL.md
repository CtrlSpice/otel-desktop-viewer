---
name: otel-desktop-viewer
description: "Use the OTel Desktop Viewer CLI to inspect stored traces, logs, and Metrics, query telemetry with SQL, or export and import OTLP JSON."
---

# Inspect OTel Desktop Viewer telemetry

Use the CLI to inspect telemetry. If you need to call the viewer over HTTP, see
`/openrpc.json` for JSON-RPC methods and `/openapi.json` for HTTP routes.

Run `otel-desktop-viewer --help` to find the command you need.

## Find telemetry

`services` lists services by namespace and name, with telemetry counts and the
last time each was seen. Use the service name to search its records. Use
`services connections <service-identity>` to see incoming and outgoing connections
from recorded parent and child spans.

Run `traces`, `logs`, or `metrics` to find records. Filter by service and time,
for example: `otel-desktop-viewer logs --service checkout --since 30m`.
By default, each command returns up to 25 rows from the last hour.

### Time windows

Service, signal, and attribute searches accept `--since` for a relative window
or `--start` and `--end` for fixed bounds. Either bound can be used alone.

### Attribute keys and values

Use `attributes keys` to find keys and their types, then `attributes values <key>`
to see which values occur. Both commands search all signals and owners.
`foundOn` shows where each key or value occurred. Narrow by signal, owner, or both:

```sh
otel-desktop-viewer attributes keys --signal logs --service checkout
otel-desktop-viewer attributes values region --owner-type resource
```

Counts refer to distinct spans, logs, or datapoints carrying a value. The
percentage uses records carrying the key as its denominator. A span with several
events can count toward several values, so percentages can sum above 100%.

## Inspect a record

- **Trace:** `trace <trace-id>` shows its spans and previews of logs with that trace ID.
- **Span:** `span <trace-id> <span-id>` shows the full span, events, links, and logs matching both IDs. If you have only the span ID, `span <span-id>` lists candidates when several traces match.
- **Log:** Pass `logRef` from `logs` to `log <log-ref>` for the full typed body, attributes, and resource and scope context.
- **Metric:** Pass `metricRef` from `metrics` to `metric <metric-ref>` for the descriptor and series list. Use `metric <metric-ref> --series <series-ref>` for datapoints and exemplars. Without `--start` or `--end`, this returns every retained datapoint in that series.

Log, Metric, and series references are local to the viewer database. Use the same
endpoint for the search and the detail command.

## Query with SQL

`query` runs one read-only DuckDB statement. Check the installed schema with
`otel-desktop-viewer query 'show tables'` and, for example,
`otel-desktop-viewer query 'describe spans'`. Select the columns or calculate the
aggregate that answers the question. The default output limit is 25 rows.

### Record lookup by attribute value

Carry the service, time bounds, owner, key, and type from discovery into SQL.
This finds the `checkout` spans with the string value `POST` during one fixed hour:

```sh
otel-desktop-viewer attributes values http.request.method --owner-type span --service checkout --start 2026-10-02T08:00:00Z --end 2026-10-02T09:00:00Z
otel-desktop-viewer query --json "
select trace_id_wire(s.trace_id) as trace_id,
       span_id_wire(s.span_id) as span_id, s.service_name, s.name
from spans s
where s.service_name = 'checkout'
  and s.start_time >= 1790928000000000000::ubigint
  and s.start_time <= 1790931600000000000::ubigint
  and otel_value_kind(s.attributes['http.request.method']) = 'string'
  and s.attributes['http.request.method']::varchar = 'POST'
order by s.start_time desc, s.trace_id, s.span_id"
```

Pass a returned trace ID and span ID to `span` to inspect that match.

### Stored fields and types

- Attributes live on their owners. Read direct attributes from `attributes`, resource attributes from `resource.attributes`, and scope attributes from `scope.attributes`. Use `otel_value_json(value)` to preserve nested OTel types, including in a log body.
- Join a span to its logs on both `trace_id` and `span_id`. Filter logs by `trace_id` alone to include trace-only logs. For log time, use `coalesce(nullif(timestamp, 0), observed_timestamp)`.
- Join `metric_datapoints` to `metric_series` on the Metric and series IDs. Keep signed `int_value` and `double_value` separate; converting both to floating point can round integers. An absent histogram `sum`, `min`, or `max` differs from a received zero.
- SQL JSON results encode integers as JSON numbers. Preserve integer precision when parsing Unix nanoseconds and 64-bit values.

## Transfer OTLP JSON

### Export

Redirect `export trace <trace-id>`, `export log <log-ref>`, or
`export metric <metric-ref>` to a file. Exports reconstruct all retained records
for that selection regardless of search filters. Export trace-linked logs separately.
Keep the output bytes unchanged to preserve numeric values such as negative zero.

### Import

When asked to import, use `import <file> [file...] --endpoint <viewer-url>` on
builds that list `import` in `--help`. Pass the viewer URL; the command discovers
its OTLP receiver. JSON and JSONL files can contain traces, logs, and Metrics.
A nonzero exit can follow a partial import. Check which requests were accepted
before replaying a file because replay can duplicate records.

## Empty or truncated results

For an empty result, check the endpoint, service, and time window. When a result
reports `truncated: true`, increase `--limit` if you need more rows.
