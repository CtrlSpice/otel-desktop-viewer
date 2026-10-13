# Viewer usability eval

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
| `verify-export.ts`, `verify-export.test.ts`             | Check session-owned export files and unchanged-byte replay into a new owned viewer                                 |
| `provider.ts`, `config.ts`                              | Normal OpenCode sessions through Promptfoo                                                                         |
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

The ten approved questions use the shared small workload. Requests, counts and
inclusive exact time bounds come from its manifest; `OTLP_DATASET` selects another
dataset, whose facts must satisfy the approved preflight before model calls.
The approved comparison may run from the corrected feature branch before merge.
`config.ts` targets GPT-6.1 Sol, GPT-5.6 Sol, GPT-6 Luna and GPT-5.6 Luna: ten tasks
times three repetitions times four models gives 120 investigator sessions.
Separate fresh GPT-6.1 Sol grading sessions are counted independently.
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
are supplied once in that session. Each criterion gets Pass/Fail and an explanation;
the task passes only when its approved criterion and any required file check pass.
Invalid grader output, provider errors and timeouts are execution errors, not
incorrect investigator answers. Grading reliability remains provisional until
comparison with human-reviewed answers; deterministic tests do not validate model judgments.

Run results, transcripts and databases stay outside the repo. These files are
the reusable suite. Run model sessions in external scratch directories.
Investigators must not receive fixture or grading files.

## Local setup

Requirements: Node >=22.22.0, Go/CGO to build the viewer, and OpenCode
1.18.10 with working model access for separately authorised model runs. All first-party
helpers and tests use TypeScript through Node's native type stripping. Dependencies
are pinned in the local lockfile. Migration checks ran on Node v26.7.0; the documented
22.22.0 minimum has not been exercised. Type checking uses the existing frontend
compiler read-only; no TypeScript dependency was added to this suite.
The `go.mod` here is a module boundary: application `go test ./...` must not
compile the Go provider templates shipped inside Promptfoo's dependencies.

From the repository root:

```sh
npm ci --prefix evals/viewer-usability --no-audit --no-fund
npm test --prefix evals/viewer-usability
npm run typecheck --prefix evals/viewer-usability
export OTEL_EVAL_RUN_DIR="$(mktemp -d "${TMPDIR:-/tmp}/otel-eval.XXXXXX")"
go build -o "$OTEL_EVAL_RUN_DIR/otel-desktop-viewer" .
node evals/viewer-usability/stage-dataset.ts
```

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
responses, waits for manifest counts, and checks stored rejection diagnostics.
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
outside model scratch. `config.ts` forwards it to each provider. The JSON accepts
`permission` (required, caller-approved OpenCode rules), `authFile` (the original
private absolute auth JSON reference), `providerIDs` (additional explicitly selected
auth records), `executablePaths` (absolute executable directories), and
`opencodeBinary` (defaults to normal `opencode` on the clean PATH).
If provider-key authentication is explicitly approved, use `authEnvironmentFile`
to reference a private JSON file of selected provider API-key/token environment
entries. Do not put credentials inline in settings: Promptfoo can retain provider
configuration in its results. No alternate provider route or model mapping is
implemented here.

No default cohort policy is supplied. The approved map must permit viewer CLI
client/HTTP access while denying viewer start/stop/reconfiguration, source/fixture/
grader/other-evaluation reads and delegation. Existing prompt restrictions remain.
The adapter retains the existing `--auto` flag: this CLI auto-approves requests not
explicitly denied. Therefore `ask` is not an effective denial in this runner.
Smoke-only permission maps in tests/verifier are not cohort policy.

Each provider call creates a new `workspaces/clean-context-*` private root beneath
the external runtime, with independent home/config/data/state/cache/tmp and empty
scratch. It copies only selected authentication records and does not merge
`process.env`. It checks the CLI version before configuration/model commands,
then checks the exact requested ID, with no model fallback or alias. The normal
build agent and built-in tools remain; no custom tool interface is introduced.
The runtime viewer executable remains on the explicitly assembled PATH.

Evidence is in separate private `runs/investigator-*` and `runs/grader-*` directories,
not model scratch. It records
actual version, requested model, session IDs, launch paths and outputs; failures
are retained. Selected credentials are redacted from captured output, including
split chunks. Credential copies and evidence files are `0600`; private roots are
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
any filename or subdirectory. A complete six-span export must retain exact values,
resource/scope context, exception and same-trace parent IDs. Candidate bytes are sent
unchanged over OTLP HTTP to a new dynamic-port viewer with browser opening disabled;
the re-exported trace is compared with the staged reference. The owned replay process
is stopped and awaited in `finally`. This checks importability without relying on
pending CLI line-file replay changes; it does not test browser import.
Rejected or incompletely retained candidate payloads fail their own file check;
other candidates still run and valid exports remain available. Viewer startup,
transport and other verifier infrastructure failures remain execution errors.
The grader checks that the answer's reported location identifies a verified file;
an unreferenced valid scratch file does not validate a reported invalid file.

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
