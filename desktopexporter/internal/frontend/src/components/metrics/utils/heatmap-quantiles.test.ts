import { describe, expect, it } from 'vitest'
import type { HistogramSlicePoint } from './histogram-aggregation'
import { buildHistogramHeatmapData } from './histogram-heatmap-data'
import { heatmapQuantilePaths } from './heatmap-quantiles'

const active = new Set(['0.5', '0.95', '0.99'])
function explicit(
  timestamp: bigint,
  quantiles: Record<string, number | null>
): HistogramSlicePoint {
  return {
    kind: 'histogram',
    timestamp,
    seriesRef: '',
    bounds: [1, 3],
    counts: [2, 7, 3],
    totals: { count: 12, sum: undefined, min: undefined, max: undefined },
    quantiles,
  }
}
function paths(points: HistogramSlicePoint[], keys = active) {
  return heatmapQuantilePaths(
    points,
    buildHistogramHeatmapData(points),
    10,
    300,
    keys
  )
}

describe('heatmapQuantilePaths', () => {
  it('plots the supplied merged estimates within the correct rows and keeps exact columns distinct', () => {
    const points = [explicit(100n, { '0.5': 2 }), explicit(900n, { '0.5': 3 })]
    expect(paths(points)).toEqual([
      { key: '0.5', label: 'p50', path: 'M0,150H10V100H20', dash: '6 4' },
    ])
    expect(points[0]!.quantiles).toEqual({ '0.5': 2 })
  })
  it('does not invent quantiles, average inputs, bridge missing estimates, or smooth bucket transitions', () => {
    const points = [
      explicit(1n, { '0.95': 2 }),
      explicit(2n, { '0.95': null }),
      explicit(3n, { '0.95': 1.5 }),
    ]
    expect(paths(points)[0]!.path).toBe('M0,150H10M20,175H30')
    expect(paths([explicit(1n, {})])).toEqual([])
    expect(paths([explicit(1n, { '0.5': NaN, '0.95': Infinity })])).toEqual([])
    expect(paths(points, new Set())).toEqual([])
  })
  it('uses logarithmic placement in positive and negative exponential buckets and centers exact zero', () => {
    const point: HistogramSlicePoint = {
      kind: 'expHistogram',
      timestamp: 1n,
      seriesRef: '',
      scale: 0,
      zeroThreshold: 0,
      zeroCount: 2,
      negativeOffset: 0,
      negativeCounts: [2],
      positiveOffset: 0,
      positiveCounts: [2],
      totals: { count: 6, sum: undefined, min: undefined, max: undefined },
      quantiles: { '0.5': 0, '0.95': Math.sqrt(2), '0.99': -Math.sqrt(2) },
    }
    const result = paths([point])
    expect(result[0]!.path).toBe('M0,150H10')
    const ys = result
      .slice(1)
      .map(line => Number(line.path.match(/^M0,([^H]+)/)![1]))
    expect(ys[0]).toBeCloseTo(50)
    expect(ys[1]).toBeCloseTo(250)
    expect(result.map(line => line.dash)).toEqual(['6 4', '6 4', '6 4'])
  })
  it('uses each column’s own schema when bucket ranges overlap', () => {
    const fine: HistogramSlicePoint = {
      kind: 'expHistogram',
      timestamp: 1n,
      seriesRef: '',
      scale: 0,
      zeroThreshold: 0,
      zeroCount: 0,
      negativeOffset: 0,
      negativeCounts: [],
      positiveOffset: 0,
      positiveCounts: [2],
      totals: { count: 2, sum: undefined, min: undefined, max: undefined },
      quantiles: { '0.5': Math.sqrt(2) },
    }
    const coarse: HistogramSlicePoint = {
      ...fine,
      timestamp: 2n,
      scale: -1,
      quantiles: { '0.5': 2 },
    }
    const result = paths([fine, coarse])[0]!.path
    const coordinates = result.match(/^M0,([^H]+)H10V([^H]+)H20$/)!
    expect(Number(coordinates[1])).toBeCloseTo(150)
    expect(Number(coordinates[2])).toBeCloseTo(50)
  })
  it('centers open-ended ranges instead of inventing a finite scale', () => {
    expect(
      paths([explicit(1n, { '0.5': 0, '0.95': 5 })]).map(line => line.path)
    ).toEqual(['M0,250H10', 'M0,50H10'])
  })
})
