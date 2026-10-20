# Evals workspace

The `/evals` frontend is a fixture-backed workspace for developing the Evals
experience before implementing ingestion and storage. It uses the existing
drawer, resizable panels, headers, field groups and trace viewer.
App-wide navigation, search and controls live above the drawer in
`shared/Toolbar/WorkspaceToolbar.svelte`. Detail field cards reuse
`shared/DetailProperty.svelte` for logs, traces and Evals. The server's
`/api/features` capability gates navigation and loading; Evals remains off by
default and is labelled experimental and unsupported.

`examples.ts` supplies clearly labelled, synthetic runs, including a completed
run, a paused investigator, missing assessments and unstarted work. Each of the
eight cases has one, two or three repetitions (16 attempts per run). Attempts retain a shared
case ID and distinct attempt/telemetry IDs; repetition 1 keeps the original URLs.
These are
not the completed viewer-usability cohort and do not establish a new rubric.
No model calls, launch controls or persistence writes occur when opening Evals.

`model.ts` is a presentation model, not an import contract. Run configuration,
assessment names, score scales, headline assessment and aggregation rules belong
to the supplied data. Storage integration should replace the fixture source;
the **Search evals** control currently filters only loaded fixture attempts and
their assessment text. It does not execute the viewer's DuckDB search language.

Run/attempt selection and the result filter are URL-backed. Browser history
returns to the selected run/attempt; detail disclosures are local UI state and
existing panel widths remain resizable. Column headings sort the loaded attempts
locally in ascending or descending order.
Scores sort by their numeric value, not outcome labels. Cases stay together,
ranked by their first sorted visible attempt; repetitions sort within each case.
Missing measurements stay last within each case in both directions; cases with
no known measurements come last. Ties preserve source order and sorting preserves selection.
Columns use the shared resize geometry, drag lifecycle and handles. Widths are
saved by column ID. Handles follow the visible table bounds, including
the footer, and stop before unused space below the last row.

The main pane has one table. Its Columns picker is shared with the waterfall and
groups options under Case, Results, Time and Cost. The table has one regular-weight
heading row. Group checkboxes toggle all optional columns in that group, including
options hidden by picker search; required columns remain selected. The same picker
serves Evals, Logs and the trace waterfall. All columns start visible; Case and
Rep stay visible to identify each execution. Column choices persist across reloads.
Each case name shares its parent row with aggregate values and wraps when needed. It branches to its repetition rows, including cases
with only one repetition. Branches reuse the waterfall's rounded 1px Muted
connectors at 50% opacity. Repetition numbers select attempts; all values stay
under their column headings. Case completion uses every attempt, including
attempts hidden by the current filter.
Case and Rep share one `Case · Rep` heading, with independently sortable labels.
Their compact heading slots and branches are measured from rendered labels,
padding and shared tree spacing. The fixed combined identity area leaves room
for case names; saved data-column widths do not stretch it. Rep numbers use Subtle.
Case and Rep pin to the left during horizontal scrolling, including their heading,
connectors and the Total label. Case titles wrap in the combined identity area.
As in Logs, pinning releases when the pane cannot fit
the pinned columns plus one complete scrolling column. Saved widths and shared
hover/selection motion remain intact.
Assessment labels/dots align left and numeric scores align right, duration is plain text, and tool
counts use `17 (1 failure)`. Cost keeps
investigator and grader estimates separate. The Total footer aligns each estimate
under its visible cost column, with unknown and partial estimates explicit. Its
top divider is a solid 1px Muted line at 60% opacity. Hiding both cost
columns hides the cost footer.
Totals cover the shown attempts; drawer summaries cover the whole run. Hiding the
sorted column temporarily restores source order instead of sorting invisibly.

Run cards show a proportional breakdown and every outcome in a visible list,
with neutral labels, small colour dots and exact attempt counts. Neutral outcome
fills use base-300. Completion dots use base-300 for Planned, Iris for in-progress,
Pine for Complete and Papaya for execution errors or interrupted runs. Assessment
success dots and bars use Foam, independently of completion. Run statuses use
neutral text and small status dots. Planned
cards show only their title and `Planned (count)` status. Completed counts are omitted only
when the run is complete and every attempt completed; graded counts appear only
when they differ from completed counts. The labelled score is not a pass rate.
Clicking the card selects the run and opens Details. Tiny bar groups have an 8px
visual minimum. Outcome grouping and the headline assessment mean remain separate.
Run metadata uses the shared working text size and continuous card rows, with
regular-weight scores. Metadata wraps when needed instead of shrinking text.
Assessments use neutral text with small decorative outcome dots;
counts are plain text. Tables, Details and summaries use the shared 14px working
size, regular weight and row spacing. Colour is never the only status label.

Details separate the question from the selected execution. Question and answer
are prose with Copy actions; repetition and execution state appear once in the
attempt heading. Timing, accounting and telemetry are under Execution details.
Assessment explanations precede optional grading metadata; execution errors stay
visible beside the answer.

This grouping follows the kit's emitters: `tasks.json` and `rubrics.json` define
task context, `config.ts` expands task/model combinations, and `native-evaluate.ts`
preserves original `testIdx` and zero-based `repeatIndex` per scheduled execution.
`provider.ts` emits each execution's answer, commands, timing, usage and session
metadata; `grade.ts` records separate grader evidence and criteria. A repeated
question does not make answers, explanations, usage or telemetry shared. Rubric
and evaluator provenance remain attached to each assessment; do not deduplicate
different versions by their display name. The illustrative model still flattens
question context onto each attempt and is not a source identity or import schema.

The standalone saved-results report remains a separate presentation. Its
Trace / Span links target this viewer's existing trace routes; the report's
score-column positions do not define Evals run identities. Import, storage-backed
search and launch behavior are still proposed in
`notes/evals-observer-integration-plan.md`.

## Try it

1. Start a local viewer with an explicit flag file enabling `evals.enabled`.
2. Open `/evals`, or start Vite with `VIEWER_PROXY_TARGET=http://127.0.0.1:PORT`
   pointing to that viewer. Vite proxies feature capabilities, RPC and export.
3. Optionally load matching synthetic investigator, tool and grader spans into
   that viewer's OTLP HTTP receiver:

   ```sh
   node scripts/seed-eval-examples.mjs http://127.0.0.1:OTLP_PORT
   ```

Use a development instance for the examples. They are labelled with the
`eval-ui-examples` service and `eval.example=true`. Without seeding, their trace
references have no corresponding telemetry in the viewer. The example timestamps
are on 16 October 2026; use an appropriate time window when browsing signals.

## Checks

`e2e/evals.spec.ts` exercises gating, selection/history, separate assessments,
filters, unknown costs, and desktop/mobile accessibility in Pine and Dawn.
Run the normal frontend checks and rebuild embedded assets after changes.

# Case totals

Each case has a collapsible parent row with its name, plain repetition count,
and normal-Text aggregates aligned with the visible data columns. The circle/count
gutter toggle (shared with the waterfall) opens its repetitions with pointer,
Enter or Space without changing
attempt selection. Expanded repetition values use Subtle. Case and Rep headings
stay together, with compact branches and repetition numbers beneath them;
the wider combined identity area accommodates the parent name. Each assessment uses its own percentage and declared
planned/scored denominator. Duration sums recorded repetition durations; calls
include failures; costs remain estimates. Missing measurements stay Unknown or
explicitly partial. Case totals include all repetitions, even when some are
filtered out, matching the case completion state; their accessible label and
tooltips identify this scope.
