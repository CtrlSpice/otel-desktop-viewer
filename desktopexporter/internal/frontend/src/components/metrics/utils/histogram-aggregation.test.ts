import { describe, expect, it } from 'vitest'
import type {
  ExponentialHistogramDataPoint,
  HistogramDataPoint,
} from '@/types/api-types'
import {
  buildHistogramTimeMergedSeries,
  heatmapColumnsForWidth,
  HEATMAP_MAX_COLUMNS,
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

describe('cumulative exponential histograms whose scale drifts mid-window', () => {
  function cumDp(
    timestamp: bigint,
    scale: number,
    offset: number,
    counts: number[],
    count: number
  ): ExponentialHistogramDataPoint {
    return {
      id: `dp-${timestamp}`,
      metricType: 'ExponentialHistogram',
      timestamp,
      startTime: 0n,
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
      positiveBucketOffset: offset,
      positiveBucketCounts: counts,
      negativeBucketOffset: 0,
      negativeBucketCounts: [],
      aggregationTemporality: 'Cumulative',
    } as unknown as ExponentialHistogramDataPoint
  }

  function activityIn(dps: ExponentialHistogramDataPoint[]) {
    const series = [
      { attributesKey: 'a', attributes: [], datapoints: dps },
    ] as unknown as Parameters<typeof buildHistogramTimeMergedSeries>[0]
    const out = buildHistogramTimeMergedSeries(
      series,
      1_000_000_000n,
      2_000_000_000n,
      1,
      'Cumulative'
    )
    expect(isHistogramAggregationError(out)).toBe(false)
    const slices = out as HistogramSlicePoint[]
    expect(slices).toHaveLength(1)
    return slices[0]!
  }

  it('subtracts across a scale change instead of reporting the running total', () => {
    // Cumulative counter: 10 observations by the first datapoint, 30 by the
    // second, and the SDK downscaled from 2 to 1 in between. The activity in
    // this bucket is 20, not 30.
    const slice = activityIn([
      cumDp(1_000_000_000n, 2, 4, [5, 5, 0, 0], 10),
      cumDp(1_000_000_001n, 1, 2, [15, 15], 30),
    ])
    expect(slice.kind).toBe('expHistogram')
    if (slice.kind !== 'expHistogram') return

    expect(slice.totals.count).toBe(20)
    // First downscales to [10, 0] at offset 2; last is [15, 15] at offset 2.
    expect(slice.scale).toBe(1)
    expect(slice.positiveOffset).toBe(2)
    expect(slice.positiveCounts).toEqual([5, 15])
    expect(slice.positiveCounts.reduce((x, y) => x + y, 0)).toBe(20)
  })

  it('still treats a genuine counter reset as a reset', () => {
    // The later slice is smaller: the counter restarted, so the activity is
    // the later value itself, not a negative difference.
    const slice = activityIn([
      cumDp(1_000_000_000n, 1, 2, [50, 50], 100),
      cumDp(1_000_000_001n, 1, 2, [3, 4], 7),
    ])
    if (slice.kind !== 'expHistogram') return
    expect(slice.totals.count).toBe(7)
    expect(slice.positiveCounts).toEqual([3, 4])
  })
})

describe('explicit-bounds histograms whose bounds change mid-series', () => {
  function boundsDp(
    timestamp: bigint,
    bounds: number[],
    counts: number[]
  ): HistogramDataPoint {
    const count = counts.reduce((a, b) => a + b, 0)
    return {
      id: `dp-${timestamp}`,
      metricType: 'Histogram',
      timestamp,
      startTime: timestamp,
      flags: 0,
      exemplars: [],
      attributes: [],
      count,
      sum: count,
      min: 0,
      max: 10,
      explicitBounds: bounds,
      bucketCounts: counts,
      aggregationTemporality: 'Delta',
    } as unknown as HistogramDataPoint
  }

  it('reports the mismatch instead of summing incompatible layouts', () => {
    // Two datapoints of one series, one bucketed at [1,2,5] and one at
    // [10,20,50]. Bucket i of the first covers nothing like bucket i of the
    // second, so adding them is meaningless -- and the old code did exactly
    // that, then labelled the result with the first datapoint's bounds.
    const series = [
      {
        attributesKey: 'a',
        attributes: [],
        datapoints: [
          boundsDp(1_000_000_000n, [1, 2, 5], [1, 1, 1, 1]),
          boundsDp(1_000_000_001n, [10, 20, 50], [2, 2, 2, 2]),
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
    expect(isHistogramAggregationError(out)).toBe(true)
    if (!isHistogramAggregationError(out)) return
    expect(out.kind).toBe('boundsMismatch')
  })

  it('still merges when the bounds actually agree', () => {
    const series = [
      {
        attributesKey: 'a',
        attributes: [],
        datapoints: [
          boundsDp(1_000_000_000n, [1, 2, 5], [1, 1, 1, 1]),
          boundsDp(1_000_000_001n, [1, 2, 5], [2, 2, 2, 2]),
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
    const slice = (out as HistogramSlicePoint[])[0]!
    if (slice.kind !== 'histogram') return
    expect(slice.counts).toEqual([3, 3, 3, 3])
  })
})

describe('heatmapColumnsForWidth', () => {
  it('derives columns from the measured width', () => {
    expect(heatmapColumnsForWidth(1200)).toBe(200)
    expect(heatmapColumnsForWidth(600)).toBe(100)
  })

  it('quantises, so a pixel of resize does not change the answer', () => {
    expect(heatmapColumnsForWidth(1200)).toBe(heatmapColumnsForWidth(1207))
    expect(heatmapColumnsForWidth(1200)).toBe(heatmapColumnsForWidth(1193))
  })

  it('clamps at both ends', () => {
    expect(heatmapColumnsForWidth(60)).toBe(50)
    expect(heatmapColumnsForWidth(100000)).toBe(HEATMAP_MAX_COLUMNS)
  })

  // The ceiling is also what a histogram asks the store for, so it is a query
  // bound and not only a drawing one. Pinned literally: a caller reading it as
  // a fetch target should see the number change here when it changes.
  it('caps at 250 columns', () => {
    expect(HEATMAP_MAX_COLUMNS).toBe(250)
    expect(HEATMAP_MAX_COLUMNS % 25).toBe(0)
  })

  it('falls back to the floor before layout has measured anything', () => {
    expect(heatmapColumnsForWidth(0)).toBe(50)
    expect(heatmapColumnsForWidth(Number.NaN)).toBe(50)
    expect(heatmapColumnsForWidth(-10)).toBe(50)
  })
})
