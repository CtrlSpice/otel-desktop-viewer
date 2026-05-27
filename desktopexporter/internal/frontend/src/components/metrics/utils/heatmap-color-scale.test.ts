import { describe, expect, it } from 'vitest'
import {
  computeHeatmapColorScale,
  heatmapCountThresholds,
} from '@/components/metrics/utils/heatmap-color-scale'

describe('computeHeatmapColorScale', () => {
  it('builds matching thresholds, range, and legend bands', () => {
    const scale = computeHeatmapColorScale({
      maxCount: 100,
      distinctNonZeroCount: 3,
      theme: 'rose-pine-moon',
    })

    expect(scale.swatchSteps).toBe(3)
    expect(scale.thresholds).toEqual(heatmapCountThresholds(100, 3))
    expect(scale.range).toHaveLength(4)
    expect(scale.range[0]).toBe('var(--color-base-200)')
    expect(scale.swatches).toHaveLength(3)
    expect(scale.legendEntries).toHaveLength(3)
    expect(scale.legendEntries[0]?.color).toBe(scale.swatches[0])
    expect(scale.legendEntries[0]?.label).toBe('(1\u2009–\u200933)')
    expect(scale.legendEntries[1]?.label).toBe('(34\u2009–\u200966)')
    expect(scale.legendEntries[2]?.label).toBe('(67\u2009–\u2009100]')
  })

  it('uses a single band when only one distinct non-zero count exists', () => {
    const scale = computeHeatmapColorScale({
      maxCount: 12,
      distinctNonZeroCount: 1,
      theme: 'rose-pine-dawn',
    })

    expect(scale.swatchSteps).toBe(1)
    expect(scale.thresholds).toEqual([1])
    expect(scale.legendEntries).toEqual([
      expect.objectContaining({ label: '(1\u2009–\u200912]' }),
    ])
  })

  it('returns no legend when maxCount is zero', () => {
    const scale = computeHeatmapColorScale({
      maxCount: 0,
      distinctNonZeroCount: 0,
      theme: 'rose-pine-moon',
    })

    expect(scale.swatchSteps).toBe(0)
    expect(scale.thresholds).toEqual([])
    expect(scale.swatches).toEqual([])
    expect(scale.range).toEqual(['var(--color-base-200)'])
    expect(scale.legendEntries).toEqual([])
  })

  it('does not produce inverted legend bands when maxCount is small', () => {
    const scale = computeHeatmapColorScale({
      maxCount: 2,
      distinctNonZeroCount: 2,
      theme: 'rose-pine-moon',
    })

    expect(scale.swatchSteps).toBe(2)
    expect(scale.thresholds).toEqual([1, 2])
    expect(scale.legendEntries.map(entry => entry.label)).toEqual([
      '(1\u2009–\u20091)',
      '(2\u2009–\u20092]',
    ])
  })
})
