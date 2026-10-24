import { describe, expect, it } from 'vitest'
import {
  getSeriesPalette,
  SERIES_COLOR_COUNT,
  THEMES,
  GRAPH_THEMES,
} from '@/themes'
import frozenPalette from '@/themes/series-palette.json'
import { COUNT_PALETTES } from '@/themes/count-palettes'
import {
  countSwatches,
  metricSeriesPalette,
  readableTextColor,
  traceSpanPalette,
} from './chart-palette'

describe('countSwatches', () => {
  it.each(THEMES)(
    'uses the approved Rikka high-count endpoint in $name',
    theme => {
      expect(countSwatches(1, theme.id)).toEqual([
        theme.scheme === 'light' ? '#512442' : '#a9e5ff',
      ])
      expect(countSwatches(12, theme.id)).toHaveLength(12)
    }
  )

  it.each(COUNT_PALETTES)(
    'keeps $name season-independent and reverses every full-curve adaptive ramp by day',
    ({ id }) => {
      const full = countSwatches(12, 'madoromi-fuyu-yoru', id)
      for (let size = 2; size <= 12; size++) {
        const night = countSwatches(size, 'madoromi-fuyu-yoru', id)
        expect(night[0]).toBe(full[0])
        expect(night.at(-1)).toBe(full.at(-1))
        for (const theme of THEMES) {
          expect(countSwatches(size, theme.id, id)).toEqual(
            theme.scheme === 'light' ? [...night].reverse() : night
          )
          expect(countSwatches(1, theme.id, id)).toEqual([
            theme.scheme === 'light' ? full[0] : full.at(-1),
          ])
        }
      }
    }
  )

  it('normalizes step counts and falls back safely', () => {
    expect(countSwatches(2.9)).toHaveLength(2)
    expect(countSwatches(0)).toEqual(countSwatches(1))
    expect(countSwatches(-5)).toEqual(countSwatches(1))
    expect(countSwatches(5, 'constructor')).toEqual(
      countSwatches(5, 'rose-pine-moon')
    )
    for (const invalid of [NaN, Infinity, 13, 2 ** 32]) {
      expect(() => countSwatches(invalid)).toThrow(RangeError)
    }
  })
})

describe.each([
  ['metric', metricSeriesPalette, 32],
  ['trace', traceSpanPalette, 32],
] as const)('%s series colours', (_, palette, size) => {
  it.each(THEMES)(
    'uses the distinct, frozen $name slots without Love',
    theme => {
      const pool = palette(size, theme.id)
      expect(pool).toEqual(getSeriesPalette(theme.id))
      expect(new Set(pool).size).toBe(size)
      for (const reserved of ['#ff8000', theme.colors.error]) {
        expect(pool).not.toContain(reserved)
      }
    }
  )

  it('keeps slot prefixes stable while resolving each theme separately', () => {
    const pool = palette(size)
    for (const theme of THEMES) {
      const themed = palette(size, theme.id)
      expect(palette(6, theme.id)).toEqual(themed.slice(0, 6))
      expect(palette(size + 3, theme.id).slice(size)).toEqual(
        themed.slice(0, 3)
      )
    }
    expect(palette(size, 'unknown')).toEqual(pool)
    expect(palette(size, 'constructor')).toEqual(pool)
    expect(palette(size, 'rose-pine-dawn')).not.toEqual(pool)
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

it('keeps the Gold 100 to Foam 50 replacement after removing Rose', () => {
  expect(SERIES_COLOR_COUNT).toBe(32)
  expect(frozenPalette.slots.every(slot => !slot.startsWith('rose:'))).toBe(
    true
  )
  for (const theme of GRAPH_THEMES) {
    const pool = getSeriesPalette(theme.id)
    const dawn = theme.id === 'rose-pine-dawn'
    expect(pool[13]).toBe(dawn ? '#32565d' : '#235058')
    expect(pool).not.toContain(dawn ? '#8f5a0f' : '#905809')
    expect(traceSpanPalette(32, theme.id)).toEqual(
      metricSeriesPalette(32, theme.id)
    )
  }
})

it('excludes only the selected theme’s canonical bases from its own table', () => {
  const bases = {
    'rose-pine': ['#c4a7e7', '#31748f', '#9ccfd8', '#f6c177', '#ebbcba'],
    'rose-pine-moon': ['#c4a7e7', '#3e8fb0', '#9ccfd8', '#f6c177', '#ea9a97'],
    'rose-pine-dawn': ['#907aa9', '#286983', '#56949f', '#ea9d34', '#d7827e'],
  }
  for (const theme of GRAPH_THEMES) {
    for (const base of bases[theme.id]) {
      expect(getSeriesPalette(theme.id)).not.toContain(base)
    }
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
