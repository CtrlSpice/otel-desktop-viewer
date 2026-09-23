# Store-backed OTLP JSON

The store can reconstruct stored traces and individual log records as standard
OTLP JSON. `spans.GetTraceOTLP` accepts a trace ID and returns one compact
`TracesData` document containing every stored span with that ID. Selection does
not depend on a search query, time range, selected span, root span, parent
reachability, or frontend collapse state. Linked spans in other traces and
correlated logs are not part of the document.

`logs.GetLogOTLP` accepts the tool-minted UUID of one stored log and returns one
compact `LogsData` document containing only that record. It preserves the full
body, log attributes, resource and scope owners, both wrapper schema URLs,
received and observed timestamps as separate fields, severity, flags, event
name, dropped count, and any stored trace and span association. An absent trace
or span ID remains absent; the getter does not invent correlation.

Reconstruction happens in one DuckDB statement. Each span keeps its own stored
resource, scope, and wrapper schema URLs, so one document can contain multiple
`resourceSpans` groups. Attributes remain normalized tagged JSON in storage;
their OTLP field names and wrappers are added only during export, without
changing received attribute keys.

`metrics.GetMetricOTLP` reconstructs one logical metric selected by
`metric_streams.id`. The output is a standard OTLP JSON object containing all
retained datapoints across that stream's stored reports. Reports with matching
resource, scope, schema URLs, description, and metadata combine into one Metric;
different source contexts remain associated with their own datapoints. A known
supported stream with no reports returns an empty `resourceMetrics` array. Gauge,
Sum, Histogram, and ExponentialHistogram are supported; Summary and unrecognised
stored metric types return an unsupported-type error. The getter preserves
temporality, monotonicity, number alternatives, optional histogram statistics,
explicit and exponential buckets, and exemplar IDs. It does not export chart
aggregates, reduced buckets, quantiles, rates, or other UI projections.

SQL converts the distinct stored values used by a result as one batch, while
keeping each body or attribute value's nodes and output separate. It first
records every node and its parent, child position, and type. A recursive query
then assigns numeric sort paths to every node at one depth together. Each node
produces only its local opening and closing text, and one ordered aggregation per
stored value assembles the result. This traversal-first shape avoids repeatedly
copying completed subtrees and avoids one recursive SQL step per opening or
closing fragment. Transformation, ordering, and serialization remain in SQL;
Go binds the signal identifier, scans the result, and maps errors. A metric
stream converts its distinct resource, scope, metadata, datapoint, and exemplar
attribute values in one shared batch before reattaching them to their stored
owners.

IDs use OTLP hexadecimal form, 64-bit
integers use exact decimal strings, ordinary bytes use base64, enums remain
numbers, and non-finite doubles use the ProtoJSON strings `NaN`, `Infinity`, and
`-Infinity`. The returned text must pass through future RPC and download code
unchanged. In particular, JavaScript `JSON.parse` followed by `JSON.stringify`
changes negative zero to zero.

The output emits retained implicit-presence scalar and repeated fields at their
default values. It omits absent optional fields and IDs and unset oneof arms.
It rejects a completed document containing JSON null rather than inventing a
source value; a valid empty `AnyValue` remains `{}`. Recursive stored values are
not given an application-defined depth limit.

This is reconstruction of the retained normalized signal, not recovery of the
original request bytes. The store cannot recover original request segmentation,
empty wrappers, wrapper/event/link/log order, duplicate span identities rejected
at ingest, unsupported fields, or the sender's original default-field omissions.
Resource, scope, span, event, link, and log ordering in one stored database is
deterministic but is not presented as received order. Standard OTLP JSON also
preserves NaN as a value but not its original payload bits.

Metric reconstruction likewise cannot recover original wrapper multiplicity or
order, datapoint or exemplar order, omitted-versus-empty repeated histogram
vectors, Summary data, or NaN payload bits. Repeated datapoints and exemplars use
stable stored timestamp-and-ID ordering instead.
