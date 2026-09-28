import { describe, expect, it } from 'vitest'
import {
  CHART_PALETTES,
  categoricalPalette,
  heatmapSwatches,
  readableTextColor,
} from './chart-palette'

describe('heatmapSwatches', () => {
  it('returns the hot endpoint colour for a single step', () => {
    expect(heatmapSwatches(1, 'rose-pine')).toEqual(['#eb6f92'])
  })

  it('returns one colour per requested step', () => {
    expect(heatmapSwatches(4, 'rose-pine')).toHaveLength(4)
  })

  it('ends on the theme hot endpoint colour', () => {
    const swatches = heatmapSwatches(3, 'rose-pine-dawn')
    // The last sample position is t=1, so the interpolator lands exactly on
    // the hot endpoint -- just reformatted as `rgb(...)` rather than hex.
    expect(swatches.at(-1)).toBe('rgb(144, 122, 169)')
  })

  it('falls back to the moon ramp for an unknown theme', () => {
    expect(heatmapSwatches(1, 'not-a-theme')).toEqual(
      heatmapSwatches(1, 'rose-pine-moon')
    )
  })

  it('falls back to the moon ramp for an empty theme', () => {
    expect(heatmapSwatches(2)).toEqual(heatmapSwatches(2, 'rose-pine-moon'))
  })

  it('falls back for inherited object property names', () => {
    expect(heatmapSwatches(2, 'toString')).toEqual(
      heatmapSwatches(2, 'rose-pine-moon')
    )
  })

  it('floors fractional steps and clamps non-positive steps to 1', () => {
    expect(heatmapSwatches(2.9, 'rose-pine')).toHaveLength(2)
    expect(heatmapSwatches(0, 'rose-pine')).toEqual(
      heatmapSwatches(1, 'rose-pine')
    )
    expect(heatmapSwatches(-5, 'rose-pine')).toEqual(
      heatmapSwatches(1, 'rose-pine')
    )
  })

  it('rejects non-finite step counts', () => {
    expect(() => heatmapSwatches(Number.NaN)).toThrow(RangeError)
    expect(() => heatmapSwatches(Number.POSITIVE_INFINITY)).toThrow(RangeError)
    expect(() => heatmapSwatches(2 ** 32)).toThrow(RangeError)
  })
})

describe('categoricalPalette', () => {
  it('uses only the approved unique colours for every theme', () => {
    for (const [theme, families] of Object.entries(CHART_PALETTES)) {
      const approved = Object.values(families).flat()
      const pool = categoricalPalette(30, 'pine', theme)
      expect(pool).toHaveLength(30)
      expect(new Set(pool)).toEqual(new Set(approved))
      expect(new Set(pool).size).toBe(30)
    }
  })

  it('interleaves families from the requested start with stable prefixes', () => {
    const pool = categoricalPalette(30, 'gold', 'rose-pine-dawn')
    expect(pool.slice(0, 6)).toEqual([
      '#f0af5d',
      '#d88480',
      '#917cab',
      '#aa546a',
      '#2b6c85',
      '#559695',
    ])
    expect(categoricalPalette(8, 'gold', 'rose-pine-dawn')).toEqual(
      pool.slice(0, 8)
    )
  })

  it('repeats the finite approved pool deterministically after capacity', () => {
    const pool = categoricalPalette(35, 'pine', 'rose-pine')
    expect(pool.slice(30)).toEqual(pool.slice(0, 5))
  })

  it('clamps non-positive counts and floors fractional counts', () => {
    expect(categoricalPalette(0, 'pine')).toEqual([])
    expect(categoricalPalette(-2, 'pine')).toEqual([])
    expect(categoricalPalette(2.9, 'pine')).toHaveLength(2)
  })

  it('falls back to the moon palette for unknown and empty themes', () => {
    const moon = categoricalPalette(5, 'pine', 'rose-pine-moon')
    expect(categoricalPalette(5, 'pine', 'not-a-theme')).toEqual(moon)
    expect(categoricalPalette(5, 'pine')).toEqual(moon)
  })

  it('falls back for inherited object property names', () => {
    expect(categoricalPalette(5, 'pine', 'constructor')).toEqual(
      categoricalPalette(5, 'pine', 'rose-pine-moon')
    )
  })

  it('rejects non-finite color counts', () => {
    expect(() => categoricalPalette(Number.NaN, 'pine')).toThrow(RangeError)
    expect(() => categoricalPalette(Number.POSITIVE_INFINITY, 'pine')).toThrow(
      RangeError
    )
    expect(() => categoricalPalette(2 ** 32, 'pine')).toThrow(RangeError)
  })
})

describe('readableTextColor', () => {
  it('picks the on-light glyph for a bright 6-digit hex colour', () => {
    expect(readableTextColor('#ffffff')).toBe('var(--color-on-light)')
  })

  it('picks the on-dark glyph for a dark 6-digit hex colour', () => {
    expect(readableTextColor('#000000')).toBe('var(--color-on-dark)')
  })

  it('picks the on-dark glyph for a dark 3-digit hex colour', () => {
    expect(readableTextColor('#000')).toBe('var(--color-on-dark)')
  })

  it('picks the on-light glyph for a bright 3-digit hex colour', () => {
    expect(readableTextColor('#fff')).toBe('var(--color-on-light)')
  })

  it('parses an rgb(...) string', () => {
    expect(readableTextColor('rgb(255, 255, 255)')).toBe(
      'var(--color-on-light)'
    )
    expect(readableTextColor('rgb(0, 0, 0)')).toBe('var(--color-on-dark)')
  })

  it('falls back to the on-dark glyph for unparseable input', () => {
    expect(readableTextColor('not-a-colour')).toBe('var(--color-on-dark)')
    expect(readableTextColor('')).toBe('var(--color-on-dark)')
  })
})
