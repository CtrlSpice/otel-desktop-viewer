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

## Browser file import

Select OTLP JSON or JSONL files on Home, or drop them anywhere in the viewer.
Files are processed sequentially, including selections made during an import.
Each file is parsed once in chunks before any of its requests are sent. The
frontend retains byte ranges, not a complete decoded telemetry object tree.

Top-level `resourceSpans`, `resourceLogs` and `resourceMetrics` collections are
grouped by signal and sent to the existing OTLP HTTP receiver. Request bodies
use the original resource/record bytes, preserving exact numbers, negative zero,
attributes and resource/scope associations. Batches include at most 20 MiB of
encoded JSON, including their wrapper, with one request in flight at a time.
Large resource groups are split at scope, span, log-record, Metric or datapoint
boundaries while repeating the associated metadata. An individual record or
required metadata that cannot fit is reported rather than truncated.

The viewer's `getImportConfig` JSON-RPC method returns `otlpHttpPort`, populated
from the CLI's `--http` setting. The browser connects using the viewer's hostname
and that HTTP port. That receiver must be reachable from the browser; a reverse
proxy serving the viewer alone does not also proxy the OTLP port. Standalone
DuckDB-extension configurations can set `otlp_http_port`; when it is unset, imports
report that OTLP HTTP import is not configured.

Malformed JSON prevents sending any of that file. Unknown wrappers, oversized
records, network errors and OTLP rejection responses appear under Home's
**Ingestion issues**. Profiles appear as **Profiles support coming soon**.
Supported signal groups and subsequent files continue after individual issues,
so a file may be partially imported. Signal pages link to Home's issue details.
Existing stored-record rejection summaries remain separate from file issues.

An HTTP response acknowledges the normal receiver handoff, not a completed
database write. Imported data is discoverable through the existing UI refresh
and query interfaces.

## CLI file import

Use an existing viewer; `--endpoint` selects its HTTP endpoint, not the OTLP port:

```sh
otel-desktop-viewer import checkout-017.json --endpoint http://localhost:8000
otel-desktop-viewer import trace.json logs.json metrics.jsonl
```

| Command | Behaviour |
| --- | --- |
| `import <file> [file...]` | Sends regular JSON or JSONL files sequentially; later files continue after an individual failure |
| `--endpoint <url>` | Discovers `otlpHttpPort` through `getImportConfig`, then uses the viewer hostname and receiver HTTP port |
| `import --help` | Shows usage, examples and aligned flag sections without contacting a viewer |

The command follows the browser's wrapper classification, complete-file syntax
validation and 20 MiB batching rules. One standard-library token scan records byte
ranges; requests read those ranges directly without re-encoding telemetry values.
Metadata is copied when a resource, scope or Metric is split. Source files are not
modified. The receiver remains responsible for validating OTel record fields.

The aligned result table reports each file, requests accepted without issues, and
any file or receiver issues. Profiles are reported as coming soon. Any issue makes
the command exit nonzero; other supported groups and later files continue. The
command does not retry, start a viewer, or confirm completed database ingestion.
Some telemetry may already be stored after a failure, so replay can duplicate it.

## File Receiver

The binary registers Contrib's `otlpjsonfilereceiver` at `v0.162.0` alongside the
network OTLP receiver. Network protobuf transport remains supported. The JSON
document identifies its signal through `resourceSpans`, `resourceLogs`, or
`resourceMetrics`; the file name is not needed for signal selection.

This registered component is not activated by the viewer CLI. Browser imports
use the network receiver described above. File-receiver tests use explicit
test-owned paths and settings; its upstream defaults start at the end of existing
files, add filename metadata, and split records over 1 MiB.

## Reconstruction limits

The store cannot recover request segmentation, empty wrappers, received order,
unsupported fields, rejected duplicate span identities, default-field omission,
or NaN payload bits. Metric reconstruction also cannot recover wrapper
multiplicity, omitted-versus-empty repeated histogram vectors, or Summary data.

Stored output order is deterministic. It is not presented as received order.
