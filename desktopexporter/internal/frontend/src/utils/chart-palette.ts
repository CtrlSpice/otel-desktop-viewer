// Single source of truth for chart colors. Two flavours live here:
//
//   - `categoricalPalette()` -- series-identity colours for line charts,
//     histogram heatmap legends, and trace waterfall bars. Uses the approved
//     five-family static table, rotated so the caller's requested family leads.
//
//   - `heatmapSwatches()` -- per-theme count ramp, interpolated in HCL.
//
// Error spans use `var(--color-error)` directly. The heatmap also uses that
// theme token for its love endpoint; the categorical palette is separate.

import { interpolateHcl } from 'd3-interpolate'
import chartPalettes from './chart-palettes.json'
import chartAccents from '@/themes/chart-accents.json'

const EXTRA_ACCENTS = new Map(Object.entries(chartAccents))

function extraAccents(theme: string) {
  return EXTRA_ACCENTS.get(theme) ?? chartAccents['rose-pine-moon']
}

const DEFAULT_HEATMAP_ENDPOINTS = ['#2a273f', '#eb6f92'] as const
const HEATMAP_ENDPOINTS = new Map<string, readonly [string, string]>([
  ['rose-pine', ['#1f1d2e', '#eb6f92']],
  ['rose-pine-moon', DEFAULT_HEATMAP_ENDPOINTS],
  ['rose-pine-dawn', ['#faf4ed', '#b4637a']],
])

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
  const [start, end] = HEATMAP_ENDPOINTS.get(theme) ?? DEFAULT_HEATMAP_ENDPOINTS
  if (safeSteps === 1) return [end]
  const interpolator = interpolateHcl(start, end)
  const out: string[] = []
  for (let i = 0; i < safeSteps; i++) {
    out.push(interpolator((i + 1) / safeSteps))
  }
  return out
}

// ── Categorical series palette ──
//
// Fixed static palette interleaving five colour families at five levels. The
// family order rotates to put `start` first; the level order deliberately
// spreads the first few series across family and lightness.
//
// Callers pass the *cap* (MAX_VISIBLE_TIMESERIES), never the current
// series count. That is the load-bearing part: a count-aware palette
// would shift every series's colour whenever the user toggled visibility
// on a metric with a different number of timeseries -- a series's colour
// shouldn't depend on how many neighbours it has. Passing a constant
// means "the 4th visible timeseries" always gets the same hue, no matter
// the metric.
//
// The scalar cap is independent of the finite table size. It keeps dense line
// charts readable without changing their selection policy.
//
export type CategoricalStem = 'pine' | 'foam' | 'gold' | 'rose' | 'iris'

// Families rotate in this order. Each level is emitted across all families
// before moving to the next level.
const FAMILY_ORDER: readonly CategoricalStem[] = [
  'pine',
  'foam',
  'gold',
  'rose',
  'iris',
] as const

const DEFAULT_CATEGORICAL_PALETTE = chartPalettes['rose-pine-moon']
const PALETTES_BY_THEME = new Map(Object.entries(chartPalettes))
const LEVEL_ORDER = [2, 0, 4, 1, 3] as const

export const CHART_PALETTES = chartPalettes

/**
 * Categorical chart palette of length `count`. The 25 approved colours are
 * emitted as family/level interleaves and repeat deterministically past that
 * finite pool. Unknown / empty `theme` falls back to moon. `count <= 0`
 * returns `[]`.
 */
export function categoricalPalette(
  count: number,
  start: CategoricalStem,
  theme: string = ''
): string[] {
  const safeCount = Math.max(0, Math.floor(count))
  if (!Number.isFinite(safeCount) || safeCount >= 2 ** 32) {
    throw new RangeError('count must produce a valid array length')
  }
  if (safeCount === 0) return []

  const palette = PALETTES_BY_THEME.get(theme) ?? DEFAULT_CATEGORICAL_PALETTE
  const startIdx = FAMILY_ORDER.indexOf(start)
  const rotated =
    startIdx >= 0
      ? [...FAMILY_ORDER.slice(startIdx), ...FAMILY_ORDER.slice(0, startIdx)]
      : [...FAMILY_ORDER]
  const approvedPool = LEVEL_ORDER.flatMap(level =>
    rotated.map(family => palette[family][level]!)
  )

  const out: string[] = []
  for (let i = 0; i < safeCount; i++) {
    out.push(approvedPool[i % approvedPool.length]!)
  }
  return out
}

/** Keep orange tones out of trace identities so papaya remains an internal-error cue. */
export function traceSpanPalette(count: number, theme: string = ''): string[] {
  const requested = categoricalPalette(count, 'iris', theme)
  if (requested.length === 0) return []
  const palette = PALETTES_BY_THEME.get(theme) ?? DEFAULT_CATEGORICAL_PALETTE
  const gold = new Set<string>(palette.gold)
  const pool = categoricalPalette(25, 'iris', theme).filter(
    color => !gold.has(color)
  )
  // Introduce new hues before repeating the original families at other levels.
  const { green, blue, pink, teal } = extraAccents(theme)
  pool.splice(4, 0, green, blue, pink, teal)
  return requested.map((_, index) => pool[index % pool.length]!)
}

/** Reserve iris for metric annotations, retaining 25 stable slots for the 22-series cap. */
export function metricSeriesPalette(
  count: number,
  start: CategoricalStem,
  theme: string = ''
): string[] {
  const requested = categoricalPalette(count, start, theme)
  if (requested.length === 0) return []
  const palette = PALETTES_BY_THEME.get(theme) ?? DEFAULT_CATEGORICAL_PALETTE
  const families = FAMILY_ORDER.filter(family => family !== 'iris')
  const startIdx = Math.max(
    0,
    families.indexOf(start === 'iris' ? 'pine' : start)
  )
  const rotated = [...families.slice(startIdx), ...families.slice(0, startIdx)]
  const pool = LEVEL_ORDER.flatMap(level =>
    rotated.map(family => palette[family][level]!)
  )
  // Measured Madoromi accents add hue variety instead of more tonal steps.
  pool.splice(4, 0, ...Object.values(extraAccents(theme)))
  return requested.map((_, index) => pool[index % pool.length]!)
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
 *  dark swatches -- both defined per theme in app.css. Using vars (not
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
