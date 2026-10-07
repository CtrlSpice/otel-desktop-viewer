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

`metrics.GetMetricOTLP` accepts one Metric reference and returns one OTLP Metric
object with all retained datapoints for that Metric. Gauge, Sum, Histogram, and
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

## Protobuf encoding

`spans.GetTraceOTLPProtobuf`, `logs.GetLogOTLPProtobuf`, and
`metrics.GetMetricOTLPProtobuf` encode the same reconstructed data as binary OTLP
export requests. These are store-level functions; API, CLI, and frontend download
controls are not yet exposed.

Binary export uses the official OTLP generated messages and the Google Go
protobuf runtime. The pinned Collector pdata encoder omits an
ExponentialHistogram `zeroThreshold` of negative zero; the Google encoder retains
its sign. The conversion adapts hexadecimal trace, span, and parent-span IDs to
the base64 representation expected by the standard ProtoJSON decoder. Exact
integer strings and other number tokens are preserved. JSON exports continue to
use the unchanged DuckDB result.

## Limits

The store cannot recover request segmentation, empty wrappers, received order,
unsupported fields, rejected duplicate span identities, default-field omission,
or NaN payload bits. Metric reconstruction also cannot recover wrapper
multiplicity, omitted-versus-empty repeated histogram vectors, or Summary data.

Stored output order is deterministic. It is not presented as received order.
