import { describe, expect, it } from 'vitest'
import type {
  ExponentialHistogramDataPoint,
  HistogramDataPoint,
} from '@/types/api-types'
import {
  HEATMAP_BUCKET_TARGET,
  seriesBucketsToSlices,
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
    // Supplied by the store, as they are in a real response. The client no
    // longer derives these, so a slice without them has no quantile to draw.
    quantiles: { '0.5': 2, '0.95': 5, '0.99': 9 },
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
    expect(result[0]!.points[0]!.timestampNs).toBe(ts1)
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

  it('orders same-millisecond points by their exact timestamps', () => {
    const earlier = 10_000_100n
    const later = 10_000_900n
    const result = buildVisibleSeriesQuantileChartTimeseries(
      [
        histSlice(later, 'host=a', [0, 50, 50, 0, 0]),
        histSlice(earlier, 'host=a', [0, 50, 50, 0, 0]),
      ],
      [0.5],
      new Set(['host=a'])
    )

    expect(result[0]!.points.map(point => point.timestampNs)).toEqual([
      earlier,
      later,
    ])
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

describe('HEATMAP_BUCKET_TARGET', () => {
  // Pinned literally because it is a request parameter: the store's ladder
  // turns it into a bucket width, and changing it changes what every histogram
  // response contains. It is deliberately not derived from the plot width --
  // see the constant's own note for why that was both wrong and self-defeating.
  it('is the resolution a histogram asks the store for', () => {
    expect(HEATMAP_BUCKET_TARGET).toBe(100)
  })
})

describe('seriesBucketsToSlices', () => {
  it('emits one slice per store bucket, keyed by its series', () => {
    const series = [
      {
        attributesKey: 'driver=ALO',
        attributes: [],
        datapoints: [
          {
            id: 'a',
            metricType: 'Histogram',
            timestamp: ts1,
            count: 3,
            sum: 6,
            min: 1,
            max: 3,
            explicitBounds: [1, 2],
            bucketCounts: [1, 1, 1],
            exemplars: [],
            flags: 0,
          },
          {
            id: 'b',
            metricType: 'Histogram',
            timestamp: ts2,
            count: 1,
            sum: 5,
            min: 5,
            max: 5,
            explicitBounds: [1, 2],
            bucketCounts: [0, 0, 1],
            exemplars: [],
            flags: 0,
          },
        ],
      },
    ] as unknown as Parameters<typeof seriesBucketsToSlices>[0]

    const slices = seriesBucketsToSlices(series)

    // Two datapoints in, two slices out: no grouping, no re-bucketing. The
    // store decided where the buckets are.
    expect(slices).toHaveLength(2)
    expect(slices.map(s => s.timestamp)).toEqual([ts1, ts2])
    expect(slices.map(s => s.sourceDatapointID)).toEqual(['a', 'b'])
    expect(slices.every(s => s.attributesKey === 'driver=ALO')).toBe(true)
    expect(slices[0]!.totals.count).toBe(3)
    expect(slices[1]!.totals.count).toBe(1)
  })

  it('carries the exponential fields through untouched', () => {
    const series = [
      {
        attributesKey: 'driver=VER',
        attributes: [],
        datapoints: [
          {
            id: 'c',
            metricType: 'ExponentialHistogram',
            timestamp: ts1,
            count: 4,
            sum: 8,
            min: 1,
            max: 4,
            scale: 2,
            zeroCount: 1,
            zeroThreshold: 0.5,
            positiveBucketOffset: 3,
            positiveBucketCounts: [1, 2],
            negativeBucketOffset: 0,
            negativeBucketCounts: [],
            exemplars: [],
            flags: 0,
          },
        ],
      },
    ] as unknown as Parameters<typeof seriesBucketsToSlices>[0]

    const slices = seriesBucketsToSlices(series)
    expect(slices).toHaveLength(1)
    const slice = slices[0]!
    expect(slice.kind).toBe('expHistogram')
    if (slice.kind !== 'expHistogram') return
    // No rescaling: the store already aligned everything inside this bucket, so
    // touching the scale here could only undo that.
    expect(slice.scale).toBe(2)
    expect(slice.positiveOffset).toBe(3)
    expect(slice.positiveCounts).toEqual([1, 2])
    expect(slice.zeroCount).toBe(1)
  })
})
