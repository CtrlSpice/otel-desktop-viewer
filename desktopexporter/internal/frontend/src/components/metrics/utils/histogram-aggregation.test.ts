import { describe, expect, it } from 'vitest'
import {
  buildPerSeriesQuantileSeries,
  buildVisibleSeriesQuantileChartTimeseries,
  parseQuantileSeriesKey,
  quantileSeriesKey,
  type HistogramSlicePoint,
} from '@/components/metrics/utils/histogram-aggregation'

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

describe('buildVisibleSeriesQuantileChartTimeseries', () => {
  it('emits one line per visible series and active quantile', () => {
    const perAttribute: HistogramSlicePoint[] = [
      histSlice(ts1, 'host=a', [0, 50, 50, 0, 0]),
      histSlice(ts1, 'host=b', [0, 30, 50, 20, 0]),
      histSlice(ts2, 'host=a', [0, 10, 90, 0, 0]),
    ]
    const visible = new Set(['host=a', 'host=b'])
    const result = buildVisibleSeriesQuantileChartTimeseries(
      perAttribute,
      [0.5, 0.95],
      visible
    )

    expect(result.map(ts => ts.key).sort()).toEqual([
      quantileSeriesKey('host=a', '0.5'),
      quantileSeriesKey('host=a', '0.95'),
      quantileSeriesKey('host=b', '0.5'),
      quantileSeriesKey('host=b', '0.95'),
    ])
    expect(parseQuantileSeriesKey(result[0]!.key)).toEqual({
      seriesKey: 'host=a',
      quantileKey: '0.5',
    })
  })

  it('respects legend visibility filter', () => {
    const perAttribute: HistogramSlicePoint[] = [
      histSlice(ts1, 'host=a', [0, 100, 0, 0, 0]),
      histSlice(ts1, 'host=b', [0, 0, 100, 0, 0]),
    ]
    const visible = new Set(['host=a'])
    const result = buildVisibleSeriesQuantileChartTimeseries(
      perAttribute,
      [0.5],
      visible
    )
    expect(result).toHaveLength(1)
    expect(result[0]!.key).toBe(quantileSeriesKey('host=a', '0.5'))
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
