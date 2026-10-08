# Viewer usability eval

TypeScript tooling for evaluating how agents investigate telemetry with the viewer.
Promptfoo runs and grades tasks; OpenCode provides the model/tool interaction loop.
These dependencies are separate from the standalone viewer's build and runtime.
Keep credentials, local settings and run artifacts outside version control.

| File                                                    | Purpose                                                                                                            |
| ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `tasks.json`, `prompt.txt`                              | Task goals, expected answers and shared prompt                                                                     |
| `stage-dataset.ts`                                      | Copy manifest-listed requests unchanged; defaults to `testdata/otlp/small/`, accepts `OTLP_DATASET`                |
| `prepare.ts`, `prepare.test.ts`                         | Start one owned persistent viewer, ingest the fixture and check failure cleanup                                    |
| `verify.ts`                                             | Check historical pilot facts through independent SQL                                                               |
| `grade.ts`, `grade.test.ts`                             | Deterministic answer checks and grader tests                                                                       |
| `provider.ts`, `config.ts`                              | Normal OpenCode sessions through Promptfoo                                                                         |
| `run-pilot.ts`, `run-pilot.test.ts`                     | Retain batch results, failures and summaries                                                                       |
| `runtime.ts`, `runtime.test.ts`                         | Require runtime paths outside repositories                                                                         |
| `dataset.test.ts`                                       | Check manifest staging, unchanged HTTP bytes, exact time bounds and partial-success rejection                      |
| `clean-environment.ts`, `clean-environment.test.ts`     | Fresh private CLI environments and isolation helper tests                                                          |
| `provider.test.ts`, `config.test.ts`, `test-fixture.ts` | Synthetic child-process adapter and explicit settings wiring tests                                                 |
| `verify-isolation.ts`                                   | Standalone loader/canary checks and optional Sol 6.1 access smoke                                                  |
| `isolation-check-settings.ts`                           | Validate private path and canary inputs for standalone isolation checks                                            |
| `verify-loader.ts`                                      | Exercise pinned Promptfoo's actual TypeScript config/provider/grader loader using local stub processes only        |
| `verify-integration.ts`                                 | Ingest an explicitly selected dataset into a new owned viewer, retain evidence, then stop and wait for that viewer |

## Status

The loader now defaults to the shared small workload and derives requests, counts
and time bounds from its manifest. The existing tasks, verifier and grading still
describe the historical pilot and must be migrated before evaluating small.
`config.ts` lists the configured model IDs. The adapter requires OpenCode 1.18.10
and refreshes its native model catalogue before rejecting a missing ID. Catalogue
presence alone is not proof of inference access. A new batch requires compatible
tasks, verified model access and an explicit permission map for that run.

The completed pilot and its transcripts, databases and run directories remain
outside the repo, unchanged. These files are the reusable suite, not those results.
Run model sessions in external scratch directories. Agents must not receive
fixture or grading files.

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
`verify.ts` still checks the old pilot's task answers; do not use it as proof of
the new small investigations until task migration is complete.

`prepare.ts` refuses an existing `runtime/` directory, starts one owned viewer
with browser opening disabled, and records its PID/command in
`$OTEL_EVAL_RUN_DIR/runtime/process.json`. It leaves the successful viewer running.
It stops only its owned process if preparation fails. No internal trace-monitoring
instance is required. Use a fresh runtime directory for another experiment.

After next-eval isolation and task changes are verified, run through
`node evals/viewer-usability/run-pilot.ts`. It disables Promptfoo telemetry and
sharing, writes Promptfoo state under the external runtime, and refuses an
existing `pilot.json`. It preserves failed runs and their transcripts.

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

Evidence is in a separate private `runs/*` directory, not model scratch. It records
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
`grade.ts`, filtering to one existing task with all four original model IDs.
The supplied executable is a local synthetic child, not OpenCode or a model service.
All proof, outputs and private synthetic
auth references remain in a new external directory. No inference is performed.

Owned-viewer integration verification requires separate permission to start a
new viewer server and stop/wait for only that owned process. CLI-client permission
alone does not authorise this command. Set `OTEL_EVAL_VIEWER_BINARY` to an absolute
viewer executable path and `OTLP_DATASET` to an absolute dataset directory.
It copies unchanged requests, checks counts
and rejection diagnostics, records PID/command, then stops and waits for its viewer.
It never reuses or stops an existing viewer. With the historical `usability-pilot`
dataset only, `--verify-pilot` also verifies all six unchanged task answers through SQL.
It is not a new-small-task verifier and does not run model inference.

```sh
export OTEL_EVAL_VIEWER_BINARY="$OTEL_EVAL_RUN_DIR/otel-desktop-viewer"
OTLP_DATASET="$PWD/testdata/otlp/small" \
  node evals/viewer-usability/verify-integration.ts
OTLP_DATASET="$PWD/testdata/otlp/usability-pilot" \
  node evals/viewer-usability/verify-integration.ts --verify-pilot
```

## Measurement boundaries

- Fresh OpenCode session, private context paths and external working directory per model/task.
- Scores check required final JSON facts; command choice earns no credit.
- Readiness/setup failures, instruction violations and factual failures must be
  distinguished during transcript review.
- Current command counters count matching shell entries, including help; loops,
  direct RPC calls and compound failures require auditing.
- Timing includes process startup and tooling; it is not viewer query latency.
- Usage counters are emitted input/output/cache fields, not a verified bill.
- One small synthetic fixture is not a general model ranking or SQL-only control.

Old runtime artifacts are never deleted by this suite. Store new results outside
Git with the viewer commit, suite file hashes, actual models and prompts.
