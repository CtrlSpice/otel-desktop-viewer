# Evals integration: evidence first

Research date: 2026-10-20. Vendor capabilities below are documented, not end-to-end verified integrations. Pin producer versions and representative fixtures before claiming support.

## Current foundation

The [ingestion path](eval-ingestion.md) stores producer context and ordered source
rows as DuckDB JSON, then builds disposable Experiment, row-membership and text-search
indexes. Unknown fields remain in retained source data. The supported production
adapter currently targets producer-native Promptfoo v3 full-result exports.

The shared projection keeps Experiment, Case and Result distinct. Attempt is a separate,
optional retry concept and must be emitted only when a producer records retry
identity. Promptfoo v3 rows currently map to Results; they do not define the model
for later adapters.

The current presentation is intentionally narrower than the retained source. It assumes string questions/answers, numeric normalized assessments and CAD estimates. An adapter must omit incompatible projections with an explanation rather than invent a score maximum, relabel USD as CAD, infer execution errors from prose or manufacture telemetry IDs.

## Prioritized UI work

| Priority | Capability | Required behavior |
| --- | --- | --- |
| P0 | Evidence and provenance | Retained source access, content identity, source record location, producer/schema/adapter version and completeness. A projected upstream export is not the producer's complete original evidence. |
| P0 | Typed input/output/expected | Text, JSON, ordered message parts and non-chat likelihood/classification/retrieval evidence. Distinguish missing, null, empty string, zero and false. |
| P0 | Arbitrary scores and aggregates | Preserve numeric, categorical, boolean and structured scores; optional scale/direction/threshold; supplied aggregates and uncertainty. Do not turn every score into a percentage. |
| P0 | Dataset and variant identity | Dataset/example revision, model/prompt variant, repetition, retry lineage and scorer version when supplied. Never group by display text alone. |
| P1 | Attachments | Ordered media/file references and bounded previews; explicit unresolved, omitted and redacted states. No automatic external fetch. |
| P1 | Agent execution context | Nested/parallel agents, model calls, tools, sessions and cross-trace links. Preserve parentage and overlaps. Span summaries link into Traces; full span inspection stays there. |
| P1 | Cost and usage | Received currency/precision, reported versus estimated cost, model/token/cache/reasoning breakdowns and inclusive versus exclusive totals. Never sum parent rollups with children. |

## Source-specific entry points

| Source | Documented read surface | Adapter concern |
| --- | --- | --- |
| [Promptfoo](https://www.promptfoo.dev/docs/configuration/outputs/) | Full JSON/YAML and streaming JSONL; reduced CSV/JUnit outputs | Preserve provider/prompt/test variants and assertion components. Reduced formats cannot recover omitted evidence. |
| [DeepEval](https://deepeval.com/docs/evaluation-flags-and-configs) | Saved TestRun JSON; version-dependent local SQLite | Preserve metric reasons, errors, configuration and SDK version. Start with the documented JSON artifact. |
| [Inspect AI](https://inspect.aisi.org.uk/eval-logs.html) | `.eval`/JSON logs; public dump/convert/read commands | Start with externally dumped JSON. Preserve sample/epoch/retry identity, score edits, reductions and attachment references. Summary reads can truncate evidence. |
| [lm-evaluation-harness](https://github.com/EleutherAI/lm-evaluation-harness/blob/main/docs/interface.md) | Results JSON and optional sample artifacts | Support aggregate-only experiments, uncertainty, direction and non-chat likelihood evidence. Do not fabricate per-case scores from aggregates. |
| [OpenAI Evals OSS](https://github.com/openai/evals/blob/main/docs/run-evals.md) / [hosted API](https://developers.openai.com/api/reference/python/resources/evals/subresources/runs/subresources/output_items/) | OSS JSONL event logs / hosted output-item reads | Separate schemas and readers. Preserve grader pass/fail independently from numeric score and sample execution error. |
| [Braintrust](https://www.braintrust.dev/docs/annotate/export) | JSON/CSV experiment exports and paginated BTQL JSON/Parquet | Preserve experiment/dataset/project identities and audit fields. Record query, projection and page completeness. |
| [LangSmith](https://docs.langchain.com/langsmith/data-export) | SDK/API run reads and bulk Parquet | Preserve run hierarchy, dataset references and raw feedback. Comments and rollups depend on export options; UUIDs are not automatically OTLP span IDs. |
| [Langfuse](https://langfuse.com/docs/api-and-data-platform/features/public-api) | UI JSON/CSV; versioned observation/score/experiment APIs | Select field groups explicitly; distinguish absent and truncated content. Preserve typed scores and physical versus logical roots. |
| [Phoenix](https://arize.com/docs/phoenix/sdk-api-reference/typescript/arizeai-phoenix-cli) / [OpenInference](https://github.com/Arize-ai/openinference/blob/main/spec/semantic_conventions.md) | Span queries; CLI trace/experiment JSON | Separate telemetry interpretation from experiment/dataset/annotation import. Preserve dataset versions and requested annotations; OpenInference costs are USD. |
| [W&B Weave](https://docs.coreweave.com/products/wandb/weave/guides/tracking/querying-calls) | Call exports and paginated/streaming API reads | Preserve UUID parents, op/object references, raw usage and optional feedback/costs. A call snapshot may omit required dataset/media objects. |
| [Comet Opik](https://www.comet.com/docs/opik/tracing/advanced/export-data) | REST/SDK reads; CLI project JSON/CSV | Prefer JSON bundle joins. Preserve reminted-ID import lineage and bounded pagination. |
| [MLflow](https://mlflow.org/docs/latest/cli.html#traces) | Full trace JSON, trace search and assessment reads | Preserve feedback versus expectations and model/run/dataset references. Metadata-only responses and telemetry exports are not complete evaluation archives. |

## OTLP is not a universal evaluation export

A platform accepting OTLP does not establish that historical experiments, feedback and datasets can be downloaded as OTLP. Public read/export adapters remain necessary.

[GenAI semantic conventions](https://github.com/open-telemetry/semantic-conventions-genai/blob/main/docs/gen-ai/README.md) are Development. Pin interpreted versions and retain original attributes, resource/scope, schema URLs, events and links. Message content is optional; absent content is not an empty conversation. Token breakdowns may overlap totals. The [evaluation-result event](https://github.com/open-telemetry/semantic-conventions-genai/blob/main/docs/gen-ai/gen-ai-events.md) carries score/label/explanation and correlation, but does not establish a universal experiment/dataset/repetition identity.

## Small extensible core

Following the [extensible-core direction](https://jeremymorrell.dev/blog/extensible-software-in-the-age-of-llms/), keep retained source, producer interpretation and shared renderers separate. A source adapter receives bounded source JSON and returns a versioned projection, original-record locations and limitations. No plugin runtime is required.

Next adapters should follow real fixtures: DeepEval JSON, Inspect dumped JSON, harness result/sample bundles and OSS Evals JSONL. Hosted read connectors need separately defined pagination, credentials, completeness and reference-resolution contracts. Original evidence stays independent so a later adapter can reinterpret it without replacing received data.

## Saved views are separate records

Recommendation: immutable versioned view definitions in DuckDB, mutable named saves pointing to them, and existing routes with `?view=<revision-id>`. Capture metric series, aggregation, overlays, submitted query and query timezone; keep theme, widths and transient hover local. Preserve explicit empty selections and missing-data states instead of silently choosing replacement records.

The prior `feat/view-snapshot-foundation` design at `8116c418` provides a schema/hash baseline, not a completed storage or routing implementation. Reconcile newer trace-log focus, log sort and histogram semantics before reuse. Start with metrics, then traces/logs and durable Evals locators. Dataset binding, revision retention and portable data-bundle contents still need a settled contract. Sharing a view definition alone does not include its telemetry.
