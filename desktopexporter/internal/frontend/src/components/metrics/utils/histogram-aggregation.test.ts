import { describe, expect, it } from 'vitest'
import type { ExponentialHistogramDataPoint } from '@/types/api-types'
import {
  buildHistogramTimeMergedSeries,
  isHistogramAggregationError,
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

describe('exponential histograms whose scale drifts within a series', () => {
  // An SDK downscales a stream mid-flight as the observed range widens, so two
  // datapoints of the *same* series can carry different scales and offsets.
  // Merging them by summing bucket vectors positionally adds counts covering
  // different value ranges together: wrong quantiles, no error.
  function expDp(
    timestamp: bigint,
    scale: number,
    positiveBucketOffset: number,
    positiveBucketCounts: number[]
  ): ExponentialHistogramDataPoint {
    const count = positiveBucketCounts.reduce((a, b) => a + b, 0)
    return {
      id: `dp-${timestamp}`,
      metricType: 'ExponentialHistogram',
      timestamp,
      startTime: timestamp,
      flags: 0,
      exemplars: [],
      attributes: [],
      count,
      sum: count,
      min: 1,
      max: 100,
      scale,
      zeroCount: 0,
      zeroThreshold: 0,
      positiveBucketOffset,
      positiveBucketCounts,
      negativeBucketOffset: 0,
      negativeBucketCounts: [],
      aggregationTemporality: 'Delta',
    } as unknown as ExponentialHistogramDataPoint
  }

  it('rescales before summing rather than trusting the first datapoint', () => {
    // Same series, one bucket of time. Scale 2 downscales to scale 1 by
    // merging adjacent bucket pairs; the offsets differ too.
    const series = [
      {
        attributesKey: 'a',
        attributes: [],
        datapoints: [
          expDp(1_000_000_000n, 2, 4, [1, 1, 1, 1]),
          expDp(1_000_000_001n, 1, 2, [5, 5]),
        ],
      },
    ] as unknown as Parameters<typeof buildHistogramTimeMergedSeries>[0]

    const out = buildHistogramTimeMergedSeries(
      series,
      1_000_000_000n,
      2_000_000_000n,
      1,
      'Delta'
    )
    expect(isHistogramAggregationError(out)).toBe(false)
    const slices = out as HistogramSlicePoint[]
    // Both datapoints must land in ONE bucket, or the merge never runs and
    // the test proves nothing -- which is exactly what the first version of
    // this test did.
    expect(slices).toHaveLength(1)

    const slice = slices[0]!
    expect(slice.kind).toBe('expHistogram')
    if (slice.kind !== 'expHistogram') return

    // The merge must land on the coarsest scale present, not the first one.
    expect(slice.scale).toBe(1)

    // Total count is conserved however the buckets are aligned: 4 + 10.
    const total = slice.positiveCounts.reduce((a, b) => a + b, 0)
    expect(total).toBe(14)

    // The scale-2 datapoint's four buckets at offset 4 collapse to two buckets
    // at offset 2, which is exactly where the scale-1 datapoint already sits --
    // so a correct merge overlaps them rather than laying them side by side.
    expect(slice.positiveOffset).toBe(2)
    expect(slice.positiveCounts).toEqual([7, 7])
  })
})
