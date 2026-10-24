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
- `count-palettes.ts`: ordered count-palette catalogue and default Rikka ink.
- `index.ts`: catalogue, seasonal selection, legacy migration and graph lookup.
- `tokens.ts` / `plugin.ts`: shared DaisyUI, chart and editor CSS adapter.
- `rose-pine.json`: retained Rosé Pine colour definitions for categorical graphs.
- `series-palette.json`: frozen 32-slot Bloom graph tables and provenance.
- `utils/color-slots.ts`: saved service/series identity allocation.

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

The generator minimally adjusts shared readable foregrounds in 0.001 OKLCH
lightness steps until they reach 4.5:1 on canvas, panel, chrome, hover and
selection. Gamut mapping reduces chroma only when needed for sRGB. Stronger
reveal highlights are outside this five-background target. Muted and
the original sakura/yamabuki colours retain their source values. Error uses the
pinned Momiji curve at each variant's original Beni lightness. Warning
and received error values use neutral Text with meaningful coloured markers.
Papaya (`#ff8000`) continues to identify internal faults rather than span errors.

## Editor roles

JSON, search, shell and read-only code views share
`utils/editor-highlighting.ts` under `components/shared`. They follow Madoromi's
[native code guide](https://github.com/the-moonwitch/madoromi/blob/f8bcc5aa1ec3f731fcba9471be7caa5df1ab5695/palettes/style-guide.md#code-符),
using the categories supplied by each language parser:

| Syntax category                       | Madoromi colour |
| ------------------------------------- | --------------- |
| Names and variables                   | Sumi            |
| Strings                               | Wakaba          |
| Keywords                              | Fuji            |
| Properties and keys                   | Ruri            |
| Operators                             | Sora            |
| Numbers, booleans, constants and null | Kaki            |
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

The generated `syntax` palette contains all fourteen named accents and readable
Sumi, Kasure 2 and Usuzumi 0. Its foregrounds meet the same five-background 4.5:1
target as interface text. Code uses `--color-code-*` tokens independently of
interface Primary/Secondary aliases. `--color-code-literal` resolves to Kaki,
including runtime literals in formatted JSON and bare query values. Comments
and self/this are italic; code keeps regular weight. Markdown code uses Wakaba.
The palette also exposes Akane, the guide's italic parameter colour. The current
JSON, query and shell grammars do not emit a distinct parameter tag.

Compact JSON shares these colours. JSON escape highlighting adds decorations
over the existing syntax tree; it does not replace the document. Decoded string
values in Properties use Wakaba, while raw escapes use Botan. Shell syntax uses
the shared StreamLanguage parser; highlighting follows its supplied tags.

Search cursors use Sakura. Editor search matches use Kaki at 18% opacity, with
35% for the current match. Matching brackets use tinted Nijimi 2 at 30% with a
Kasure 0 outline. Selection uses the shared selection token.

UI field names use Subtle, data values use Text, and metadata uses Secondary.
Code uses Atkinson Hyperlegible Mono; interface text uses Next. Both retain the
shared 14px working size.

## Graph palettes

Every seasonal Day variant uses the Dawn Bloom table; every Night variant uses
the Moon Bloom table. The explicit legacy Pine lookup remains available.
Changing season within a mode does not recolour graph data.

Services and metric series share 32 logical Bloom slots. Entire Love and Rose
families, canonical bases and shade 50 are excluded, except Foam 50 replaces Gold
100 in slot 14. Remaining slots retain their frozen relative order. The source
revision, ordering recipe and MIT attribution are recorded in the palette and
`third_party/rose-pine-bloom/`; the complete MIT licence is distributed at
`/licenses/rose-pine-bloom.txt`.

Exact identities use FNV-1a with a 32-bit avalanche, then next-free-slot probing.
Assignments are saved under the palette version, with one viewer-wide service
scope and a separate scope per native metric reference. Filtering does not release
slots. Saved owners never move; fresh collision winners can depend on encounter
order. Blocked/full storage falls back to in-memory stability.

After all slots are occupied, new identities reuse their hashed slot without
moving previous owners. `colorSlotReuses()` exposes read-only diagnostics and one
console warning is emitted per exhausted scope. The scalar chart's 22-visible-
series cap is independent of the 32-colour capacity. No storage namespace or
identity allocation changes are introduced by the seasonal interface.

Histogram and exponential-histogram heatmaps, distribution bars and count
legends share one app-wide ink choice. The paintbrush in the selected metric
header opens the shared filter popover with six ramps: Edo-murasaki, Rikka,
Ku-jaku, Shin-ryoku, Ina-ho and To-ro. Its footer links to Madoromi with visible
ink attribution. Rikka is the default. The choice is saved
in localStorage under `graph-count-palette`; invalid saved values fall back
to Rikka, and blocked storage retains the choice for the current page.

The generator samples each full pinned measured curve at equally spaced OKLab
lightness values, including its endpoints, interpolates a/b at that lightness,
then uses upstream `oklabToHex`. Every adaptive ramp size from 1–12 is generated
separately. Night uses dark-to-light count order; Day uses the exact same shades
reversed. A singleton uses its mode's high-count endpoint. Season changes do
not change the chosen ink. Dropdown ramps use the same 12-shade count order.

These controls are independent of metric context: `themes/count-palettes.ts`
owns the catalogue, `utils/chart-palette.ts` exposes the pure
`countSwatches(steps, theme, palette)` sampler, and `state/count-palette.svelte.ts`
owns the shared saved preference. `components/shared/CountPalettePicker.svelte`
reuses `FilterPopover` and accepts an optional accessible label. Charts pass
`countPalette.value` into their scale helpers; the metric header only mounts
the picker for histogram kinds.

Count thresholds, source zero/empty buckets and categorical identity assignments
retain their existing treatment. Semantic error overrides and chart guides use
the active interface's shared tokens.
