import { describe, expect, it } from 'vitest'
import { scaleThreshold } from 'd3-scale'
import {
  computeHeatmapColorScale,
  heatmapCountThresholds,
} from '@/components/metrics/utils/heatmap-color-scale'

describe('computeHeatmapColorScale', () => {
  it('rejects invalid threshold array lengths', () => {
    expect(() => heatmapCountThresholds(10, Number.NaN)).toThrow(RangeError)
    expect(() => heatmapCountThresholds(10, Number.POSITIVE_INFINITY)).toThrow(
      RangeError
    )
    expect(() => heatmapCountThresholds(10, 2.5)).toThrow(RangeError)
    expect(() => heatmapCountThresholds(10, 2 ** 32)).toThrow(RangeError)
  })

  it('builds matching thresholds, range, and legend bands', () => {
    const scale = computeHeatmapColorScale({
      maxCount: 100,
      distinctNonZeroCount: 3,
    })

    expect(scale.swatchSteps).toBe(4)
    expect(scale.thresholds).toEqual(heatmapCountThresholds(100, 3))
    expect(scale.range).toHaveLength(5)
    expect(scale.range[0]).toBe('var(--color-base-200)')
    expect(scale.swatches).toHaveLength(4)
    expect(scale.legendEntries).toHaveLength(4)
    expect(scale.legendEntries[0]?.color).toBe(scale.swatches[0])
    expect(scale.legendEntries[0]?.label).toBe('1\u2009–\u200933')
    expect(scale.legendEntries[1]?.label).toBe('34\u2009–\u200966')
    expect(scale.legendEntries[2]?.label).toBe('67\u2009–\u200999')
    expect(scale.legendEntries[3]?.label).toBe('100')
  })

  it('uses a single band when only one distinct non-zero count exists', () => {
    const scale = computeHeatmapColorScale({
      maxCount: 12,
      distinctNonZeroCount: 1,
    })

    expect(scale.swatchSteps).toBe(1)
    expect(scale.thresholds).toEqual([12])
    expect(scale.legendEntries).toEqual([
      expect.objectContaining({ label: '12' }),
    ])
  })

  it('returns no legend when maxCount is zero', () => {
    const scale = computeHeatmapColorScale({
      maxCount: 0,
      distinctNonZeroCount: 0,
    })

    expect(scale.swatchSteps).toBe(0)
    expect(scale.thresholds).toEqual([])
    expect(scale.swatches).toEqual([])
    expect(scale.range).toEqual(['var(--color-base-200)'])
    expect(scale.legendEntries).toEqual([])
  })

  it('groups counts into pairs and gives the maximum its own colour', () => {
    const scale = computeHeatmapColorScale({
      maxCount: 10,
      distinctNonZeroCount: 5,
    })
    expect(scale.legendEntries.map(entry => entry.label)).toEqual([
      '1\u2009–\u20092',
      '3\u2009–\u20094',
      '5\u2009–\u20096',
      '7\u2009–\u20098',
      '9',
      '10',
    ])
    const colour = scaleThreshold<number, string>()
      .domain(scale.thresholds)
      .range(scale.range)
    expect(colour(0)).toBe('var(--color-base-200)')
    for (const [index, counts] of [
      [1, 2],
      [3, 4],
      [5, 6],
      [7, 8],
      [9],
      [10],
    ].entries()) {
      for (const count of counts)
        expect(colour(count)).toBe(scale.legendEntries[index]!.color)
    }
    expect(colour(9)).not.toBe(colour(10))
  })

  it('keeps the maximum separate within the twelve-colour limit', () => {
    const scale = computeHeatmapColorScale({
      maxCount: 1000,
      distinctNonZeroCount: 1000,
    })
    expect(scale.swatches).toHaveLength(12)
    expect(scale.legendEntries.at(-1)?.label).toBe('1000')
    expect(scale.thresholds.at(-1)).toBe(1000)
  })

  it('does not produce inverted legend bands when maxCount is small', () => {
    const scale = computeHeatmapColorScale({
      maxCount: 2,
      distinctNonZeroCount: 2,
    })

    expect(scale.swatchSteps).toBe(2)
    expect(scale.thresholds).toEqual([1, 2])
    expect(scale.legendEntries.map(entry => entry.label)).toEqual(['1', '2'])
  })
})
