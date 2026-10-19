import { describe, expect, it } from 'vitest'
import { THEMES } from '@/themes'
import {
  heatmapSwatches,
  metricSeriesPalette,
  readableTextColor,
  traceSpanPalette,
} from './chart-palette'

describe('heatmapSwatches', () => {
  it.each(THEMES)(
    'uses the $name density endpoint for the maximum count',
    theme => {
      expect(heatmapSwatches(1, theme.id)).toEqual([
        theme.scheme === 'light' ? theme.colors.primary : theme.colors.error,
      ])
      expect(heatmapSwatches(12, theme.id)).toHaveLength(12)
    }
  )

  it('ends on Dawn Iris purple', () => {
    expect(heatmapSwatches(3, 'rose-pine-dawn').at(-1)).toBe(
      'rgb(144, 122, 169)'
    )
  })

  it('normalizes step counts and falls back safely', () => {
    expect(heatmapSwatches(2.9)).toHaveLength(2)
    expect(heatmapSwatches(0)).toEqual(heatmapSwatches(1))
    expect(heatmapSwatches(-5)).toEqual(heatmapSwatches(1))
    expect(heatmapSwatches(5, 'constructor')).toEqual(
      heatmapSwatches(5, 'rose-pine-moon')
    )
    for (const invalid of [NaN, Infinity, 2 ** 32]) {
      expect(() => heatmapSwatches(invalid)).toThrow(RangeError)
    }
  })
})

describe.each([
  ['metric', metricSeriesPalette, 22],
  ['trace', traceSpanPalette, 20],
] as const)('%s series colours', (_, palette, size) => {
  it('provides a distinct pool without reserved status/annotation colours', () => {
    const pool = palette(size)
    expect(new Set(pool).size).toBe(size)
    for (const reserved of [
      '#ff8000',
      ...THEMES.flatMap(t => [t.colors.primary, t.colors.error]),
    ]) {
      expect(pool).not.toContain(reserved)
    }
  })

  it('keeps identities stable across theme changes and count changes', () => {
    const pool = palette(size)
    for (const theme of THEMES) expect(palette(size, theme.id)).toEqual(pool)
    expect(palette(6)).toEqual(pool.slice(0, 6))
    expect(palette(size + 3).slice(size)).toEqual(pool.slice(0, 3))
    expect(palette(size, 'unknown')).toEqual(pool)
  })

  it('validates requested counts', () => {
    expect(palette(0)).toEqual([])
    expect(palette(-2)).toEqual([])
    expect(palette(2.9)).toHaveLength(2)
    for (const invalid of [NaN, Infinity, 2 ** 32]) {
      expect(() => palette(invalid)).toThrow(RangeError)
    }
  })
})

it('allows gold/yellow in normal span identities', () => {
  expect(metricSeriesPalette(22)).toContain('#e3940b')
  expect(metricSeriesPalette(22)).toContain('#eec307')
  expect(traceSpanPalette(20)).toContain('#e3940b')
  expect(traceSpanPalette(20)).toContain('#eec307')
})

it('keeps error-like pinks and mauves out of normal span identities', () => {
  for (const color of ['#d18bb7', '#f894d8', '#ce37a6', '#b560a0']) {
    expect(traceSpanPalette(44)).not.toContain(color)
  }
})

describe('readableTextColor', () => {
  it.each(['#ffffff', '#fff', 'rgb(255, 255, 255)'])(
    'uses dark glyphs on %s',
    color => {
      expect(readableTextColor(color)).toBe('var(--color-on-light)')
    }
  )
  it.each(['#000000', '#000', 'rgb(0, 0, 0)', 'not-a-colour', ''])(
    'uses light glyphs on %s',
    color => {
      expect(readableTextColor(color)).toBe('var(--color-on-dark)')
    }
  )
})
