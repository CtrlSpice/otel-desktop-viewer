# Historical OpenCode evaluation harness

This document describes the retained OpenCode runner. The active Pi/Promptfoo
workflow is documented in `README.md`; its conventional config is `config.ts`.
The old expanded config is now `legacy-config.ts`. Historical helpers and tests
remain available for reading and reproducing existing evidence.

TypeScript tooling for evaluating how agents investigate telemetry with the viewer.
Promptfoo runs and grades tasks; OpenCode provides the model/tool interaction loop.
These dependencies are separate from the standalone viewer's build and runtime.
Keep credentials, local settings and run artifacts outside version control.

| File                                                    | Purpose                                                                                                            |
| ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `tasks.json`, `prompt.txt`                              | Exact approved questions and investigator context only                                                             |
| `rubrics.json`                                          | Private reference facts and criteria, loaded only by the separate grader                                           |
| `stage-dataset.ts`                                      | Copy manifest-listed requests unchanged; defaults to `testdata/otlp/small/`, accepts `OTLP_DATASET`                |
| `prepare.ts`, `prepare.test.ts`                         | Start one owned persistent viewer, ingest the fixture and check failure cleanup                                    |
| `verify.ts`                                             | Check approved small-dataset facts against stored traces, logs and metrics                                         |
| `grade.ts`, `grade.test.ts`                             | Separate fresh Sol6.1 grading and synthetic wiring tests                                                           |
| `verify-export.ts`, `verify-export.test.ts`             | Check session-owned JSON or tar.gz exports and unchanged-byte replay into a new owned viewer                        |
| `native-lifecycle.ts`, `native-hooks.mjs`, `config.ts`    | Native Promptfoo OpenCode SDK providers with private attempt runtimes and lifecycle hooks                           |
| `native-evaluate.ts`                                    | Programmatic Promptfoo evaluation in the same module graph as the native provider                                   |
| `eval-telemetry.ts`, `native-telemetry-plugin.mjs`        | Attempt/turn/tool spans, native detail links and context propagation into viewer RPCs                               |
| `native-telemetry.test.ts`                               | Local scripted-model integration check using the viewer CLI to verify stored parentage and error status             |
| `run-pilot.ts`, `run-pilot.test.ts`                     | Retain batch results, failures and summaries                                                                       |
| `model-summary.ts`, `pricing.ts`, `api-prices.json`     | Model totals, per-response API-price estimates and a run-fixed USD/CAD rate                                        |
| `runtime.ts`, `runtime.test.ts`                         | Require runtime paths outside repositories                                                                         |
| `dataset.test.ts`                                       | Check manifest staging, unchanged HTTP bytes, exact time bounds and partial-success rejection                      |
| `clean-environment.ts`, `clean-environment.test.ts`     | Fresh private CLI environments and isolation helper tests                                                          |
| `openai-models.json`                                    | Explicit public model definitions for the latest GPT tiers                                                         |
| `provider.test.ts`, `config.test.ts`, `test-fixture.ts` | Synthetic child-process adapter and explicit settings wiring tests                                                 |
| `verify-isolation.ts`                                   | Standalone loader/canary checks and optional Sol 6.1 access smoke                                                  |
| `isolation-check-settings.ts`                           | Validate private path and canary inputs for standalone isolation checks                                            |
| `verify-loader.ts`                                      | Exercise pinned Promptfoo's actual TypeScript config/provider/grader loader using local stub processes only        |
| `verify-integration.ts`                                 | Ingest an explicitly selected dataset into a new owned viewer, retain evidence, then stop and wait for that viewer |

## Status

The eleven approved questions use the shared small workload. Requests, counts and
inclusive exact time bounds come from its manifest; `OTLP_DATASET` selects another
dataset, whose facts must satisfy the approved preflight before model calls.

`cpu-comparison` replaces the historical `cpu-causation` question for future runs:
“How do the recorded CPU readings around checkout-019 compare with the rest of
the time window?” Its completion score is Pass/Fail. Preserve frozen historical
questions, answers and grades. For the approved recount, exclude `cpu-causation`
from both earned and possible points; keep `cpu-evidence` included. This leaves
ten eligible historical questions per repetition. Do not grade historical
causation answers against the replacement comparison question.

The approved pilot may run from the corrected feature branch before merge.
`config.ts` targets GPT-6.1 Sol, GPT-5.6 Sol, GPT-6 Luna and GPT-5.6 Luna: eleven tasks
times three repetitions times four models gives 132 investigator sessions.
Separate fresh GPT-6.1 Sol grading sessions are counted independently.
The currently authorised pilot is GPT-6.1 Sol and GPT-6 Luna once per task:
22 investigator sessions and 22 separate grader sessions, after verification.
The full 132-investigation comparison is not authorised.
The adapter requires
OpenCode 1.18.10 and registers the exact public definitions in `openai-models.json`
for each requested model. This corrects the CLI's ChatGPT-login name filter, which
recognises decimal-version names but omits the plain GPT-6 Luna/Astra IDs.
Authentication continues through the caller's selected existing login; model
definitions contain no endpoint, credential or alias changes. The adapter refreshes
its native catalogue before rejecting other missing IDs. Catalogue presence alone
is not proof of inference access. A new batch requires explicit authorisation,
verified model access and an approved permission map for that run.

Investigators receive only the exact question, executable, endpoint, dataset window
and scratch/lifecycle restrictions. They answer naturally, with no required JSON
or evidence field. `tasks.json` contains no answer key; private reference facts and
criteria are loaded separately by `grade.ts`, never into investigator vars or scratch.
Each answer is graded in a new `openai/gpt-6.1-sol` context using the same selected
credential references and existing isolation boundaries. Common grading instructions
are supplied once in that session. The harness runs the evaluated binary's offline
`skills` command and supplies its exact app guide as `viewerSkill` in the grading
prompt. The guide is reference context for interpreting recorded commands/results;
the grader is still instructed not to investigate or execute tools. The retained
grader prompt includes the guide used for that attempt. A missing, failed or invalid
guide read is a setup error before the grader starts. Each question has one
completion criterion with explicit scoring descriptions in `rubrics.json`:
Fail earns 0, Partial earns 0.5, and Pass earns 1. Customer count, trace export,
request count and CPU comparison allow only 0 or 1; a Partial verdict for those tasks is invalid grader
output. Partial credit rewards correct supported results, not effort or tool use.
The grader supplies a verdict and explanation; code derives its numeric score.
The task score is that single criterion's score, except an unsuccessful required
export-file check makes it 0. Only score 1 counts as a complete pass. Saved grading
reports and task summaries distinguish partial completion from failure and retain
the numeric score. Execution errors have no completion score.
The failure-rate question asks only for the rate change; the log question asks
only for the associated logs; the sequence question asks which types are stored.
Optional reference details do not add requirements. Completion grading remains
separate from unsupported-claim assessment.
Invalid grader output, provider errors and timeouts are execution errors, not
incorrect investigator answers. An investigator model execution error pauses that
model for the rest of the run. The queue continues with unstarted slots for other
models, without retrying the failed slot or rerunning completed answers. Paused
slots remain pending and unscored. `model-errors.json` preserves each model's first
error; `deferred-slots.json` lists the unexecuted slots. Original Promptfoo summaries
are saved under `evaluation-batches/`; the combined `pilot.json` contains only actual
attempts. The run returns nonzero if any model was paused. Shared grader errors,
harness/evidence failures and blocking viewer permission denials still stop the
whole run. Ordinary recoverable tool denials do not pause a model.
Grading reliability remains provisional until
comparison with human-reviewed answers; deterministic tests do not validate model judgments.

Before each attempt, the hook creates a private OpenCode profile and a seed containing
only `bin/otel-desktop-viewer`. Promptfoo's native `copy_working_dir: 'copy'` supplies
a fresh investigator workspace. The prompt uses `./bin/otel-desktop-viewer`, never
the run-level executable path. No fixture, rubric or answer files are copied.
Artifacts are retained in that attempt's owned `scratch` before assertions or
framework cleanup. Export verification reads those retained bytes; verified filenames
are mapped back to the original workspace paths by preserving each relative suffix.

Native permission checks use only local synthetic responses and a new owned
viewer. With an externally built verification binary, run
`OTEL_EVAL_VIEWER_BINARY=/absolute/path/to/viewer node --test evals/viewer-usability/verify-permissions.ts`.
The check retains tool events and waited viewer-shutdown evidence outside the
repository. It also verifies active-session stopping on a native viewer-help
working-directory denial. It does not run or grade a model evaluation.

The provider detects native permission-denial events while OpenCode is running.
A denial or pending native permission request for the supplied viewer's help or client commands stops the investigator
and the remaining Promptfoo queue, skips grading the blocked attempt, and returns
a nonzero harness-blocker result. Viewer commands include help/version discovery,
skills, services, attributes, query, trace/span/log/metric clients, export and import.
Protected-file, lifecycle, delegation and other-tool denials remain visible in
the transcript and `permission-denials.jsonl`, but do not stop the run. Ordinary
command failures, empty searches and JSON-RPC errors are not permission denials;
successful output quoting a denial does not trigger stopping.

The shell recogniser covers literal executable paths, quoted arguments, common
wrappers, value flags, literal `cd` and redirections. It is not a complete shell
interpreter: aliases, dynamically constructed paths and nested shell scripts are
not claimed covered. Raw native permission events remain available for inspection.

`harness-blocker.json` identifies the attempt, role, model, exact blocked input,
native reason and raw event. The blocked attempt retains its partial transcript,
failure record and command list; completed attempt records are preserved. The
runner signals only its owned Promptfoo and OpenCode process groups and retains
the existing 30-second cleanup grace for resistant descendants. The overnight
runner starts shutdown of its owned viewer immediately and waits for it. Callers
of `runPilot` that own a viewer supply the same shutdown callback; the pilot does
not infer ownership of a separately started viewer from a stored PID.

`node --test evals/viewer-usability/native-lifecycle.test.ts` checks the current hook
path with pinned Promptfoo, real native OpenCode and a scripted local model service.
It checks fresh profiles/workspaces/conversations, saved-output retrieval, all-step
usage, viewer denial/pending-permission stopping, deadlines and owned cleanup.
Set `OTEL_EVAL_VIEWER_BINARY` to an externally built viewer to exercise its real help;
otherwise the test uses a deterministic executable. It makes no paid/live model calls.
The older CLI-specific `verify-stop.ts` is retained as historical verification source,
not a current native-hook verification command.

Run results, transcripts and databases stay outside the repo. These files are
the reusable suite. Run model sessions in external scratch directories.
Investigators must not receive fixture or grading files.

## Local setup

Requirements: Node >=22.22.0, Go/CGO to build the viewer, and OpenCode
1.18.10 with working model access for separately authorised model runs. All first-party
helpers and tests use TypeScript through Node's native type stripping. Dependencies
are pinned in the local lockfile. Migration checks ran on Node v26.7.0; the documented
22.22.0 minimum has not been exercised. The suite pins its own TypeScript compiler
and Node types, so `typecheck` and `verify:loader` do not require a frontend install.
The full `npm test` suite includes presentation-builder tests that require the
frontend dependencies, as does the optional `typecheck:presentation` command.
Run preparation is explicit (`npm run prepare:run`); installing dependencies
does not create a run or start a viewer.
The `go.mod` here is a module boundary: application `go test ./...` must not
compile the Go provider templates shipped inside Promptfoo's dependencies.

From the repository root:

```sh
npm ci --prefix evals/viewer-usability --no-audit --no-fund
npm run typecheck --prefix evals/viewer-usability
npm run verify:loader --prefix evals/viewer-usability
# Install presentation-test prerequisites before running the full suite.
npm ci --prefix desktopexporter/internal/frontend --no-audit --no-fund
npm test --prefix evals/viewer-usability
export OTEL_EVAL_RUN_DIR="$(mktemp -d "${TMPDIR:-/tmp}/otel-eval.XXXXXX")"
go build -o "$OTEL_EVAL_RUN_DIR/otel-desktop-viewer" .
node evals/viewer-usability/stage-dataset.ts
```

### Evaluation traces

Use a separate observer viewer so evaluation telemetry does not enter the fixture
being investigated. Keep it running until Promptfoo, OpenCode and the evaluated
viewer have shut down and exported their telemetry. Set these before the existing
prepare/pilot/overnight workflow:

```sh
export OTEL_EVAL_OBSERVER_HTTP=http://127.0.0.1:4318
export OTEL_EVAL_OBSERVER_GRPC=http://127.0.0.1:4317
```

These are the observer's OTLP HTTP **base URL** and OTLP gRPC endpoint, respectively,
not its UI port. `prepare.ts` and the overnight workflow forward the gRPC endpoint
to the newly owned fixture viewer. Programmatic callers can use
`prepare(root, selfTelemetryEndpoint)`. `runPilot` forwards the HTTP setting to
its isolated Promptfoo worker. An already running fixture viewer must have been
started with `--self-telemetry-endpoint` to export its RPC spans.

Promptfoo owns the tracing SDK and flushes it after evaluation. The adapter uses
only the OTel API and the explicit trusted plugin in each private OpenCode profile.
Tracing enables private Promptfoo SQLite bookkeeping under the run directory;
`writeLatestResults: true` at suite level initializes the required tables.

Each test-case trace contains its provider attempt, numbered agent turns, tool
calls and grading assertion. The separate grader's native attempt inherits the
active assertion context. Each shell tool passes W3C `TRACEPARENT`/`TRACESTATE`
to the CLI, which injects them into `/rpc`; the viewer extracts them before
decoding. An explicit Go context takes precedence over environment context.
RPC decoding/query failures and nonzero tool-process exits receive error status.
Argument/result payloads, output text, reported usage and exact session/message/
call IDs remain available. Reported usage is not inferred when absent.

`metrics.json` records the attempt's trace/span IDs. `telemetry-events.jsonl`
retains redacted hook events; `telemetry.json` records plugin readiness/drain,
operation counts and incomplete operations. A failed hook or incomplete operation
cannot silently report a successful telemetry receipt. Known selected credential
values are redacted from bridge payloads; the private hook token is removed from
the tool's shell environment. Ordinary permission and execution-stop rules apply.

Pinned OpenCode **1.18.10 accepts HTTP trace context but does not pass it into its
internal session work**. The adapter builds the readable tree from explicit native
session/message/call identities, and links those spans to unmodified native detail
spans. Native model-call/internal traces remain separate. Instance disposal drains
the plugin before process shutdown; a shorter native export interval and larger
queue reduce buffering pressure. This does **not** establish native export
completeness: the local check still found missing parents in the detailed native
trace. Do not interpret those links as proof of a complete native trace.

For a no-inference integration check, build the viewer outside the repo and run:

```sh
OTEL_EVAL_VIEWER_BINARY=/absolute/path/to/viewer \
  node --test evals/viewer-usability/native-telemetry.test.ts
```

It uses two owned viewers, pinned OpenCode, dummy credentials and a loopback model
fixture. The viewer CLI checks the stored root/parents, cross-service RPCs, both
successful and failed queries, full tool payloads, and native detail-link targets.
All evidence is retained; owned processes are stopped. Set
`OTEL_EVAL_OPENCODE_BINARY` if the pinned binary is not `/opt/homebrew/bin/opencode`.

`OTLP_DATASET` selects another dataset directory using the same setting as the
development Make targets. To select a dataset from another worktree, use its
absolute dataset directory. Staging copies the
manifest and its listed request files without parsing/re-encoding payloads.

That stages the shared fixture but launches no viewer or model. To start a NEW
owned fixture viewer and verify it:

```sh
node evals/viewer-usability/prepare.ts
```

Preparation posts unchanged request bytes, checks Collector partial-success
responses, and polls final stored counts derived from the input. Spans count unique
trace/span ID pairs; logs and supported metric datapoints count each occurrence.
Stored rejection diagnostics are checked during polling, even if counts differ.
The existing 100-attempt/100-ms wait is unchanged. Unconfirmed visibility at the
deadline is a verifier infrastructure error, not evidence of payload rejection.
The connection window is formatted from manifest nanoseconds without rounding.
`verify.ts` checks 32 checkout roots, regional/version cohorts, exact sequence types,
payment failure and latency, all five trace logs, histogram spike/recovery, cumulative
request count and the limits of sparse gateway-only CPU samples. Trace/context values
are checked against staged fixture data. Parent relationships use same-trace
`parentSpanId`, not the resource or scope wrapper. Histogram interval mean is derived
as received binary64 sum divided by exact count, in seconds; displayed decimals
are approximate. Span duration is end timestamp minus start timestamp, computed
as an exact integer in nanoseconds; seconds are a display conversion. CPU percentages
are received gauge ratios multiplied by 100, displayed as approximate binary64
percentages. These associations do not establish deployment or CPU causation.

`prepare.ts` refuses an existing `runtime/` directory, starts one owned viewer
with browser opening disabled, and records its PID/command in
`$OTEL_EVAL_RUN_DIR/runtime/process.json`. It leaves the successful viewer running.
It stops only its owned process if preparation fails. No internal trace-monitoring
instance is required. Use a fresh runtime directory for another experiment.

After corrections are verified and a model batch is explicitly authorised, run through
`node evals/viewer-usability/run-pilot.ts`. It disables Promptfoo telemetry and
sharing, writes Promptfoo state under the external runtime, and refuses an
existing `pilot.json`. It preserves failed runs and their transcripts.
The runner performs three repetitions of every task/model pair. Each fresh
OpenCode call has a ten-minute deadline including startup/model discovery and
tool use. A timeout terminates the owned process group, retains partial output
and usage, records an infrastructure error, and lets Promptfoo continue the queue.

## Isolated provider setup

Set `OTEL_EVAL_ISOLATION_FILE` to an absolute private JSON file (mode `0600`)
outside model scratch. Lifecycle hooks read it for each provider. The JSON accepts
`permission` (required, caller-approved OpenCode rules), `authFile` (the original
private absolute auth JSON reference), `providerIDs` (additional explicitly selected
auth records), `executablePaths` (absolute executable directories), and
`opencodeBinary` (defaults to normal `opencode` on the clean PATH).
Optional `modelCatalogueFile` references an absolute frozen models.dev catalogue.
Each attempt copies its bytes outside model scratch and sets `OPENCODE_MODELS_PATH`
with model fetching disabled. This supplies current model definitions without
changing the pinned CLI, authentication or model IDs. Save the catalogue source,
capture time and hash with the run. Ambient catalogue overrides remain excluded.

Go estimates use the selected Go model's published token rates, recorded separately
from OpenAI rates. They estimate subscription usage, not the subscription invoice.
DeepSeek peak rates apply Monday–Friday UTC during [01:00, 04:00) and [06:00, 10:00).
Each native usage step retains its assistant message creation time in Unix
milliseconds; absent response times make time-dependent costs unknown. Qwen3.7
Plus uses its published 256K context boundary. The official Go price table supplies
MiniMax M3 rates; catalogue prices are not used to override the serving provider's
published price table. Estimates use recorded input, output/reasoning and cache
categories, with the frozen Bank of Canada rate for CAD display.
If provider-key authentication is explicitly approved, use `authEnvironmentFile`
to reference a private JSON file of selected provider API-key/token environment
entries. Do not put credentials inline in settings: Promptfoo can retain provider
configuration in its results. No alternate provider route or model mapping is
implemented here.

No default cohort policy is supplied. The approved map must permit viewer CLI
client/HTTP access while denying viewer start/stop/reconfiguration, source/fixture/
grader/other-evaluation reads and delegation. Existing prompt restrictions remain.
Native sessions do not use CLI `--auto` or silently approve permission requests.
An investigator or grader execution error stops the run before another session
starts. The failed row and evidence are retained. Valid `0/1` and `0.5/1` results
continue normally; they are not execution failures.
Pending viewer-help/client requests stop the run, just as native denials do.
The session policy explicitly allows only its own native saved-output directory
after the approved external-directory rules, so truncated tool results remain readable.
Smoke-only permission maps in tests/verifier are not cohort policy.

Each provider call creates a new `workspaces/clean-context-*` private root beneath
the external runtime, with independent home/config/data/state/cache and empty
scratch. It copies only selected authentication records and does not merge
`process.env`. It checks the CLI version before configuration/model commands,
then checks the exact requested ID, with no model fallback or alias. The normal
build agent and built-in tools remain; no custom tool interface is introduced.
The run-level viewer executable is not added to the investigator PATH.
Server startup and native provider loading wait until Promptfoo assigns the actual
working directory. Runtime `TMPDIR` is that workspace's `tmp` directory (the
uncopied scratch for graders), so OpenCode's `tmp/opencode` instructions, tool
environment and retained exports refer to the same workspace. Private authentication
and data stay outside it; no additional external-directory permission is granted.
Each attempt uses an owned `opencode serve` process with its explicit clean environment;
the native provider connects through documented `baseUrl`, preserving selected OAuth
authentication without injecting a substitute API key. Fresh provider instances retain
their own session until evidence is recorded; no conversation is reused.

Evidence is in separate private `runs/investigator-*` and `runs/grader-*` directories,
not model scratch. It records
actual version, requested model, session IDs, launch paths and outputs; failures
are retained. Selected credentials are redacted from captured output, including
complete native event frames and history responses. `native-events.jsonl` retains raw
events and `native-history.json` retains the complete session response. Tool strings
and numeric text remain intact; usage is summed across every assistant step, not just
the final answer. `afterEach` stops and waits for the owned runtime after assertions,
with the existing descendant cleanup grace. Credential copies and evidence files are `0600`; private roots are
`0700`. Original auth/config files are never written by the adapter.
Context isolation is not a filesystem sandbox: tools can still access outside
scratch according to permissions. Generic built-in prompts and the built-in
`customize-opencode` skill remain. Synthetic outbound-request canaries establish
checked instruction-loading behaviour, not zero possible influence on a live model.

Standalone isolation checks use a separate private JSON settings file selected
by `OTEL_EVAL_ISOLATION_CHECK_FILE`. The file must have an absolute path and mode
`0600`; store it outside the repository or in an ignored `*.local.json` file.
Its fields are:

| Field              | Value                                                                                                |
| ------------------ | ---------------------------------------------------------------------------------------------------- |
| `authFile`         | Absolute path to the existing OpenCode authentication file; no credential values in settings         |
| `personalConfig`   | Absolute path to the existing personal configuration directory to fingerprint                        |
| `originalSuite`    | Absolute path to preserved earlier eval evidence to fingerprint                                      |
| `opencodeBinary`   | Absolute path to the OpenCode executable                                                             |
| `viewerBinary`     | Absolute path to the viewer executable, used only for offline help/skill checks                      |
| `forbiddenMarkers` | Array of nonempty personal-context strings that must not appear in an isolated request; may be empty |

With that environment variable set, run checks without a viewer lifecycle command
or evaluation batch:

```sh
node evals/viewer-usability/verify-isolation.ts
```

This creates a new verification directory beneath Node's temporary directory
(`TMPDIR` on Unix), inspects resolved paths/config/skills/tools and the clean catalogue,
captures synthetic HTTP canaries with project instructions enabled/disabled,
and runs only offline viewer `--help`/`skills`. It fingerprints personal auth/config
and the original pilot/helper evidence before/after. It performs no external model
inference. `--live-sol61` additionally runs one fresh adapter `pwd` access smoke
with explicit Sol 6.1 and exports its session; it never runs task-suite prompts.
`--audit <verification-directory>` creates separate audit artifacts, reruns TypeScript
tests and type checking without inference, checks retained fingerprints and private auth
copies, scans verification artifacts for credential leakage, and inspects a new
session-database snapshot read-only so SQLite sidecars do not alter retained proof.
None of these commands authorises an evaluation batch.

Offline TypeScript loader proof:

```sh
npm run verify:loader --prefix evals/viewer-usability
```

This runs the actual pinned Promptfoo CLI against `config.ts`, `provider.ts` and
`grade.ts`, filtering to one existing task with all four configured model IDs.
The supplied executable is a local synthetic child, not OpenCode or a model service.
The loader verifies natural prose, fresh exact-model grader sessions, private prompt
separation, factual-failure classification and execution-error handling.
All proof, outputs and private synthetic
auth references remain in a new directory under the system temporary directory
(or the caller's `TMPDIR`).
No inference is performed.

Owned-viewer integration verification requires separate permission to start a
new viewer server and stop/wait for only that owned process. CLI-client permission
alone does not authorise this command. Set `OTEL_EVAL_VIEWER_BINARY` to an absolute
viewer executable path. `OTLP_DATASET` is optional and defaults to shared `small`.
It copies unchanged requests, checks counts
and rejection diagnostics, records PID/command, then stops and waits for its viewer.
Readiness retains `traces` / `span_already_stored` diagnostics without failing on
deduplication alone; receiver partial failures and all other store rejection reasons fail.
It never reuses or stops an existing viewer. It verifies the approved investigation
facts against the new store and does not run model inference.

```sh
export OTEL_EVAL_VIEWER_BINARY="$OTEL_EVAL_RUN_DIR/otel-desktop-viewer"
OTLP_DATASET="$PWD/testdata/otlp/small" \
  node evals/viewer-usability/verify-integration.ts
```

Task 7 also requires a produced file. The runner keeps the investigator's scratch
path in response metadata. The verifier inspects only regular files in that scratch,
ignores symbolic links, and does not follow paths supplied in the answer. It accepts
OTLP JSON or a gzip-compressed tar archive containing OTLP JSON, with any filename
or subdirectory. Archive regular-file members are read in memory using the pinned
`tar` dependency; member paths are evidence labels, never extraction destinations.
Directories and links do not supply payloads. Malformed archives and duplicate
file paths fail their own file check. Duplicate checks treat `trace.json`,
`./trace.json` and `dir/../trace.json` as the same path. Repeated directory headers
are allowed. A complete six-span export must retain exact values, resource/scope
context, exception and same-trace parent IDs. Candidate JSON bytes, including those
read from archive members, are sent unchanged over OTLP HTTP to a new dynamic-port
viewer with browser opening disabled;
readiness uses the candidate's final stored counts, not the reference's six spans;
the re-exported trace is then compared with the staged reference. Counts alone
cannot establish correct IDs or contents. Dedup-only diagnostics permit a pass only
when the separate stored ID/content comparison matches; the diagnostics remain in
replay evidence. The owned replay process
is stopped and awaited in `finally`. This checks importability without relying on
pending CLI line-file replay changes; it does not test browser import.
Rejected or incompletely retained candidate payloads fail their own file check;
other candidates still run and valid exports remain available. Viewer startup,
transport and other verifier infrastructure failures remain execution errors.
Archive replay evidence identifies both the delivered archive and the verified
member. The grader's verified file list contains the delivered archive path once,
even if more than one member passes. The grader checks that the answer's reported
location identifies a verified file;
an unreferenced valid scratch file does not validate a reported invalid file.
Relative answer locations resolve against the original investigator working
directory supplied to the grader, not the archived scratch directory. Verified
file paths are mapped to that same original directory.

## Offline human correction and signoff

`human-review.ts` reviews one saved **current-question/current-rubric** grading report.
Old scores are obsolete; this command does not migrate or finalise old-rubric attempts.
Original automated results and run evidence remain unchanged. Human decisions are
stored separately in a new private directory outside repositories and the run.
The command makes no model calls, network requests or viewer lifecycle changes.

From the repository root, use absolute external paths for your run and new review
directory. `--grading` is relative to the run:

```sh
node evals/viewer-usability/human-review.ts prepare --run /absolute/run --grading runs/grader-attempt/grading.json --review-dir /absolute/review
node evals/viewer-usability/human-review.ts record --review-dir /absolute/review --decision /absolute/review/draft.json
node evals/viewer-usability/human-review.ts signoff --review-dir /absolute/review --revision 1 --reviewer your-label --confirm-final
node evals/viewer-usability/human-review.ts show --review-dir /absolute/review
```

Preparation writes `audit.md`, `source.json`, selected evidence snapshots and
`draft.json`. It supplies no human decisions. Read the audit and edit the draft:
enter your reviewer label; choose `completion.action` (`accept` or `correct`), the
score, reason and evidence citations; enter explicit `claims`, `findings` and
`toolReviews` arrays, including `[]` when appropriate. Each citation has a `source`
ID from the audit and an exact `quote` from that source. For example:

```json
{"source":"answer","quote":"The payment was declined."}
```

`accept` must retain the automated score. `correct` accepts only the task's approved
0/0.5/1 scores; customer count, export and request count remain binary. Claim entries
retain exact answer text, its `claimSource` citation, `classification` (`supported`,
`contradicted` or `unverified`), manual `decision` (`Pass`, `Fail` or unresolved null),
evidence and reason. Unverified Pass means human acceptance, not proof. Additional
findings require evidence and reason but do not change the completion score or add
requirements. This workflow does not automatically extract or grade claims.

Each recorded failed tool needs a `toolReviews` entry with its source ID,
`disposition` (`not-blocking` or `blocking`) and reason. Blocking failures and unresolved
claim decisions prevent signoff. Execution errors and harness blockers stay unscored.
For export score 1, add `completion.export` with `file` selected from the audit's
verified export list and `location` citing the exact absolute or relative filename
in the answer. A saved successful content/reloadability check and matching retained
replay bytes are mandatory; a human cannot override a failed deterministic check.
No export is replayed by this command.

`record` appends a decision revision but leaves it **pending**. Only explicit signoff
of the current revision makes its numeric score **final**. A correction appends a new
revision, preserves earlier decisions/signoffs and requires new signoff. Changed
source bytes or review history fail validation. Reviewer identity is a self-declared
local label, not authentication or a cryptographic signature; hashes do not prevent
someone with filesystem access from rewriting all records. Keep the directory 0700
and decision/review files 0600.

For a read-only consumer, import `showReview(directory: string)`. It validates sources
and event history, then returns `run`, `taskID`, `investigator` (evidence directory,
session IDs and model), `revision`, `latestRevisionSigned`, pending/final `status`, and
`signedOff`, `finalScore` (signed numeric score or null), and human `claimCounts`
(`supported`, `contradicted`, `unverified`; null before a human revision exists).
Recorded counts remain provisional until that revision is signed; they are not a
numeric hallucination measure or evidence of exhaustive claim coverage.
Invalid evidence throws. Do not treat
raw review JSON or a previous revision's signoff as the current final result.

## Owned overnight run

`run-overnight.ts` stages shared `small` by default, honours `OTLP_DATASET`,
starts one dedicated viewer, verifies approved facts before model calls,
derives investigator session count from tasks × models × three repetitions, then stops
and waits for its owned viewer on completion or failure. It requires separate
permission for this viewer lifecycle and an absolute `OTEL_EVAL_VIEWER_BINARY`.
Set `OTEL_EVAL_ISOLATION_FILE` to the private approved settings. Each completed
investigation gets a separate fresh Sol6.1 grading session, not an added investigator
cohort. Source commits, binary hash, settings and ownership are recorded externally.

## Model summary and CAD API-price estimate

Each run records `pricing-snapshot.json` before model execution, using the tracked
standard OpenAI API rate card and the latest available Bank of Canada USD/CAD
daily observation. The saved prices, exchange rate and source dates stay fixed
throughout that run. Existing snapshots are reused without overwriting them.

`pilot-summary.md` shows model totals, median seconds per task attempt, tool calls,
recorded failed tools, uncached input, output including reasoning, cache reads and
estimated API cost in CAD. Per-task details follow the model table.
`model-summary.json` retains investigator costs, usage and attempt counts.
`grader-summary.json` separately retains grader costs, usage, timing, per-criterion
judgments and execution failures. Markdown has separate investigator and grader tables.
Retained investigator process evidence preserves usage even when Promptfoo drops
its response from a row after a grading assertion throws.

OpenCode's input count excludes cache reads/writes; its output count excludes
reasoning. For each model response, the estimate uses:

```text
USD = (input × input_rate + cache_read × cached_input_rate
     + cache_write × cache_write_rate
     + (output + reasoning) × output_rate) / 1,000,000
CAD = USD × saved_CAD_per_USD_rate
```

Rates are selected per response: full input (`input + cache_read + cache_write`)
above 272,000 tokens uses long-context pricing. Costs are summed before conversion
and display rounding. Usage missing from the recorded responses or an unknown
model price produces `N/A` or an explicitly partial estimate. This is a comparison
at standard API prices; GPT-5.6 Sol/Luna prices are unavailable in the tracked rate card.
Subscription accounting and taxes are outside its scope.
Local CLI/HTTP tools have no additional hosted-tool charge in this estimate.

## Measurement boundaries

- Fresh OpenCode session, private context paths and external working directory per model/task.
- Separate provisional grading checks correctness and support, not answer format or command choice.
- Readiness/setup failures, instruction violations and factual failures must be
  distinguished during transcript review.
- Current command counters count matching shell entries, including help; loops,
  direct RPC calls and compound failures require auditing.
- Timing includes process startup and tooling; it is not viewer query latency.
- Usage counters are emitted input/output/cache fields, not a verified bill.
- One small synthetic fixture is not a general model ranking or SQL-only control.

Old runtime artifacts are never deleted by this suite. Store new results outside
Git with the viewer commit, suite file hashes, actual models and prompts.
