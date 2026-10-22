// Colour allocation from the shared theme definitions.
//
//   - `metricSeriesPalette()` -- one type-independent sequence for plotted series.
//   - `traceSpanPalette()` -- identities, without error-like pinks or fault-like orange.
//
//   - `heatmapSwatches()` -- per-theme count ramp, interpolated in HCL.
//
// Density ramps end at Iris in Dawn and Love in the dark themes.
// Error spans use `var(--color-error)` directly.

import { interpolateHcl } from 'd3-interpolate'
import { getTheme, METRIC_SERIES, TRACE_SERIES } from '@/themes'

/**
 * Sample the HCL ramp at 1/steps through 1, excluding the cold endpoint.
 * A single step uses the hot endpoint. Unknown themes
 * use the Moon ramp.
 */
export function heatmapSwatches(steps: number, theme: string = ''): string[] {
  const safeSteps = Math.max(1, Math.floor(steps))
  if (!Number.isFinite(safeSteps) || safeSteps >= 2 ** 32) {
    throw new RangeError('steps must produce a valid array length')
  }
  const definition = getTheme(theme)
  const { colors } = definition
  const start = definition.scheme === 'light' ? colors.canvas : colors.panel
  const end = definition.scheme === 'light' ? colors.primary : colors.error
  if (safeSteps === 1) return [end]
  const interpolator = interpolateHcl(start, end)
  const out: string[] = []
  for (let i = 0; i < safeSteps; i++) {
    out.push(interpolator((i + 1) / safeSteps))
  }
  return out
}

/** Stable prefixes: visibility and theme changes never shuffle colour identities. */
function repeatPalette(count: number, pool: readonly string[]): string[] {
  const safeCount = Math.max(0, Math.floor(count))
  if (!Number.isFinite(safeCount) || safeCount >= 2 ** 32) {
    throw new RangeError('count must produce a valid array length')
  }
  if (safeCount === 0) return []

  const out: string[] = []
  for (let i = 0; i < safeCount; i++) {
    out.push(pool[i % pool.length]!)
  }
  return out
}

/** Keep error-like pinks and fault-like orange out of trace identities. */
export function traceSpanPalette(count: number, _theme: string = ''): string[] {
  return repeatPalette(count, TRACE_SERIES)
}

/** One shared 22-colour sequence; no metric-type colour coding. */
export function metricSeriesPalette(
  count: number,
  _theme: string = ''
): string[] {
  return repeatPalette(count, METRIC_SERIES)
}

/** Neutral swatch / unchecked-checkbox colour. */
export function chartNeutral(): string {
  return 'var(--color-neutral)'
}

/** Aggregate line colours: Checked → primary, All → secondary. */
export const AGG_COLOR_SELECTED = 'var(--color-primary)'
export const AGG_COLOR_ALL = 'var(--color-secondary)'

// On-swatch glyph colour (checkbox tick, etc.) for a categorical-palette
// entry. The legend swatches use the same colours as the chart lines, which
// span pine through gold/foam (light) through rose toward iris -- some are
// dark enough for a white glyph, some are light enough to need a dark one.
// Pre-baked fg tables don't survive theme switches or palette length changes,
// so we do the contrast check at read time.

/** Pick the active theme's on-light or on-dark glyph colour for a given
 *  background swatch. Accepts d3's `rgb(r, g, b)` (from interpolateHcl) or
 *  `#rrggbb`. Returns one of two CSS variables -- `--color-on-light` for
 *  dark glyphs on bright swatches, `--color-on-dark` for light glyphs on
 *  dark swatches -- both defined by the shared theme tokens. Using vars (not
 *  literal hex) means a theme switch reroutes glyphs without touching
 *  this function. Uses Rec. 709 luminance: WCAG-style without gamma, fine
 *  for "which of two glyphs reads better here". Unparseable input falls
 *  back to the on-dark glyph so the answer always resolves to a defined
 *  theme variable. */
export function readableTextColor(bg: string): string {
  const rgb = parseRgbLike(bg)
  if (!rgb) return 'var(--color-on-dark)'
  const [r, g, b] = rgb
  const luminance = 0.2126 * r + 0.7152 * g + 0.0722 * b
  // Threshold at ~140/255 picks the same dark/light split as eyeballing
  // each Rosé Pine stem against the theme's surface + text colours.
  return luminance > 140 ? 'var(--color-on-light)' : 'var(--color-on-dark)'
}

function parseRgbLike(s: string): [number, number, number] | null {
  if (!s) return null
  if (s.startsWith('#')) {
    const hex = s.slice(1)
    if (hex.length === 3) {
      const r = parseInt(hex[0] + hex[0], 16)
      const g = parseInt(hex[1] + hex[1], 16)
      const b = parseInt(hex[2] + hex[2], 16)
      return Number.isNaN(r) ? null : [r, g, b]
    }
    if (hex.length === 6) {
      const r = parseInt(hex.slice(0, 2), 16)
      const g = parseInt(hex.slice(2, 4), 16)
      const b = parseInt(hex.slice(4, 6), 16)
      return Number.isNaN(r) ? null : [r, g, b]
    }
    return null
  }
  // d3's default rgb() output: "rgb(r, g, b)" with integers.
  const m = s.match(/rgb\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)/)
  if (!m) return null
  return [parseInt(m[1], 10), parseInt(m[2], 10), parseInt(m[3], 10)]
}
