# Eval results presentation

- Edit the typed Svelte templates in `components/`. Keep report layout rules in
  `results.css`, consolidate existing rules, and reuse the viewer's theme tokens.
- Keep saved answers, prompts, grading evidence and private paths out of the
  reader-facing report. Templates receive the existing aggregate presentation.
- The static builder writes formatted HTML plus `OUTPUT.html.assets/`. Keep these
  together. Do not embed bundled styles or base64 fonts back into the HTML.
- Run `npm run typecheck:presentation` and the affected presentation tests from
  `evals/viewer-usability`. Use `presentation/check-browser.ts` on a generated
  report for layout, keyboard, theme and accessibility changes; use
  `presentation/check-live-browser.ts` for changes affecting saved-results updates.
- For previews of recorded results, reconcile every selected batch and approved
  recovery against the saved stop/completion audits before reporting coverage.
  Do not use an earlier snapshot as the current cohort results.
