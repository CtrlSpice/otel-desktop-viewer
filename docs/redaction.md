# Collector secret redaction

The viewer enables OpenTelemetry Contrib's `redactionprocessor` v0.162.0 in
every OTLP pipeline, before `batch` and the `desktop` exporter. Both HTTP and
gRPC traffic use it. Browser OTLP JSON/JSONL telemetry imports use the HTTP
receiver and receive the same processing. Stored queries and OTLP downloads
therefore see the processed values.

The embedded policy is [`collector_redaction.yaml`](../collector_redaction.yaml).
It uses `allow_all_keys: true`, `redact_all_types: true`, and `summary: silent`:
ordinary attributes stay present, matching values are masked with `****`, and
no redaction-summary attributes are added. Records are not dropped because
they discuss authentication or credentials.

## Covered data

- Resource and instrumentation-scope attributes for traces, logs and metrics.
- Span attributes and span-event attributes.
- Log-record attributes and log bodies, recursively through body maps/arrays.
- Metric datapoint attributes.
- Secret assignments inside strings, including JSON encoded as a string in a
  log-body array, shell assignments, and query-string assignments.

Secret-key rules cover authorization, API keys, access/refresh/session/ID/auth
tokens, client secrets, passwords, private keys, cookies, `secret`, and `token`.
Bare `access` and `refresh` keys are treated as OAuth secret fields. Matching is
case-insensitive and allows common camel-case, underscore and hyphen spellings.
Value rules cover Bearer/Basic authorization, PEM private keys, JWT shapes, and
selected OpenAI, Anthropic, GitHub and Slack token shapes.

Token-shape patterns come from a pinned subset of **Gitleaks v8.30.1**.
[Source rule IDs, adaptations and MIT attribution](../third_party/gitleaks/README.md)
are included. The assignment/key rules are viewer policy. This configuration
does not run the Gitleaks detector: it does not implement entropy thresholds,
secret capture groups, stopwords or Gitleaks allowlists. Distinctive provider
token shapes are masked even when they are low-entropy examples. The broad
Gitleaks generic-key rule is deliberately excluded to preserve identifiers.

For example, this body remains unchanged:

```json
["credential created", "{\"credentialID\":\"keep-id\",\"integrationID\":\"keep-integration\",\"type\":\"oauth\",\"active\":true}"]
```

Adding `access` or `refresh` secrets to that encoded object masks those
assignments while retaining the event and its nonmatching metadata. Credential
IDs, trace/span IDs, timestamps, and counters such as
`gen_ai.usage.input_tokens` and `gen_ai.usage.output_tokens` are not blocked keys.
Nonmatching values retain their original types and numeric precision, including
64-bit integers and IEEE double bits.

## Boundaries

This is pattern-based protection against common accidental leaks, not universal
secret detection. Unknown token formats, encoded/escaped secrets, and secrets
without recognizable keys or value shapes can remain. False positives are
possible, for example a benign value under a secret-key name.

The upstream component does not apply these patterns to span-link attributes,
metric metadata or exemplar attributes, span/event names, span status messages,
log event names/severity text, scope names/versions, schema URLs, or metric
names/descriptions/units.

Log-body maps/arrays are processed recursively, preserving nonmatching members.
Attribute maps/arrays behave differently: upstream scans their string
representation and converts the entire attribute to a string if it matches.
Their original structure and typed numeric representation are then lost.
Nonmatching composite attributes retain their types. Composite attributes that
cannot be represented as JSON (such as a nested non-finite double) cannot be
reliably scanned by this upstream path. A structured secret assignment inside a
string masks the containing string because regexes cannot delimit arbitrary
nested JSON safely. Masked JSON-encoded strings need not remain valid JSON.

Promptfoo Evals JSON imports are decoded to structured historical log observations
and submitted through the same receiver and processor before persistent storage.
Their source/index caches are reconstructed only from retained processed logs;
see [Evaluation data ingestion](eval-ingestion.md). Request-owned plaintext
temporary validation files are removed on return; a process crash can leave a
temporary file. Existing database contents and original source files are not
rewritten. The viewer's own optional service telemetry
uses a separate export path and is not processed by these ingestion pipelines.

The tests resolve the production Collector configuration and instantiate the
registered upstream component. They check masking, preservation, known upstream
boundaries, and HTTP/gRPC receive → processor → storage → OTLP download behavior
using synthetic data.
