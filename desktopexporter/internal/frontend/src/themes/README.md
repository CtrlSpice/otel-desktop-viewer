# Rosé Pine themes and chart colours

## Ownership

- `rose-pine.json`: approved Dawn, Moon, and Pine interface colours and metadata.
- `series-palette.json`: individually selected series colours, shared across themes.
- `index.ts`: shared catalogue, palette assembly, and saved-theme resolution.
- `tokens.ts` / `plugin.ts`: generate DaisyUI, chart, and code-view CSS during
  the normal Tailwind build. No separate generation step or generated source file.
- `utils/chart-palette.ts`: stable series allocation and theme-aware density ramps.

The three-way theme toggle, charts, and generated CSS use this catalogue. Components use
semantic CSS variables. Papaya remains `#ff8000` for internal errors.
Dawn's approved flipped surfaces assign Surface (`#fffaf3`) to bg-100/canvas,
Base (`#faf4ed`) to bg-200/panel, and Overlay (`#f2e9e1`) to bg-300/chrome,
ordering the surfaces from brightest to darkest.

Pine/Secondary is adjusted theme-wide for regular-weight text contrast:
Pine `#31748f` → `#70b3cf`, Moon `#3e8fb0` → `#6bbadc`, and
Dawn `#286983` → `#266781`. Only OKLCH lightness was adjusted before rounding
to sRGB hex values. Each colour reaches at least 4.5:1 on bg-100, bg-200,
bg-300, Highlight Low and Highlight Medium. Medium is the limiting background:
4.502:1 in Pine, 4.504:1 in Moon and 4.542:1 in Dawn.

Dawn's foreground adjustments below use the same minimum contrast on those
five backgrounds. Text already passes at its original value. These colours
are darkened while preserving OKLCH hue; Gold also needs reduced chroma to stay
within sRGB. These are shared theme colours, including for syntax and metadata.

| Dawn colour | Original | Adjusted | Minimum contrast |
| --- | --- | --- | --- |
| Subtle | `#797593` | `#625d7a` | 4.506:1 |
| Muted | `#9893a5` | `#635e6f` | 4.513:1 |
| Iris/Primary | `#907aa9` | `#6d5784` | 4.532:1 |
| Foam/Accent | `#56949f` | `#296974` | 4.505:1 |
| Gold/Warning | `#ea9d34` | `#895500` | 4.510:1 |
| Love/Error | `#b4637a` | `#95485f` | 4.501:1 |

The user rejected the darkened Dawn Rose. Its palette value is restored to
`#d7827e`; it does not meet 4.5:1 on these surfaces. Dawn literals instead use
the adjusted Iris (`#6d5784`), which meets the contrast target.

## Editor syntax

JSON, search queries and shell examples share
`components/shared/utils/editor-highlighting.ts`. Colours follow the
[official Rosé Pine VS Code themes](https://github.com/rose-pine/vscode/tree/d8f5ebe8e096fa833e997c07eb7685ee1677a4ba/themes),
using the shared Pine, Moon or Dawn palette, including the contrast adjustments
above, without editor-specific colour overrides.
User-approved exceptions: strings use Text instead of Gold, and keys use Pine
instead of Foam, in every editor and compact JSON preview.
Literals use Iris through `--color-code-literal` in all three themes, including
compact JSON previews. Minimum contrast across the five backgrounds above is
4.996:1 in Pine, 4.665:1 in Moon and 4.532:1 in Dawn.

| Syntax role                                   | Rosé Pine colour | VS Code scope                           |
| --------------------------------------------- | ---------------- | --------------------------------------- |
| JSON keys and query field names               | Pine (override)  | `support.type.property-name.json`       |
| Strings                                       | Text (override)  | `string`                                |
| Numbers, booleans, null and bare query values | Iris (override) | `constant.numeric`, `constant.language` |
| Keywords, operators                           | Pine             | `keyword`                               |
| Escapes                                       | Pine             | `constant.character.escape`             |
| Punctuation                                   | Subtle           | `punctuation`                           |
| Comments                                      | Subtle, italic (override) | `comment`                         |
| Invalid syntax                                | Love             | `invalid`                               |
| Other names                                   | Text             | `variable.other`                        |

This maps TextMate scope roles to the tags supplied by our Lezer parsers. Query
field names and bare values use the corresponding JSON roles. Compact JSON
previews use the same colours in `app.css`. Plain field labels and scalar Details
values retain the UI colours. Editors retain Mono at 14px/21px and the app's
Low-hover/Medium-selection rules.

Code surfaces and search inputs use bg-100: the darkest surface in Pine/Moon
and the brightest in Dawn. This includes JSON editors, shell examples, compact
JSON previews, help snippets, query inputs and filter searches. Comments use
Subtle, which passes 4.5:1 on these normal backgrounds (Pine 5.48:1,
Moon 4.86:1, Dawn 6.01:1). Subtle still fails on Medium selection in the dark
themes (Pine 3.25:1, Moon 3.03:1); Moon Subtle also fails on Low at 4.43:1.

JSON strings soft-wrap at word boundaries with a hanging indent after the opening
quote. The document text is unchanged. JSON's notebook-guide trial replaces the
fold-arrow gutter with a sparse grid only in leading indentation: 1.5px dots
in Subtle at 60% opacity in Dawn and 40% in Pine/Moon, 2ch apart horizontally
and one text line apart vertically,
positioned on row boundaries rather than the middle of the text.
The Mono font keeps dots aligned with character columns. These decorative dots
do not carry text or replace the existing folding commands and inspector controls.

The shared `--guide-dot-radius`, `--guide-dot-color` and `--guide-dot-opacity`
tokens also style waterfall time guides. Their positions follow time ticks and
span-row boundaries; the ruler and first span row meet without a gap.
Column and panel resize controls use a shared short grip, hidden at rest and
visible on hover, keyboard focus or drag. Hover uses Text; focus and drag use Iris
at full opacity. The full drag areas and keyboard behaviour remain available.

## Series colours

The 22-series cap is supported by 22 distinct colours, selected from the existing
Rosé Pine chart palette and exact published [Madoromi](https://madoromi.moonwit.ch/)
accents by samhain ([source](https://github.com/the-moonwitch/madoromi), MPL-2.0).
The shared set removes the five-step family ramps, very pale variations, and
peach/salmon near-neighbours. Blue, pink, green, purple, and yellow are interleaved.
Themes can share colours: a theme switch does not change series identity.

Trace identities allow golden/lemon but exclude the error-like pink/mauve family,
with lavender/bluebell additions giving 20 colours. Normal spans retain
distinct service/operation colours; errors use Love. Exact Love/error colours,
primary annotation colours, and papaya are not categorical series colours.

Metric summaries group type, temporality and monotonicity in plain Pine/Secondary
text without badge backgrounds; counts remain Subtle.
Sum and Gauge share the same series sequence, with
stable assignments for their lines/bars, legends, and sparklines. Histogram
series selectors are neutral because the distribution combines their data.

Heatmaps and histogram density use a separate surface-to-Love ramp for the active
theme, with identical count-legend swatches. Percentiles and genuine status
indicators retain success/warning/error colours.
