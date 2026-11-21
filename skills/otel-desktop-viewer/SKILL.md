---
name: otel-desktop-viewer
description: Inspect stored OpenTelemetry traces, logs, and Metrics with the OTel Desktop Viewer CLI. Query records with SQL or transfer OTLP JSON.
---

# Inspect telemetry using OTel Desktop Viewer

Use the CLI. Run `otel-desktop-viewer --help` first.

Reuse the user's viewer and leave it running. Start a viewer only if asked. Keep its process handle, then stop it and wait for it to exit when you're done so it doesn't keep running in the background.

- JSON-RPC (`POST /rpc`): `/openrpc/index.json` lists methods. Read one at `/openrpc/{method}.json` or all at `/openrpc.json`.
- HTTP: `/openapi.json` lists routes. Read one at `/openapi/{operationId}.json`.

## Find telemetry

`services` lists services and telemetry counts. Pass `serviceIdentity` to `services connections <service-identity>` to see incoming and outgoing connections from recorded parent and child spans.

Use `traces`, `logs`, or `metrics` to find records. Searches default to the last hour and 25 rows:

```sh
otel-desktop-viewer logs --service checkout --since 30m
```

For a fixed window, use `--start` or `--end` or both instead of `--since`. Check the endpoint and filters before reporting no matches. Increase `--limit` when a result reports truncation and you need more rows.

### Attribute keys and values

Use `attributes keys` to find keys and types. `attributes values <key>` shows values across all signals and owners; `foundOn` lists matching locations. Narrow with `--signal`, `--owner-type`, or both. Each record counts once per value. Percentages use only matching records that carry the key.

## Inspect a record

- `trace <trace-id>` shows spans and trace-linked log previews. `span <trace-id> <span-id>` shows one full span and logs matching both IDs.
- `log <log-ref>` shows the full typed body and context. Use `logRef` from `logs`.
- `metric <metric-ref>` lists series for a `metricRef` from `metrics`. Add `--series <series-ref>` for datapoints and exemplars. Use `--start` or `--end` to bound the series; otherwise it returns every retained point.

## Read-only SQL

Use `query` to run DuckDB SQL. Inspect the schema with `show tables` and `describe spans`. Read direct attributes from `attributes`, resource attributes from `resource.attributes`, and scope attributes from `scope.attributes`. Use `otel_value_json(value)` for nested OTel types.

### Record lookup by attribute value

Keep the service, time window, owner, and type from discovery:

```sh
otel-desktop-viewer attributes values http.request.method --owner-type span --service checkout --start 2026-10-02T08:00:00Z --end 2026-10-02T09:00:00Z
otel-desktop-viewer query "
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

## Transfer OTLP JSON

Redirect `export trace <trace-id>`, `export log <log-ref>`, or `export metric <metric-ref>` to a file. Trace exports contain spans; export logs separately. When asked to import, use `import <file> --endpoint <viewer-url>`. Receiver acceptance does not confirm storage. Retrying the whole file can duplicate records.
