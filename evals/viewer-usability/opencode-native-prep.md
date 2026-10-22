# OpenCode native Promptfoo preparation

## Native telemetry capability check (2.0.29)

The authorized full-capture continuation completed successfully in
`opencode-v2-native-eQUlOk`: one investigator, one separate grader, score **1/1**,
eight model requests and ten successful tools. Captured and verified **1,268
native spans, 27 native logs, 4 wrapper spans and 10 wrapper tool events**.
Native scopes are `opencode` (traces version `2.0.29`); wrapper scope is
`promptfoo.providers` version `1.0.0`. No native metrics were emitted.
All 14 native batches and two wrapper batches were accepted live at 56010;
original native export imported once at 56008. No sampling/filtering or source
rewriting. The run's `handoff.md` records IDs, exact commands, usage, received
errors, span-volume assessment and propagation/tool-identity boundaries.

Native traces and logs are now opt-in for owned evaluation servers with
`--run-one --native-telemetry`. The isolated process receives
`OTEL_EXPORTER_OTLP_ENDPOINT` pointing to an owned byte-preserving relay, which
forwards to `http://127.0.0.1:56010`. Wrapper OTLP remains separately recorded.
The relay records original bodies, encoding, SHA-256 and delivery receipts without
filtering, deduplication or generated spans. API requests carry the actual
Promptfoo traceparent when available.

Official V2 config, CLI and troubleshooting docs were checked first. The installed
binary's help and embedded startup implementation establish the actual controls;
discovery evidence is in
`/Users/moya/.local/share/otel-desktop-viewer/evals/opencode-v2-telemetry-discovery-C4uDgz/`.

Scripted real-V2 results:

| Canary | Native spans | Native logs | Metrics | Delivery |
| --- | ---: | ---: | ---: | --- |
| `opencode-v2-canary-4kAYAN` (3 requests, 2 tools) | 611 | 13 | 0 | Captured locally to measure volume |
| `opencode-v2-canary-4yUopF` (one 429 request) | 374 | 13 | 0 | Accepted at 56010; counts verified at 56008 |

Both use native scope `opencode` (trace scope version `2.0.29`); wrapper spans and
wrapper tool events are zero in these direct-provider canaries. Full 429 bodies
survived with no model retry. The positive canary includes the expected protected
read denial. No raw batches were resent, filtered, or silently deduplicated.

Limitations established by source and canaries:

- No metric reader/exporter is wired by the V2 startup observability layer.
  Present library metric code does not make metrics export configurable.
- `--log-level` / `OPENCODE_LOG_LEVEL` control logs, not span selection. No exposed
  semantic span/scope/minimum-trace-level control was found. The 611-span canary
  includes 89 `Plugin.load` spans and many registry/snapshot operations.
- Each injected traceparent produced twelve direct HTTP server child spans, but
  zero session/tool-work descendants on that trace. Work stays on the server's
  native trace; no reparenting or invented links are applied.
- The two native `Tool.execute` spans have no call IDs; there are no `gen_ai.*`
  or usage attributes on any received spans. `SessionProjector.publishUsage` is
  a function span, not a usage measurement. Native errors do carry exceptions.
- Count wrapper `tool.execution` events using their actual native call IDs.
  Native `Tool.execute` names alone cannot establish identity or deduplication;
  do not add them to the tool total or match them by time/order.

The focused native mode is unsupported in this build. The user's subsequent
instruction explicitly authorized full capture before filtering or tail sampling.
One new bounded investigator/grader run completed with the following command;
do not repeat it as a retry:

```sh
OTEL_EVAL_OPENCODE_AUTH_FILE=/Users/moya/.local/share/otel-desktop-viewer/evals/pi-trial-20261021-SZzw5b/private-auth/auth.json \
OTEL_EVAL_OPENCODE_AUTH_ENTRY=openai-codex \
node evals/viewer-usability/opencode-v2-evaluate.mjs --run-one --native-telemetry
```

This command creates a new root; it must not be repeated as a retry. The ordinary
Promptfoo investigator and approved Pi grader remain unchanged. Import the new
original export once with `opencode-v2-verify.mjs <new-root> --import` after the
run; live OTLP must not be resent. The verifier checks native/wrapper identities
and counts separately, including the native records on independent traces.

Checks: all previous 27 evidence regressions plus 2 byte-preservation/propagation
tests passed (29 total). Scripted canaries used actual V2 with loopback responses,
not paid inference. Prior runs, credentials and shared service settings are intact.

## Future-run evidence corrections

The independent review fixes apply to new runs. Tool events retain native status;
only `completed` and `error` carry `tool.is_error`. Absent input/output is omitted,
while recorded empty values survive. Every V2 tool-state schema requires input;
missing input records a distinct `metadata.evidenceErrors` entry and fails the
attempt without changing native tool status or raw evidence.

The captured 2.0.29 `/openapi.json` at
`/Users/moya/.local/share/otel-desktop-viewer/evals/opencode-v2-discovery-6UIt2K/_openapi.json.json`
defines optional `Session.Message.Assistant.tokens` and required numeric fields
inside `TokenUsage.Info`: input, output, reasoning, cache.read and cache.write.
It does not define overlap/additive accounting, including cache writes. The
provider therefore omits `tokenUsage.total`. It publishes each field's sum only
when all assistant records supply that field. `metadata.usageAvailability`
records complete/partial/unavailable status and per-field coverage; received zero
remains zero. No usable fields means no provider `tokenUsage` property. Promptfoo
may still add defaults; verification checks the raw provider response separately.

SSE headers and body share the attempt deadline, caller cancellation and stream
shutdown signals. A stalled header request now rejects so attempt cleanup runs.
Future-run verification compares tool records with native context and checks
normalized IDs, status, input, output, outcome, duration and timestamp provenance
independently of the raw JSON attribute. The verifier requires the new availability
metadata; it is not a migration or rewrite tool for previous runs.

Offline regression command (fixtures plus loopback HTTP; no model calls):

```sh
node --test evals/viewer-usability/opencode-v2-evidence.test.mjs
```

The fixtures include deliberately incomplete/malformed records to test presence
semantics, zero/absent/partial usage, nonzero cache writes, altered normalized
attributes, and stalled SSE headers/body for each cancellation source.
The evaluation source snapshot now includes both new runtime helper modules.
Review-fix checks passed: 27 offline regression tests and syntax checks for all
12 `opencode-*.mjs` modules. These checks do not run the paid evaluator or write
to saved run roots.

## Completed real V2 pass

The explicitly authorized new attempt using the existing Pi OAuth credential
completed successfully in `opencode-v2-native-qeYG2j` under the persistent root.
It contains one V2 Sol investigator, nine native model contexts, twelve successful
viewer shell calls, and one separate Pi Sol rubric grader. Native score is `1`,
Pass. No retries or replacement providers were used. The previous 401 run remains
unchanged.

The original native export was imported once into backend 56008. Both OTLP batches
had already been accepted live at 56010 and were not retransmitted. Verification
found four correlated native Promptfoo spans and twelve rich V2 tool events on
the actual investigator parent. The stored result equals the source result, and
the export SHA-256 remains unchanged.

```sh
# Exact authorized real-run command used (creates a new paid attempt if repeated):
OTEL_EVAL_OPENCODE_AUTH_FILE=/Users/moya/.local/share/otel-desktop-viewer/evals/pi-trial-20261021-SZzw5b/private-auth/auth.json \
OTEL_EVAL_OPENCODE_AUTH_ENTRY=openai-codex \
node evals/viewer-usability/opencode-v2-evaluate.mjs --run-one

# Verification/import command already completed; do not resend this run:
node evals/viewer-usability/opencode-v2-verify.mjs \
  /Users/moya/.local/share/otel-desktop-viewer/evals/opencode-v2-native-qeYG2j --import
```

See the run's `handoff.md`, `received-data-verification.json`, and
`preview-verification.json` for native IDs, usage, hashes and abstraction findings.
At verification time the frontend counted Pi event markers only and recognized
zero of the twelve received V2 calls. The main frontend owner has since generalized
`tool.execution` recognition. The successful export also preserves two usage
limitations: Promptfoo defaulted detailed reasoning to zero despite V2's received
eleven reasoning tokens; the approved Pi grader omits usage in its ProviderResponse
despite recording it in raw events. Future V2 responses now pass through detailed
reasoning/cache fields explicitly. The historical export and Pi provider were not
modified.

## Actual V2 integration (subsequent authorized work)

The desktop application contains a separate real V2 binary:

```text
/Users/moya/Library/Application Support/ai.opencode.desktop/cli/2.0.29-arm64/opencode-cli
```

It reports `opencode v2.0.29`. `opencode-v2-provider.mjs` is an ordinary
Promptfoo custom provider using this binary's actual V2 HTTP API. It does not
use the incompatible built-in `opencode:sdk` provider described below. It starts
its own foreground `serve` process with fresh HOME/XDG directories and a macOS
filesystem sandbox, authenticates to that owned server with its generated
password, and stops/waits for it after evidence capture. It never discovers or
attaches to the user's shared service.

No package installation was required. The server's `/openapi.json` was saved
and inspected at `opencode-v2-discovery-6UIt2K` under the persistent evidence root.
The V2 credential API imports only the selected existing OpenAI OAuth entry into
the owned database. The original credential file remains unchanged.

The policy plugin uses V2 `setup(ctx)` hooks: retries are vetoed, automatic title
inference is suppressed, and skills are removed. Full request context and received
HTTP error bodies are retained privately. The actual executable recognizes
`OPENCODE_CONFIG_PROJECT_DISABLE`; the provider verifies effective config,
plugins, skills and MCP state before model submission. Native permissions allow
only workspace file tools and explicit viewer discovery/client commands.

Validation commands (scripted loopback inference only):

```sh
node evals/viewer-usability/opencode-v2-canary.mjs
node evals/viewer-usability/opencode-v2-canary.mjs --error
```

Canaries verified a real viewer help command, denied protected evidence access,
native session/tool IDs, full raw error bodies and exactly one HTTP request after
a synthetic 429. The final permission-policy canary is
`opencode-v2-canary-6JnTpb`; the error/no-retry canary is
`opencode-v2-canary-LBO6Ij`.

Explicit real one-question command:

```sh
node evals/viewer-usability/opencode-v2-evaluate.mjs --run-one
```

This creates a fresh `opencode-v2-native-*` directory, starts/verifies an owned
fixture, schedules one V2 Sol investigator and a separate Pi Sol native
`llm-rubric` grader, then preserves the full native Promptfoo export and raw
attempt evidence. `opencode-v2-grading-evidence.mjs` supplies the unchanged
approved question/rubric/shared instructions with V2 tool records. It does not
modify the Pi/Claude evidence loader. Provider tool records become rich events
on the existing Promptfoo parent span during that actual provider call; there
are no fabricated child spans or evidence replay. OTLP receipts are retained
before forwarding to the existing observer on port 56010.

Each real run writes `handoff.md`, `result-summary.json` when available,
`runner-error.json` on blockers, and source-preservation fingerprints. A failed
investigator remains a native execution error and does not trigger a grader.
This command is a paid run, not a preparation command; each invocation is a new
attempt. Read the previous handoff before deciding to invoke it.

The one authorized real attempt is preserved in `opencode-v2-native-lTw5Rv`.
It stopped with native `provider.auth: Request failed: 401` during credential
resolution, before the model-context hook or any tool call. No grader ran and
the attempt was not retried. The selected original OpenCode OAuth entry expired
on 2026-10-17. The separately selected Pi `openai-codex` entry expires on
2026-10-27 and was not substituted automatically. A new attempt can explicitly
select that existing source with `OTEL_EVAL_OPENCODE_AUTH_FILE` and
`OTEL_EVAL_OPENCODE_AUTH_ENTRY=openai-codex`, after deciding to proceed past this
recorded access blocker. See the run's `handoff.md` for the exact command.

## Initial finding: built-in provider blocked on V2 compatibility

Checked 2026-10-22. The installed native Promptfoo provider cannot run the
documented OpenCode V2 API. No paid investigations or model-access requests were
made. The preparation script has no live-evaluation mode.

| Component | Observed | Consequence |
| --- | --- | --- |
| CLI | `/opt/homebrew/Cellar/opencode/1.18.10/bin/opencode` | V1, not V2 |
| Promptfoo | `0.124.1` | Pinned installed version; also npm's current latest when checked |
| Native provider | `opencode:sdk` | Uses `@opencode-ai/sdk/v2`, then falls back to the package's default entrypoint |
| SDK | `@opencode-ai/sdk@1.18.35` | Its `/v2` entrypoint still uses V1 product routes and response shapes |
| Documented V2 client | `@opencode/client` | Not installed in this suite; npm latest was `2.1.2` |
| Documented V2 plugin package | `@opencode/plugin` | Not installed in this suite; npm latest was `2.1.2` |
| Existing auth | Private OpenCode `auth.json` has an `openai` OAuth entry | Presence only; token validity and isolated V2 model access are untested |

The SDK entrypoint name is not the OpenCode product generation. Its generated
client creates sessions at `POST /session` and prompts at
`POST /session/{sessionID}/message`. Documented V2 uses `POST /api/session` and
`POST /api/session/{sessionID}/prompt`. V2 prompt admission returns an inbox item;
the caller must wait and retrieve session context to obtain the completed answer.
Changing a URL prefix or updating the CLI alone cannot repair the contract.

The installed implementation can be inspected in
`node_modules/promptfoo/dist/src/opencode-sdk-BAV4ym_Q.js`:

- `loadOpenCodeSDK`: lines 265–303, legacy SDK imports and fallback.
- `buildServerEnv`: lines 458–492, copies the parent environment, then overlays
  supplied values. `copy_working_dir` clears Git selectors; it does not isolate
  HOME, credentials, automatic context or filesystem access.
- `ensureClient`: lines 644–697, legacy startup and authentication contract.
- `buildCreateSessionParameters` / `buildPromptParameters`: lines 768–795.
- `buildProviderResponse`: lines 897–916, final-message text, tokens and cost.
  `info.error` is not mapped to `ProviderResponse.error`.
- `callApi`: lines 954–1052, deletes ephemeral sessions before returning.
  Intermediate history is fetched only for enabled skill tracking, and only
  derived skill calls are returned. Full tool evidence is not retained.

## Reproducible preparation

From the repository root:

```sh
# Inventory and one-question design; no model/server startup.
node evals/viewer-usability/opencode-native-preflight.mjs

# Same preparation plus real Promptfoo/native-provider calls to a synthetic
# loopback HTTP server. No OpenCode process or inference service is started.
node evals/viewer-usability/opencode-native-preflight.mjs --probe
```

Each invocation creates a new private directory under
`/Users/moya/.local/share/otel-desktop-viewer/evals/opencode-native-prep-*`.
Optional inputs are `OTEL_EVAL_OPENCODE_PREP_PARENT`,
`OTEL_EVAL_OPENCODE_BINARY`, and `OTEL_EVAL_OPENCODE_AUTH_FILE`. The selected
auth file is read only to report OpenAI credential presence/type; credentials
are neither copied into the probe nor printed. The script pins Promptfoo
`0.124.1` and SDK `1.18.35`; version changes require reinspection.

Exit success means preparation completed, not that V2 is ready. Read the
`status` and `blockers` in `compatibility.json`. The generated
`one-question-plan.json` is a design record, not a runnable paid config.

The completed probe is retained at:

```text
/Users/moya/.local/share/otel-desktop-viewer/evals/opencode-native-prep-a6rA3F
```

It confirms:

- Normal `evaluate()` scheduling: one test, one provider, repeat one, concurrency
  one, no cache; native `llm-rubric` invokes a separate native OpenCode provider.
- Two distinct synthetic session IDs; the approved question is the investigator's
  first line and the rubric is confined to the grader.
- A full native export with `evalId`, `config`, one result row, and the native
  component assertion. A synthetic Partial remains score `0.5`, success `false`.
- Actual requests use the legacy `/session` routes.
- A third direct diagnostic call returns a synthetic assistant `info.error`.
  Promptfoo returns its text without a top-level error. This confirms an execution
  error can be mistaken for a valid answer unless the provider boundary is fixed.

`synthetic-promptfoo.json`, `synthetic-rows.jsonl`, and the HTTP request log are
synthetic compatibility evidence, not viewer-investigation results. The probe
does not export telemetry to the shared observer. Source fingerprints before and
after confirm that approved task/rubric/prompt/grading files and the original
six-run Promptfoo exports and run plan were unchanged.

## Minimal eventual live pass

Use one ordinary provider entry, one `checkout-failure` test and one separate
native `llm-rubric` grader. Proposed model for both roles:
`openai/gpt-6.1-sol`, each in its own clean profile/session. Keep
`repeat: 1`, `maxConcurrency: 1`, `cache: false`, `share: false`.

Load the exact question from `tasks.json`, the investigator context from
`prompt.txt`, and the criterion/reference from `rubrics.json`. Keep the approved
shared grading instructions and native Fail/Partial/Pass values `0/0.5/1`.
Promptfoo owns scheduling, grading, result identities and full exports. Retain
`promptfoo.json` and `promptfoo-rows.jsonl`; do not rewrite exported rows or use
the legacy echo-provider replacement/expanded-slot runner.

The investigator needs a fresh external workspace containing only
`bin/otel-desktop-viewer`. Start and verify a new owned fixture viewer with the
existing `stageDataset`, `prepare`, and `verify` helpers. Keep its owner alive
until evaluation completes, then stop and wait for that owned process. The old
six-run fixture was shut down; its saved `connection.json` endpoint is historical.

The shared observer is separate:

- backend: `http://127.0.0.1:56008`
- OTLP HTTP: `http://127.0.0.1:56010` (`/v1/traces` for traces)
- preview: `http://127.0.0.1:56007`

All three ports were listening during inspection. Observer and preview are not
the investigation fixture. Keep the original
`native-otel-six-gLWzRd` directory read-only.

## Exact needs before a paid pass

1. **An isolated V2 executable and compatible native Promptfoo provider.** Supply
   an absolute V2 binary path and verify its version. Promptfoo's current latest
   native provider still requires a V2 port: `@opencode/client`, explicit owned
   service/authentication, create/prompt/wait/context/interrupt/remove semantics,
   complete usage/evidence capture, and execution-error propagation. A config-only
   change cannot provide these. Keep the requested ordinary native provider path;
   resolve this upstream dependency/scope before implementing a substitute.
2. **A V2-verified clean profile and permission policy.** The current
   `clean-environment.ts` pins `1.18.10`, writes V1 auth/config and a
   version-specific plugin dependency skip. Its `OPENCODE_*` isolation flags are
   not proof of V2 behavior. Verify system/managed configuration, every ancestor
   directory, disabled context discovery, workspace-only access, allowed viewer
   commands and denied delegation. Use the selected existing OAuth login
   privately; verify V2's credential migration/import in a disposable profile.
   A subsequent authorized access smoke must establish actual model access.
3. **V2 telemetry and evidence capture.** `native-telemetry-plugin.mjs` is a V1
   function returning event/tool/shell hooks. V2 requires `Plugin.define`,
   `setup(ctx)`, `ctx.event.subscribe`, `ctx.tool.hook`, and
   `ctx.shell.hook('create.before')`. The legacy
   `experimental.openTelemetry` config field is explicitly unsupported in V2.
   Verify trace parenting and flushing against actual V2, rather than treating
   that field as an exporter switch. The documented shell hook does not expose
   the legacy `sessionID`/`callID` fields: correlation needs a demonstrated V2
   mechanism, not a name-for-name port. Capture all tool successes/errors and
   associate investigator/grader spans with the existing Promptfoo parents.
4. **OpenCode-specific rubric evidence adapter.** The current
   `grading-evidence.ts` intentionally accepts only `pi` and `claude-code`
   identities. Add a new OpenCode-only loader that validates owned evidence,
   saved answer, session identity and raw tool records before supplying the
   unchanged rubric/shared instructions to native `llm-rubric`. Grader tools
   must be disabled. Provider errors, permission blockers, timeout/incomplete
   sessions and malformed grader output must remain execution errors.

Validate these boundaries with scripted local responses before the first paid
question. The current synthetic probe validates the installed Promptfoo contract,
not V2 runtime isolation, model access or telemetry.

## V2 sources

- <https://opencode.ai/v2/docs/migrate-v1>
- <https://opencode.ai/v2/docs/build/client>
- <https://opencode.ai/v2/docs/api>
- <https://opencode.ai/v2/docs/config>
- <https://opencode.ai/v2/docs/cli>
- <https://opencode.ai/v2/docs/build/plugins>
- <https://opencode.ai/v2/docs/build/plugins/migrate-v1>

The V2 migration guide identifies server/client and plugin API breaks explicitly.
The public `config.json` schema was not used to infer V2 configuration fields.
