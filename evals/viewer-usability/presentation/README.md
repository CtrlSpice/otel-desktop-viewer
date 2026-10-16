# Compact results preview

Generate a local HTML page and its sibling `OUTPUT.html.assets/` directory from
a selected saved run. Keep them together when moving the report. No server,
model calls or external network requests are required. Output must be outside
tracked source; generation refuses to overwrite existing output or assets.

```sh
node presentation/build.ts /absolute/saved-run /absolute/local-preview.html
node presentation/build.ts /absolute/saved-run /absolute/reviewed-preview.html /absolute/review-directory
node --test presentation/aggregate.test.ts
EVAL_PRESENTATION_RUN=/absolute/saved-run node --test presentation/aggregate.test.ts
npm run typecheck:presentation
node presentation/check-browser.ts /absolute/local-preview.html
```

The browser check reuses installed frontend Playwright and axe tooling, with no
dependency installation or viewer lifecycle changes.
Dark mode uses the app's Rosé Pine OG colours; light mode uses its Dawn colours.
The build reuses the viewer's bundled component stylesheet and Hugeicons glyphs
for its cards, controls, warning and disclosure carets. Metric controls share the
content background, with normal text colour marking the selected metric.
The second-column header contains the Results, Time and Cost selectors in every
view, including mobile. Icon-and-label controls use subtle vertical separators
and normal text colour for the selected choice in both themes. The Cost selector uses a coins icon.
Cost labels include `(est., CAD)`, except the compact `Cost/run` value heading:
the amounts already carry their currency label. The selectable `Cost (est., CAD)`
label stays separate from that aligned per-run heading.
Controls group the model rows into Results, Time and Cost. Provider and full model
ID appear on separate lines. Results show grouped outcome segments with labelled
counts and percentages; the model mean remains separate from that distribution.
Time bars share one scale across investigators and graders: zero to median uses
the theme's success colour, median to P95 uses warning, and P95 to the slowest
recorded attempt uses error. These segments are duration ranges, not proportions
of attempts. Labels show median, P95 and slowest; total time is a separate value.
The build copies the viewer's existing Atkinson Hyperlegible Next font files into
the report's asset directory. CSS and browser scripts are separate files, and the
HTML is formatted for inspection.
Browser checks cover both themes on desktop and mobile.

### Template source

- `components/Results.svelte` is the typed page template. Model rows, task cards,
  grader rows, grading tables and metric displays are separate Svelte components.
- `components/model.ts` shares the existing presentation types and pure formatting
  and grading helpers. Svelte escapes displayed values; raw run records stay in
  the evaluation evidence.
- `results.css` contains the consolidated layout rules and uses the viewer's
  theme tokens. `viewer-components.ts` reads the viewer's built stylesheet.
- `theme.ts` and `interactions.ts` provide system-theme selection, metric selection
  and table disclosure for the static export.
- `render.ts` compiles the Svelte templates once per Node process with the viewer's
  installed Vite/Svelte tooling, then renders static HTML. `build.ts` writes the
  formatted page and assets. The export needs no Svelte runtime or hydration data.

The page includes only allowlisted model/task labels, counts, coverage, timing,
usage and estimated costs. Answers, full prompts, rubric text, grader explanations,
tool evidence, private paths and configuration are never serialized into it.
Model names come from saved pricing/summary metadata. No generation badge is
inferred from names. Task titles use the exact approved questions in `tasks.json`,
not untrusted provider prompts or answer text. Titles do not change investigations.

Recorded complete/partial/wrong/error counts retain every pilot attempt. Scores
come from the current saved grading result, not inferred from old pass/fail labels.
Task cards use the same coloured result bar for one or many attempts, followed by
text-labelled counts and percentages. Each percentage is the category count divided
by all recorded attempts, displayed with at most one decimal place; it describes
the distribution of outcomes, not the mean score. Aligned metric rows replace
the nested run table. Task-card dividers keep their spacing without a visible line.
Errors stay labelled as errors, not zero scores. Errors with recorded
`harnessBlocker: true` appear as **Harness
error** in papaya orange (`#FF8000`) in both themes; they remain unscored and
included in the error total. Execution errors also use papaya orange in both themes.
Unrecorded artifact checks and human-review fields are omitted.
Task cards show failed tool calls and execution errors in separate rows, including
zero counts. Failed tool calls count unsuccessful tool operations even when the
investigation recovers; execution errors count unscored attempts, including harness
errors. Nonzero execution-error counts use papaya orange. Missing tool-call counts
remain unknown rather than becoming zero.
Human review directories are read through `showReview`, which validates evidence
and revision history. Original automated scores and human-signed scores stay
separate. Displayed results and means use signed corrections where available,
with review coverage and original-to-signed corrections visible; incomplete planned
runs remain provisional.
Final three-run results require three signed-off attempts. Errors stay unscored.
Human claim counts
reflect explicitly reviewed claims, not exhaustive automated detection or a numeric
hallucination score. Missing review data is pending, never a clean bill.
Model order follows run metadata. Coverage uses manifest task/repetition counts;
when the manifest supplies only a task count, wholly absent tasks cannot be named.

Saved investigator and grader totals remain separate. Task usage comes from
available pilot metadata; saved investigator totals may include sessions omitted
by Promptfoo after grading execution errors. Timing uses the existing median
helper and becomes unknown for incomplete timing coverage. Total is the sum of
recorded `elapsedSeconds` values; slowest is their maximum, both in seconds with
binary64 precision. They include recorded error attempts and become unknown when
any duration is missing. Total is accumulated session time, not batch wall time;
investigator and grader time stay separate. Continuous p95 sorts
recorded elapsed seconds and interpolates at `(n - 1) * 0.95`, matching
`quantile_cont`. It uses binary64 precision and requires complete timing coverage.
Small samples give only a rough tail-latency indicator. Per-task cost is the
total estimated CAD cost across its recorded repetitions. Model cost/run is
saved investigator cost divided by investigator sessions, shown only for complete
estimates. Prices and exchange rates come from the run snapshot; these are not billed costs.
Output token counts include reasoning; tool calls are not labelled as turns.

The Grader summary shows grading work separately from investigator performance.
Its summary metrics stay on one line and scroll horizontally on narrow screens. Its table
and three-colour bar retain original grades, with counts for `1/1`, `.5/1` and
`0/1`; execution errors and unavailable outputs remain separate. The table
shows original automated outputs, recorded time, tokens and estimated cost without
grader explanations, evidence or rubric text. Human corrections do not rewrite
the original grading output.

Each grader has one flat grading table with a collapsible section per investigator
model. Each section has its model name in the first column header, one row per question
and a separate column for each run's original grade. Columns cover the planned
repetition count and extend if more outputs were recorded; missing outputs show
a dash, never zero. Score column numbers are display order within each question, not stored
planned-repetition IDs. The table scrolls horizontally. Time and token columns sum
the recorded grading outputs for that question; missing values remain unknown.
Cost sums known estimates and marks incomplete coverage as partial or unknown.
There are no nested tables or expandable question rows. Each model section starts
with only its header and totals row visible; clicking anywhere in its header
toggles the question rows. Its model-name button provides keyboard access.
Carets sit to the left of model names in both investigator rows and table
headers. The totals row remains visible and uses the normal body-row styling.
Headers use the same soft theme background as task-card headers, with rounded
outer corners and carets in the header-label muted colour. Sections are
continuous, with no gaps or divider lines between rows or model sections.
Each score column sums recorded numeric points out of the manifest's
total question count, for example `10.5/11`; it is not an average. Each model
header shows a status indicator immediately beside its name: a coloured dot and
label in the header-label muted colour, on a soft pill using the active theme's
panel background. There is no separate Status label:
Not Started, In Progress,
Completed or Needs Attention. Needs Attention uses papaya and marks recorded execution
errors; a valid zero score is still a completed grade. There is no separate
Status column or row, and no status badges on question or Total rows. Status describes saved
grading progress, not live process health. Pending score totals display zero
earned points without creating stored grades; individual missing grades remain
unscored. Time, cost and token totals cover recorded
grading work for that model.
Partial-score labels display `.5/1`; stored scores remain numeric `0.5`.
All sections share one column layout: the question column takes the remaining
width, run columns have equal widths, and numeric columns have equal widths.
The same run columns appear in every model section for a grader. Summary rows,
task cards and tables use shared horizontal spacing; narrow tables scroll.

This is a local presentation candidate, not a hosting/integration decision.

### Experimental saved-results updates

`serve.ts`, `live-data.ts` and `live-client.ts` preserve the in-progress local
results-server prototype. It watches selected saved result files and updates the
page while retaining the selected metric, expanded sections, focus and scroll.
It binds to loopback and serves aggregate presentation fields only.

This prototype is unfinished: recoveries appear separately rather than replacing
the approved failed attempts in the main table, and its status comes from saved
files rather than process health. User-cancellation markers are not yet handled.
Use the static builder for reviewed results. The prototype is not started by the
evaluation workflow.
