import { describe, expect, it } from 'vitest'
import {
  buildMergedQuantileSeries,
  buildPerSeriesQuantileSeries,
  isHistogramAggregationError,
  quantileLineKey,
  type HistogramSlicePoint,
} from '@/components/metrics/utils/histogram-aggregation'
import { histQuantile } from '@/components/metrics/utils/histogram-quantile'

const ts1 = 1_000_000_000n
const ts2 = 2_000_000_000n

function histSlice(
  timestamp: bigint,
  attributesKey: string,
  counts: number[]
): HistogramSlicePoint {
  return {
    kind: 'histogram',
    timestamp,
    attributesKey,
    bounds: [1, 2, 5, 10],
    counts,
    totals: {
      count: counts.reduce((a, b) => a + b, 0),
      sum: 0,
      min: 0,
      max: 10,
    },
  }
}

describe('buildMergedQuantileSeries', () => {
  it('computes merged quantiles per timestamp from visible series', () => {
    const perAttribute: HistogramSlicePoint[] = [
      histSlice(ts1, 'host=a', [0, 50, 50, 0, 0]),
      histSlice(ts1, 'host=b', [0, 30, 50, 20, 0]),
      histSlice(ts2, 'host=a', [0, 10, 90, 0, 0]),
    ]
    const visible = new Set(['host=a', 'host=b'])
    const result = buildMergedQuantileSeries(
      perAttribute,
      [0.5],
      'selected',
      visible
    )
    expect(isHistogramAggregationError(result)).toBe(false)
    if (isHistogramAggregationError(result)) return

    expect(result).toHaveLength(1)
    expect(result[0]!.key).toBe(quantileLineKey('selected', '0.5'))
    expect(result[0]!.points).toHaveLength(2)

    const mergedCounts = [0, 80, 100, 20, 0]
    const expected = histQuantile([1, 2, 5, 10], mergedCounts, 0.5)
    expect(result[0]!.points[0]!.value).toBeCloseTo(expected!, 9)
  })

  it('all-series scope ignores legend filter', () => {
    const perAttribute: HistogramSlicePoint[] = [
      histSlice(ts1, 'host=a', [0, 100, 0, 0, 0]),
      histSlice(ts1, 'host=b', [0, 0, 100, 0, 0]),
    ]
    const visible = new Set(['host=a'])
    const result = buildMergedQuantileSeries(
      perAttribute,
      [0.5],
      'all',
      visible
    )
    expect(isHistogramAggregationError(result)).toBe(false)
    if (isHistogramAggregationError(result)) return

    const mergedCounts = [0, 100, 100, 0, 0]
    const expected = histQuantile([1, 2, 5, 10], mergedCounts, 0.5)
    expect(result[0]!.points[0]!.value).toBeCloseTo(expected!, 9)
  })

  it('omits null quantile buckets', () => {
    const perAttribute: HistogramSlicePoint[] = [
      histSlice(ts1, 'host=a', [0, 0, 0, 0, 0]),
    ]
    const result = buildMergedQuantileSeries(
      perAttribute,
      [0.95],
      'selected',
      null
    )
    expect(isHistogramAggregationError(result)).toBe(false)
    if (isHistogramAggregationError(result)) return
    expect(result[0]!.points).toHaveLength(0)
  })
})

describe('buildPerSeriesQuantileSeries', () => {
  it('emits one line per visible attributesKey', () => {
    const perAttribute: HistogramSlicePoint[] = [
      histSlice(ts1, 'host=a', [0, 50, 50, 0, 0]),
      histSlice(ts1, 'host=b', [0, 30, 50, 20, 0]),
      histSlice(ts2, 'host=a', [0, 10, 90, 0, 0]),
    ]
    const visible = new Set(['host=a', 'host=b'])
    const lines = buildPerSeriesQuantileSeries(perAttribute, 0.5, visible)
    expect(lines.map(l => l.key).sort()).toEqual(['host=a', 'host=b'])
    expect(lines[0]!.points).toHaveLength(2)
  })
})
