# Store-backed OTLP JSON

The store can reconstruct traces, logs, and Metrics as standard OTLP JSON.
Reconstruction uses retained normalized data. It does not recover the original
request bytes.

## Traces

`spans.GetTraceOTLP` accepts a trace ID and returns one `TracesData` document.
It includes every stored span with that ID, including cycles, orphans, and
disconnected components.

Selection does not depend on search filters, time range, root-span reachability,
or frontend state. Linked spans from other traces and correlated logs are not
included.

Each span keeps its stored resource, scope, and wrapper schema URLs. One result
may therefore contain several `resourceSpans` groups.

## Logs

`logs.GetLogOTLP` accepts one viewer-generated log reference and returns one
`LogsData` document. It includes that record's complete body, attributes,
resource, scope, wrapper schema URLs, timestamps, severity, flags, event name,
dropped count, and stored trace and span association.

Absent trace and span IDs remain absent.

## Metrics

`metrics.GetMetricOTLP` accepts one Metric reference and returns a `MetricsData`
document containing that Metric, its resource/scope wrappers, and all retained
datapoints. Gauge, Sum, Histogram, and
ExponentialHistogram are supported. Summary and unrecognised stored Metric types
return an unsupported-type error.

The result preserves temporality, monotonicity, number alternatives, optional
histogram statistics, bucket data, and exemplar IDs. It does not include chart
aggregates, reduced buckets, quantiles, rates, or other UI projections.

`GetMetricOTLP` is store-only. The public `getMetric` and `getMetricSeries`
methods return normalized viewer data without OTLP wrappers.

## Encoding

DuckDB performs reconstruction in one statement per request. Stored tagged
values are converted in batches and attached to their owners. Go binds the
identifier, scans the JSON text, and maps errors.

The output follows these rules:

- IDs use OTLP hexadecimal form.
- 64-bit integers use exact decimal strings.
- bytes use base64.
- enums remain numbers.
- non-finite doubles use `NaN`, `Infinity`, and `-Infinity`.
- absent optional fields, IDs, and oneof arms are omitted.
- retained implicit-presence scalar and repeated fields include their default
  values.

The JSON text must pass through RPC and download paths unchanged. Parsing and
re-encoding it in JavaScript changes negative zero to zero.

## Download API and CLI

The viewer serves individual exports through:

```text
GET /export/traces/{traceID}
GET /export/logs/{logRef}
GET /export/metrics/{metricRef}
```

JSON is the sole file export format, for straightforward sharing and re-consumption.
Responses return `application/json`. Existing `?format=json` requests are accepted;
unsupported or duplicate format parameters return 400. Responses use
`Cache-Control: no-store` and attachment filenames `trace-{traceID}.json`,
`log-{logRef}.json`, or `metric-{metricRef}.json`, with canonical identifiers. Errors return plain text
without an attachment: 400 for invalid IDs/formats, 404 for missing records,
422 for unsupported stored Metric types, and 500 for reconstruction/encoding
failures.

The CLI writes the exact response bytes to stdout, without an added newline:

```sh
otel-desktop-viewer export trace 4bf92f3577b34da6a3ce929d0e0e4736 > trace.json
```

Use `export log <log-ref>` and `export metric <metric-ref>` for the other signals.
`--endpoint` follows the existing viewer convention; there is no format selector.
The selected signal header downloads JSON with one click.
Search/time filters and collapsed rows do not limit exported members.

## File Receiver

The binary registers Contrib's `otlpjsonfilereceiver` at `v0.162.0` alongside the
network OTLP receiver. Network protobuf transport remains supported. The JSON
document identifies its signal through `resourceSpans`, `resourceLogs`, or
`resourceMetrics`; the file name is not needed for signal selection.

Registration does not activate file ingestion or provide a browser-import backend.
Runtime paths, watching versus read-once behavior, upload handling, and completion
are not configured by this slice. The upstream receiver defaults to starting at the
end of existing files, adding file-name metadata, and splitting records over 1 MiB.
Those defaults are not a fidelity-preserving import policy. Tests use explicit
test-owned paths and configuration; production configuration still needs a decision.

## Reconstruction limits

The store cannot recover request segmentation, empty wrappers, received order,
unsupported fields, rejected duplicate span identities, default-field omission,
or NaN payload bits. Metric reconstruction also cannot recover wrapper
multiplicity, omitted-versus-empty repeated histogram vectors, or Summary data.

Stored output order is deterministic. It is not presented as received order.
