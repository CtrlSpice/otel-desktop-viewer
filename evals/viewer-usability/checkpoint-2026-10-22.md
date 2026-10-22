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
- `native-otel-six-gLWzRd/observer/telemetry.duckdb`: current preview database.

Native OpenCode telemetry is now visible in the collector. The inspected
`cli.web-ui.handler` span has resource evidence ID
`investigator-opencode-v2-D6MM9N`, native scope `opencode` 2.0.29 and SDK
`@effect/opentelemetry`. A newer 1,252-span OpenCode trace also appeared in the
list. The native telemetry worker is completing its run-level accounting and
handoff; these observations alone do not establish the new assessment outcome.

## Remaining work and boundaries

- Review and save the native V2 exporter/provider worker's final changes and
  exact run report separately. Capture full telemetry first; filtering and tail
  sampling are deferred. Do not launch the 132-investigation comparison.
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
