# Evals checkpoint — 2026-10-22

Branch: `fix/eval-terminology`. User authorized saving, committing and pushing this checkpoint.

## Viewer changes

- Evals timeline uses shared activity markers for received events, owned logs,
  uniquely call-ID-correlated ancestor logs, and assessment annotations.
- Correlations are viewer-only `Eval annotation` records. Assessment markers
  anchor to recorded span end; that is not an assessment timestamp.
- Stored reconstructed events are labelled `Reconstructed Pi annotation` or
  `Reconstructed OpenCode annotation`. Recorded tool names title cards and
  popovers; `tool.execution` remains a typed field. Explicit tool errors use
  neutral text. Event Recorded rows show offsets with full dates on hover.
- Tool counts appear only on owning rows. The saved V2 provider shows 12 calls.
- Details resizing uses DOM order rather than registration order. Assessments
  follows the pointer without moving the Telemetry boundary. Obsolete fallback
  layout styles were removed. Pointer interruption and keyboard resizing retain
  measured geometry.
- Initial JSON previews open an array's first container item one extra level,
  retaining deeper/later folds, manual collapse and the original editor text.
- Cost headings identify API estimates in CAD. Missing usage/prices/FX remain
  Unknown; no source usage or producer cost was rewritten.

## Verification

- Frontend type check: zero errors/warnings; lint passed.
- 305 focused Evals, Waterfall, trace Details, JSON and DetailStack tests passed.
- `make build-ts` passed; embedded assets rebuilt.
- Headless browser activity checks passed for Pi, Claude and V2: marker geometry,
  provenance, assessment selection and exact event/log navigation, no page errors.
- Details worker verified real pointer drags in Dawn/Pine/Moon, with normal and
  reduced motion, plus reversal, blur cleanup, saved sizing, keyboard controls,
  unavailable ordering and retained mounted nodes. Its focused 36 tests and
  existing drag/collapse and mounted-selection browser tests passed.
- Full repository release gate is not established by these focused checks.

## Preserved evidence

Evidence lives outside Git under `~/.local/share/otel-desktop-viewer/evals/`:

- `pi-parent-events-NnaB1g/`: Sol and Luna passed; 13 reconstructed tool events,
  including Luna's recorded failed tool call, and eight native Promptfoo spans.
  The early five-span observer report remains with its batching explanation.
- `opencode-v2-native-lTw5Rv/`: original V2 authentication failure, preserved.
- `opencode-v2-native-qeYG2j/`: successful original V2 export, 12 tool calls,
  four Promptfoo spans and separate Pi grader. No retries rewrote the failure.
- `opencode-v2-native-eQUlOk/`: completed full native-telemetry run, Pass 1/1,
  eight model requests and ten successful tools; separate Pi Sol grader.
  Captured 1,268 native spans and 27 logs, plus four wrapper spans and ten tool
  events. All original uploads were accepted; the original export was imported
  once. Owned server, fixture and relays were stopped.
- `native-otel-six-gLWzRd/observer/telemetry.duckdb`: current preview database.

Native OpenCode telemetry is now verified in the collector. The inspected canary
`cli.web-ui.handler` span has resource evidence ID
`investigator-opencode-v2-D6MM9N`, native scope `opencode` 2.0.29 and SDK
`@effect/opentelemetry`. The completed real run has 1,252 spans on native trace
`580bc0cc7f23c6a5bda4bee569df489f`; 13 native HTTP spans join the four wrapper
spans on Promptfoo trace `26a114661a4dd402002167a46b958cf4`; three native spans
have independent traces. Its experiment is
`76c7a9ec8e0cf2aaf06af0b574e466436b3de800fbc6ecab02acc6ce45816100`.

Native `Tool.execute` spans lack call IDs; they cannot be matched to wrapper
events by order/time or counted as extra calls. Native HTTP spans propagate
traceparent, but session work stays on its separate recorded trace. Received
session headers permit cross-trace correlation without inventing ancestry.
No native metrics exporter or usage attributes were present in this V2 build.
Two native ENOENT discovery errors remain alongside successful tools and Pass.
Full bytes, IDs, usage and source-preservation hashes are documented in the
run's `handoff.md` and `opencode-native-prep.md`.

The V2 implementation was reviewed; 29 offline evidence/usage/SSE/relay tests
and all 15 module syntax checks passed. Missing tool fields stay missing,
recorded zero usage survives, unavailable usage does not become zero, and SSE
headers/body share cancellation and deadlines. No paid runs were repeated.

## Remaining work and boundaries

- Native V2 exporter/provider code and run report are saved separately from the
  viewer checkpoint (`69e78b67`). Filtering and tail sampling are deferred.
  Do not launch the 132-investigation comparison.
- Details worker observed that double-clicking bottom Telemetry after Home/End
  under reduced motion can miss the second click when disclosure moves the
  header. Assessments double-click reset passes; bottom-header reset needs follow-up.
- Production CAD recomputation still needs explicit pricing/FX provenance and
  trustworthy per-response usage; original exports must remain intact.
- Telemetry-driven Evals indexing, shared saved/live projection and opt-in
  refresh orchestration remain unfinished. Arrival must not navigate or refresh
  the user's current selection automatically.
- The original preview checkout is older and requires a new compatible database
  before upgrade; preserve its data and untracked `HANDOFF.md`.

Current preview: `http://127.0.0.1:56007`; backend 56008; OTLP gRPC 56009,
HTTP 56010. Keep the user on Vite hot reload. Local replay scripts beside this
checkout: `verify-eval-activity.mjs` and `verify-detail-drag.mjs`.
