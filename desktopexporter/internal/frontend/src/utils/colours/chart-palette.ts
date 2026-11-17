// Colour allocation from the shared theme definitions.
//
//   - `metricSeriesPalette()` -- one type-independent sequence for plotted series.
//   - `traceSpanPalette()` -- the same theme-specific identity slots.
//
//   - `countSwatches()` -- perceptually spaced ink spectrum in count order.
//
// Error spans use `var(--color-error)` directly.

import { getSeriesPalette, getTheme } from '@/themes'
import heatmapPalette from '@/themes/heatmap-palette.json' with { type: 'json' }

const countRamps: Record<number, { day: string[]; night: string[] }> =
  heatmapPalette.ramps

/**
 * Ink-derived sequential spectrum at 1–12 perceptually spaced samples.
 * Day reverses Night's exact shades; a singleton uses its mode's high-count end.
 */
export function countSwatches(steps: number, theme: string = ''): string[] {
  const safeSteps = Math.max(1, Math.floor(steps))
  if (!Number.isFinite(safeSteps) || safeSteps > 12) {
    throw new RangeError('steps must produce a count ramp length from 1 to 12')
  }
  const mode = getTheme(theme).scheme === 'light' ? 'day' : 'night'
  return [...countRamps[safeSteps]![mode]]
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

/** Official Madoromi ink identities; actual errors retain the Error role. */
export function traceSpanPalette(count: number, theme: string = ''): string[] {
  return repeatPalette(count, getSeriesPalette(theme))
}

/** The same 224 logical slots for every metric shape. */
export function metricSeriesPalette(
  count: number,
  theme: string = ''
): string[] {
  return repeatPalette(count, getSeriesPalette(theme))
}

/** Neutral swatch / unchecked-checkbox colour. */
export function chartNeutral(): string {
  return 'var(--color-neutral)'
}

/** Aggregate line colours: Checked → primary, All → secondary. */
export const AGG_COLOR_SELECTED = 'var(--color-primary)'
export const AGG_COLOR_ALL = 'var(--color-secondary)'

/** Choose the higher-contrast seasonal glyph using linear-sRGB luminance. */
export function readableTextColor(bg: string, theme: string = ''): string {
  const rgb = parseRgbLike(bg)
  if (!rgb) return 'var(--color-on-dark)'
  const { colors, scheme } = getTheme(theme)
  const dark = scheme === 'light' ? colors.text : colors.canvas
  const light = scheme === 'light' ? colors.canvas : colors.text
  const background = luminance(rgb)
  const contrast = (color: string) => {
    const foreground = luminance(parseRgbLike(color)!)
    return (
      (Math.max(background, foreground) + 0.05) /
      (Math.min(background, foreground) + 0.05)
    )
  }
  return contrast(dark) >= contrast(light)
    ? 'var(--color-on-light)'
    : 'var(--color-on-dark)'
}

function luminance(rgb: [number, number, number]): number {
  const [r, g, b] = rgb.map(channel => {
    const value = channel / 255
    return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
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
