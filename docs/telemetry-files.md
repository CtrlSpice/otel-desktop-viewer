# Telemetry files

The viewer imports OTLP JSON and JSONL and downloads stored telemetry as OTLP
JSON. Downloads reconstruct retained records; they do not recover the original
request bytes. Imported data passes through [Secret redaction](secret-redaction.md).

## Import telemetry

Choose OTLP JSON or JSONL files on Home, or drop them anywhere in the viewer.
You can add more files while an import runs; the viewer processes them in order.

Check **Ingestion issues** on Home for errors. Invalid JSON prevents the whole
file from being imported. Other errors can leave a file partly imported.

Data can take a moment to appear. Refresh Traces, Metrics or Logs after importing.

Your browser must be able to reach the viewer's OTLP HTTP port (`4318` by default,
set with `--http`). If you use a reverse proxy, forwarding the web UI alone is
not enough.

## Download telemetry

Use an item's Download button or the CLI:

```sh
otel-desktop-viewer export trace 4bf92f3577b34da6a3ce929d0e0e4736 > trace.json
```

The CLI also accepts `export log <log-ref>` and `export metric <metric-ref>`.
It writes the response bytes to stdout without an extra newline. `--endpoint`
selects the viewer HTTP address. JSON is the only file export format.

The HTTP routes are:

```text
GET /export/traces/{traceID}
GET /export/logs/{logRef}
GET /export/metrics/{metricRef}
```

A trace download includes every retained span with that trace ID, including
cycles, orphans and disconnected components. Linked spans from other traces and
correlated logs are excluded. A log download contains one complete record,
including its body, context, attributes, timestamps, severity, flags, event name,
dropped count and correlation IDs. Absent IDs stay absent.

A metric download includes each received descriptor with its owning context and
retained datapoints, preserving temporality, monotonicity, numeric types,
optional statistics, buckets and exemplars. Gauge, Sum, Histogram and
ExponentialHistogram are supported. Summary and unrecognised stored types return
an unsupported-type error. Chart aggregates, quantiles and rates are excluded.

Resource/Scope wrappers and schema URLs stay with their records, so one download
can contain several resource groups. Search filters, time ranges, tree
reachability and collapsed rows do not narrow downloads.

### Select part of a record

| Parameter | Route | Selection |
| --- | --- | --- |
| `spanId` | Traces | One span in the trace, using its 16-character hexadecimal ID. |
| `seriesRef` | Metrics | All retained datapoints in one series, using its viewer-generated UUID. |
| `datapointRef` | Metrics | One retained datapoint, using its viewer-generated UUID. |

For example, when the trace and span are stored:

```sh
curl -fS 'http://localhost:8000/export/traces/4bf92f3577b34da6a3ce929d0e0e4736?spanId=00f067aa0ba902b7' -o span.json
```

Selectors must belong to the requested trace or metric. When both metric
selectors are supplied, the datapoint must also belong to that series.

Metric exports accept `includeDatapoints=false` for metadata only, without
reconstructing points or exemplars. The default is `true`. This option works
with either selector; membership is still checked.

### HTTP responses

Downloads use `application/json`, `Cache-Control: no-store` and attachment names
`trace-{traceID}.json`, `log-{logRef}.json` or `metric-{metricRef}.json`.
Selected downloads append selector names and normalized IDs. HEAD and byte
ranges describe the requested representation. `?format=json` is accepted.

Errors return plain text without an attachment:

| Status | Cause |
| --- | --- |
| `400` | Invalid IDs; unknown, misplaced, empty or repeated selectors; invalid, empty or repeated `includeDatapoints`; unsupported or repeated `format`. |
| `404` | Missing record or a selector that does not belong to it. |
| `422` | Unsupported stored Metric type. |
| `500` | Reconstruction or encoding failure. |

### Metric previews

The metric Details JSON tab loads metadata only, while Copy and Download return
the complete metric. Its preview is limited to 2 MiB. Formatting also has depth
and output-size limits; exceeding them preserves the source whitespace in the preview.

## Import limits

Large files are split into requests of at most 20 MiB. Each record and the JSON
needed to send it must fit in one request. Oversized records are reported rather
than cut down. Profiles are not supported.

## Encoding

Browser imports preserve exact numbers, including negative zero, and keep
Resource and Scope data with each record.

DuckDB selects, orders and groups records. A Go function converts native
`VARIANT` values to OTLP `AnyValue` JSON. The store's `GetTraceOTLP`, `GetLogOTLP`
and `GetMetricOTLP` methods reconstruct these documents; the public `getMetric`
and `getMetricSeries` methods instead return normalized viewer data.

- IDs use OTLP hexadecimal form; bytes use base64.
- 64-bit integers use decimal strings and enums use numbers.
- Non-finite doubles use `NaN`, `Infinity` and `-Infinity`.
- Absent optional fields, IDs and oneof alternatives are omitted.
- Fields without presence tracking and repeated fields include retained defaults.

Pass the JSON text through unchanged. Parsing and re-encoding it in JavaScript
changes negative zero to zero.

## Reconstruction limits

Downloads cannot recover request segmentation, empty wrappers, received order,
unsupported fields, rejected duplicate span identities, default-field omission
or NaN payload bits. Metric downloads also cannot recover wrapper multiplicity,
omitted-versus-empty repeated histogram vectors or Summary data.

Output order is deterministic, not the received order.
