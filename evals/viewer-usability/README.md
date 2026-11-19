# Viewer usability evaluation

Promptfoo schedules providers, questions and repetitions. Pi 1.5.2 executes OpenAI
investigations and each separate grading session. Official Claude Code 2.1.307
executes Opus investigations using its own subscription login. Dependencies are pinned;
credentials, evidence and datasets live outside the repository.

The lockfile includes patched transitive dependencies. Overrides pin
`@hono/node-server` 2.1.6, `basic-ftp` 6.2.4, `glob` 14.0.2 and `minimatch` 11.0.1
because Promptfoo's dependency constraints still select affected versions. When
updating these pins, run `npm audit`, `npm run typecheck`,
`npm run typecheck:presentation` and `npm test` from `evals/viewer-usability`
after the setup in [Checks](#checks).

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

Use the evaluation launcher so cleanup failures stop the batch. Direct
Promptfoo CLI launches are rejected before any attempts.

```sh
npm run evaluate --prefix evals/viewer-usability
```

## Additional Opus grading of saved GPT answers

`opencode-v2-regrade.mjs` grades the Astra and Sol answers from a completed
`opencode-v2-evaluate.mjs --run-full` run with Claude Opus 5.5. The preceding
Pi `npm run evaluate` path is not a supported source. The original OpenCode run
must finish first. It reads the saved answers,
tool evidence and rendered rubric; it does not repeat investigations or change
the original grades. Recorded artifact checks are reused, not executed again.
Promptfoo parses the new judgments and combines their scores.

This grading mode uses the installed official Claude Code 2.1.326 and an existing
subscription profile. It supplies no tools and rejects a different model or any
recorded tool call. The existing Claude investigation mode still requires 2.1.307.
Grading prompts go through stdin rather than command-line arguments, retaining
large saved tool packets in full. Credentials remain managed by Claude Code.

After explicit approval for the additional grading, set
`OTEL_EVAL_CLAUDE_BINARY` and `OTEL_EVAL_CLAUDE_CONFIG_DIR` to that executable and
profile. Set `OTEL_EVAL_SOURCE_RUN` to the completed original OpenCode run and
`OTEL_EVAL_RUN_DIR` to an already-created, empty external comparison directory.
Keep that run's observer API and OTLP receiver running throughout grading and
import. Their URLs come from the original `run-plan.json`; this command does not
start or stop the observer.

```sh
node evals/viewer-usability/opencode-v2-regrade.mjs --plan "$OTEL_EVAL_SOURCE_RUN"
caffeinate -i node evals/viewer-usability/opencode-v2-regrade.mjs \
  --run "$OTEL_EVAL_SOURCE_RUN" "$OTEL_EVAL_RUN_DIR"
```

The comparison directory retains each grading session, raw telemetry, judgments,
original evidence hashes and a derived `promptfoo.json` report. Its investigator
responses and result IDs are unchanged, but its evaluation ID and grades are new.
The report is imported separately into the original run's observer; investigation
telemetry is not replayed. Execution errors stop grading without a retry. A valid
Fail or Partial judgment does not stop grading.

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

The harness runs per-test teardown for OpenCode V2, Pi and Claude Code attempts,
including errors, timeouts and cancellation. It stops the attempt's owned process
group. On a shared OpenCode host, it waits until the session stops, interrupting
the session if needed; the host stays running. Export-verification viewers use
the same teardown. After shutdown, the harness removes an executable copy only
if its SHA-256 hash matches the retained binary supplied in the provider
configuration, or in the run's `connection.json` for export replay. Answers, tool
records, telemetry, exported files and replay databases stay.

Each attempt's evidence directory, or export replay directory, contains the
teardown records. `teardown-started.json` records the verified hashes before
deletion. `teardown.json` records removal, a missing executable, or failure.
Changed executables and paths containing symlinks are retained as failures.
A teardown failure stops scheduling further tests; it does not discard the saved
answer. Teardown does not depend on model instructions. Killing the harness with
SIGKILL, or loss of power, can prevent teardown from running.

The stable question key is `vars.task_id`. Provider identity comes from the normal
provider configuration. Promptfoo 0.125.1 sends `evaluationId`, `testIdx` and
`repeatIndex` to the provider; these remain in local `identity.json` diagnostics.
They are not copied into response metadata for the viewer.
Native exports expand `testIdx` across repetitions and include a top-level
`repeatIndex` matching the provider context. The viewer groups the complete test
definition and resolved variables and preserves each result ID. Rep numbers are unnecessary. Available
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
Fail/Partial/Pass remain 0/0.5/1 under the approved rubrics. The default Pi/Claude
runner continues after individual errors, except teardown failures. The full
OpenCode runner stops on execution errors; error-free investigator deadlines
remain no-answer outcomes.

## Checks

From the repository root:

The V2 offline fixtures use an `opencode` subdirectory in the system temporary directory.

```sh
npm ci --prefix desktopexporter/internal/frontend
npm ci --prefix evals/viewer-usability
npm audit --prefix evals/viewer-usability
npm run typecheck --prefix evals/viewer-usability
npm run typecheck:presentation --prefix evals/viewer-usability
node -e 'require("node:fs").mkdirSync(require("node:os").tmpdir() + "/opencode", { recursive: true })'
npm test --prefix evals/viewer-usability
```

Dependabot opens weekly dependency update PRs for this runner and groups minor
and patch updates. CI runs these checks on macOS. `npm test` includes TypeScript,
presentation and MJS tests with at most two test files running at once. Tests
that require `OTEL_EVAL_VIEWER_BINARY` or `EVAL_PRESENTATION_RUN` skip when those
inputs are absent; CI does not supply them.

`pi-provider.test.ts` runs six actual Pi processes against a scripted
loopback model service. It checks native exports, received repetition indices,
fresh conversations, successful viewer-tool execution and blocked protected reads.
It makes no paid model requests.

The separate OpenCode V2 runner uses the private OpenCode 2.3.0 executable at
`~/.local/share/otel-desktop-viewer/evals/runtime/opencode/2.3.0/opencode`.
It calls the native API directly; Promptfoo does not need a package patch.
Saved 2.0.29 runs remain readable by the verification helpers.

`node evals/viewer-usability/opencode-v2-evaluate.mjs --plan-full` prints the
ten investigator model IDs, eleven questions, grader and repetition count without
starting a run. `--run-full` executes that plan through a shared host and requires
an explicit fresh run directory, prebuilt viewer, separate observer and private
membership references.

For `checkout-failure`, `--run-one` selects
Sol once; `--run-sol-luna` selects Sol and Luna three times each. This is separate
from the default Pi/Claude Code workflow.

The normal tests do not require an OpenCode executable. To check the installed
2.3.0 runtime with a scripted model and synthetic protected files:

```sh
OTEL_EVAL_VIEWER_BINARY=/absolute/path/to/otel-desktop-viewer \
  node evals/viewer-usability/opencode-v2-canary.mjs --telemetry
```

Add `--error` to check that a failed model request is retained without retrying.
These checks use fresh private servers and allow outbound network access only
to their scripted model and telemetry receiver. They require no model credentials.
