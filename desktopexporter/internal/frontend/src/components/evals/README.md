# Evals workspace

Evals is experimental and unsupported. It requires an `EVALS=1` build and an
enabled local `evals.enabled` flag. Public `EVALS=0` builds omit the workspace and
its HTTP handlers; a runtime flag cannot add omitted code.

The build and capability contract is documented in `docs/experimental-features.md`.
Supported input, exact source preservation, database indexing and search are
documented in `docs/eval-ingestion.md`.

## Data and navigation

The global file picker/drop queue imports complete Promptfoo v3 envelopes through
the Evals ingestion endpoint. Experiment cards read database-wide summaries; the
workspace loads source results for the selected experiment. The shared database
retains the processed source evidence and rebuildable indexes.

`model.ts` is a presentation model, not an import contract. Experiment
configuration, assessment names, score scales and overall judgments come from
received data. Opening the workspace does not launch a model or grading job.
Synthetic examples in `examples.ts` support development and tests; they are not
the database storage contract or a completed usability cohort.

Experiment/result selection and search are URL-backed. Browser history restores
selection. Disclosures, pane geometry and column widths retain their own state.
The workspace reuses shared navigation, signal cards, field cards, resizable
panels and tree controls.

## Results and evidence

The normalized hierarchy is **Experiment → Test case → Result**. Native result
identities remain intact. Results have no displayed ordinal; ordering uses
available start times, with stable source order for ties and unavailable times.
A singleton without repetition metadata displays its evidence on the test case.

Execution and grading are roles, not mandatory wrapper rows. Explicit execution
spans and recorded tool/grader evidence can supply children. Assessments alone
do not manufacture grading spans, parentage or execution timing. A failed
assessment remains distinct from an execution error.

Results, Tool calls and Cost form a single-select view control. Selection,
disclosure and section sizes persist across views. Tool calls and Cost are
unavailable when the experiment has no corresponding recorded data; known zero
measurements remain available. Missing measurements are not inferred as zero.

Received trace/span references connect evidence to exact stored telemetry.
Telemetry cards open the trace workspace; Evals does not embed a second span
inspector. Tree disclosures and row selection remain independent.

Timing prefers referenced spans, then available child timing extents, then
recorded durations. Child durations are not summed into an elapsed window.
Duration-only records do not acquire an invented start offset or trace reference.
Missing timing stays unavailable. The timeline uses shared waterfall geometry,
service/span colours and error indicators.

Costs are presentation-only API estimates in CAD, with dated pricing and exchange
rate provenance. Investigator and grader costs stay separate. Unknown and partial
estimates are explicit, and parent estimates are not added again to child totals.

## Details

Details shows the selected experiment, case, result or evidence row. Output,
assessments, source context and Telemetry retain their own sections. A received
overall assessment appears as Overall; independent judgments keep their names,
scores, explanations and provenance. Copy and raw source inspection preserve
received values and types.

Experiment Configuration and Grading are sibling, initially collapsed sections.
The shared Details stack keeps section identity and disclosure during navigation
and resizing. Plain text, small meaningful dots and regular Atkinson typography
match the other signal workspaces.

## Local use

From the repository root:

```sh
make build EVALS=1
./otel-desktop-viewer --feature-flags-file ./feature-flags.json
```

Set the local `evals.enabled` flag's `defaultVariant` to `"on"`; the repository
configuration defaults it to off. Then open `/evals` and import supported results.

For hot-reload development, run this from `desktopexporter/internal/frontend`
against that enabled backend:

```sh
EVALS=1 VIEWER_PROXY_TARGET=http://127.0.0.1:8000 npm run dev
```

Use the backend's configured browser port if it differs from 8000. Vite proxies
capabilities, RPC, ingestion and export to the existing backend.

## Checks

Run frontend checks with `EVALS=1` to include the Evals browser suites:
`e2e/evals.spec.ts`, `e2e/eval-source-copy.spec.ts` and
`e2e/eval-mounted-state.spec.ts`. They cover gating, source fidelity, navigation,
selection/disclosure independence and mounted state. Rebuild enabled assets with
`make build-ts EVALS=1`; committed embedded assets remain the public build.
