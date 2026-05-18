// Categorical colour palette for per-timeseries chart series. One
// global 10-colour sequence used in both Rosé Pine themes (moon and
// dawn) so a timeseries keeps the same colour when the user toggles
// theme mid-debug.
//
// Why 10: that's the cap on simultaneously-checked timeseries in the
// legend. Above 10 lines a chart is unreadable regardless of palette,
// and above 10 distinct colours nothing reads as "obviously different"
// anyway. The legend disables further checkboxes once this many are
// selected.
//
// Why one palette across both themes: timeseries identity is a
// per-timeseries concept, not a per-theme concept. Picking distinct
// hex values per theme would mean colours shift on theme toggle,
// which makes "the purple line is the GET timeseries" a learned
// association that silently breaks when the user switches modes.
//
// Selection rules used to build the sequence:
//   - Excludes `love` (#eb6f92 / #b4637a) because that hex is the
//     error semantic colour elsewhere in the app; a timeseries chip
//     in that colour would read as "this timeseries is in an error
//     state".
//   - Each colour reads against both backgrounds (#232136 dark moon
//     base, #faf4ed cream dawn base). Lightness sits roughly in the
//     35-75 % L band so a 1.5px chart line is visible on either bg.
//   - Adjacent slots alternate warm / cool so consecutive timeseries
//     in a chart never collide on similar hue.
//   - Order is "best-first": when only 3-4 timeseries are checked,
//     the chart uses slots 0-3, which are the most-vivid most-distinct
//     four.
//
// Sources:
//   - main Rosé Pine palette (e.g. #31748f pine, #ebbcba rose)
//   - moon variant (#c4a7e7 iris, #f6c177 gold)
//   - dawn variant (#286983 pine, #907aa9 iris, #ea9d34 gold,
//     #d7827e rose, #56949f foam, #908caa subtle)
const TIMESERIES_PALETTE: readonly string[] = [
  '#31748f', // main pine            saturated teal
  '#ea9d34', // dawn gold            amber
  '#907aa9', // dawn iris            muted purple
  '#f6c177', // moon gold            soft yellow
  '#56949f', // dawn foam            sea green
  '#d7827e', // dawn rose            terracotta
  '#c4a7e7', // moon iris            lavender
  '#286983', // dawn pine            deep teal
  '#9ccfd8', // main/moon foam       light cyan
  '#908caa', // subtle               lavender-grey
] as const

/**
 * Maximum number of timeseries that can be visible (checked) at once
 * in a chart legend. Equal to the palette length so every visible
 * timeseries gets a unique colour. The legend is expected to enforce
 * this by disabling unchecked checkboxes once this many are selected.
 */
export const MAX_VISIBLE_TIMESERIES = TIMESERIES_PALETTE.length

/**
 * How many timeseries to auto-select on first load (before the user
 * has made any explicit choices). Lower than MAX_VISIBLE_TIMESERIES
 * so the initial chart is readable; the user can manually check more
 * up to the palette cap.
 */
export const DEFAULT_VISIBLE_TIMESERIES = 5

/**
 * Colour for the n-th visible timeseries in a chart, by position in
 * the legend (0-indexed). Wraps with a modulo so callers that exceed
 * MAX_VISIBLE_TIMESERIES still get a colour rather than `undefined`,
 * but collision is then on them.
 *
 * Position-based (not key-hashed) so the legend always uses slot 0,
 * 1, 2... in the order timeseries are rendered. No collisions until
 * the cap. Trade-off: a timeseries that disappears and reappears on a
 * subsequent fetch may pick up a different colour if the visible-set
 * order changed; for a debugging tool with the legend always visible
 * next to the chart, that's acceptable in exchange for guaranteed
 * uniqueness within a single view.
 */
export function timeseriesColor(index: number): string {
  return TIMESERIES_PALETTE[index % TIMESERIES_PALETTE.length]
}

/**
 * Foreground colour (checkmark / icon) that reads against
 * `timeseriesColor(index)`. Uses precomputed picks per palette slot
 * rather than runtime luminance maths because the palette is fixed
 * and the answer never changes -- a per-slot literal is cheaper and
 * doesn't expose a third "this should be obvious" calculation to
 * future maintainers.
 *
 * Picks were chosen by eyeballing each palette colour against white
 * and against the dark base text colour, picking whichever has more
 * apparent contrast. The light palette entries (#f6c177, #9ccfd8)
 * use a dark glyph; the rest use white.
 */
const TIMESERIES_PALETTE_FG: readonly string[] = [
  '#ffffff', // pine            on-dark
  '#ffffff', // gold (dawn)     on-mid
  '#ffffff', // iris (dawn)     on-mid
  '#1f1d2e', // gold (moon)     LIGHT slot, dark glyph
  '#ffffff', // foam            on-mid
  '#ffffff', // rose (dawn)     on-mid
  '#1f1d2e', // iris (moon)     LIGHT slot, dark glyph
  '#ffffff', // pine (dawn)     on-dark
  '#1f1d2e', // foam (moon)     LIGHT slot, dark glyph
  '#ffffff', // subtle          on-mid
] as const

export function timeseriesForegroundColor(index: number): string {
  return TIMESERIES_PALETTE_FG[index % TIMESERIES_PALETTE_FG.length]
}
