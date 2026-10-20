# Frontend checks

- Use the original Rosé Pine, Moon and Dawn palette values. Use `subtle` for
  keys, column/section headers, helper text and app-added bracketed annotations,
  and `text` for data values and main page/pane titles. Reserve muted or dimmed
  text for unavailable or disabled states.
- Put item Export actions at the far right of the selected item's header,
  after other header controls, across traces, logs and metrics. Counts are plain
  `subtle` text without badge backgrounds or parentheses; service names retain
  their parentheses.
- Trace counts read `x spans, y err` (singular `span` for one), with error text
  in Rosé Pine love (`text-error`). Omit the error suffix when there are no errors.
  Error navigation uses small filled carets and plain theme-error text, without
  a badge background: `err (x/y)` when an error is selected, otherwise `err`.
- Main header order: title · (service) · counts on the left, optional error
  navigation and Export on the right; time and view controls below. The separate
  service/count row was rejected because it wastes space.
  Export keeps its label, uses a transparent background and the theme's normal
  text colour for both label and icon. Its right edge aligns to
  the trailing control in the row below (Columns in the trace header).
- Trace waterfall headers use two rows: title / Export, then service · span
  count / error navigation and view controls. The service/count replaces the
  repeated time range; do not add a third row.
- Header spacing uses 0.75rem horizontal padding, 0.5rem vertical padding and
  0.5rem gaps, with 1.5 line-height. Header controls share a 2rem height; avoid per-component compact
  height or line-height overrides.
  Header tab groups add no outer padding; detail actions are vertically centred
  in their heading rather than positioned by a fixed top inset.
  Pane titles share the cards' 16px semibold heading style and normal letter
  spacing. The title row uses the same 2rem height as its controls.
- Typography is an accessibility requirement across the entire UI: use 0.875rem
  (14px at the default root size) for all working text and 1rem (16px) for
  main page/pane headings. This includes tables, Details, controls, badges,
  charts, tooltips, editors and dialogs. Use the shared text-size tokens;
  never shrink text to fit. Adjust layout instead. Keep Atkinson Hyperlegible
  Next for interface text and Details field names. Field names retain their
  exact spelling, 14px size and subtle colour. Values and code retain their
  existing font until a separate value-font decision is approved.
- Signal card text uses the theme's 1.5 line-height; do not override it with
  `leading-none`, `leading-tight` or `leading-snug` on card text.
  Card titles use the shared 16px heading size and semibold weight; details
  remain 14px. Use continuous rows without an extra gap separating details
  into a visual group.
  Card identifiers use `subtle`: they are for checking identity, not primary scanning.
  Apply these rules to every summary card: traces, logs, metrics, eval runs and
  disclosure summaries. Reuse `SignalCard`; avoid per-summary typography or
  extra vertical padding between detail rows.
- Trace, log and metric summary timestamps use the selected timezone without
  repeating its suffix on each card. Keep timezone labels in Details.
  Trace summaries omit the visible `Start:` label; keep `Duration:` labelled.
- User-requested exception: waterfall tree controls and log severity use shared
  16px numbered circles and 8px numerals. Tree columns use 22px spacing. Guides must span
  the shared table row height so they remain connected.
  Configure these sizes in `src/themes/tokens.ts` alongside the DaisyUI theme
  tokens; components consume them without hardcoded size overrides.

- Display opaque identifiers with the shared `Identifier.svelte` component, which
  uses Atkinson Hyperlegible Next and truncates the middle while retaining both
  ends. Keep full IDs in copy values,
  accessible text, hover titles, routes, queries, and stored telemetry. Use the
  `identifier` prop for plain ID fields in `DetailProperty.svelte`.
- Reuse `TableValueCell.svelte` for inspectable telemetry cells, including the
  planned logs table: click reveals the exact field in Details. Keep copy controls
  in Details rather than adding a copy button to every table cell.
  Preserve text selection and distinguish field/attribute kind and provenance
  when using `revealDetailField`.
- In Details, copy only through explicit copy buttons. Links navigate; plain
  values and card backgrounds have no click action. Preserve native text selection
  and do not restore card-click or tap-to-copy handlers or card pointer cursors.
- Detail-section resize precedence: preserve header order and at least one open,
  non-empty section; open sections fill the remaining height. A divider grows the
  nearest available section on one side by shrinking the nearest open section on
  the other. Pass movement to another donor only after the first exhausts its
  content and collapse resistance. Collapsed sections can reopen through dragging;
  empty sections remain headers. Heading collapse hands space to one available
  neighbour: upper half (including the exact middle) prefers below; lower half
  prefers above. Skip empty sections and fall back to the opposite direction.
  Open a closed receiver before closing the source; only the sole non-empty
  section cannot collapse. Other sections keep their allocation.
  Reuse `startDrag` and `resolvePanelResize` for pointer lifecycle, resistance, and
  release thresholds. Keep resize previews separate from saved sizing; queued
  native disclosure events must not reapply internal resize changes.

- `npm run lint` rejects both Oxlint errors and warnings. Keep the standard lint
  gate at zero warnings without disabling rules or adding blanket suppressions.
- `npm run lint:anti-slop:evaluate` reports the production anti-slop cleanup
  backlog; tests and test harnesses are excluded from this advisory scan only.
  Enforced lint, type checks, Vitest, and Playwright retain their existing scope.
- Reviewed external-value decoding and runtime-capability checks may use a
  next-line `anti-slop/no-runtime-typeof` exemption with a concrete boundary
  justification. Scope it to one finding; keep all other findings visible.
- After frontend changes, run `make build-ts` from the repository root and commit
  the updated `desktopexporter/internal/server/static` assets. `make test` runs
  the full local quality gate, including bundle freshness.
- Keep preview-only diagnostics in the console or local logs rather than
  rendering them in the application UI.
- Remove unused CSS encountered during frontend work after checking for remaining
  consumers. Do not retain obsolete styles for possible future reuse; Git keeps
  their history.
- For incremental UI work, implement only the requested step. Do not infer search
  behaviour, receiver or storage contracts, or actions for unwired controls. Ask
  before filling those gaps.
