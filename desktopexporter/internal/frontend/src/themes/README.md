# Seasonal themes and graph colours

## Interface themes

The interface uses Madoromi's four seasons, each in Day and Night:

| Season | Day                   | Night                 |
| ------ | --------------------- | --------------------- |
| Spring | `madoromi-haru-hiru`  | `madoromi-haru-yoru`  |
| Summer | `madoromi-natsu-hiru` | `madoromi-natsu-yoru` |
| Autumn | `madoromi-aki-hiru`   | `madoromi-aki-yoru`   |
| Winter | `madoromi-fuyu-hiru`  | `madoromi-fuyu-yoru`  |

The theme picker and day/night button share one compact control group.
The picker opens below its button, end-aligned and clamped to the viewport, with
a compact swatch strip below the full-width live app-style sample preview.
The shared section header shows `Preview: <original ink name>` at the left and four icon-only
season controls at the right. The shared sample trace summary and service breakdown sit above the JSON
inspector in one narrow column, without a separate preview header or attributes
section. Surfaces use shared rounded corners without nested borders. Sample
service colours are fixed and do not allocate saved assignments.
The ink strip sits below the JSON and above attribution.
There is no separate season row or redundant Theme heading.
Ink and season changes apply immediately and keep the
picker open; Escape, clicking outside or the trigger dismisses it. There is no
separate close button in the anchored dropdown.
Its centred footer reads “Colours: Madoromi” with the attribution heart;
only Madoromi is the link.
Ink choices show three vertically stacked colour dots, brightest at the top
(official shades 7, 4, 1), in adjacent 24px-wide, 48px-high targets in one horizontal row,
with the selected original ink name shown once above them. All choices retain
original-name tooltips and accessible labels, without English translations.
The chooser is at most 28rem wide. The ink row uses ordinary horizontal scrolling,
with left/right caret buttons as a secondary way to scroll. Each caret is disabled
at its end; scrolling does not change the ink selection. There is no snapping or
auto-advance. Four icon-only season choices sit at the right of the Preview header.
Section names
remain accessible but visually hidden; there is no filter field. Flower, sun,
leaf and snowflake icons select Haru (Spring), Natsu (Summer), Aki (Autumn) and
Fuyu (Winter). Season changes keep the picker open so its inherited paper,
hover and selection surfaces update around the unchanged ink swatches. Changing
season retains the current mode and ink; changing mode retains the season. The complete
theme ID is saved in localStorage under `theme`. A valid saved choice takes
precedence over the operating system's mode. Otherwise, Autumn uses the system
preference read on load. If storage is blocked, controls still work for the
current page.

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
- `accent-inks.json` / `accent-inks.ts`: generated analogous families and seed catalogue.
- `count-palettes.json`: generated Madoromi histogram count ramps, sizes 1–12.
- `count-palette-names.json`: generated English ink names from the pinned source.
- `count-palettes.ts`: ordered count-palette catalogue and default To-ro ink.
- `index.ts`: catalogue, seasonal selection and graph lookup.
- `tokens.ts` / `plugin.ts`: shared DaisyUI, chart and editor CSS adapter.
- `graph-inks.json`: official ink swatches and 224 categorical slots.
- `graph-ink-curves.json`: generated slider colours and exact preset positions.
- `graph-inks.ts`: shared seven-shade catalogue and stable ink/shade identities.
- `../utils/color-slots.ts`: saved service/series identity allocation.
- `../state/graph-color-overrides.svelte.ts`: persisted, reactive per-identity overrides.
- `../state/accent-ink.svelte.ts` / `../state/count-palette.svelte.ts`: independent saved ink choices.

The Madoromi source is pinned to
[`f8bcc5aa1ec3f731fcba9471be7caa5df1ab5695`](https://github.com/the-moonwitch/madoromi/tree/f8bcc5aa1ec3f731fcba9471be7caa5df1ab5695).
Its original palette, conversion code and full MPL-2.0 licence are in
`third_party/madoromi/`. From the frontend directory, regenerate the adapter with:

```sh
node scripts/generate-madoromi.mjs
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

Search and filter inputs use the darker Chitsu for their fill through
`--color-input`, without decorative borders. This includes
query entry, picker filters and editor Find fields. Query editors inside the
shared search bar remain transparent so the input surface stays continuous.
Pane and section headers, code panels, popovers and outer pane surfaces are
borderless too. Keyboard focus rings, chart axes, tree connectors, meaningful
dividers, checkbox outlines and the file-drop affordance remain intact.

Hover uses tinted nijimi 0. Persistent selection uses tinted nijimi 2 at 45%
over the panel, blended in sRGB. Stronger reveal highlights use tinted
nijimi 1. Hover takes precedence over selection while the pointer is present.
Ghost buttons use the same hover and active/selected surfaces. Tooltips use
Panel with regular-weight neutral Text. Dropdowns, colour pickers, filter
popovers and search completions share the same opaque Panel surface, rounded
corners and soft `--popover-shadow` in `app.css`, without translucent fills or blur.
Picker-specific styles control layout rather than adding another surface theme.
CodeMirror's portaled completion menu uses the shared stylesheet too; the editor
does not inject a separate dropdown stylesheet at runtime. Timeline activity markers retain their
assigned ink or Error colour without brightening. Swatch glyphs choose the
higher-contrast seasonal light/dark foreground using linear-sRGB luminance,
recalculating when the theme changes.

List filters use `ListFilter.svelte`: a header icon reveals and focuses the shared
32px filter field. The icon marks a nonempty query even while collapsed. Escape
collapses the field without clearing its query and restores focus to the icon;
inside a popover, this first Escape leaves the popover open. Attribute keys,
service choices, columns, inks and timezones share this behaviour. Main workspace
search and editor Find remain separate controls.

| Shared role                           | Madoromi source |
| ------------------------------------- | --------------- |
| Text                                  | Sumi            |
| Subtle: keys, headers and helper text | Usuzumi 1       |
| Muted: unavailable and disabled       | Kasure 1        |
| Primary / focus                       | Fuji            |
| Secondary / metadata                  | Ruri            |
| Accent                                | Tokiwa          |
| Info                                  | Sora            |
| Success                               | Wakaba          |
| Attribution heart                     | Sakura          |
| Warning markers and fills             | Yamabuki        |
| Error markers                         | Beni            |

These role assignments apply to all eight variants. Each variant supplies its
own source ink and shade. Info and Success have independent Sora and Wakaba
colours rather than sharing Accent and Secondary.

The generator minimally adjusts Text and accent foregrounds in 0.001 OKLCH
lightness steps until they reach 4.5:1 on canvas, panel, chrome, hover and
selection. Gamut mapping reduces chroma only when needed for sRGB. Stronger
reveal highlights are outside this five-background target. Subtle, Muted and
the original sakura/yamabuki colours retain their source values without contrast
adjustments. Error uses each variant's original Beni colour from the pinned
upstream palette. Warning
and received error values use neutral Text with meaningful coloured markers.
Papaya (`#ff8000`) continues to identify internal faults rather than span errors.

## Editor roles

The top-bar appearance group contains **Theme · Day/Night**. The theme picker
contains background-season choices and ink selection. Ink chooses
one of the 32 named seed inks, defaulting to To-ro (warm lantern). The pinned
Madoromi mixer supplies its analogous family: the seed and neighbouring hues.
To-ro's family is To-ro, Fuyu-gaki and Daikokuten. Compact swatches show three dots
in each named ink's official shades 1, 4 and 7. The selected ink's name appears
once; each choice's original name remains in its accessible label and tooltip.
The selected ink is focused and revealed when the picker opens.
The swatch group has one Tab stop. Arrow keys and Home/End select inks, updating
the preview immediately. The row scrolls horizontally when the window is narrower.
The analogous family applies when the seed is selected; mixed syntax-role
colours do not replace the named ink's preview.
The top-bar control shows three 8px dots in the selected ink's official shades
1, 4 and 7, followed by a simple monitor. Its tooltip shows only the Japanese
ink name, which is also used in its accessible name and inside the picker.

The choice supplies interface Primary, Secondary and Accent, plus the fourteen
syntax accent slots below. Season supplies the softened paper, neutral text,
punctuation and Nijimi highlights. Semantic status colours and the Sakura
attribution heart retain their existing roles. Each family has Day/Night
strengths checked against the five actual seasonal UI surfaces. The theme
generator writes these overrides to `accent-inks.json` with the source family.

`theme-accent-ink` persists the seed independently of `theme`; invalid or absent
values use To-ro. Changes propagate across tabs, and blocked storage retains
the selection in memory. Selecting an accent ink also applies that ink to the
histogram/heatmap count palette and its legend. The chart's count picker can then
select a different ink. Startup restores both choices independently: loading the
saved accent does not overwrite the saved count ink. Service/series assignments
remain independent. Navigation stays left-aligned; search fills the
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

The role names identify slots in the selected analogous mix, rather than fixed
rainbow hues. The generated `syntax` palette also contains each season's readable
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
root size, with regular weight throughout. Home's welcome title and decorative
prompt use 16px Mono with 1.25 line-height and stay on one line. The adjacent
image can shrink to make room. Field names keep their exact spelling.
The complete time-range menu uses Next, including dates, times, calendars,
timezone names and ambiguity choices. Its 32rem preferred width is clamped to
the viewport; endpoint controls wrap into two rows on narrow menus.
Keys, section/column headers and helper text use Subtle; data and main titles use
Text; type/provenance metadata uses Secondary. Muted is for unavailable or
disabled content.

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

Metric Y-axis tick values and horizontal unit headings start at the chart's left
edge. The gutter is measured from rendered Y labels and angled X labels, with
space between labels and the plot. It updates after label, font and size changes;
fixed chart insets are only pre-measurement fallbacks.

Expanded Home attribute-value rows order value, bar, count, %, Copy on one line.
Copy has its own far-right column, with its tooltip opening left.
Count and % columns are fixed-width and right-aligned, with sortable headings.
The value column takes one third of the available row width, capped at 12rem.
Long values use an ellipsis, with the full value available on hover and through Copy.
The 8px-high percentage bar uses an aligned track that fills the remaining width.
Short bars stop at the canonical 8px dot. Colours use
official middle-strength shades 3–5 across the 32 graph inks, chosen by a stable
hash of attribute source, key and typed value identity. Sorting, paging and theme
changes preserve each value's colour. Parent attribute rows remain plain; counts
and percentages keep their record denominator, and Copy retains the full value.

Services and metric series use the first seven official swatches from the same
32 colourful Madoromi inks: 224 distinct colours. The original eighth (palest)
shade remains in the distributed source but is omitted from the automatic pool
and seven-dot override picker. UI and syntax shades are allowed. These are the
upstream eight-step ramp's original colour values, not lightness-resampled count
bands. Each colour retains its exact hex value across seasons and Day/Night.

Slot order visits every ink in Madoromi's pinned colour order at shade 1, then
at shade 2, through shade 7. Ink/shade IDs, such as `rikka:4`, identify overrides.
Assignments use the `madoromi-inks-v1` allocation namespace.

Exact identities use FNV-1a with a 32-bit avalanche, then next-free-slot probing.
Assignments are saved under the palette version, with one viewer-wide service
scope, one separate exact-span-name scope and a separate scope per `metricRef`. Filtering does not release
slots. Saved owners never move; fresh collision winners can depend on encounter
order. Blocked/full storage falls back to in-memory stability.

After all slots are occupied, new identities reuse their hashed slot without
moving previous owners. `colorSlotReuses()` exposes read-only diagnostics and one
console warning is emitted per exhausted scope. The scalar chart's 22-visible-
series cap is independent of the 224-colour capacity.

Each Home service name has its assigned 8px colour dot in a 32px circular target.
The target stays transparent at rest, with shared Low hover and Medium open-state
highlights and a visible keyboard focus ring. The dot opens
the shared ink picker. It is a sibling of the card button, so changing colour does not select the card. Gauge
and Sum series have the same control in their metric Details row; it does not
toggle visibility or disclosure. Seven official shades per ink have 32px click
targets around their 8px dots. Arrow keys move between shades and ink rows;
Home/End move to the ends of a row (Control+Home/End reaches the entire list's
ends). Enter/Space selects. Opening focuses and reveals the current shade;
selection and Escape return focus to the trigger without moving the page.
Both pickers measure the widest full ink name at the current font size and add
room for their dots and controls. Filtering does not change that width. The
viewport caps the width; on narrow screens shade buttons move below the name,
while count-picker names wrap beside their seven aligned dots.

Overrides are saved under `graph-colour-overrides:madoromi-inks-v1:<scope>` as
identity/ink-shade pairs. Services share the `services` scope, keyed by service-name
text rather than Home's typed discovery identity. Series use `metric:<metricRef>`,
keyed by `seriesRef`. Changes propagate to chart lines, sparklines,
service dots, summary bars and multi-service non-error waterfall bars, including across open tabs. Shades
owned by another identity in the same scope are visibly disabled, and keyboard
navigation skips them. Reservations include hidden owners, their saved automatic
reset slots and explicit overrides. The current choice remains available even
when older saved assignments already share that colour. New automatic assignments
avoid overridden shades as well as occupied automatic slots. The reset icon to the right of Find
ink restores the owner's saved automatic slot and is disabled when no override
is set. Resetting never reallocates another identity. Blocked storage keeps
overrides in memory. Actual error bars retain the semantic Error colour.

Waterfall dots and duration bars use exact span-name colours when the full trace
has only one service identity, including an unnamed service. Repeated names share
a colour; these assignments use the separate `span-names` scope and never reserve
or change service colours. With multiple service identities, waterfall rows use
their service colours and overrides. Collapsing, searching or scrolling does not
change the colour basis. Trace summary bars and service indicators always retain
service colours. Error spans and exception events retain the semantic Error override.
This follows [Honeycomb's default waterfall colour basis](https://docs.honeycomb.io/investigate/analyze/explore-traces/#change-color-in-waterfall-representation).

When fewer than 32 of the 224 preset colours remain unreserved in a scope, the
picker opens with one shade slider per ink instead of seven dots. This leaves
about one spare preset per ink before using the finer curve. The layout stays
fixed while the picker is open. A saved curve choice also opens in slider mode,
so it stays editable if reservations later fall below the threshold.

Sliders follow each pinned Madoromi curve between official shades 1 and 7, using
1,001 positions. The seven preset positions keep their exact official colours;
the remaining positions sample equal OKLab lightness steps with upstream colour
conversion. Positions that produce an occupied hex colour are skipped, including
duplicate hex values at adjacent positions. Native arrow, Page Up/Down and
Home/End controls adjust the shade live; the adjacent swatch selects it and
closes the picker. Escape and light dismissal retain the chosen colour. Curve
overrides persist as `ink:curve:position`, in the same owner scope as preset IDs.
Reset restores the saved automatic slot. Histogram/count pickers use whole inks
and do not use reservations or sliders.

Histogram and exponential-histogram heatmaps, distribution bars and count
legends share one app-wide ink choice. The paintbrush in the selected metric
header opens one searchable ink picker with 32 colourful ramps, in Madoromi's
pinned colour order. This preserves the source's full-ramp similarity sequence
after omitting excluded inks; search retains that order. It excludes the four
seasonal neutral sources (Kiri-same, Fukagawa-nezu, Yama-guri and Fuyu-syogun),
plus Take-sumi, Hoteison, Syun-gyo and
Tsukushi. These are the grey, black, muted-green and brown-dominant families.
Each original ink name is followed by Madoromi's English name in Subtle
parentheses, such as `Ajisai (hydrangea)`. Search matches either name.
The seven-dot previews show each full ramp in the current mode. Its footer links
to Madoromi with visible ink attribution. To-ro is the default. The choice is saved
in localStorage under `graph-count-palette`; invalid saved values fall back
to To-ro, and blocked storage retains the choice for the current page. Existing
saved choices retain their ink and shades. Palette changes propagate across tabs.
Opening the picker focuses and reveals
the saved choice inside its scrollable list; searching does not change the choice.

The generator samples each full pinned measured curve at equally spaced OKLab
lightness values, including its endpoints, interpolates a/b at that lightness,
then uses upstream `oklabToHex`. Every adaptive ramp size from 1–12 is generated
separately. Night uses dark-to-light count order; Day uses the exact same shades
reversed. A singleton uses its mode's high-count endpoint. Season changes do
not change the chosen ink. Dropdown previews use seven evenly sampled shades from
the full curve, with the same Day/Night reversal.

These controls are independent of metric context: `themes/count-palettes.ts`
owns the catalogue, `utils/chart-palette.ts` exposes the pure
`countSwatches(steps, theme, palette)` sampler, and `state/count-palette.svelte.ts`
owns the shared saved preference. `components/shared/CountPalettePicker.svelte`
uses `InkPicker` with whole-ink selection; `GraphColorPicker` uses the same
search, attribution and popover with individually selectable shades. Charts pass
`countPalette.value` into their scale helpers; the metric header only mounts
the picker for histogram kinds.

Count thresholds, source zero/empty buckets and categorical identity assignments
retain their existing treatment. Semantic error overrides and chart guides use
the active interface's shared tokens.
