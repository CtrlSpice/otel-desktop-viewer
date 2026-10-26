# Seasonal themes and graph colours

## Interface themes

The interface uses Madoromi's four seasons, each in Day and Night:

| Season | Day                   | Night                 |
| ------ | --------------------- | --------------------- |
| Spring | `madoromi-haru-hiru`  | `madoromi-haru-yoru`  |
| Summer | `madoromi-natsu-hiru` | `madoromi-natsu-yoru` |
| Autumn | `madoromi-aki-hiru`   | `madoromi-aki-yoru`   |
| Winter | `madoromi-fuyu-hiru`  | `madoromi-fuyu-yoru`  |

The season picker and Day/Night button share one compact control group. The
picker opens below its icon and season name, end-aligned and viewport-clamped.
Four dropdown rows select Spring, Summer, Autumn or Winter. The radio group has one Tab
stop and supports arrow keys and Home/End. Opening focuses the selected season.
Changes keep the picker open; Escape, the trigger or clicking outside dismisses
it. Madoromi attribution and the pinned source licence are retained in this
documentation; no attribution footer is displayed in the workspace.
There are no ink or swatch controls.

One six-role ink palette supplies accents across all four seasons.
Season changes affect the paper, neutral text and highlights; Day/Night adjusts
foreground strength for readability. The complete theme ID is saved under
`theme`. Saved choices take precedence; otherwise Autumn uses the system mode.
Blocked storage retains the current page's choice. Appearance changes do not
reassign graph colours.

`data-theme` selects the generated CSS; `data-color-scheme` identifies light/dark
behaviour. Charts observe the same theme identity through `state/theme.svelte.ts`.
Font families, sizes, weights and layout use the existing shared tokens.
Summary card titles use the shared 17px regular Atkinson Hyperlegible Next size,
with 14px metadata and unchanged Text colour. Pane headings remain 16px.
Trace-service and evaluation-outcome breakdowns keep their 8px bars when only
one category is present, filling the available width. Card bars retain shared
hover details and accessible category counts without additional keyboard stops.
Card date rows use relative time, with the full recorded timestamp and selected
timezone on hover. Parent-relative event offsets remain signed durations.
Trace and referenced-telemetry cards share one bottom row: Start at the left,
Duration at the right. Narrow cards wrap the two groups without reducing text
size. Durations of at least a minute use minutes, and durations of at least an
hour use hours, with at most one fractional digit. Finer durations retain their
existing seconds, milliseconds, microseconds or nanoseconds.
Metric summary cards omit descriptions; received descriptions remain in Details
and the complete source/Copy/Download views.
Metric cards put Last seen at the left and Last value at the right of the same
bottom row when a reading is available.

## Ownership and generation

- `madoromi.json`: generated interface colours for all eight variants.
- `interface-palette.json`: the fixed Iris, Pine, Foam, Gold, Rose and Love accents.
- `heatmap-palette.json`: generated sequential ink spectrum, sizes 1–12.
- `count-palettes.json`: source ink curves sampled at sizes 1–12.
- `count-palette-names.json`: generated English ink names from the pinned source.
- `count-palettes.ts`: source ink names and identities for the graph catalogue.
- `index.ts`: catalogue, seasonal selection and graph lookup.
- `tokens.ts` / `plugin.ts`: shared DaisyUI, chart and editor CSS adapter.
- `graph-inks.json`: official ink swatches and 224 categorical slots.
- `graph-ink-curves.json`: generated slider colours and exact preset positions.
- `graph-inks.ts`: shared seven-shade catalogue and stable ink/shade identities.
- `service-ink-slots.ts`: the shared automatic slot sequence.
- `bright-ink-slots.json`: one generated bright-only ranking across all 32 inks.
- `../utils/color-slots.ts`: saved service/series identity allocation.
- `../state/graph-color-overrides.svelte.ts`: persisted, reactive per-identity overrides.

The Madoromi source is pinned to
[`f8bcc5aa1ec3f731fcba9471be7caa5df1ab5695`](https://github.com/the-moonwitch/madoromi/tree/f8bcc5aa1ec3f731fcba9471be7caa5df1ab5695).
Its original palette, conversion code and full MPL-2.0 licence are in
`third_party/madoromi/`. From the frontend directory, regenerate the adapter with:

```sh
node scripts/generate-madoromi.mjs
node scripts/generate-bright-ink-slots.mjs
```

The licence, original inputs, generator and adapted theme JSON are distributed
at `/licenses/madoromi/`. Madoromi-derived files remain MPL-2.0; the project's
Apache-2.0 licence is unchanged.

## Surfaces and readable text

The approved background tint is **20% of the source OKLCH chroma**, retaining its
lightness and hue. This applies to kami, hyoshi, chitsu and nijimi 0–2 in all
variants. Surface roles follow Madoromi's pinned
[paper guide](https://github.com/the-moonwitch/madoromi/blob/f8bcc5aa1ec3f731fcba9471be7caa5df1ab5695/palettes/style-guide.md#paper-紙)
in both Day and Night:

| Viewer role            | Day             | Night           |
| ---------------------- | --------------- | --------------- |
| Canvas / bg-100 / code | Kami, brightest | Kami, brightest |
| Panel / bg-200         | Hyoshi          | Hyoshi          |
| Chrome / bg-300        | Chitsu, darkest | Chitsu, darkest |

The workspace, List and Details bodies, outer navigation and pagination share
one uniform Kami background. Resting signal-list and Details cards are
transparent; only hover and selection add a fill. Service-map nodes retain
their existing Panel surface. Pane/section headings use
Hyoshi, and code toolbars use Chitsu.
The upstream VS Code port also places sidebar section headings on Hyoshi. This
keeps the viewer's original Subtle labels readable: Autumn Day's native Usuzumi 1
falls just below 4.5:1 on Chitsu. Data-column headings stay on the data canvas.

Search, filters, selects, date/time groups and editor Find fields use Hyoshi
through `--color-input`, with a 1px Nijimi 0 boundary (`--color-border`). The
pinned [VS Code port](https://github.com/the-moonwitch/madoromi/blob/f8bcc5aa1ec3f731fcba9471be7caa5df1ab5695/src/ports/vscode.ts)
explicitly assigns these input roles; the
[Zed port](https://github.com/the-moonwitch/madoromi/blob/f8bcc5aa1ec3f731fcba9471be7caa5df1ab5695/src/ports/zed.ts)
also uses Hyoshi surfaces and Nijimi 0 boundaries. Focus uses the selected
ink's Primary; validation uses Error. Composite fields have one outer boundary
and one focus ring, with transparent, borderless inner editors. Text and carets
use the working line-height, not the full control height.

Borders mark an editable control, floating surface or meaningful divider; they
do not enclose every visual group. Pane bodies, headers and ordinary signal
cards stay borderless. Code is flat unless it is part of an explicitly joined
terminal, which has one enclosing boundary. Unchecked checkbox outlines use
Nijimi 1; round checked controls use neutral Text, with Primary keyboard focus. Cards use shared hover/selection fills
without acquiring shadows. Chart axes, tree connectors and keyboard focus rings
retain their distinct meanings. Home's dotted import box sits below the Getting started header,
matches the terminals' width and border colour, and explains that files can be
dropped anywhere.

Workspace edges and pane gutters use a shared 16px spacing. Pane headings,
signal cards and Details fields use 16px horizontal content insets; the List
pane uses the adjacent gutter rather than adding another trailing outer inset.
Related header icons use 2px gaps, choices/tabs 4px and distinct groups 8px;
data-row heights stay compact.

Hover uses tinted nijimi 0. Persistent selection uses tinted nijimi 2 at 45%
over the panel, blended in sRGB. Stronger reveal highlights use tinted
nijimi 1. Hover takes precedence over selection while the pointer is present.
Ghost buttons use the same hover and active/selected surfaces. Tooltips use
Panel with regular-weight neutral Text. Dropdowns, colour pickers, filter
popovers, pinned chart readouts, query-help dialogs and search completions share the same opaque Panel
surface, Nijimi 0 boundary, rounded corners and soft `--popover-shadow` in
`app.css`, without translucent fills or blur.
Picker-specific styles control layout rather than adding another surface theme.
CodeMirror's portaled completion menu uses the shared stylesheet too; the editor
does not inject a separate dropdown stylesheet at runtime. Timeline activity markers retain their
assigned ink or Error colour without brightening. Swatch glyphs choose the
higher-contrast seasonal light/dark foreground using linear-sRGB luminance,
recalculating when the theme changes.

These are viewer mappings of upstream roles, not a claim that the guide specifies
every component's geometry. The viewer retains its approved 20% paper chroma,
regular Atkinson typography, readable Subtle placeholders and neutral telemetry
values. Existing selection opacity, shared status colours and fixed categorical
assignments remain the viewer's documented adaptations.

List filters use `ListFilter.svelte`: a header icon reveals and focuses the shared
32px filter field. The icon marks a nonempty query even while collapsed. Escape
collapses the field without clearing its query and restores focus to the icon;
inside a popover, this first Escape leaves the popover open. Attribute keys,
service choices, columns, inks and timezones share this behaviour. Main workspace
search and editor Find remain separate controls.

| Shared role                           | Palette role |
| ------------------------------------- | ------------ |
| Text                                  | Sumi         |
| Subtle: keys, headers and helper text | Usuzumi 1    |
| Muted: unavailable and disabled       | Kasure 1     |
| Primary / focus                       | Iris         |
| Secondary / metadata                  | Pine         |
| Accent                                | Foam         |
| Info                                  | Pine         |
| Success                               | Foam         |
| Attribution heart                     | Sakura       |
| Warning markers and fills             | Gold         |
| Error markers                         | Love         |

These role assignments apply to all eight variants. The six accents are identical
across seasons within each mode. Info uses blue Pine and Success uses green Foam.

The generator minimally adjusts Text and accent foregrounds in 0.001 OKLCH
lightness steps until they reach 4.5:1 on canvas, panel, chrome, hover and
selection. The six ink-derived accents are softened before these corrections and
checked against all four seasons together. Stronger reveal highlights are outside
this five-background target. Subtle, Muted and the original Sakura attribution
colour retain their source values without contrast adjustments. Warning
and received error values use neutral Text with meaningful coloured markers.
Papaya (`#ff8000`) continues to identify internal faults rather than span errors.

## Editor roles

The interface uses six complementary roles inspired by Rosé Pine's balance, with
colours derived from the pinned Madoromi ink curves:

| Role | Ink              | Character and use                                                 |
| ---- | ---------------- | ----------------------------------------------------------------- |
| Iris | Murasaki-shikibu | Soft violet controls, focus and keywords                          |
| Pine | Tsuki-yo         | Definite blue for Secondary, information, code keys and operators |
| Foam | Shin-ryoku       | Mint green for success and accents                                |
| Gold | To-ro            | Honey-gold warnings and code types                                |
| Rose | Fuyu-gaki        | Warm rose annotations, functions and cursors                      |
| Love | Momiji           | Berry-rose errors, built-ins and escapes                          |

Pine is deeper and bluer; Foam is lighter and greener. Each source is sampled at
a fixed point on its curve, with softened chroma and a stable hue for Day/Night.
Readable Day strengths are deeper; Night uses the lighter colours. This is a
viewer adaptation, not an upstream Madoromi mix or a copy of Rosé Pine's hexes.
Season supplies the existing paper, neutral text, punctuation and Nijimi
highlights. The theme generator writes the shared accents to
`interface-palette.json`; the Sakura attribution heart retains its original shade.
Semantic roles retain the guide's slots: Fuji for focus, Sora for information,
Wakaba for success, Yamabuki for warnings and Beni for errors. The custom palette
changes their ink/shade values. Secondary uses Pine/Tsuki-yo blue. The Rose
syntax slots retain Fuyu-gaki's warm hue.

Navigation stays left-aligned; search fills the
available desktop space and appearance controls stay together at the right.
Narrow screens place navigation above search and appearance controls; the
smallest screens give search a full-width row.

JSON, search, shell and read-only code views share
`utils/editor-highlighting.ts` under `components/shared`. They follow Madoromi's
[native code guide](https://github.com/the-moonwitch/madoromi/blob/f8bcc5aa1ec3f731fcba9471be7caa5df1ab5695/palettes/style-guide.md#code-符),
using the categories supplied by each language parser. The viewer keeps strings,
including JSON/query quote marks, and literal values in neutral Sumi for calmer,
cohesive reading. Keys, punctuation and escapes retain their distinct roles:

| Syntax category                       | Madoromi role slot |
| ------------------------------------- | ------------------ |
| Names and variables                   | Sumi               |
| String bodies                         | Sumi               |
| JSON/query quote marks                | Sumi               |
| Keywords                              | Fuji               |
| Properties and keys                   | Ruri               |
| Operators                             | Sora               |
| Numbers, booleans, constants and null | Sumi               |
| Escapes, regular expressions and tags | Botan              |
| Characters, enum members and macros   | Tokiwa             |
| Functions and methods                 | Ai                 |
| Built-ins and self/this               | Beni               |
| Types, classes and attributes         | Yamabuki           |
| Namespaces                            | Kikyo              |
| Annotations                           | Nadeshiko          |
| Labels and editor cursors             | Sakura             |
| Punctuation                           | Usuzumi 0          |
| Comments                              | Kasure 2           |
| Invalid syntax                        | Beni / Error       |

The Madoromi syntax slot names map to the six roles above. The resolved `syntax`
palette also contains each season's readable
Sumi, Kasure 2 and Usuzumi 0. Its foregrounds meet the same five-background 4.5:1
target as interface text. Code uses `--color-code-*` tokens independently of
interface Primary/Secondary aliases. `--color-code-literal` resolves to Sumi,
including runtime literals in formatted JSON and bare query values. Comments
and self/this are italic; code keeps regular weight. Markdown code uses Sumi.
The palette also exposes Akane, the guide's italic parameter colour. The current
JSON, query and shell grammars do not emit a distinct parameter tag.

Compact JSON shares these colours. JSON quote and escape highlighting adds
decorations over the existing syntax tree; it does not replace the document.
Decoded string values in Properties use Sumi, while raw escapes use Botan. Shell syntax uses
the shared StreamLanguage parser; highlighting follows its supplied tags.

Search cursors use Sakura. Editor search matches use Kaki at 18% opacity, with
35% for the current match. Matching brackets use tinted Nijimi 2 at 30% with a
Kasure 0 outline. Selection uses the shared selection token.

## Typography and data fidelity

Interface text, field names, formatted values and search/filter inputs use
Atkinson Hyperlegible Next, including placeholders and editor Find fields.
JSON and code use Atkinson Hyperlegible Mono. Shared tokens in `../app.css`
set 14px working text, 16px page/pane headings and 1.5 line-height at the default
root size, with regular weight for data and controls. Field names keep their
exact spelling.
The complete time-range menu uses Next, including dates, times, calendars,
timezone names and ambiguity choices. Its 32rem preferred width is clamped to
the viewport; endpoint controls wrap into two rows on narrow menus.
Details field names use Primary, with type/provenance metadata in Subtle.
Section/column headers and helper text use Subtle; data and main titles use
Text. Muted is for unavailable or disabled content.

Properties shows ordinary strings decoded without outer JSON quotes, retaining
meaningful quotes/backslashes and displaying empty strings as `""`. Shared
`../components/shared/detail-value.ts` formatting can expand complete embedded
JSON-like containers, including `undefined`, `NaN` and `Infinity`, for display.
It preserves numeric lexemes and duplicate keys without evaluating code or
changing received types. Copy uses the complete original value; raw JSON and
Download retain source values. Wrapping, folding, syntax decorations and
middle-truncated identifiers are display-only. Copy actions are explicit;
ordinary values remain selectable text.

## Graph palettes

Metric Y-axis tick values are right-aligned 8px before the axis. Horizontal unit
headings start above the axis, with a 32px top inset that accommodates larger text.
The left gutter is measured from rendered Y labels and angled X labels. It updates
after label, font and size changes; fixed left insets are only pre-measurement
fallbacks. Metric chart headers keep the service-colour dot without repeating the
service name beside the metric title.

The categorical catalogue contains the first seven official swatches from the same
32 colourful Madoromi inks: 224 distinct colours. Automatic allocation uses its
118 bright swatches across all 32 families. The original
eighth (palest) shade remains in the distributed source but is omitted from automatic allocation.
These are the
upstream eight-step ramp's original colour values, not lightness-resampled count
bands. Each colour retains its exact hex value across seasons and Day/Night.

Slot order visits every ink in Madoromi's pinned colour order at shade 1, then
at shade 2, through shade 7. Ink/shade IDs, such as `rikka:4`, identify overrides.
Assignments use the `madoromi-inks-v1` allocation namespace.

Services, metric series and single-service span names share one bright ranking.
They retain separate scopes: exact service-name text viewer-wide, native series
identity per `metricRef`, and exact span-name text. Each scope has one saved set
of assignments, independent of appearance.
Filtering, ordering and paging do not release slots. New identities never move
existing owners. Fresh assignments can depend on encounter order. Blocked/full
storage falls back to in-memory stability.

`scripts/generate-bright-ink-slots.mjs` admits all official swatches with OKLab
lightness 0.55–0.87 and chroma at least 0.10. It starts with Shin-kai and visits
one bright representative from every family before using additional shades.
Within each stage, it chooses the swatch whose minimum OKLab distance from
already-used colours is greatest. There is no selected-ink restriction or 3:2
ratio. `node scripts/generate-bright-ink-slots.mjs --check` verifies the generated
sequence. This is a viewer ordering rule over unchanged Madoromi swatches.

Each new identity hashes into the bright sequence using FNV-1a with a 32-bit
avalanche, then probes forward past occupied slots and manual choices. Already
saved assignments retain their slots; changing allocation does not recolour them.
The keys are `graph-colour-slots:madoromi-inks-v1:224:<scope>:fixed-bright-v1`, where
scope is `services`, `metric:<metricRef>` or `span-names`.
Prior automatic assignments retain their exact slots, including hidden owners;
their original storage remains intact. The bright-only rule applies to new owners. Explicit
overrides stay in their existing ownership scopes and remain fixed. Previous
per-ink owners are carried forward with the last active ink's encounter order
first, including hidden identities. Old palette settings no longer control the UI.

After all bright slots are occupied, new identities reuse a hashed bright slot without
moving previous owners. `colorSlotReuses()` exposes read-only diagnostics and one
console warning is emitted per exhausted scope. The scalar chart's 22-visible-
series cap is independent of the 118-colour automatic capacity. Exhaustion stays
within this pool rather than adding near-black or pale extremes.

Home service names retain their assigned 8px dots with an 8px gap before the name.
Service dots open a shared colour popover independently of card selection.
Gauge/Sum series with assigned colours have the same editable dot before their
attribute values. The popover uses vanilla-colorful's dependency-free hex picker,
with a hue slider, colour field, compact 32px hex input and Reset to automatic. Its surrounding
controls use the app's seasonal surfaces, regular Atkinson text and focus styles.
Seven 8px dots offer the current ink's official shades. Named preset and curve
overrides retain their ink identity; custom RGB values use the closest official
shade in OKLab. Ink names remain in the shade tooltips and accessible labels,
without a separate visible heading. The rounded colour field and hue strip fit
inside the shared popover padding. The picker has no opacity control.
Scalar charts show individual series by default. Sum and Average are independent
checkboxes where supported, and can appear together as `agg=sum-avg`; Gauge offers
Average. Values and Rate tabs appear only for metrics that support Rate; returning
to Values restores the prior combined-line choice for the current metric visit.
Existing `agg=raw|sum|avg|rate` links and saved choices retain their meanings.
Source lines keep their recorded timestamps when combined lines are displayed.
Whole-series average annotations are labelled Series avg. The chart measures
right-side label widths to reserve space for their full text.
Checkboxes use
the shared neutral Text checkmark and round outline on a transparent background, independent of series
colour. Saved choices survive visibility changes. Reset restores the owner's
saved automatic assignment without moving other owners. Previously coloured series
retain their editor while unchecked; editing them does not change chart visibility.

The sortable Series table uses an exact minimum set of attribute columns that distinguishes
all series in the metric, including hidden series. Equivalent choices favour
shorter labels, with deterministic key ordering. Full attributes remain on hover
and in the expanded fields. Ordinary labels wrap in narrow panes; single string
values in `id`/`uuid` fields use the shared one-line, middle-truncated Identifier
with the full value on hover and for accessibility. Missing values, received types and duplicate keys
remain distinct. If attributes alone cannot distinguish rows, the heading adds
the native series identifier; this never changes grouping or colour ownership.
Sorting cycles ascending, descending and received order without changing visibility
or datapoint selection. Column filters are omitted.

Metric Details has separate Metric, Series and Datapoints sections. Selecting a
series loads its received datapoints in Datapoints; changing series clears the
selected datapoint. The left caret reveals series attributes independently, and
the rightmost Show checkbox controls chart visibility. The editable colour dot
has its own narrow column directly before the first distinguishing value. Inspection, including URL
restoration, does not change saved visibility or colours.

Selecting a datapoint reveals its fields and its existing chart highlight or
histogram contribution. Chart point picks stay within the inspected series;
chart interval picks highlight that series' received rows in the interval.
They do not switch series or reopen closed sections. Attributes stay under the
series row, not in the datapoint table. All three sections use the shared Details
stack, resizing and independent scrolling.

Existing overrides are read from `graph-colour-overrides:madoromi-inks-v1:<scope>` as
identity/ink-shade pairs; new series choices store six-digit RGB hex values in
the same scope. Services share the `services` scope, keyed by service-name
text rather than Home's typed discovery identity. Series use `metric:<metricRef>`,
keyed by `seriesRef`. These still apply to chart lines, sparklines, service dots,
summary bars and non-error waterfall bars. New automatic assignments avoid saved
manual shades and occupied automatic slots. Actual errors retain the Error role.

Waterfall dots and duration bars use exact span-name colours when the full trace
has only one service identity, including an unnamed service. Repeated names share
a colour; these assignments use the separate `span-names` scope and never reserve
or change service colours. With multiple service identities, waterfall rows use
their service colours and overrides. Collapsing, searching or scrolling does not
change the colour basis. Trace summary bars and service indicators always retain
service colours. Error spans and exception events retain the semantic Error override.
This follows [Honeycomb's default waterfall colour basis](https://docs.honeycomb.io/investigate/analyze/explore-traces/#change-color-in-waterfall-representation).

Histogram and exponential-histogram heatmaps, distribution bars and count
legends share a Murasaki-shikibu (Primary/Iris) → Momiji (Love) → Fuyu-gaki (Rose) ramp.
Counts remain sequential: perceived lightness and grayscale luminance increase
steadily at Night.
Legacy count-palette preferences are left in storage but no
longer select a different ramp.

The generator takes hue/chroma anchors from Murasaki-shikibu, Tsuki-yo,
Shin-ryoku and To-ro. Their OKLab lightnesses are 0.48, 0.61, 0.74 and 0.87,
avoiding near-black and paper-white endpoints. It interpolates in OKLab,
maps into sRGB gamut, and samples at equal perceptual arc-length intervals.
Each adaptive ramp size from 1–12 is generated separately. Night uses
dark-to-light count order; Day uses the exact same shades reversed. A singleton
uses its mode's high-count endpoint. Seasons do not change the ramp.
`themes/heatmap-palette.json` records the source anchors and generated ramps;
`utils/chart-palette.ts` provides the pure sampler.

Percentile guides use fixed official chart swatches: Shin-ryoku shade 6 (green)
for p50, Daikokuten shade 2 (amber-yellow) for p95 and Benzaiten shade 2 (red)
for p99. Histogram and heatmap views share these colours in all seasons and
Day/Night modes. Labels remain neutral text.

Count thresholds, source zero/empty buckets and categorical identity assignments
retain their existing treatment. Semantic error overrides retain the active
interface's shared tokens.
