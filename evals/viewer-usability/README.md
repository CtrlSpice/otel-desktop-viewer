# Viewer usability evaluation

Promptfoo schedules providers, questions and repetitions. Pi 1.3.0 executes OpenAI
investigations and each separate grading session. Official Claude Code 2.1.307
executes Opus investigations using its own subscription login. Dependencies are pinned;
credentials, evidence and datasets live outside the repository.

## Configuration

`config.ts` defines:

- Four ordinary providers: Pi `openai-codex/gpt-6.1-sol`, `openai-codex/gpt-6-luna`,
  `openai-codex/gpt-5.6-luna`, and Claude Code `claude-opus-5-5`.
- Eleven tests, one per approved question in `tasks.json`.
- `evaluateOptions: { repeat: 3, maxConcurrency: 1, cache: false }`.
- Native `llm-rubric` assertions with a separate Sol 6.1 grading provider.
  `grading-evidence.ts` supplies the approved rubric and recorded evidence;
  Promptfoo owns grader invocation, verdict parsing, thresholds and results.
  Export verification is a separate native JavaScript assertion.

Questions are not expanded by model or repetition. There is no echo provider,
provider-replacement hook, manual slot scheduler or result-row rewrite on this
path. The full configured suite is 132 investigations plus separate graders.
The small pilot selects `checkout-failure`: four providers × one question × three
native repetitions, or twelve investigations plus twelve graders.

## Run

Requirements: macOS (the tested filesystem sandbox), Node >=22.22.0, an externally
built viewer, and the selected Pi credential in a private `auth.json` file.

```sh
npm ci --prefix evals/viewer-usability
export OTEL_EVAL_RUN_DIR=/absolute/persistent/path/to/a/new/run
export OTEL_EVAL_PI_AUTH_FILE=/absolute/private/path/to/auth.json
export OTEL_EVAL_CLAUDE_BINARY=/absolute/path/to/claude
export OTEL_EVAL_OBSERVER_HTTP=http://127.0.0.1:4318

# Create the run directory and put the evaluated binary at:
# $OTEL_EVAL_RUN_DIR/otel-desktop-viewer
node evals/viewer-usability/stage-dataset.ts
node evals/viewer-usability/prepare.ts
node evals/viewer-usability/verify.ts
npm run pilot --prefix evals/viewer-usability
```

Preparation starts an owned fixture viewer and records its process and connection.
Keep that viewer running during evaluation; the process that owns preparation is
responsible for stopping and waiting for it afterward. Do not reuse the UI demo's
database as the investigation fixture.

The observer URL must point to a separate running OTLP HTTP receiver. Loading the
config enables Promptfoo's native tracing/exporter when this URL is supplied.
The Opus provider requires this receiver and the official CLI version above;
run `claude auth login` beforehand. Its login remains managed by Claude Code.

`npm run evaluate` runs the full configured suite. To select a different single
question, run `node pi-evaluate.ts --task <task-id>` from this directory. Add
`--model gpt-5.6-luna` to run only that configured provider with native repetitions.
Existing result files or Rep evidence cause the runner to refuse a rerun in that directory.

The same config can be loaded with the normal Promptfoo CLI:

```sh
cd evals/viewer-usability
PROMPTFOO_DISABLE_TELEMETRY=1 \
  PROMPTFOO_CONFIG_DIR="$OTEL_EVAL_RUN_DIR/promptfoo-state" \
  npx promptfoo eval --config config.ts --no-cache
```

## Evidence and identities

- `promptfoo.json`: full native v3 export, including `evalId` and `config`.
- `promptfoo-rows.jsonl`: incremental native result rows.
- `runs/investigator-*/`: prompt, streamed runtime events, answer, response and
  workspace for each fresh Rep; Pi also retains its session and sandbox policy,
  while Claude Code retains its launch arguments and usage.
- `runs/grader-*/`: separate no-tool Pi session, prompt, events and response.
  Grades are stored in native `gradingResult.componentResults`, including the
  rendered grading prompt and provider metadata.

Each investigator receives a clean workspace with only the evaluated binary.
Automatic context, extensions, MCP, skills and prompt-template discovery are off.
Pi's sandbox blocks reads of the development tree, other Reps and reference files.
Claude Code uses safe/restricted modes, workspace-confined file tools and a Bash
allow rule for the supplied viewer. These are different runtime boundaries;
neither provides VM or network-service isolation.

The stable question key is `vars.task_id`. Provider identity comes from the normal
provider configuration. Promptfoo 0.124.1 sends `evaluationId`, `testIdx` and
`repeatIndex` to the provider; these remain in local `identity.json` diagnostics.
They are not copied into response metadata for the viewer.
Native exports expand `testIdx` across repetitions and omit a top-level
`repeatIndex`. The viewer groups the complete test definition and resolved
variables and preserves each result ID. Rep numbers are unnecessary. Available
start times determine default order; source order breaks ties and is the fallback.

## Live telemetry

For reusable agent instructions, read the [starter evaluation skill](../../skills/otel-desktop-viewer-evals/SKILL.md).

With `OTEL_EVAL_OBSERVER_HTTP` set, Promptfoo exports its native test-case and
provider spans over OTLP during evaluation. Its SDK flushes on evaluation shutdown.
The provider saves the active `traceparent` in local `identity.json` diagnostics.
`PROMPTFOO_DISABLE_TELEMETRY=1` disables product analytics, not this OTel exporter.

Pi's internal model/tool records are streamed to durable JSONL. The adapter emits
named `tool.execution` log records through the OTel Logs SDK, with the invocation's
native trace/span IDs. It joins inputs and outputs only through unique recorded
call IDs. The logs retain source records, tool duration and failure when supplied.
Their timestamp is the recorded tool-result message time, or explicitly labelled
collector observation time; observed time is also retained. Incomplete starts use
`tool.execution.start`. OpenCode V2 uses the same log exporter for its recorded
session tool evidence, retaining its recorded completion/creation timestamps.
These logs are adapter-created enrichment, separate from native runtime telemetry.
Genuine received span events remain span events. No tool spans or associations to
native tool spans are invented. Log export flushes before the provider returns;
logging is optional when `OTEL_EVAL_OBSERVER_HTTP` is absent.
Claude Code exports its native interaction/model/tool
spans, logs and metrics, and receives the Promptfoo parent through `TRACEPARENT`.

The viewer's Evals saved-import/live-update association still needs integration.
Receiving spans in the trace store does not by itself update Evals result rows.
Row-only JSONL is not a full import envelope.

Investigator and grader execution errors remain errors. Grades are provisional;
Fail/Partial/Pass remain 0/0.5/1 under the approved rubrics. Native scheduling
continues after individual errors; the old per-model pause/resume queue is not used.

## Checks

```sh
npm run typecheck --prefix evals/viewer-usability
node --test evals/viewer-usability/config.test.ts \
  evals/viewer-usability/pi-provider.test.ts \
  evals/viewer-usability/grade.test.ts
```

The provider integration test runs six actual Pi processes against a scripted
loopback model service. It checks native exports, received repetition indices,
fresh conversations, successful viewer-tool execution and blocked protected reads.
It makes no paid model requests.

The former OpenCode workflow is retained in `LEGACY.md`, `legacy-config.ts`,
`native-evaluate.ts` and the `*:legacy` commands for historical work.
