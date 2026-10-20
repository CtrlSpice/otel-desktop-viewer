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
  Subtle is #aba7c5 in Pine and #b1adcc in Moon app-wide. Both meet 4.5:1 on
  canvas, panel, chrome, Highlight Low and Highlight Medium. Highlight High
  remains below 4.5:1 for Subtle. Pine and Moon's remaining foreground colours
  retain their original values.
  Love/Error uses the original palette colour in every theme, including Dawn
  #b4637a. Do not use a separate darkened error-text colour.
  Dawn Gold/Warning is the original #ea9d34. The user rejected #895500.
  Gold is for markers and fills, never text. Warning text uses neutral Text.
  Never darken, replace or locally override Gold to satisfy contrast checks.
  Report contrast failures; obtain explicit user approval for any colour/theme
  change, including changes proposed during cleanup or independent review.
  Contrast adjustments apply only to colours actually used for text, only in
  themes where those text/background pairs fail, and only by the smallest amount
  needed to pass. Preserve marker/fill palette colours; do not darken them to
  satisfy a text-contrast target. Show measured pairs and proposed values before
  any further colour change.
  Span and summary breakdown bars use solid palette fills without Dawn-specific
  fading or borders. Error status code/message values use neutral Text with an
  8px Love dot before the value, only when the received status code is 2.
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
- Trace counts read `● x err · y spans` (singular `span` for one), with both counts
  and the separator in Subtle, and an 8px Love dot before the error count.
  Error span names use neutral Text
  without an extra dot; the waterfall already has coloured tree markers and bars.
  Omit the error count, dot and separator when there are no errors.
  Error navigation uses open Hugeicons left/right carets and neutral Text, with
  an 8px Love dot between the left caret and label, without a badge background:
  `err (x/y)` when an error is selected, otherwise `err`.
  Only the parenthesised `(x/y)` position uses Subtle; `err` and carets use Text.
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
  Raw JSON places Download at the far right after its circular Collapse/Expand
  controls.
  Metric Details JSON loads automatically without datapoints using the export
  option `includeDatapoints=false`; do not show a manual Load button. Preserve
  complete-metric Copy/Download and the existing bounded preview behaviour.
  Details field cards place Copy at the top-right, aligned with the first heading
  line even when the heading wraps. Its tooltip opens left into the card.
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
  Primary navigation counters use neutral DaisyUI ghost badges and regular Text
  in active and inactive tabs, rather than Iris. Local pane-tab counters retain
  their existing active-colour styling.
  Primary navigation count badges are circles and omit parentheses; grow them
  to fit the full count. This overrides the parenthesised-count rule above.
  User-approved typography exception: these small circular navigation badges
  use 10px regular numerals in a minimum 20px circle. Other working text stays 14px.
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
  JSON inside a Details field card uses a transparent editor background so the
  card's shared hover, reveal and selection highlights remain continuous.
  Properties presents ordinary string values as decoded text without their outer
  JSON quotes. Retain meaningful quotes/backslashes, and show empty strings as
  `""`. Raw JSON and explicit Copy/Download retain the original value. Keep the
  editor document and syntax tree intact for folding, search and exact numbers.
  Keep header surfaces separate; retain shared hover and selection highlights.
  Structured attribute wrappers add no extra left padding. JSON uses the shared
  Hugeicons fold-caret gutter; retain existing folding commands and raw-inspector
  controls. JSON, read-only code panels and text-heavy signal tables have plain
  backgrounds without decorative dot grids or indentation dots. Use spacing,
  wrapping and shared field highlights to convey structure. JSON stays Mono.
  JSON uses CodeMirror's normal word wrapping, breaking oversized words only
  when needed. Continuations align under the key's first character (after its
  opening quote). Array strings wrap under their own first character.
  Wrapping is display-only; preserve document text, Copy and Download.
  No decorative dot grids anywhere, including waterfall timing and all metric
  charts. Retain ruler labels, axis ticks, tree connectors and meaningful data
  markers. Keep data bounds, span geometry, chart cells and scrolling intact.
  Trace and Evals waterfalls share solid 1px Muted time guides at 35% opacity,
  aligned only to labelled ruler ticks behind the bars. Keep ruler text Subtle.
  Guides are decorative and cannot intercept pointer input.
  Column and panel resizing share a short 3rem by 0.1875rem grip, rotated for
  horizontal dividers. Hide it at rest; show Text on hover and Iris on keyboard
  focus or drag at full opacity. Preserve full hit strips and keyboard controls.
  Table headings use `name · source` for external provenance: name and separator
  use Subtle, and source names use Secondary. Shrink resource/scope labels down
  to their first letter before clipping the field name; show the full source when
  space permits and retain its full accessible label and tooltip.
  Column/service picker searches reuse FilterInput's
  shared 32px height rather than the taller query-entry field.
  Do not restore persistent column guide lines or duplicate header grab handles.
  Keep row hover continuous across the full column resize hit strips. Pinned log
  cells inherit the row's animated background; do not overlay a separate state
  gradient. Compact JSON in a row must not cover its hover/selection background.
  The time-range trigger uses the shared ghost-button style without its own
  background, border or hover-background overrides.
  In Details, use `key · type` in the heading, omitting hidden types. Put provenance
  such as Resource/Scope on its own line below the heading when present.
  Provenance and type use plain Secondary text to distinguish metadata from the
  Subtle field name, without pills or right-aligned badges. The `·` separator
  uses Subtle. Preserve the actual field name.
  Span-event metadata labels are plain Subtle text. Exception events use an 8px
  Love dot with neutral Text titles in cards and neutral metadata in timeline
  popovers. Do not colour exception text red or restore pill backgrounds.
  Apply the same plain-label rules to activity popovers, link summaries and
  dropdown metadata. Search suggestions use Next at 14px/21px, with plain
  Secondary metadata and Subtle dot separators. Service/severity filter options
  use plain text with meaningful colour dots and the shared item selection/hover
  highlights, not coloured pills. Datapoint flags use plain neutral Text.
  Timeline activity popover entries use two lines: event name or shared log
  severity in Text, with the Subtle offset at the right; Subtle record type below.
  Logs put a received event name after `Log ·` on the second line when present.
  Align metadata under the title text after a dot when present. Keep chronological
  order; nameless entries have no separator or invented name.
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
- Evals uses plain text and counts without badges. Use small 8px decorative
  colour dots only for meaningful statuses or outcomes, alongside neutral Text
  labels. Keep Results, Time, Cost, summaries and Details consistent with shared
  table and field-card typography and spacing.
  Results, Tool calls and Cost are a single-select radio group. The active view
  uses Highlight Medium and Highlight Low on hover. Results has recorded score
  with its dot/label, without a duplicate case-percentage Summary column. Tool calls
  combines tool total and failed count, with plain failed labels and no status dot.
  Omit zero tool/failed counts and show the separator only when both counts appear.
  Default the resizable identity portion to 320px, Results to 160px and Tool calls
  to 176px; keep saved widths and let the timeline fill the remaining space.
  Cost has Investigator, Grader and Total cost columns, in that
  order. Cells and footer totals show only numeric amounts (Unknown if missing);
  CAD and estimate labels stay in headers, and partial-estimate context is on hover.
  Results reveals
  Case → Attempt; Tool calls reveals Execution and its tool descendants. Grading
  is a sibling of Execution under the attempt. Preserve manual branch toggles.
  Cost shows each visible level in the summary column, without adding a tree depth or summing
  parent estimates together with their child estimates. Missing costs are Unknown.
  Attempt labels are left-aligned beside their tree dots.
  The identity heading reads Case · Rep on one line with uniform separator
  spacing and no extra gap between the labels.
  An explicit attemptTelemetry reference identifies the enclosing attempt span;
  telemetry identifies target execution. Without it, label derived timing as the
  extent of available references rather than claiming another stored span.
  Align recorded starts to the earliest available span in the run, including
  when there is no enclosing run span. Keep recorded-duration-only fallbacks.
  Missing timing on started/completed rows uses a faint 1px Muted line with one
  central break. Planned/unstarted rows stay blank. Ruler ticks
  extend 4px into the header below their labels, aligned with the time guides.
  The results header legend always describes execution statuses: Complete,
  In progress, Span error, Internal fault and Planned, in that order. It does not switch to
  assessment outcomes or percentages when Results columns are shown.
  Its Duration column uses bounded referenced trace views and shared waterfall
  bars/colours/connectors. Keep received parent IDs intact when compressing
  ancestry; mark skipped or unavailable parent connections. Include referenced
  graders, and retain recorded durations when a span is unavailable. Span
  inspection belongs in the trace workspace. Row names and bars select eval
  Details; case, execution, grading, tool and grader rows show their own context.
  Tree controls only expand/collapse and do not change selection.
  Do not embed trace Span, Events or Links Details in Evals.
  Child execution errors propagate to parent indicators, including collapsed
  branches, without changing recorded assessment scores.
  Propagated span errors also colour ancestor duration bars Love, using the same
  execution-error state as the indicators, whether the branch is open or closed.
  Distinguish Love "Span error" from Papaya "Internal fault" (evaluation
  machinery). A failed assessment is "Fail", not an execution error. Preserve
  these meanings during parent propagation, including in dogfooding datasets.
  Evals Details opens only its first section by default; later sections start
  collapsed and remain manually expandable.
  Keep tool execution in the waterfall; do not repeat its Tool calls list in
  attempt Details.
- Trace, log and metric summary timestamps use the selected timezone without
  repeating its suffix on each card. Keep timezone labels in Details.
  Trace summaries omit the visible `Start:` label; keep `Duration:` labelled.
- Log severity uses an 8px coloured dot followed by neutral `LABEL (number)`
  text in summaries and tables through LogSeverity; no numbered circles.
  Waterfall tree nodes
  use filled 8px dots matching the span bar colour for both parents and leaves,
   without a visible number inside the dot. Direct-child counts read `[4]` in
   Subtle 14px regular interface text, after the dot: `● [4] Name`. The dot and count
   form one group that moves with the row's nesting indentation, without a fixed
   count column. Centre counts vertically with their dots, without a horizontal
   line between the dot and count. Counts have no badge background or semantic accent colour.
   The whole bracketed label uses Subtle, including the brackets; no Mono or Muted.
  Align the waterfall table's name heading with the root title, reserving the
  same count slot when present.
  Keep counts visible when collapsed; omit counts for leaves.
  Do not add separate count gutters.
  Each parent has one native expand/collapse button covering the whole left
  gutter (ancestor columns, dot and count), at full row height. Clicking any
  part toggles that row, not an ancestor, and does not select the span. Names
  still open Details. Include the span name, direct-child count and action in
  the button label, and expose aria-expanded. Retain native Enter/Space and a
  visible focus outline; use Highlight Low on hover. Leaves have decorative,
  non-focusable gutters without collapse buttons. Dots stay visible on hover;
  there is no caret.
  Timeline activity markers use the same 14px circle icon for single and grouped
  records. Grouped markers show a small 8px plus icon instead of a visible count;
  keep exact event/log counts in the accessible label and tooltip.
   Tree columns use 18px spacing. Guides must span
  the shared table row height so they remain connected.
  Tree connector trunks, arms and stems use solid 1px Muted lines at 50% opacity.
  Centre the line strokes on node centres; elbow connectors use the cards'
  --radius-xl corner radius as one continuous rounded bend.
  Configure these sizes in `src/themes/tokens.ts` alongside the DaisyUI theme
  tokens; components consume them without hardcoded size overrides.
  Metric line, histogram and heatmap charts use a continuous solid 1px Muted
  left/bottom axis border with the cards' --radius-xl bottom-left join. Keep
  shared plot insets (24px top, 72px right, 96px bottom, 136px left), labels and scales; disable the old
  separate axis rules. Axis tick marks also use 1px Muted. The shared border is
  decorative, hidden from assistive technology and cannot intercept pointers.
  Keep axis range labels on one line with both bounds and brackets visible.
  Graph measurements share grouped decimal formatting from 0.000001 inclusive
  to 1,000,000 exclusive, and explicit ×10ⁿ outside that range. Use three
  significant digits on graphs and six on hover, increasing precision when
  needed to distinguish adjacent bounds. Counts use whole grouped integers.
  Keep the received unit separate; do not append standalone SI-prefix suffixes
  or convert received values/units. These are display rules only.
   Heatmap labels use the line charts' 80px X / 50px Y spacing targets instead
   of a fixed seven-label limit. Select labels from existing time columns and
   bucket rows; preserve exponential-bucket identities, bounds, counts and layout.
   Heatmap labels remain centred. When either cell dimension is below 12px, join neighbouring occupied
   cells across colour bands and round only their shared outer boundaries.
   Keep each cell's count colour and preserve empty buckets and holes.
   Joined shapes retain the existing density-scaled border around their shared
   perimeter and holes, following each cell's colour without internal borders.
   Do not add decorative grid styling.
  Histogram/heatmap count legends sit on the left of the chart header's lower
  row, replacing the displayed chart time range. Keep chart tabs/controls on the
  right and retain the global time-range selector. At the regular 14px size,
  legend rows fit as many whole entries as the measured width permits. Move one
  entry down only to avoid a lone final item when the pair fits and at least two
  entries remain in the previous row. Keep count order and whole
  labels; do not keep a separate legend above the plot.
  Use plain 8px colour dots beside numeric count ranges, without borders or outlines.

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
