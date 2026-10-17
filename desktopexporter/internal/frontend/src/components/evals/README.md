# Evals workspace

The `/evals` frontend is a fixture-backed workspace for developing the Evals
experience before implementing ingestion and storage. It uses the existing
drawer, resizable panels, headers, field groups, badges and trace viewer.
App-wide navigation, search and controls live above the drawer in
`shared/Toolbar/WorkspaceToolbar.svelte`. The existing log field renderer is
shared as `shared/DetailField.svelte` for logs, traces, Home and Evals. The server's
`/api/features` capability gates navigation and loading; Evals remains off by
default and is labelled experimental and unsupported.

`examples.ts` supplies clearly labelled, synthetic runs, including a completed
run, a paused investigator, missing assessments and unstarted work. These are
not the completed viewer-usability cohort and do not establish a new rubric.
No model calls, launch controls or persistence writes occur when opening Evals.

`model.ts` is a presentation model, not an import contract. Run configuration,
assessment names, score scales, headline assessment and aggregation rules belong
to the supplied data. Storage integration should replace the fixture source;
the local text filter is labelled **Filter loaded results** and does not claim
to execute the viewer's DuckDB search language.

Selection, detail tab, result filter and sort are URL-backed. Browser history
returns to the selected run/attempt; existing panel widths remain resizable.

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
