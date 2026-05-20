// Single source of truth for chart colors.
//
// Categorical (series-identity) accessors are placeholders for now -- everything
// returns primary so we can rebuild the categorical palette step by step without
// scattering colour choices across components. Each chart consumer (metric
// timeseries, waterfall bars, histogram fills, ...) should call into one of the
// `chartColor*` functions below instead of hardcoding `var(--color-...)` so the
// next palette pass only edits this file.
//
// `chartErrorColor()` is intentionally separate from the categorical rotation
// -- rose/error stays reserved for "this datum is an error" semantics
// (e.g. error spans in the waterfall) and must never appear in the regular
// series rotation, even when categorical colours get filled in later.
//
// `heatmapSwatches()` returns a smooth, theme-aware count ramp -- different
// job from categorical series. Endpoints are per-theme literal hex; we
// interpolate in HCL (hue/chroma/lightness) so the middle stops stay along
// a clean hue arc rather than drifting through grey/purple the way sRGB
// interpolation does for dark-to-saturated pairs. Try `interpolateLab` if
// HCL hue arcs ever look wrong; sRGB (`interpolateRgb`) is generally too
// muddy for these endpoints.

import { interpolateHcl } from 'd3-interpolate'

/** Categorical colour for the n-th series (0-indexed). Wraps modulo so callers
 *  past the eventual palette size still get a colour. For now: always primary. */
export function chartColor(_index: number): string {
  return 'var(--color-primary)'
}

/** Foreground that reads against `chartColor(index)` (legend swatches, checkmarks). */
export function chartForeground(_index: number): string {
  return 'var(--color-primary-content)'
}

/** Reserved for error-state datapoints (e.g. error spans). Not part of the
 *  categorical rotation -- never returned by `chartColor()`. */
export function chartErrorColor(): string {
  return 'var(--color-error)'
}

// Per-theme heatmap endpoints. base-200 on the cold end so the lowest swatch
// blends into the chart surface; the "hot" end varies by theme so the ramp
// reads as theme-native rather than imported.
const HEATMAP_ENDPOINTS: Record<string, readonly [string, string]> = {
  'rose-pine': ['#1f1d2e', '#eb6f92'], // base-200 → error (love)
  'rose-pine-moon': ['#2a273f', '#eb6f92'], // base-200 → error (love)
  'rose-pine-dawn': ['#faf4ed', '#b4637a'], // base-200 → error (love)
}
const DEFAULT_HEATMAP_ENDPOINTS = HEATMAP_ENDPOINTS['rose-pine-moon']

/**
 * Visually-stepped colour ramp for a heatmap with `steps` swatches. Interpolates
 * between a theme-specific [cold, hot] hex pair so the lowest active cell is
 * already one perceptual step in from the chart surface and the hottest cell
 * reads as the theme's accent.
 *
 * Sample positions are `1/steps, 2/steps, ..., 1` -- intentionally excluding
 * t=0 (the cold endpoint) so the first swatch is visible against the chart
 * background. Callers that need a true "empty" colour should prepend the
 * `base-200` (or whatever surface) swatch themselves.
 *
 * No upper cap on steps -- callers that need 16 or 32 swatches get them.
 * `steps <= 1` returns just the hot end.
 *
 * Returns CSS-compatible `rgb(r, g, b)` strings (d3's default output format).
 * Unknown / empty `theme` falls back to the moon ramp.
 */
export function heatmapSwatches(steps: number, theme: string = ''): string[] {
  const safeSteps = Math.max(1, Math.floor(steps))
  const [start, end] = HEATMAP_ENDPOINTS[theme] ?? DEFAULT_HEATMAP_ENDPOINTS
  if (safeSteps === 1) return [end]
  const interpolator = interpolateHcl(start, end)
  const out: string[] = new Array(safeSteps)
  for (let i = 0; i < safeSteps; i++) {
    out[i] = interpolator((i + 1) / safeSteps)
  }
  return out
}

// ── Categorical series palette ──
//
// Fixed-size 10-colour palette walking the five stem waypoints in hue
// order (pine → foam → gold → rose → iris, rotated so the caller's
// `start` stem is first). Sampled at 10 evenly-spaced positions across
// the combined HCL arc, so:
//   - Slot 0 is always the rotated start stem; slot 9 the last stem.
//   - count=5 returns all five stems exactly.
//   - Other counts mix literal stems with HCL interpolations between
//     adjacent waypoints (four segments).
//
// Why 10 and not parametric: the chart caps visible series at 10
// (MAX_VISIBLE_TIMESERIES). Generating a count-aware palette meant
// shifting series colours every time the user toggled visibility on a
// metric with a different count of timeseries -- a series's colour
// shouldn't depend on how many neighbours it has. Locking the palette
// at 10 means "the 4th visible timeseries" always gets the same hue,
// no matter the metric.
//
export type CategoricalStem = 'pine' | 'foam' | 'gold' | 'rose' | 'iris'

// Hue-monotonic waypoint order: pine (teal) → foam (cyan) → gold
// (yellow) → rose (pink) → iris (violet), wrapping back to pine.
// Adjacent waypoints share neighbouring hues so HCL interpolation
// stays on a short arc (the iris→pine wrap is the long hop).
const WAYPOINT_ORDER: readonly CategoricalStem[] = [
  'pine',
  'foam',
  'gold',
  'rose',
  'iris',
] as const

// Per-theme stem hexes.
type StemPalette = {
  pine: string
  foam: string
  gold: string
  rose: string
  iris: string
}

const CATEGORICAL_PALETTES: Record<string, StemPalette> = {
  'rose-pine': {
    pine: '#31748f',
    foam: '#9ccfd8',
    gold: '#f6c177',
    rose: '#ebbcba',
    iris: '#c4a7e7',
  },
  'rose-pine-moon': {
    pine: '#3e8fb0',
    foam: '#9ccfd8',
    gold: '#f6c177',
    rose: '#ea9a97',
    iris: '#c4a7e7',
  },
  'rose-pine-dawn': {
    pine: '#286983',
    foam: '#56949f',
    gold: '#ea9d34',
    rose: '#d7827e',
    iris: '#907aa9',
  },
}
const DEFAULT_CATEGORICAL_PALETTE = CATEGORICAL_PALETTES['rose-pine-moon']

/**
 * Categorical chart palette of length `count`, sampled evenly across the
 * four-segment HCL arc connecting the five stem waypoints
 * (pine → foam → gold → rose → iris, rotated so `start` is at slot 0).
 *
 * Sample positions: `t = i / (count - 1) * 4` for i = 0..count-1, with
 * `seg = floor(t)` clamped to [0, 3] picking which segment's HCL
 * interpolator to use, and `segT = t - seg` driving the interpolation.
 *
 * Behaviour at common counts:
 *   - count=1: just the start stem (single-fill callers like histogram bars).
 *   - count=2: start stem and the opposite-end stem (maximum hue distance).
 *   - count=5: all five stems exactly, in rotated order.
 *   - count=10: start stem at slot 0, end stem at slot 9, blends between.
 *   - count=N: stems land approximately at boundaries; interpolations fill in.
 *
 * Returns CSS-compatible strings (`#rrggbb` for stems, `rgb(...)` for
 * interpolated colours -- d3 HCL default). Unknown / empty `theme` falls
 * back to moon. `count <= 0` returns `[]`.
 */
export function categoricalPalette(
  count: number,
  start: CategoricalStem,
  theme: string = ''
): string[] {
  const safeCount = Math.max(0, Math.floor(count))
  if (safeCount === 0) return []

  const palette = CATEGORICAL_PALETTES[theme] ?? DEFAULT_CATEGORICAL_PALETTE

  // Rotate WAYPOINT_ORDER so `start` is at index 0. Walk continues
  // forward through pine→foam→gold→rose→iris, wrapping the *waypoint
  // sequence* (not the colour loop) -- so start=gold gives
  // gold→rose→iris→pine→foam.
  const startIdx = WAYPOINT_ORDER.indexOf(start)
  const rotated = startIdx >= 0
    ? [...WAYPOINT_ORDER.slice(startIdx), ...WAYPOINT_ORDER.slice(0, startIdx)]
    : [...WAYPOINT_ORDER]

  const waypointCount = rotated.length
  const segmentCount = waypointCount - 1
  const hexAt = (i: number) => palette[rotated[i]]

  if (safeCount === 1) return [hexAt(0)]

  // Pre-build segment interpolators -- cheaper than calling
  // interpolateHcl() once per palette slot when callers ask for large N
  // (e.g. traces with hundreds of colours).
  const interps = Array.from({ length: segmentCount }, (_, seg) =>
    interpolateHcl(hexAt(seg), hexAt(seg + 1))
  )

  const out: string[] = new Array(safeCount)
  for (let i = 0; i < safeCount; i++) {
    const t = (i / (safeCount - 1)) * segmentCount
    const seg = Math.min(Math.floor(t), segmentCount - 1)
    const segT = t - seg
    if (segT === 0) {
      out[i] = hexAt(seg)
    } else if (segT === 1 && seg === segmentCount - 1) {
      out[i] = hexAt(waypointCount - 1)
    } else {
      out[i] = interps[seg](segT)
    }
  }
  return out
}

/** Neutral swatch / unchecked-checkbox colour. */
export function chartNeutral(): string {
  return 'var(--color-neutral)'
}

/** Palette slot for a series index. Wraps modulo palette length so row
 *  25 and row 5 share slot 5 when the palette has 10 entries -- the
 *  legend can list more streams than palette slots, but only 10 can be
 *  checked at once and each checked row still gets a stable hue. */
export function categoricalColorAt(
  palette: readonly string[],
  index: number
): string {
  if (palette.length === 0) return chartNeutral()
  return palette[index % palette.length]!
}

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

