import { describe, expect, it } from 'vitest'
import chartAccents from '@/themes/chart-accents.json'
import {
  CHART_PALETTES,
  categoricalPalette,
  heatmapSwatches,
  metricSeriesPalette,
  readableTextColor,
  traceSpanPalette,
} from './chart-palette'

describe('heatmapSwatches', () => {
  it('returns the hot endpoint colour for a single step', () => {
    expect(heatmapSwatches(1, 'rose-pine')).toEqual(['#eb6f92'])
  })

  it('returns one colour per requested step', () => {
    expect(heatmapSwatches(4)).toHaveLength(4)
  })

  it('ends on the theme hot endpoint colour', () => {
    const swatches = heatmapSwatches(3, 'rose-pine-dawn')
    expect(swatches.at(-1)).toBe('rgb(180, 99, 122)')
  })

  it('floors fractional steps and clamps non-positive steps to 1', () => {
    expect(heatmapSwatches(2.9)).toHaveLength(2)
    expect(heatmapSwatches(0)).toEqual(heatmapSwatches(1))
    expect(heatmapSwatches(-5)).toEqual(heatmapSwatches(1))
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
      const pool = categoricalPalette(25, 'pine', theme)
      expect(pool).toHaveLength(25)
      expect(new Set(pool)).toEqual(new Set(approved))
      expect(new Set(pool).size).toBe(25)
    }
  })

  it('interleaves families from the requested start with stable prefixes', () => {
    const pool = categoricalPalette(25, 'gold', 'rose-pine-dawn')
    expect(pool.slice(0, 5)).toEqual([
      '#f0af5d',
      '#d88480',
      '#917cab',
      '#2b6c85',
      '#559695',
    ])
    expect(categoricalPalette(8, 'gold', 'rose-pine-dawn')).toEqual(
      pool.slice(0, 8)
    )
  })

  it('repeats the finite approved pool deterministically after capacity', () => {
    const pool = categoricalPalette(30, 'pine', 'rose-pine')
    expect(pool.slice(25)).toEqual(pool.slice(0, 5))
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

describe('traceSpanPalette', () => {
  it('excludes orange shades across themes, including after palette reuse', () => {
    for (const [theme, families] of Object.entries(CHART_PALETTES)) {
      const pool = traceSpanPalette(48, theme)
      const accents = new Map(Object.entries(chartAccents)).get(theme)!
      expect(new Set(pool)).toEqual(
        new Set([
          ...families.iris,
          ...families.pine,
          ...families.foam,
          ...families.rose,
          accents.green,
          accents.blue,
          accents.pink,
          accents.teal,
        ])
      )
      for (const orange of families.gold) expect(pool).not.toContain(orange)
      expect(pool).not.toContain('#ff8000')
      expect(pool).not.toContain(accents.yellow)
      expect(pool.slice(24)).toEqual(pool.slice(0, 24))
      expect(traceSpanPalette(3, theme)).toEqual(pool.slice(0, 3))
    }
    expect(traceSpanPalette(20, 'unknown')).toEqual(
      traceSpanPalette(20, 'rose-pine-moon')
    )
  })
})

describe('metricSeriesPalette', () => {
  it('reserves iris for overlays without reducing unique series capacity', () => {
    for (const [theme, families] of Object.entries(CHART_PALETTES)) {
      const pool = metricSeriesPalette(25, 'pine', theme)
      expect(pool).toHaveLength(25)
      expect(new Set(pool).size).toBe(25)
      expect(pool.slice(4, 9)).toEqual(
        Object.values(new Map(Object.entries(chartAccents)).get(theme)!)
      )
      for (const reserved of ['#ff8000', '#eb6f92', '#b4637a']) {
        expect(pool).not.toContain(reserved)
      }
      expect(new Set(pool.slice(0, 22)).size).toBe(22)
      for (const iris of families.iris) expect(pool).not.toContain(iris)
      expect(pool.slice(0, 4)).toEqual([
        families.pine[2],
        families.foam[2],
        families.gold[2],
        families.rose[2],
      ])
      expect(metricSeriesPalette(2, 'pine', theme)).toEqual(pool.slice(0, 2))
      expect(metricSeriesPalette(22, 'pine', theme)).toEqual(pool.slice(0, 22))
    }
  })
  it('keeps deterministic fallbacks, repetition, and count validation', () => {
    const pool = metricSeriesPalette(28, 'foam', 'rose-pine-moon')
    expect(pool[0]).toBe(CHART_PALETTES['rose-pine-moon'].foam[2])
    expect(pool.slice(25)).toEqual(pool.slice(0, 3))
    expect(metricSeriesPalette(24, 'iris', 'unknown')).toEqual(
      metricSeriesPalette(24, 'pine', 'rose-pine-moon')
    )
    expect(metricSeriesPalette(0, 'pine')).toEqual([])
    expect(() => metricSeriesPalette(NaN, 'pine')).toThrow(RangeError)
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
