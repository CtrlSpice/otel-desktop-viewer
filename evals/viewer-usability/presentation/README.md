# Compact results preview

Generate a local, self-contained HTML page from a selected saved run. No server,
model calls or network requests are required. Output must be outside tracked
source; generation refuses to overwrite an existing file.

```sh
node presentation/build.ts /absolute/saved-run /absolute/local-preview.html
node presentation/build.ts /absolute/saved-run /absolute/reviewed-preview.html /absolute/review-directory
node --test presentation/aggregate.test.ts
EVAL_PRESENTATION_RUN=/absolute/saved-run node --test presentation/aggregate.test.ts
node ../../desktopexporter/internal/frontend/node_modules/typescript/bin/tsc -p presentation/tsconfig.json
node presentation/check-browser.ts /absolute/local-preview.html
```

The browser check reuses installed frontend Playwright and axe tooling, with no
dependency installation or viewer lifecycle changes.
Dark mode uses the app's Rosé Pine OG colours; light mode uses its Dawn colours.
The build reuses the viewer's bundled component stylesheet and Hugeicons glyphs
for its cards, controls, warning and disclosure carets. Metric controls share the
content background, with an iris underline marking the selected metric.
The build embeds the viewer's existing Atkinson Hyperlegible Next font files into
the HTML; no font dependency or external request is added.
Browser checks cover both themes on desktop and mobile.

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
the distribution of outcomes, not the mean score. Thin dividers separate results,
performance, and usage/cost; aligned metric rows replace the nested run table.
Errors stay labelled as errors, not zero scores. Errors with recorded
`harnessBlocker: true` appear as **Harness
error** in Rosé Pine rose; they remain unscored and included in the error total.
Unrecorded artifact checks and human-review fields are omitted.
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
helper and becomes unknown for incomplete timing coverage. Continuous p95 sorts
recorded elapsed seconds and interpolates at `(n - 1) * 0.95`, matching
`quantile_cont`. It uses binary64 precision and requires complete timing coverage.
Small samples give only a rough tail-latency indicator. Per-task cost is the
total estimated CAD cost across its recorded repetitions. Model cost/task is
saved investigator cost divided by investigator sessions, shown only for complete
estimates. Prices and FX come from the run snapshot; these are not billed costs.
Output token counts include reasoning; tool calls are not labelled as turns.

Grading is its own collapsible row, not an investigator accuracy score. Its table
and three-colour bar retain original grades, with counts for `1/1`, `0.5/1` and
`0/1`; execution errors and unavailable outputs remain separate. The table
shows original automated outputs, recorded time, tokens and estimated cost without
grader explanations, evidence or rubric text. Human corrections do not rewrite
the original grading output.

This is a local presentation candidate, not a hosting/integration decision.
