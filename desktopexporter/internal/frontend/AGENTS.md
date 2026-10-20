# Frontend checks

- For every colour change, proactively report text-contrast pass/fail against
  4.5:1 on the affected base, hover and selected backgrounds. Include the lowest
  measured ratio and any remaining failures; the user should not need to ask.
  Distinguish a passing colour pair from full accessibility conformance.

- Use the original Rosé Pine, Moon and Dawn palette values with the approved
  adjustments below. Keep Dawn's approved flipped surfaces:
  bg-100/canvas is #fffaf3, bg-200/panel is #faf4ed and
  bg-300/chrome remains #f2e9e1 (brightest to darkest). Use `subtle` for
  keys, column/section headers, helper text and app-added bracketed annotations,
  and `text` for data values and main page/pane titles. Reserve muted or dimmed
  text for unavailable or disabled states.
  User-approved Pine/Secondary contrast adjustment: Pine #70b3cf, Moon #6bbadc,
  Dawn #266781. These theme-wide colours meet 4.5:1 on all three base surfaces,
  Highlight Low and Highlight Medium. Keep regular weight; do not use bold or
  semibold to strengthen text or compensate for low contrast.
  Dawn's other foreground colours also use the contrast-adjusted values in
  `src/themes/rose-pine.json`, documented in `src/themes/README.md`. Keep these
  theme-wide; do not restore original low-contrast colours in individual views.
  Pine and Moon's other foreground colours retain their original values.
- Shared item states use Highlight Low for hover and Highlight Medium for
  persistent selection. Hover temporarily uses Low even on selected items;
  leaving the item restores Medium. Text selection also uses Medium.
  Text selection is transient; it does not use the persistent-selection token.
- Put item Download actions at the far right of the selected item's header,
  after other header controls, across traces, logs and metrics. Counts are plain
  `subtle` text without badge backgrounds or parentheses; service names retain
  their parentheses.
  Card counts sit in the title row's normal layout. Do not paint a separate
  wrapper background or gradient over the title; truncate the title beside them.
- Trace counts read `x spans, y err` (singular `span` for one), with error text
  in Rosé Pine love (`text-error`). Omit the error suffix when there are no errors.
  Error navigation uses open Hugeicons left/right carets and plain theme-error text, without
  a badge background: `err (x/y)` when an error is selected, otherwise `err`.
- Main header order: title · (service) · counts on the left, optional error
  navigation and Download on the right; time and view controls below. The separate
  service/count row was rejected because it wastes space.
  Use only Copy (clipboard) and Download (file) as action names, not Export.
  Both use icon-only Hugeicons controls with tooltips and accessible names that
  identify the copied or downloaded scope. Download uses Download01Icon; Copy
  uses Copy01Icon. Use the shared header-icon-button styling. Download's right edge aligns to
  the trailing control in the row below (Columns in the trace header).
  Keep one record-level Copy action in the section header; do not add another
  Copy action inside the raw JSON toolbar. Field-level copy actions remain in
  Properties.
- Trace waterfall headers use two rows: title / Download, then service · span
  count / centred error navigation / right-aligned view controls. Centre error
  navigation horizontally within the row. The service/count replaces the
  repeated time range; do not add a third row.
- Header spacing uses 0.75rem horizontal padding, 0.5rem vertical padding and
  0.5rem gaps, with 1.5 line-height. Header controls share a 2rem height; avoid per-component compact
  height or line-height overrides.
  Header tab groups add no outer padding; detail actions are vertically centred
  in their heading rather than positioned by a fixed top inset.
  Header tabs share the 8px header gap. Reserve at least 16px for counters via
  the shared counter token; full counts may grow the slot. Known tab counts stay
  visible whether selected or inactive. Reserve the slot when a count is
  unavailable; keep unavailable counts out of accessible descriptions.
  Centre tab counts in the icon-height slot. Counts use text colour when inactive
  and Primary when selected, matching the tab icons.
  Tab counters display `(count)`, including `(0)`; this overrides the plain-count
  rule for tab counters only. An unavailable count is not a received zero.
  Pane titles share the cards' 16px regular-weight heading style and normal letter
  spacing. The title row uses the same 2rem height as its controls.
- Typography is an accessibility requirement across the entire UI: use 0.875rem
  (14px at the default root size) for all working text and 1rem (16px) for
  main page/pane headings. This includes tables, Details, controls, badges,
  charts, tooltips, editors and dialogs. Use the shared text-size tokens;
  never shrink text to fit. Adjust layout instead. Keep Atkinson Hyperlegible
  Next for interface text and Details field names. Field names retain their
  exact spelling, 14px size and subtle colour. Formatted Details values,
  timestamps, datapoint values and summary metadata use Next at 14px with
  1.5 line-height. JSON editors retain Mono at the shared working size and
  line-height; code views retain Mono. Do not select a display font from the
  data's scalar type.
  Search text and placeholders use the shared 21px line-height. Centre text with
  padding, not a control-height line box that stretches the blinking caret.
  JSON, search and code views share the official Rosé Pine editor syntax mapping
  documented in `src/themes/README.md`, through `utils/editor-highlighting.ts`.
  Use the shared theme palette, including the approved Pine contrast adjustment;
  do not add separate Dawn code colours or
  flatten JSON syntax to the plain-value colour rule. Keep UI field names Subtle.
  User-approved editor exceptions: strings use Text instead of Gold and keys use
  Pine instead of Foam, including compact JSON previews.
  Literals (numbers, booleans, null and bare query values) use Iris/Primary in
  all three themes through `--color-code-literal`, including compact JSON. Dawn's
  Rose palette value is restored to its original #d7827e at the user's request.
  Code comments use Subtle, not Muted. Code surfaces and all search inputs use
  bg-base-100: darkest in Pine/Moon, brightest in Dawn. Apply this to JSON,
  shell examples, code snippets, compact JSON, query inputs and filter searches.
  Keep header surfaces separate; retain shared hover and selection highlights.
  Structured attribute wrappers add no extra left padding. JSON has no fold-arrow
  gutter; retain existing folding commands and raw-inspector controls. The current
  notebook-guide trial uses 1.5px Subtle dots at 60% opacity in Dawn and 40% in
  Pine/Moon, in leading indentation
  only, spaced 2ch horizontally and one text line vertically. Place dots on text
  row boundaries (top/bottom), not at the middle of each row. JSON stays Mono.
  JSON strings soft-wrap with continuation text aligned after the opening quote.
  Wrapping and guides are display-only; preserve document text, Copy and Download.
  JSON and waterfall guides share dot radius, colour and opacity through the
  --guide-dot-* CSS tokens. Waterfall dots align to time ticks and span-row
  boundaries. Its ruler uses --table-header-h and meets the first span row without
  an extra border or gap. Graph spacing follows axes, not JSON character spacing.
  Column and panel resizing share a short 3rem by 0.1875rem grip, rotated for
  horizontal dividers. Hide it at rest; show Text on hover and Iris on keyboard
  focus or drag at full opacity. Preserve full hit strips and keyboard controls.
  Do not restore persistent column guide lines or duplicate header grab handles.
  Keep row hover continuous across the full column resize hit strips. Pinned log
  cells inherit the row's animated background; do not overlay a separate state
  gradient. Compact JSON in a row must not cover its hover/selection background.
  The time-range trigger uses the shared ghost-button style without its own
  background, border or hover-background overrides.
  In Details, group the heading as `key · provenance · type`, omitting absent
  provenance or hidden types. Provenance and type use plain Secondary text to
  distinguish metadata from the Subtle field name, without pills or right-aligned
  badges. The `·` separators always use Subtle, independently of adjacent text
  colours. Preserve the actual field name.
  Span-event summary labels are plain Subtle text; exception labels use the
  theme error colour. Do not restore their pill backgrounds.
  Apply the same plain-label rules to activity popovers, link summaries and
  dropdown metadata. Search suggestions use Next at 14px/21px, with plain
  Secondary metadata and Subtle dot separators. Service/severity filter options
  use plain text with meaningful colour dots and the shared item selection/hover
  highlights, not coloured pills. Datapoint flags use plain warning text.
  Log summary labels are plain Subtle text. Shared log severity labels use
  normal text with a small dot in the existing severity colour, not a pill.
  Retain the severity text and number alongside the dot.
  Activity summary provenance sits beside its title, separated by `·`, such as
  `api-gateway · Log` or `exception · Span event`. Log severity and its colour
  dot stay at the right of the title row.
  Metric summaries group type, aggregation temporality and monotonicity as one
  plain Secondary-coloured label, such as `Sum Σ ↗`. On metric cards, place counts
  at the right of the name and the type group directly below them, right-aligned
  opposite the service. Keep full meanings on hover, counts in Subtle, and no pill
  background or border. Names remain 16px regular; metadata stays 14px/21px.
  Use continuous 24px rows and the shared 8px horizontal gap.
- Signal card text uses the theme's 1.5 line-height; do not override it with
  `leading-none`, `leading-tight` or `leading-snug` on card text.
  Card titles use the shared 16px heading size and regular weight; details
  remain 14px. Use continuous rows without an extra gap separating details
  into a visual group.
  Card text rows share the 1.5rem `--signal-card-row-h` minimum. Only a present
  breakdown bar adds separation before its service/outcome row; do not leave
  that gap on cards without a bar.
  Do not use bold or semibold for titles; the user prefers regular weight.
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
- Details has no fixed maximum width. Preserve its saved width above 40rem and
  let the shared split limit the displayed width using the main pane's existing
  minimum (30% or 28rem, whichever is larger). The drawer retains its 40rem cap.
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
