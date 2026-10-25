import { describe, expect, it } from 'vitest'
import { getSeriesPalette, SERIES_COLOR_COUNT, THEMES } from '@/themes'
import { GRAPH_INKS, GRAPH_SWATCHES } from '@/themes/graph-inks'
import graphInks from '@/themes/graph-inks.json'
import { COUNT_PALETTES } from '@/themes/count-palettes'
import {
  countSwatches,
  metricSeriesPalette,
  readableTextColor,
  traceSpanPalette,
} from './chart-palette'

describe('countSwatches', () => {
  it.each(THEMES)(
    'uses the fixed Shin-kai high-count endpoint in $name',
    theme => {
      expect(countSwatches(1, theme.id)).toEqual([
        theme.scheme === 'light' ? '#0d124f' : '#c5d6ff',
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
      countSwatches(5, 'madoromi-fuyu-yoru')
    )
    for (const invalid of [NaN, Infinity, 13, 2 ** 32]) {
      expect(() => countSwatches(invalid)).toThrow(RangeError)
    }
  })
})

describe.each([
  ['metric', metricSeriesPalette, SERIES_COLOR_COUNT],
  ['trace', traceSpanPalette, SERIES_COLOR_COUNT],
] as const)('%s series colours', (_, palette, size) => {
  it.each(THEMES)('uses the distinct official ink slots in $name', theme => {
    const pool = palette(size, theme.id)
    expect(pool).toEqual(getSeriesPalette(theme.id))
    expect(new Set(pool).size).toBe(size)
  })

  it('keeps slot prefixes and exact shades stable across all themes', () => {
    const pool = palette(size)
    for (const theme of THEMES) {
      const themed = palette(size, theme.id)
      expect(themed).toEqual(pool)
      expect(palette(6, theme.id)).toEqual(themed.slice(0, 6))
      expect(palette(size + 3, theme.id).slice(size)).toEqual(
        themed.slice(0, 3)
      )
    }
    expect(palette(size, 'unknown')).toEqual(pool)
    expect(palette(size, 'constructor')).toEqual(pool)
    expect(palette(size, 'madoromi-fuyu-hiru')).toEqual(pool)
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

it('uses seven official shades per ink, excluding only each palest band', () => {
  expect(SERIES_COLOR_COUNT).toBe(224)
  expect(GRAPH_INKS.map(ink => ink.id)).toEqual(
    COUNT_PALETTES.map(ink => ink.id)
  )
  const pool = getSeriesPalette()
  for (const ink of GRAPH_INKS) {
    expect(ink.swatches.map(swatch => swatch.color)).toEqual(
      graphInks.inks[ink.id].slice(0, 7)
    )
    expect(pool).not.toContain(graphInks.inks[ink.id][7])
  }
  expect(traceSpanPalette(224)).toEqual(metricSeriesPalette(224))
  expect(new Set(GRAPH_SWATCHES.map(swatch => swatch.id)).size).toBe(224)
})

it('allows official shades shared with UI and code roles', () => {
  expect(getSeriesPalette()).toEqual(
    expect.arrayContaining(['#ddb3ff', '#8ac3ff', '#384cac'])
  )
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
