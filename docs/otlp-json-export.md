# Store-backed OTLP JSON

The store can reconstruct a complete stored trace as standard OTLP JSON with
`spans.GetTraceOTLP`. The input is a trace ID and the output is one compact
`TracesData` document containing every stored span with that ID. Selection does
not depend on a search query, time range, selected span, root span, parent
reachability, or frontend collapse state. Linked spans in other traces and
correlated logs are not part of the document.

Reconstruction happens in one DuckDB statement. The SQL groups spans by their
stored resource, scope, and wrapper schema URLs, and converts canonical stored
values recursively to OTLP `AnyValue`. IDs use OTLP hexadecimal form, 64-bit
integers use exact decimal strings, ordinary bytes use base64, enums remain
numbers, and non-finite doubles use the ProtoJSON strings `NaN`, `Infinity`, and
`-Infinity`. The returned text must pass through future RPC and download code
unchanged. In particular, JavaScript `JSON.parse` followed by `JSON.stringify`
changes negative zero to zero.

The output emits retained implicit-presence scalar and repeated fields at their
default values. It omits absent optional fields and IDs and unset oneof arms.
It rejects a completed document containing JSON null rather than inventing a
source value; a valid empty `AnyValue` remains `{}`.

This is reconstruction of the retained normalized signal, not recovery of the
original request bytes. The store cannot recover original request segmentation,
empty wrappers, wrapper/event/link order, duplicate span identities rejected at
ingest, unsupported fields, or the sender's original default-field omissions.
Resource, scope, span, event, and link ordering in the output is deterministic
but is not presented as received order. Standard OTLP JSON also preserves NaN
as a value but not its original payload bits.
