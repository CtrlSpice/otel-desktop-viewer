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

Subtle is adjusted app-wide in Pine (`#908caa` → `#aba7c5`) and Moon
(`#928eac` → `#b1adcc`). Only OKLCH lightness was increased before rounding
to sRGB hex values, retaining the hue and chroma. All five backgrounds pass:

| Subtle contrast | Pine | Moon |
| --- | --- | --- |
| Canvas | 7.622:1 | 7.241:1 |
| Panel | 7.119:1 | 6.655:1 |
| Chrome | 6.550:1 | 5.384:1 |
| Highlight Low | 6.916:1 | 6.605:1 |
| Highlight Medium | 4.516:1 | 4.521:1 |

Highlight High, used for matched waterfall rows, is outside that five-background
target. The Subtle/High pairs still fall below 4.5:1 (Pine 3.391:1, Moon 3.437:1).

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

The user rejected the darkened Dawn Rose. Its palette value is restored to
`#d7827e`; it does not meet 4.5:1 on these surfaces. Dawn literals instead use
the adjusted Iris (`#6d5784`), which meets the contrast target.

Love/Error also uses its original colour across the app: Dawn `#b4637a`,
Pine/Moon `#eb6f92`. The darkened Dawn error colour was rejected; there is no
separate error-text colour. Error navigation uses neutral Text for its carets
and label, with an 8px Love dot before `err` or `err (x/y)`. Other Love-coloured
text retains the original palette's contrast limitations.
The parenthesised `(x/y)` error-navigation position uses Subtle; `err` and the
carets remain Text.
Across canvas, panel, chrome, Low and Medium, neutral Text has a minimum of
5.252:1. Love text fails 4.5:1 on Dawn's five backgrounds (minimum 3.030:1),
Moon chrome (4.001:1) and Medium (3.359:1), and Pine Medium (3.597:1).
The navigation dot repeats the written error label; it is not the sole signal.
Trace counts read `● x err · y spans`, with both counts and the separator in
Subtle and an 8px Love dot. Error status code/message values use neutral Text
with 8px Love dots. Error span names use neutral Text without an additional dot;
the waterfall retains its coloured tree markers and bars. Span and summary
breakdown bars use solid palette fills without borders in Dawn as well as the
dark themes.
Waterfall tree nodes use filled 8px dots matching the span bar colour for both
parents and leaves, without counts inside the dots. Parent
buttons cover the entire left gutter, including ancestor columns, dot and count,
at full row height. Each toggles its own row without selecting the span; its
accessible label includes the action, span name and direct-child count. Hover
uses Highlight Low and keyboard focus uses the shared outline. Leaf gutters are
decorative and have no collapse button. Dots stay visible on hover; there is no caret.
Parent rows read `●4   Parent Title`, with the direct-child count in plain Subtle
14px regular text between the dot and title. Counts remain visible when collapsed;
leaves have no count. Counts have no parentheses or badge background. Each count
is centred in a 22px slot above its children's dots, with 8px from slot to title.
The name column heading reserves the same root count slot to align with the title.
Tree columns retain 22px spacing without separate count gutters.
Tree connector trunks, arms and stems are solid 1px lines in Muted at 50% opacity.
Their strokes align with node centres; elbows share the cards' `--radius-xl`
corner radius and use a continuous rounded border.
Metric line, histogram and heatmap charts use solid 1px Muted axes/tick marks.
The left and bottom axes share a continuous rounded bottom-left join using
`--radius-xl`, matching cards and waterfall connectors. The border is centred
on the original plot bounds; plot insets and scales stay unchanged. It is
decorative and does not intercept pointer or keyboard input.
Line charts, histograms and heatmaps share a full grid of 1.5px Muted dots at
full opacity, behind data. Each labelled X/Y interval has two equal subdivisions,
with a dot at every intersection. Cells need not be square. The nearest interval's
spacing continues within the plot edges without adding dots beyond the plot.
Dot rows/columns on all four plot boundaries fill the edge gaps without moving
existing label-aligned dots or their midpoints. The edge gaps can be shorter.
Only the outermost top row and rightmost column use 50% opacity, including
their shared corner; all other dots stay at full opacity. Decoration stays inside
the plot bounds without changing plot
size, data bounds or scrolling. Tick positions include filtered heatmap
ticks; the grid adds no labels. Dot size, colour and opacity use the shared
`--guide-dot-*` tokens.
Heatmap label density responds to plot size with the line charts' 80px horizontal
and 50px vertical spacing targets. Labels are selected from existing time columns
and bucket rows; bucket identities, bounds, counts and geometry stay unchanged.
Histogram and heatmap count legends replace the displayed time range on the
left of the header's lower row. Entries wrap by halves at the regular working
text size: 12 entries become 6+6, then 3+3+3+3 as available width decreases.
Odd counts are balanced across rows; labels stay whole and in count order.
chart controls remain on the right and the global time selector remains available.
Count-range markers are plain 8px round dots without borders or outlines;
numeric ranges retain the meaning independently of colour.
Timeline activity markers use a 14px circle icon for both single and grouped
records. Groups show an 8px plus icon; exact counts remain in their accessible
labels and tooltips.
Table headings show external provenance as `name · source`: the name and middle
dot use Subtle, and the source uses Pine/Secondary. Name and source can shrink
and truncate normally; the full source name remains available on hover.
Log tables and summaries share `LogSeverity`: an 8px severity-coloured dot
followed by neutral `LABEL (number)` text. Received labels are retained; only
missing labels are derived from the severity number.
Exception event cards use an 8px Love dot with a neutral Text title. Timeline
popovers put the dot beside the neutral event name, with `Span event` below in
Subtle. Log entries put shared severity first, with `Log` and any received event
name below. Offsets use Subtle and align right on the first line.

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
Subtle, which passes 4.5:1 on these normal backgrounds (Pine 7.62:1,
Moon 7.24:1, Dawn 6.01:1) and on Low hover and Medium selection in every theme.

JSON strings soft-wrap at word boundaries with a hanging indent after the opening
quote. The document text is unchanged. JSON's notebook-guide trial replaces the
fold-arrow gutter with a sparse grid only in leading indentation: 1.5px dots
in Muted at full opacity in all themes, 2ch apart horizontally
and one text line apart vertically,
positioned on row boundaries rather than the middle of the text.
The Mono font keeps dots aligned with character columns. These decorative dots
do not carry text or replace the existing folding commands and inspector controls.

The shared `--guide-dot-radius` and `--guide-dot-opacity` tokens apply to
waterfall time guides and all raw/nested JSON grids. Every grid uses Muted at
full opacity through the shared `--guide-dot-color` token.
Three unlabelled guides divide each labelled interval into quarters.
Use fewer divisions when needed to retain whole nanoseconds.
Labels keep their existing spacing; ruler, span rows and faded tail use the
same guide positions. These positions are display-only, calculated from exact
nanosecond offsets as `floor(offsetNs * 10000 / durationNs) / 100` percent.
It is painted behind span bars, labels and activity markers.
Their positions follow time ticks and
span-row boundaries, with two additional dot rows at one-third and two-thirds
of each span row's height. The ruler and first span row meet without a gap.
If the spans leave empty space below the table, the guides continue for up to
one row and fade to transparent. The continuation adds no scrollable height.
The grid also continues through the existing right gutter at the same horizontal
spacing. Its final five guide columns fade to transparent; earlier columns
retain Muted at full opacity. Ruler, rows and below-table continuation share positions and
the right fade. This decoration adds no labels, changes no time bounds and adds
no horizontal scroll width. Added guide positions use the same whole-nanosecond
steps and 0.01%-resolution calculation; their opacity decreases linearly across
the last five columns, ending at zero. When fewer than five extra columns fit,
the fade also includes the final existing guide columns; time labels do not fade.
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
