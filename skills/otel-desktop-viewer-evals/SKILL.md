---
name: otel-desktop-viewer-evals
description: "Use when preparing or running evaluations with OTel Desktop Viewer, configuring live evaluation telemetry, or checking result-to-trace correlation. Covers native results, Promptfoo instrumentation, runtime spans, and evidence retention."
---

# Run evaluations with observable evidence

This starter skill covers evaluation execution and telemetry setup.
Use the viewer's inspection guide for telemetry queries: `otel-desktop-viewer skills`.
The setup below was checked with Promptfoo 0.124.1, Pi 1.3.0, and Claude Code 2.1.307.
Check capabilities again when runtime versions change.

## Establish the run

1. Identify the evaluation runner, agent runtime, and their versions.
2. Read the user's approved questions, rubrics, providers, and repetition count.
3. Locate the viewer UI/API endpoint and the OTLP receiver endpoint separately.
4. Identify the fixture store and the observer store when evaluating the viewer itself.
5. Create a fresh durable evidence directory.
6. Record the required detail: result rows, outer spans, or internal model/tool spans.

Preserve the approved questions and rubrics.
Use ordinary producer providers and native repetitions when available.
Preserve recorded answer failures. Do not rerun them to obtain passing answers.
Record execution errors separately from assessment failures.
Keep grader sessions separate when the evaluation requires independent grading.

## Enable the evaluation runner's tracing

For Promptfoo 0.124.1, use both the configuration and exporter settings below.

```yaml
tracing:
  enabled: true
```

```sh
# Replace the example host and port with the actual OTLP receiver.
export PROMPTFOO_OTEL_ENABLED=true
export PROMPTFOO_OTEL_ENDPOINT=http://127.0.0.1:4318/v1/traces
```

Set these variables before the tracing SDK initializes.
`PROMPTFOO_DISABLE_TELEMETRY=1` disables product analytics, not this OTel exporter.

In this repository's viewer-usability harness, use `OTEL_EVAL_OBSERVER_HTTP` with the receiver's base URL instead.
The harness config enables Promptfoo tracing and constructs the trace-export URL.
Start or locate the receiver separately.

## Check the runtime's instrumentation

Promptfoo spans describe evaluation and provider execution.
Custom subprocesses need their own instrumentation for internal model and tool spans.

For each subprocess, propagate the active provider context through the runtime's supported mechanism.
For the tested Claude Code CLI, inject that invocation's parent as `TRACEPARENT`.
Enable its native exporter with these settings:

```sh
export CLAUDE_CODE_ENABLE_TELEMETRY=1
export CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1
export OTEL_TRACES_EXPORTER=otlp
export OTEL_LOGS_EXPORTER=otlp
export OTEL_METRICS_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_PROTOCOL=http/json
export OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318
```

Replace the example base URL with the actual receiver.
Preserve runtime-native parent relationships and links.
Record actual model IDs and session IDs when the runtime supplies them.

Pi 1.3.0 in this harness retains streamed JSON events.
The current harness does not convert Pi model/tool events into OTel spans.
Report that limitation when a user requests internal execution detail.

## Preserve identities

Keep native evaluation IDs, provider IDs, result IDs, test indices, and variant context unchanged.
Record explicit repetition identity when the exported result omits it.
Use the agreed Experiment → Test case → Result mapping for saved results and live updates.
Create Attempt identity only for a producer-recorded retry.
Do not infer identity from display labels, timestamps, or row positions.

This harness records received Promptfoo context in the invocation's local `identity.json`.
It includes repetition context and the active provider `traceparent` when available.
Provider responses do not copy that diagnostic object into their metadata.
Use `vars.task_id` as this suite's question key.
Treat these fields as harness-specific evidence, not universal producer fields.

## Check a small run first

1. Execute a small run within the user's approved scope.
2. Include a tool call when tool telemetry is required.
3. Retain the full native result envelope and streamed runtime evidence.
4. Check that the receiver stored the expected test-case and provider spans.
5. Check the recorded result-to-span association.
6. Check runtime child spans when the runtime claims internal instrumentation.
7. Check distinct repetition identities when the run uses repetitions.
8. Allow exporter shutdown to flush completed spans.
9. Wait for expected storage completion with a bounded check.
10. Record missing spans, ingestion errors, and incomplete operations explicitly.

An HTTP success response does not prove database ingestion is complete.
An unfinished parent span can arrive after its completed children.
Report the verified coverage before starting a larger evaluation.

## Retain and report

Retain native exports, session evidence, tool failures, grades, and available telemetry receipts.
Keep existing evidence when correcting a harness error. Use a new directory for the corrected run.
Record runtime differences that affect the comparison, including tool permissions.
Stop and wait for only the viewer and agent processes owned by the run.

Report assessment outcomes separately from execution errors and telemetry completeness.
State which evidence paths contain the original answers and failures.
State which capability was checked: result import, trace ingestion, or live Evals association.

Current limitation: production live Evals association remains unfinished.
Do not claim that stored spans automatically populate Evals rows.
Use full supported envelopes for current imports. Native bare-row JSONL lacks required run context.
