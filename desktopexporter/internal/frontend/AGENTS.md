# Frontend checks

- During interactive frontend work, always use Vite with hot reload against the
  existing backend (`VIEWER_PROXY_TARGET`). Keep the user's preview on that dev
  server. Do not rebuild/restart the Go binary for each UI edit. Build embedded
  assets for final verification and commits. Concurrent test/preview servers must
  use separate Vite caches so dependency optimization cannot break the preview.

- Use the Madoromi seasonal interface themes in `src/themes/madoromi.json`.
  The approved background tint is 20% of upstream chroma for all four seasons,
  day and night. Generate shared colours with `scripts/generate-madoromi.mjs`;
  retain the pinned source and full MPL-2.0 licence. Routine frontend work does
  not require colour reviews or contrast reports. Day canvas/panel/chrome map
  to kami/hyoshi/chitsu, brightest to darkest; Night maps to
  chitsu/hyoshi/kami, darkest to brightest. Use `subtle` for
  keys, column/section headers, helper text and app-added bracketed annotations,
  and `text` for data values and main page/pane titles. Reserve muted or dimmed
  text for unavailable or disabled states.
  Keep regular weight; do not use bold or semibold to strengthen text.
  Text and accent foreground corrections are generated theme-wide to meet
  4.5:1 on canvas, panel, chrome, hover and selection. Subtle and Muted retain
  their original source shades without contrast adjustments. Text uses Sumi,
  Subtle uses Usuzumi 1, Primary uses Fuji and Secondary uses Ruri. Preserve the
  service/series ownership and saved assignments when changing graph palettes.
  Group appearance controls as Ink, Season, Day/Night, with pill-rounded exterior
  corners and flat internal joins. The Season button has an icon and label,
  without a dropdown caret. Accent ink selects one of the 32 named seed
  inks, with an analogous family from the pinned upstream
  mixer; default to To-ro (warm lantern) with Autumn and the system's Day/Night
  preference. Saved user choices take precedence. Persist ink as `theme-accent-ink`. Use that family for
  Primary/Secondary/Accent and syntax accent slots. Season owns paper, neutral
  text and Nijimi highlights; retain semantic status colours and the Sakura
  attribution heart. Generate readable strengths against all five UI surfaces.
  Changing the global ink also selects that ink for histogram/heatmap counts
  and their legends. The chart picker can subsequently choose a different count
  ink. Retain saved choices on load and keep service/series colours independent.
  Keep ink-family names visible in every picker.
  Accent-ink previews show the named seed's exact official shades 1–7, matching
  the graph ink catalogue. Do not use mixed syntax-role colours as ink previews.
  The top-bar ink control shows a 16px brush, the selected ink's original name,
  then three 8px dots using official shades 1, 4 and 7. Keep the English name in
  its tooltip and picker rows. Put the parenthesised English name on a separate
  line in the top-bar tooltip, using the standard tooltip style.
  Left-align top navigation beside the leading pane control. Let search fill
  the remaining desktop width, with appearance controls grouped at the right.
  Use additional rows on narrow screens so search and appearance do not overlap.
  Expanded Home attribute values use one row: value, bar, count, %, Copy.
  Keep Copy in its own far-right column, with its tooltip opening left.
  Bars are 8px high with round ends and a minimum 8px dot, using stable
  typed-value hashes into official shades 3–5 across the 32 graph inks. Keep
  parent attribute rows plain and preserve Name, Count, %, exact Copy and disclosures.
  Value columns use one third of the available row width, capped at 12rem;
  truncate long values with an ellipsis and retain the full value on hover.
  Aligned bar tracks fill the remaining width and keep their percentage scale.
  Home Count and % columns stay fixed-width and right-aligned; the value/bar
  column fills the remaining table width. Retain sorting and far-right Copy.
  Home's left pane contains Lulu above the shared section stack. Getting started
  is the primary open section, with setup instructions scrolling inside it.
  Omit its redundant visible section heading beneath the welcome card.
  Home attribute, telemetry and storage loading failures use Papaya text for
  viewer/infrastructure faults; received telemetry errors retain their own roles.
  Storage and Ingestion issues are sibling sections using the standard disclosure,
  grouping and header-resize rules. Closed and inactive sections dock at the
  bottom; open sections share the available height. Use the shared sidebar toggle
  and saved width. Ingestion issue links reveal the left pane and their section.
  The welcome app name uses 22px regular Atkinson Mono in Primary, with a 1.25
  line-height and natural wrapping, as a display-title exception to the 16px
  pane headings. Lulu uses 40% of the welcome width, capped at 160px, so the
  name fits beside her.
  Prefix the name with a decorative `>` in the Madoromi Sora operator role.
  Hide the prompt from assistive technology and text selection.
  Welcome axolotl facts use concise, qualified claims from peer-reviewed research,
  with IEEE-style numbered citations in first-appearance order. Put the reference
  number and DOI beside each fact in the source. Keep full IEEE references and
  claim context in `src/components/home/README.md`; update both when changing a
  fact. Omit the "Axolotl fact:" prefix.
  Keep welcome taglines and facts left-aligned with balanced wrapping
  (`text-wrap: balance`) for more even line lengths.
  Exporter help sits beside Copy in the terminal header and describes the
  selected HTTP or gRPC protocol, including default-port guidance when needed.
  Home Telemetry uses counted Traces, Metrics and Logs header tabs, with
  the tab group at the far right of the Telemetry section header and
  square-bracketed counts, such as `[66]`. Retain independent 25-card pages
  and scroll positions. Only the active card list
  scrolls; keep tabs at the top and shared page controls in a stable bottom
  footer. Tab changes are immediate and support arrow/Home/End navigation.
  Disable confirmed-empty or inapplicable signals, retaining loading/error
  access. Reset pages when service, attribute/value, search, time range or
  refreshed results change. Keep other Home section disclosures independent.
  Categorical graph colours use the official first seven swatches of the same
  32 Madoromi inks: 224 fixed shades across Day/Night and seasons. UI shades are
  allowed. The palest, eighth swatch is omitted from automatic assignments and
  the seven-dot override picker. Use current Madoromi theme IDs and graph
  palettes directly; do not add compatibility migrations for prior UI experiments.
  Histogram and heatmap count ramps use 32 colourful Madoromi inks, default To-ro, with one
  app-wide saved choice. Generate each adaptive size 1–12 from the full pinned
  curve; Day reverses Night's exact shades, and seasons do not change the ink.
  The count-palette picker omits its redundant visible heading; retain its
  accessible name and selected-option focus. Centre the Madoromi attribution
  link in the footer, with a decorative 16px Hugeicons heart in theme Sakura pink.
  Preview each histogram ink with seven aligned 8px dots, evenly sampled across its full
  ramp, with 8px gaps and no outlines. Retain exact Day/Night reversal.
  The shared picker is searchable, ordered by Madoromi's pinned full-ramp colour
  sequence and scrollable; retain that order in search results. Focus
  and reveal the saved choice on opening without moving the page. Exclude the
  four seasonal neutral inks (Kiri-same, Fukagawa-nezu, Yama-guri, Fuyu-syogun),
  plus Take-sumi, Hoteison, Syun-gyo and Tsukushi. Preserve existing saved choices.
  Keep the original ink name first, followed by the pinned Madoromi English name
  in Subtle parentheses. Search matches both names; allow the label to wrap at
  the regular text size while keeping the seven preview dots aligned.
  Size both pickers from the widest full ink label plus their dots and controls,
  capped by the viewport. Keep that width stable when search filters the list.
  Both ink pickers share InkPicker. Graph overrides use seven individually
  clickable official shades, with 32px targets around 8px dots, arrow-key
  navigation and selected-shade focus. Preserve native Escape/light dismissal.
  Disable shades reserved by other owners in the same scope, including hidden
  owners, saved reset slots and overrides; skip them during keyboard navigation.
  Disabled shade dots use solid base-200, rather than faded ink colours.
  Keep the current choice available, including pre-existing saved collisions.
  With fewer than 32 unreserved presets, open per-ink sliders along the pinned
  curves between official shades 1 and 7. Skip occupied hex colours. Keep the
  layout fixed while open and retain slider mode for a saved curve choice.
  Preserve exact preset colours and persist curve positions in the same scope.
  Home service controls replace the name's colour dot with a DaisyUI
  `btn btn-circle btn-sm` customised to a 24px circle in its 32px alignment slot,
  filled with that service's colour and a readable 16px brush icon. Keep the
  swatch flat, without borders or shadows, and retain its keyboard focus ring. Keep it before
  the name and outside the whole-card button. Centre the service summary icons
  under the brush button, with their labels aligned under the service name.
  Align Last seen beneath the left edge of the log icon. Gauge/Sum series
  overrides live in metric Details, independently of visibility and disclosure.
  Series visibility checkboxes sit between the disclosure caret and series text.
  Persist overrides by service name or metric reference + native series identity;
  Put an icon-only reset to the right of the ink search field; it restores the
  saved automatic assignment and is disabled when no override is set. Keep only
  Madoromi attribution in the footer. New automatic assignments
  probe past occupied slots and explicit overrides without moving existing owners.
  Error uses each variant's original Beni colour from the pinned upstream
  palette. Warning fills use original Yamabuki. Warning
  text uses neutral Text. Do not add separate darkened error text colours.
  Preserve marker/fill palette colours. Obtain explicit user
  approval before changing the palette or adding local colour overrides.
  Span and summary breakdown bars use solid palette fills without light-mode
  fading or borders. Error status code/message values use neutral Text with an
  8px Error dot before the value, only when the received status code is 2.
  Timeline activity markers retain their exact assigned colour without brightening.
  Swatch glyphs choose between the active theme's light/dark foregrounds using
  linear-sRGB contrast ratios, recalculating on theme changes.
  Tooltips use the seasonal Panel surface with regular-weight neutral Text.
  Ghost buttons use the shared Low hover and Medium active/selected backgrounds;
  hover takes precedence while the pointer remains over the button.
- Shared item states use Highlight Low for hover and Highlight Medium for
  persistent selection. Medium blends tinted Nijimi 2 at 45% over the panel in
  sRGB. Hover temporarily uses Low even on selected items;
  leaving the item restores Medium. Text selection also uses Medium.
  Signal cards use the same precedence: activation changes selection, but the
  pointer keeps Low until it leaves, then selected cards show Medium. Cards share
  Low across the whole parent card, including its tooltip-only breakdown bars.
  Text selection is transient; it does not use the persistent-selection token.
  Sidebar and Details toggles keep neutral Text icons in both states. Use the
  shared item highlights: Medium when open, Low on hover, transparent when
  closed. Retain their directional icons, circular shape and keyboard focus.
- Buttons stay still on press and release. Use colour and focus feedback;
  remove DaisyUI's active press translation from buttons and controls that apply
  its button styles. Do not add bounce, scale or press/release movement.
- Put item Download actions at the far right of the selected item's header,
  after other header controls, across traces, logs and metrics. Summary counts use
  plain bracketed counts; metric counts share one group, such as
  `[3 series | 42 pts]`, in regular Subtle text,
  without badge backgrounds; service names retain
  their parentheses.
  Card counts sit in the title row's normal layout. Do not paint a separate
  wrapper background or gradient over the title; truncate the title beside them.
- Trace counts read `● [x err | y spans]` (singular `span` for one), with both counts
  and the separator in Subtle, and an 8px Error dot before the error count.
  Error span names use neutral Text
  without an extra dot; the waterfall already has coloured tree markers and bars.
  Omit the error count, dot and separator when there are no errors.
  Error navigation uses open Hugeicons left/right carets in Text, with an 8px
  Error dot between the left caret and its Subtle label: `[x/y err]` for a selected
  error, otherwise `[y err]`. No badge background. The Span section heading adds
  `· root` in Secondary only when the selected span has no recorded parent.
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
  Properties/JSON Details tabs switch immediately. Apply the selected background
  to the active tab itself; do not slide a shared indicator between these tabs or
  animate their content on selection. Preserve keyboard tablist behavior.
  Header tabs share the 8px header gap. Reserve at least 16px for counters via
  the shared counter token; full counts may grow the slot. Known tab counts stay
  visible whether selected or inactive. Reserve the slot when a count is
  unavailable; keep unavailable counts out of accessible descriptions.
  Centre tab counts in the icon-height slot. Counts use text colour when inactive
  and Primary when selected, matching the tab icons.
  Tab counters display `(count)`, including `(0)`; this overrides the plain-count
  rule for tab counters except Home Telemetry, which uses `[count]`.
  An unavailable count is not a received zero.
  Primary navigation counters use plain bracketed text, such as `[42]`, in
  Text at the regular 14px working size, matching the tab labels. No badge
  background, border, circle or reduced numeral size. Keep full counts and
  unavailable-count semantics. Local pane-tab counters retain their existing
  active-colour styling; only Home Telemetry uses square brackets.
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
  JSON, search and code views share the semantic editor syntax mapping
  documented in `src/themes/README.md`, through `utils/editor-highlighting.ts`.
  Use the shared theme palette and generated foreground adjustments;
  do not add separate light-mode code colours. Keep UI field names Subtle.
  Use Madoromi's native code guide for syntax accents, with neutral values for
  readability: strings including their quote marks and literals (numbers,
  booleans, null and bare query values) use Sumi/Text in all eight variants.
  `--color-code-literal` resolves to Sumi. Keys use Ruri, keywords Fuji, operators
  Sora, and escapes or regular expressions Botan, including compact JSON previews.
  Plain Markdown code uses Sumi too. Functions use Ai, built-ins Beni, types/attributes
  Yamabuki, namespaces Kikyo, annotations Nadeshiko, and labels/cursors Sakura.
  Invalid syntax uses the Beni Error role. Comments use readable Kasure 2 in
  italic; punctuation uses readable Usuzumi 0. Generate syntax foreground
  corrections theme-wide with the same shared readability target. JSON quote and
  escape accents are display decorations over the intact document; decoded
  Properties values use neutral Text. Code surfaces and all search inputs use
  bg-base-100: darkest at night, brightest by day. Apply this to JSON,
  shell examples, code snippets, compact JSON, query inputs and filter searches.
  JSON inside a Details field card uses a transparent editor background so the
  card's shared hover, reveal and selection highlights remain continuous.
  Properties presents ordinary string values as decoded text without their outer
  JSON quotes. Retain meaningful quotes/backslashes, and show empty strings as
  `""`. Raw JSON and explicit Copy/Download retain the original value. Keep the
  editor document and syntax tree intact for folding, search and exact numbers.
  Shared value formatting recursively recognizes complete embedded JSON-like
  containers, including runtime literals `undefined`, `NaN` and `Infinity`.
  Use the same literal classification and palette in table previews and Details;
  never add per-field or per-signal formatting exceptions. This is display-only:
  keep received types, exact Copy/raw JSON, duplicate keys and numeric lexemes.
  Do not evaluate code, coerce runtime literals to null, or parse prose as code.
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
  Trace and Evals waterfalls share solid 1px Muted time guides at 18% opacity,
  aligned only to labelled ruler ticks behind the bars. Keep ruler text Subtle.
  Guides are decorative and cannot intercept pointer input.
  Column and panel resizing share a short 3rem by 0.1875rem grip, rotated for
  horizontal dividers. Hide it at rest; show Text on hover and Primary on keyboard
  focus or drag at full opacity. Preserve full hit strips and keyboard controls.
  Table headings use `name · source` for external provenance: name and separator
  use Subtle, and source names use Secondary. Shrink resource/scope labels down
  to their first letter before clipping the field name; show the full source when
  space permits and retain its full accessible label and tooltip.
  Column/service picker searches reuse FilterInput's
  shared 32px height rather than the taller query-entry field.
  Signal filter popovers omit the redundant Filters heading and explanatory
  paragraphs. Use labelled controls and a Clear / Apply footer; show only short
  status text when a custom query must be edited in search.
  Logs use their sortable table headings without a duplicate header sort picker.
  Do not restore persistent column guide lines or duplicate header grab handles.
  Keep row hover continuous across the full column resize hit strips. Pinned log
  cells inherit the row's animated background; do not overlay a separate state
  gradient. Compact JSON in a row must not cover its hover/selection background.
  The time-range trigger uses the shared ghost-button style without its own
  background, border or hover-background overrides.
  In Details, use `key · type · provenance` on the heading line, omitting hidden
  types and absent provenance. Separate Resource/Scope provenance with a small `·`.
  Provenance and type use plain Secondary text to distinguish metadata from the
  Subtle field name, without pills or right-aligned badges. The `·` separator
  uses Subtle. Preserve the actual field name.
  Span-event metadata labels are plain Subtle text. Exception events use an 8px
  Error dot with neutral Text titles in cards and neutral metadata in timeline
  popovers. Do not colour exception text red or restore pill backgrounds.
  Apply the same plain-label rules to activity popovers, link summaries and
  dropdown metadata. Search suggestions use Next at 14px/21px, with plain
  Secondary metadata and Subtle dot separators. Service/severity filter options
  use plain text with meaningful colour dots and the shared item selection/hover
  highlights, not coloured pills. Datapoint flags use plain neutral Text.
  Timeline activity popover entries put the event/tool name or received log name
  (falling back to body preview) in Text, with the Subtle offset at the right and
  Subtle record type below. Retain the shared log severity dot and accessible label.
  Timeline markers represent stored span events and logs, with received ownership
  and timestamps. Do not manufacture viewer-only annotation markers or place
  assessments at span ends. Adapter-created tool enrichment is stored as named telemetry
  logs with native trace/span correlation fields, not added to span events.
  Genuine received span events retain their original signal and content.
  Enrichment labels use name · kind · source, such as
  `shell · Log event · OpenCode`. Pi/OpenCode identifies the runtime source,
  not the record type or model provider. Retain reconstruction and timestamp
  provenance in the source tooltip.
  Recorded `tool.is_error: true` adds neutral `Tool error` text, not warning dots
  or assessment outcomes. Tool names come only from received attributes.
  Align metadata under the title text after a dot when present. Keep chronological
  order; nameless entries have no separator or invented name.
  Log summary labels are plain Subtle text. Shared log severity labels use
  normal text with a small dot in the existing severity colour, not a pill.
  Retain the severity text and number alongside the dot.
  Activity summary provenance sits beside its title, separated by `·`, such as
  `api-gateway · Log` or `exception · Span event`. Log severity and its colour
  dot stay at the right of the title row.
  Metric summaries group type, aggregation temporality and monotonicity as one
  plain Secondary-coloured label, such as `Sum Σ ↗` in detailed summaries.
  On metric cards, use `Histogram` and `Exp. Histogram`; put
  temporality/monotonicity meanings on hover rather than clustering symbols.
  Show units beside actual readings, without a separate Units row. Place counts
  at the right of the name and the type group directly below them, right-aligned
  opposite the service. Keep full meanings on hover, counts in Subtle, and no pill
  background or border. Names remain 16px regular; metadata stays 14px/21px.
  Use continuous 24px rows and the shared 8px horizontal gap.
- Signal card text uses the theme's 1.5 line-height; do not override it with
  `leading-none`, `leading-tight` or `leading-snug` on card text.
  Resting cards have transparent surfaces, without background fills. Colour
  belongs in meaningful dots and bars; retain shared hover/selection feedback.
  Card titles use the shared 16px heading size and regular weight; details
  remain 14px. Use continuous rows without an extra gap separating details
  into a visual group.
  Card text rows share the 1.5rem `--signal-card-row-h` minimum. Only a present
  breakdown bar adds separation before its service/outcome row; do not leave
  that gap on cards without a bar.
  Do not use bold or semibold for titles; the user prefers regular weight.
  Card identifiers use `subtle`: they are for checking identity, not primary scanning.
  Apply these rules to every summary card: traces, logs, metrics, eval experiments and
  disclosure summaries. Reuse `SignalCard`; avoid per-summary typography or
  extra vertical padding between detail rows.
- Evals uses plain text and counts without badges. Use small 8px decorative
  colour dots only for meaningful statuses or outcomes, alongside neutral Text
  labels. Keep Results, Time, Cost, summaries and Details consistent with shared
  table and field-card typography and spacing.
  Evals has no legacy snapshot/archive workflow. The global drop/file-picker
  queue ingests producer-native Promptfoo v3 envelopes into DuckDB; the Experiment list
  reads the database-wide materialized index.
  Evals experiment cards show name/state, an outcome bar and the shared date row when
  a valid start time is available.
  Outcome-bar segments use the same compact hover tooltips as trace-service
  segments: coloured dot, status label and `[count]`. Card breakdowns are
  non-interactive content inside the card's native button or link. A bar click
  bubbles to the card exactly once; do not select on pointerdown or add segment
  filtering, pinning, click forwarding or nested controls. Preserve native link
  modifiers, keyboard card activation and accessible category counts.
  Experiment summary cards keep status labels and `[count]` in the compact segment
  tooltips; do not repeat an outcome legend below the bar.
  Do not show an execution-status legend in the results header for now.
  A single breakdown category uses its colour dot and name instead of a full-width
  bar. Multiple categories retain the proportionate bar with hover details.
  Results, Tool calls and Cost are a single-select radio group. The active view
  uses Highlight Medium and Highlight Low on hover. Results has recorded score
  with its dot/label, without a duplicate case-percentage Summary column. Tool calls
  combines tool total and failed count, with plain failed labels and no status dot.
  Show these counts only on the row that owns the calls: a native tool span or
  the span owning correlated tool logs or received tool events. Do not repeat
  subtree totals on ancestors. Correlation requires recorded trace/span and call
  IDs; never match by tool name, command, timing or display order.
  Omit zero tool/failed counts and show the separator only when both counts appear.
  Default the resizable identity portion to 320px, Results to 160px and Tool calls
  to 176px; keep saved widths and let the timeline fill the remaining space.
  Cost has Investigator, Grader and Total cost columns, in that
  order. Cells and footer totals show only numeric amounts (Unknown if missing);
  CAD and estimate labels stay in headers, and partial-estimate context is on hover.
  The Cost view control reads `Cost · CAD est.` so its estimate/currency meaning
  is visible before entering that view.
  Costs are presentation-only API-price estimates in CAD. Label them `API estimate`
  and retain the dated pricing/FX provenance. Do not relabel producer costs as CAD,
  infer missing usage as zero, or treat estimates as actual subscription charges.
  Evals lists database-wide experiment summaries and loads full experiment data only for the
  selected experiment, as the other signals do. Summary cards consume summary data rather
  than loading Results to calculate their counts. Reuse existing signal cards,
  headers, tree rows, Details panels and loading states; do not invent new styles.
  The normalized hierarchy is Experiment → Test case → Result. Show only levels
  supported by the source. Native result IDs identify individual results; Result
  labels have no ordinal. Default to available start-time order, retaining source
  order for ties and unknown times. A singleton without repetition metadata puts
  its evidence directly on the test case. Execution/grading are roles, not required
  wrappers. Only an explicit distinct execution span creates an execution row;
  assessment results alone do not create grading spans. Preserve manual toggles.
  Evals tree nodes use standard 8px Primary dots (bg-primary, no outline) when
  no correlated span is available, including experiment roots.
  Available spans supply their service/span colours.
  Duration-only and derived timing bars use Primary too when no span supplies
  a colour. Keep error overrides. Trace-only telemetry cards link to the trace's
  default root view and omit missing-span commentary.
  Telemetry cards omit Started when no start timestamp is available. Confirm
  trace absence before marking IDs Unavailable and disabling navigation; a
  pending or failed lookup does not establish absence. Retain selectable IDs.
  Keep recorded and propagated error indicators; lifecycle status is not a
  substitute for a span colour.
  Omit unavailable experiment lifecycle status from summary cards, headers and
  Details. Do not show Unknown or explanatory lifecycle boilerplate there.
  Cost shows each visible level in the summary column, without adding a tree depth or summing
  parent estimates together with their child estimates. Missing costs are Unknown.
  Result labels are left-aligned beside their tree dots. Attempt is a distinct retry
  node only when a producer explicitly records retries within a Result; adapters do
  not invent it.
  The Experiment root is selectable and collapsible, opens Experiment details, and is initially
  expanded in every view. Experiment timing/cost/outcome summaries include the full experiment,
  even when a case or repetition is filtered out. Never add child costs to an
  already aggregated parent estimate.
  The identity heading reads Experiment · Test case · Result on one line with uniform separator
  spacing and no extra gap between the labels.
  An explicit resultTelemetry reference identifies the enclosing Result span;
  attemptTelemetry does the same for a producer-recorded retry. Telemetry identifies
  target execution. Without an enclosing reference, label derived timing as the
  extent of available references rather than claiming another stored span.
  Align recorded starts to the earliest available span in the experiment, including
  when there is no enclosing experiment span. Keep recorded-duration-only fallbacks.
  Case root bars span available Result references across all repetitions,
  including hidden repetitions. Distinguish elapsed windows from summed durations.
  Never display summed result durations as a case duration in the timeline or
  Details. Without a recorded duration or span-based window, omit the duration.
  Timing fallback order at each level: valid referenced span, available child
  timing extent, recorded duration, then unavailable. Put duration-source context
  in the field heading as `duration · source · type`, using the shared small
  middle-dot separators and Subtle source text.
  Omit the redundant `Recorded duration` qualifier for plain duration values.
  Keep zero durations and never sum overlapping child durations
  into an elapsed window or invent a start offset for duration-only values.
  Missing timing rows stay blank, including started/completed rows; do not draw
  broken-line placeholders. Omit the separator under the Evals column headings.
  When timing is unavailable, omit Duration's heading and sort control and let
  empty space separate left-aligned identities from the right-hand data columns.
  Ruler ticks extend 4px into the header below their labels,
  aligned with the time guides.
  Do not show an execution-status legend in the results header for now; reserve
  that space while a less crowded placement is being selected.
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
  Propagated span errors also colour ancestor duration bars Error, using the same
  execution-error state as the indicators, whether the branch is open or closed.
  Distinguish Error "Span error" from Papaya "Internal fault" (evaluation
  machinery). A failed assessment is "Fail", not an execution error. Preserve
  these meanings during parent propagation, including in dogfooding datasets.
  Evals keeps Details and Telemetry across row modes, with a sibling Assessments
  section when judgments are present. Empty Telemetry retains a muted, disabled
  heading. Assessment details show explanation, criterion, scoring and source
  evidence fields directly, without a duplicate Evidence wrapper. JSON fields
  use folding controls. Keep grader instructions collapsed separately and the
  original input copyable. Collapsible Detail field-group headings have carets.
  These subsection headings use borderless rounded bg-base-200 surfaces,
  regular Subtle labels and Text carets, with shared hover feedback. Unavailable
  top-level headers keep the normal base background and border, with a Highlight
  Low overlay and Muted text.
  Each scoring level (such as Fail, Partial and Pass) has its own field card.
  When the source supplies Overall, put its judgment at the far right of the
  Assessments header and omit the repeated Overall subheading. Preserve its
  explanation, score and full grader-input Copy; do not infer a combined judgment.
  Keep the selected row name and parent context in Details. Results shows case
  questions/outcomes, Result answers/scores, tool output/errors and grader
  scores/explanations at their own level. Tool calls shows the selected level's
  execution/output, timing, faults and tool totals. Cost shows the selected
  level's estimates; Case/Result totals separate investigator and grader costs.
  The whole table row selects Details; tree controls only expand/collapse.
  Native row-name buttons preserve keyboard access. Keep text selection intact.
  Mode changes preserve the selected row, manual disclosure and section sizing.
  Evals Details opens its Details section and available Telemetry section by
  default, sharing the available height equally unless saved sizing exists.
  Experiment Configuration and Grading use sibling section headers in the
  Details stack, initially collapsed.
  The experiment heading uses `· experiment` in the same Secondary style as a
  trace's `· root` suffix.
  Experiment outcome counts sit at the right of this header, not in a repeated
  outcomes field in the body.
  Test-case headers likewise show the recorded outcome breakdown at the right,
  replacing execution completion status and the repeated body legend. Do not
  infer a single combined case verdict.
  Grading contains assessment coverage; investigator/grader totals belong in a
  separate Cost section. Cost and Tool calls view controls are disabled and muted
  when the full experiment has no corresponding recorded data. Recorded zero
  costs or explicitly known zero tool calls remain available. Keyboard navigation
  skips unavailable views; an unavailable selected view falls back to Results.
  Omit Evals Details duration fields for durations of 0–1ns; preserve the recorded
  values in source data and timing geometry.
  Omit 0–1ns duration bars and empty timeline rulers too. Promptfoo's built-in
  echo provider latency measures saved-answer replay, not original execution:
  retain it in the source export but exclude it from execution duration displays.
  Other later sections start collapsed. Preserve manual disclosure and resizing.
  Evals telemetry links reuse SignalCard: service-colour dot and span name,
  neutral status text with its status dot, Secondary provenance, duration,
  and the shared bottom-left Started date. Service identity is on hover. Omit
  navigation hints; the summary card itself is the exact native trace link.
  Inside each Telemetry card, prefix the inline trace and span IDs with `Traces ·`
  to name the navigation destination. Use shared Identifier rendering, without
  visible TID/SID labels, brackets or Copy buttons. Use Subtle
  for the trace ID and Text for the span ID. The IDs remain part of the native
  card link; brighten the trace ID and underline both IDs on card hover and
  keyboard focus to indicate navigation. Keep Telemetry as a sibling section in every
  row mode, including Tool calls. Do not substitute a parent's IDs or invent a
  span ID when only a trace reference is recorded.
  Span-event summaries follow the log-card layout: event identity and plain
  Secondary `Span event` provenance, recorded exception message when present,
  and the shared bottom-left Recorded date/offset. Preserve the exception dot
  and disclosure controls; do not invent severity or span status for events.
  Event cards use the recorded tool name when supplied, retaining a different
  underlying event name in a typed field. Their Recorded row shows the offset from
  the parent span; retain the full timestamp in its field and on hover.
  Keep tool execution in the waterfall; do not repeat its Tool calls list in
  Result Details.
- All signal cards place the shared date row at the bottom-left, after summary
  content. Label its meaning: `Started` for traces/Evals,
  `Last seen` for metrics and `Recorded` for logs. Align values through the shared
  SignalDate component. Use the selected timezone without repeating its suffix;
  retain the full timezone on hover and in Details. Missing dates stay unavailable,
  not invented. Keep trace `Duration:` separately labelled.
  Missing dates display `—` with the reason on hover.
  Evals experiment cards omit the entire Started row when no valid start is available.
  Let date labels use their natural width with the shared gap; do not reserve a
  wide empty label column.
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
  Keep counts visible when collapsed; omit counts and their reserved space for leaves.
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
  Activity dots overlay duration bars at the same Y position, with the bar below
  the dots. Position logs by their recorded timestamps on the trace timeline,
  even when outside their owning span's duration. Preserve exact log timestamps
  and offsets; logs must not extend the time axis or distort bars.
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
  separate axis rules. All charts place their Y-axis label horizontally above
  the axis, including histogram count and heatmap value/unit labels. Histogram
  value/unit labels sit above the right end of the X-axis.
  Time-series charts use a 96px left inset to reclaim the side-label gutter, with 80px
  on the right for average/slope labels.
  When zero coincides with the bottom axis, use its rounded join without a
  duplicate straight zero rule, zero tick or zero label. Retain zero rules and labels inside the plot.
  Statistics can show min/max/average for an explicitly selected series or the
  sole visible source series without selecting a datapoint. Rate slope still
  requires a selected point; do not choose an arbitrary series when several are visible.
  Average guides use the percentile guides' background backing and emphasis widths.
  Average and slope guides render behind the data lines.
  Their labels sit outside the right plot edge like percentile labels, with exact
  formatted values on hover; slope is not repeated in the selected-point card.
  Max labels move below their marker when their measured height will not fit above it.
  Slope guides extend backward from the selected point and stop there, without
  implying a forecast beyond it.
  Axis tick marks also use 1px Muted. The shared border is
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
  cells across colour bands and round their shared outer boundaries. Round internal
  two-colour turns with a shared curve: three matching occupied quadrants fill the
  corner of the fourth, with no background gap. Keep multi-colour junctions and
  diagonal-only contacts intact. Use the same density-scaled corner radius.
  Keep each cell's count colour and preserve empty buckets and holes.
  Joined shapes retain the existing density-scaled border around their shared
  perimeter and holes, following each cell's colour. Give internal colour regions
  the same liner: 85% of their fill mixed with 15% Text in OKLab, clipped inside
  each region's shared rounded boundary. Keep the fills touching without gaps.
  Do not add decorative grid styling.
  Histogram/heatmap count legends sit outside the header, directly above the
  graph on the chart surface, with breathing room above and below. Right-align a
  compact group of at most six columns, with dots aligned vertically across rows.
  Size columns to the widest whole entry and use consistent gaps; reduce the
  column count when the available width requires it.
  Keep Heatmap/Histogram tabs and Percentiles
  together in the compact header's second row, right-aligned. Retain the global
  time-range selector without repeating the chart time range. At the regular 14px size,
  legend rows fit up to six whole entries as the measured width permits. Move one
  entry down only to avoid a lone final item when the pair fits and at least two
  entries remain in the previous row. Keep count order and whole
  labels.
  Use plain 8px colour dots beside numeric count ranges, without borders or outlines.
  Horizontally overflowing histograms and heatmaps expose their native scrollbar
  until the first deliberate horizontal scroll, then resume normal platform
  scrollbar behaviour for the rest of that chart visit. Chromium/WebKit use a
  temporary 8px native scrollbar with a Muted thumb; standard-only engines retain
  their platform visibility policy. Keep an active thumb drag intact until release.
  Loading, resizing, focus and restored scroll positions do not mark scrolling as
  discovered. Omit the former text/caret hint. Preserve chart geometry and native
  wheel, touch, scrollbar and keyboard scrolling.

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
  Field values that fit in three rendered lines stay whole. Longer values use
  native two-line ellipsis and Show more / Show less on the next line, including
  multiline text and Markdown across all signals. Structured JSON instead uses
  Expand all / Collapse all controls in its field header, with only the first
  container level open by default and nested objects and arrays folded. When the
  first array item is a container, open that item one additional level too; keep
  deeper contents and later items folded. Preserve manual folds. Keep Copy exact and
  complete. Evals answers use an Output field
  card rather than unlabeled prose.
  Evals type labels describe the underlying value, not its formatted display.
  Scores, counts and costs are numbers; duration is numeric (int64 for span
  nanoseconds). Preserve native output and configuration types through projection;
  do not infer types by parsing strings that happen to resemble JSON or numbers.
- Details has no fixed maximum width. Preserve its saved width above 40rem and
  let the shared split limit the displayed width using the main pane's existing
  minimum (30% or 28rem, whichever is larger). The drawer retains its 40rem cap.
- Persistent-shell rule: pane identity follows its role, not its position or the
  number of visible panes. Use optional List, required Main, and optional Details
  slots. Keep shared navigation, outer shell and surviving pane containers mounted
  across routes; replace workspace content inside those slots.
  Use one app-wide saved set of List/Details widths and responsive split sizing,
  not separate sizes per signal or route. Main takes the remaining space.
  When a route omits a pane, Main absorbs that space; do not redistribute widths
  among surviving side panes or overwrite the omitted pane's saved width/open
  preference. Restore those preferences when that role returns.
  Resolve route pane availability and saved geometry before showing destination
  content. Loading, empty results and failed requests render inside the declared
  slots; they must not temporarily remove panes or introduce a second layout shift.
  Route changes apply their destination geometry immediately, without width/height
  tweening, entrance choreography, or replaying panel-open transitions. Explicit
  pointer toggles may use the shared interruptible settle motion; keyboard changes
  are immediate and dragging follows the pointer directly. Preserve reduced motion.
  Responsive clamping affects rendered geometry only, not saved user sizes. Keep
  outer insets, header heights and gaps consistent between two- and three-pane
  layouts. Verify cross-route DOM identity and geometry as well as restored state.
- Disabled and zero-count Details headers use Muted for their labels, icons and counts.
  Pin these closed, unavailable sections to the bottom, including when every
  section is unavailable. Keep the normal top-level
  background and border with a Highlight Low overlay and Muted text. Keep canonical
  order within each group: open, closed available, then inactive. The primary
  (first) section stays open at the top and cannot collapse by click, keyboard or
  drag. Use the existing sticky heading and scroll container, without extra
  wrappers or CSS overrides. Restore an
  available section to the group matching its retained disclosure state.
  Muted uses the source Kasure 1 colour; do not darken disabled text.
- Detail-section resize precedence: keep the primary section open.
  Use each non-first open section header as the resize
  handle for the boundary above it; omit separate resize strips. A click toggles
  disclosure, while a drag beyond the click threshold resizes without toggling.
  Keep header actions independent and preserve keyboard resizing and size reset.
  Draggable headers use grab/grabbing cursors; stacked sections have a 4px gap.
  Open sections fill the remaining height. Dragging a header grows the
  adjacent open section on one side by shrinking the adjacent open section on
  the other. Closed and inactive sections never become resize donors or reopen
  through dragging. Defer drag-induced collapse and regrouping until release.
  Heading collapse hands space to the nearest open section above.
  Reopening restores remembered size within
  available bounds, taking space from above first. Preserve mounted section
  identity, focus, scroll positions and JSON folds when regrouping or reopening.
  Use stable keyed snippets in the shared Details stack across signals and Evals.
  Use native details/summary disclosure with immediate grouping. Keep one open
  state; do not add delayed docking, fold animations or a reveal registry.
  Pointer and keyboard changes are immediate, including reduced motion.
  Press 0 on a resize header to reset section sizes.
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
