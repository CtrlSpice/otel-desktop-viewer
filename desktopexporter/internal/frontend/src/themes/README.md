# Seasonal themes and graph colours

## Interface themes

The interface uses Madoromi's four seasons, each in Day and Night:

| Season | Day                   | Night                 |
| ------ | --------------------- | --------------------- |
| Spring | `madoromi-haru-hiru`  | `madoromi-haru-yoru`  |
| Summer | `madoromi-natsu-hiru` | `madoromi-natsu-yoru` |
| Autumn | `madoromi-aki-hiru`   | `madoromi-aki-yoru`   |
| Winter | `madoromi-fuyu-hiru`  | `madoromi-fuyu-yoru`  |

The Season dropdown and day/night button share one compact control group, with
the season on the left. Hugeicons identify Spring (flower), Summer (sun), Autumn
(leaf) and Winter (snowflake) in the trigger and the menu. Changing
season retains the current mode; changing mode retains the season. The complete
theme ID is saved in localStorage under `theme`. Previous Rosé Pine IDs migrate
to Winter in their existing light/dark mode. With no saved choice, Winter uses
the operating system's preferred mode. If storage is blocked, controls still work
for the current page.

`data-theme` selects the generated CSS; `data-color-scheme` identifies light/dark
behaviour. Charts observe the same theme identity through `state/theme.svelte.ts`.
Font families, sizes, weights and layout use the existing shared tokens.

## Ownership and generation

- `madoromi.json`: generated interface colours for all eight variants.
- `count-palettes.json`: generated Madoromi histogram count ramps, sizes 1–12.
- `count-palette-names.json`: generated English ink names from the pinned source.
- `count-palettes.ts`: ordered count-palette catalogue and default Rikka ink.
- `index.ts`: catalogue, seasonal selection, legacy migration and graph lookup.
- `tokens.ts` / `plugin.ts`: shared DaisyUI, chart and editor CSS adapter.
- `graph-inks.json`: official ink swatches, 224 categorical slots and legacy migration.
- `graph-inks.ts`: shared seven-shade catalogue and stable ink/shade identities.
- `series-palette.json`: legacy Bloom tables retained as a migration input.
- `utils/color-slots.ts`: saved service/series identity allocation.
- `state/graph-color-overrides.svelte.ts`: persisted, reactive per-identity overrides.

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
variants. The shared surface hierarchy flips with the mode:

| Viewer role                     | Day             | Night           |
| ------------------------------- | --------------- | --------------- |
| Canvas / bg-100 / code / search | Kami, brightest | Chitsu, darkest |
| Panel / bg-200                  | Hyoshi          | Hyoshi          |
| Chrome / bg-300                 | Chitsu, darkest | Kami, brightest |

Hover uses tinted nijimi 0. Persistent selection uses tinted nijimi 2 at 55%
over the panel, blended in sRGB. Stronger reveal highlights use tinted
nijimi 1. Hover takes precedence over selection while the pointer is present.

| Shared role                           | Madoromi source            |
| ------------------------------------- | -------------------------- |
| Text                                  | Sumi                       |
| Subtle: keys, headers and helper text | Usuzumi 1                  |
| Muted: unavailable and disabled       | Kasure 1                   |
| Primary / focus                       | Fuji                       |
| Secondary / metadata                  | Ruri                       |
| Accent                                | Tokiwa                     |
| Info                                  | Sora                       |
| Success                               | Wakaba                     |
| Rose compatibility token              | Sakura                     |
| Warning markers and fills             | Yamabuki                   |
| Error markers                         | Momiji at Beni's lightness |

These role assignments apply to all eight variants. Each variant supplies its
own source ink and shade. Info and Success have independent Sora and Wakaba
colours rather than sharing Accent and Secondary.

The generator minimally adjusts Text and accent foregrounds in 0.001 OKLCH
lightness steps until they reach 4.5:1 on canvas, panel, chrome, hover and
selection. Gamut mapping reduces chroma only when needed for sRGB. Stronger
reveal highlights are outside this five-background target. Subtle, Muted and
the original sakura/yamabuki colours retain their source values without contrast
adjustments. Error uses the
pinned Momiji curve at each variant's original Beni lightness. Warning
and received error values use neutral Text with meaningful coloured markers.
Papaya (`#ff8000`) continues to identify internal faults rather than span errors.

## Editor roles

The top-bar appearance group orders **Ink · Season · Day/Night**. Ink chooses
one of the 32 named seed inks, defaulting to Ajisai (hydrangea). The pinned
Madoromi mixer supplies its analogous family: the seed and neighbouring hues.
Ajisai's family is Ajisai, Ama-iro and Murasaki-shikibu. Original names and English
names stay visible and searchable; seven preview dots show that named ink's
official shades 1–7, matching the graph colour picker across seasons and modes.
The analogous family applies when the seed is selected; mixed syntax-role
colours do not replace the named ink's preview.
The top-bar control shows a brush, the selected ink's original name, then three
8px dots in its official shades 1, 4 and 7. Its tooltip puts the parenthesised
English name on a separate second line.

The choice supplies interface Primary, Secondary and Accent, plus the fourteen
syntax accent slots below. Season supplies the softened paper, neutral text,
punctuation and Nijimi highlights. Semantic status colours and the Sakura
attribution heart retain their existing roles. Each family has Day/Night
strengths checked against the five actual seasonal UI surfaces. The theme
generator writes these overrides to `accent-inks.json` with the source family.

`theme-accent-ink` persists the seed independently of `theme`; invalid or absent
values use Ajisai. Changes propagate across tabs, and blocked storage retains
the selection in memory. Service/series assignments and histogram/count inks
remain independently selected. Navigation stays left-aligned; search fills the
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
| ------------------------------------- | --------------- |
| Names and variables                   | Sumi            |
| String bodies                         | Sumi            |
| JSON/query quote marks                | Sumi            |
| Keywords                              | Fuji            |
| Properties and keys                   | Ruri            |
| Operators                             | Sora            |
| Numbers, booleans, constants and null | Sumi            |
| Escapes, regular expressions and tags | Botan           |
| Characters, enum members and macros   | Tokiwa          |
| Functions and methods                 | Ai              |
| Built-ins and self/this               | Beni            |
| Types, classes and attributes         | Yamabuki        |
| Namespaces                            | Kikyo           |
| Annotations                           | Nadeshiko       |
| Labels and editor cursors             | Sakura          |
| Punctuation                           | Usuzumi 0       |
| Comments                              | Kasure 2        |
| Invalid syntax                        | Momiji / Error  |

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

UI field names use Subtle, data values use Text, and metadata uses Secondary.
Code uses Atkinson Hyperlegible Mono; interface text uses Next. Both retain the
shared 14px working size.

## Graph palettes

Expanded Home attribute-value rows order value, bar, count, %, Copy on one line.
Copy has its own far-right column, with its tooltip opening left.
The percentage bar is 8px high in a 96px slot after the value. Short bars stop at
the canonical 8px dot. Colours use
official middle-strength shades 3–5 across the 32 graph inks, chosen by a stable
hash of attribute source, key and typed value identity. Sorting, paging and theme
changes preserve each value's colour. Parent attribute rows remain plain; counts,
percentages and Copy retain the received values and record denominator.

Services and metric series use the first seven official swatches from the same
32 colourful Madoromi inks: 224 distinct colours. The original eighth (palest)
shade remains in the distributed source but is omitted from the automatic pool
and seven-dot override picker. UI and syntax shades are allowed. These are the
upstream eight-step ramp's original colour values, not lightness-resampled count
bands. Each colour retains its exact hex value across seasons and Day/Night.

Slot order visits every ink in Madoromi's pinned colour order at shade 1, then
at shade 2, through shade 7. Ink/shade IDs, such as `rikka:4`, identify overrides.
The `madoromi-inks-v1` allocation namespace replaces Bloom. On first use of a
scope, existing Bloom owners migrate through a generated one-to-one mapping to
the closest available Madoromi shades, comparing both old Day and Night colours
in OKLab. The old records remain intact. Migration retains ownership and shared
slots, including any existing exhaustion collisions; subsequent loads use the
saved Madoromi assignments. The legacy input and MIT attribution are distributed
alongside the generator; the full MIT licence remains at `/licenses/rose-pine-bloom.txt`.

Exact identities use FNV-1a with a 32-bit avalanche, then next-free-slot probing.
Assignments are saved under the palette version, with one viewer-wide service
scope and a separate scope per native metric reference. Filtering does not release
slots. Saved owners never move; fresh collision winners can depend on encounter
order. Blocked/full storage falls back to in-memory stability.

After all slots are occupied, new identities reuse their hashed slot without
moving previous owners. `colorSlotReuses()` exposes read-only diagnostics and one
console warning is emitted per exhausted scope. The scalar chart's 22-visible-
series cap is independent of the 224-colour capacity.

Each Home service name has a DaisyUI `btn btn-circle btn-sm` customised to a
24px circle, centred in its 32px alignment slot and filled with its assigned colour,
with a readable 16px paintbrush icon. It replaces the small colour dot and opens
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
identity/ink-shade pairs. Services share the `services` scope, while series use
`metric:<native metric reference>`. Changes propagate to chart lines, sparklines,
service dots and non-error waterfall bars, including across open tabs. Shades
owned by another identity in the same scope are visibly disabled, and keyboard
navigation skips them. Reservations include hidden owners, their saved automatic
reset slots and explicit overrides. The current choice remains available even
when older saved assignments already share that colour. New automatic assignments
avoid overridden shades as well as occupied automatic slots. The reset icon to the right of Find
ink restores the owner's saved automatic slot and is disabled when no override
is set. Resetting never reallocates another identity. Blocked storage keeps
overrides in memory. Actual error bars retain the semantic Error colour.

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
to Madoromi with visible ink attribution. Rikka is the default. The choice is saved
in localStorage under `graph-count-palette`; invalid saved values fall back
to Rikka, and blocked storage retains the choice for the current page. Existing
saved choices retain their ink and shades. Opening the picker focuses and reveals
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
