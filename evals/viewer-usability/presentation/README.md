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

The page includes only allowlisted model/task labels, counts, coverage, timing,
usage and estimated costs. Answers, prompts, rubric text, grader explanations,
tool evidence, private paths and configuration are never serialized into it.
Model names come from saved pricing/summary metadata. No generation badge is
inferred from names. Task labels come from saved task identifiers, not goals.

Recorded complete/partial/wrong/error counts retain every pilot attempt. Scores
come from the current saved grading result, not inferred from old pass/fail labels.
Task cards show individual automated scores, provisional means and scored coverage.
Human review directories are read through `showReview`, which validates evidence
and revision history. Original automated scores and human-signed scores stay
separate. Final means require exactly three signed-off attempts; one-run pilots
remain incomplete three-run results. Errors stay unscored. Human claim counts
reflect explicitly reviewed claims, not exhaustive automated detection or a numeric
hallucination score. Missing review data is pending, never a clean bill.
Model order follows run metadata. Coverage uses manifest task/repetition counts;
when the manifest supplies only a task count, wholly absent tasks cannot be named.

Saved investigator and grader totals remain separate. Task usage comes from
available pilot metadata; saved investigator totals may include sessions omitted
by Promptfoo after grading execution errors. Timing uses the existing median
helper and becomes unknown for incomplete timing coverage. Per-task cost is the
total estimated CAD cost across its recorded repetitions. Model cost/task is
saved investigator cost divided by investigator sessions, shown only for complete
estimates. Prices and FX come from the run snapshot; these are not billed costs.
Output token counts include reasoning; tool calls are not labelled as turns.

This is a local presentation candidate, not a hosting/integration decision.
