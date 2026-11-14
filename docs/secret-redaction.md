# Secret redaction

The viewer runs OpenTelemetry Contrib's `redactionprocessor` v0.164.1 before
batching and storage in every OTLP pipeline. This covers HTTP, gRPC and browser
telemetry imports. Queries and downloads see the processed values.

[`collector_redaction.yaml`](../collector_redaction.yaml) defines the policy.
It keeps ordinary attributes, masks matches with `****`, and adds no summary
attributes (`allow_all_keys: true`, `redact_all_types: true`, `summary: silent`).
Discussing credentials or authentication does not itself cause a record to be dropped.

## Covered data

- Resource and instrumentation-scope attributes across all three signals.
- Span and span-event attributes.
- Log attributes and bodies, including nested body maps and arrays.
- Metric datapoint attributes.
- Secret assignments in strings, including JSON-encoded text, shell assignments
  and query strings.

Key rules cover authorization, API keys, access/refresh/session/ID/auth tokens,
client secrets, passwords, private keys, cookies, `secret` and `token`.
Bare `access` and `refresh` keys are treated as OAuth secret fields. Matching
ignores case and accepts common camel-case, underscore and hyphen spellings.
Value rules cover Bearer/Basic authorization, PEM private keys, JWT shapes and
selected OpenAI, Anthropic, GitHub and Slack token shapes.

Token patterns come from a pinned subset of Gitleaks v8.30.1; see the
[source rules and attribution](../third_party/gitleaks/README.md).
The viewer uses these patterns without Gitleaks' entropy checks, capture-group
semantics, stopwords or allowlists. Distinctive token shapes are masked even
in low-entropy examples. The broad generic-key rule is excluded to preserve
identifiers; assignment and key rules are viewer policy.

This log body stays unchanged:

```json
["credential created", "{\"credentialID\":\"keep-id\",\"integrationID\":\"keep-integration\",\"type\":\"oauth\",\"active\":true}"]
```

Adding `access` or `refresh` secrets inside the encoded object masks those
assignments while keeping the event and nonmatching metadata. Credential IDs,
trace/span IDs, timestamps and usage counters such as `gen_ai.usage.input_tokens`
and `gen_ai.usage.output_tokens` are not blocked keys. Nonmatching values keep
their types and precision, including 64-bit integers and IEEE double bits.

## Limits

Unknown token formats, encoded or escaped secrets, and values without matching
keys or patterns can remain. False positives are possible, such as a benign
value under a secret-key name.

The processor does not cover span-link attributes, metric metadata, exemplar
attributes, span/event names, span status messages, log event names/severity
text, scope names/versions, schema URLs or metric names/descriptions/units.

Log-body maps and arrays are processed recursively, keeping nonmatching members.
For composite **attributes**, upstream scans their string representation and
turns the whole attribute into a string on a match, losing its original
structure and numeric types. Nonmatching composites keep their types. Composites
that cannot be represented as JSON, such as a nested non-finite double, cannot
be reliably scanned this way.

A structured secret assignment inside a string masks the containing string;
regular expressions cannot delimit arbitrary nested JSON. Masked JSON-encoded
strings may therefore be invalid JSON.

Existing database contents and original source files are not rewritten. The
viewer's optional self-telemetry uses a separate export path and does not pass
through these ingestion pipelines.
