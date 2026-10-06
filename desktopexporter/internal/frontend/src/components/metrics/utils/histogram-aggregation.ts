import type {
  ExponentialHistogramDataPoint,
  HistogramDataPoint,
  MetricSeriesViewData,
} from '@/types/api-types'
import type { ChartPoint, ChartTimeseries } from '@/types/metric-chart-types'

/** @derived Numeric chart projection of a histogram bucket. Count comes from
 * received or SQL-reduced uint64 counts and may approximate past 2^53; the
 * remaining values use the metric's unit and IEEE-754 precision. */
export type HistogramTotals = {
  count: number
  sum: number | undefined
  min: number | undefined
  max: number | undefined
}

export type HistogramSlicePoint =
  | {
      kind: 'histogram'
      timestamp: bigint
      sourceDatapointID?: string
      seriesRef: string
      bounds: number[]
      counts: number[]
      totals: HistogramTotals
      /** The store's quantiles for this bucket, when they were requested. */
      quantiles?: Record<string, number | null> | null
    }
  | {
      kind: 'expHistogram'
      timestamp: bigint
      sourceDatapointID?: string
      seriesRef: string
      scale: number
      zeroThreshold: number
      zeroCount: number
      positiveOffset: number
      positiveCounts: number[]
      negativeOffset: number
      negativeCounts: number[]
      totals: HistogramTotals
      /** The store's quantiles for this bucket, when they were requested. */
      quantiles?: Record<string, number | null> | null
    }

export type HistogramChartDataPoint =
  | (Omit<
      HistogramDataPoint,
      'count' | 'bucketCounts' | 'sum' | 'min' | 'max'
    > & {
      count: number
      bucketCounts: number[]
      sum: number | undefined
      min: number | undefined
      max: number | undefined
    })
  | (Omit<
      ExponentialHistogramDataPoint,
      | 'count'
      | 'zeroCount'
      | 'positiveBucketCounts'
      | 'negativeBucketCounts'
      | 'sum'
      | 'min'
      | 'max'
    > & {
      count: number
      zeroCount: number
      positiveBucketCounts: number[]
      negativeBucketCounts: number[]
      sum: number | undefined
      min: number | undefined
      max: number | undefined
    })

export type HistogramAggregationError =
  | { kind: 'unspecified'; message: string }
  | { kind: 'boundsMismatch'; message: string }

export function isHistogramAggregationError(
  value: unknown
): value is HistogramAggregationError {
  return (
    typeof value === 'object' &&
    value !== null &&
    'kind' in value &&
    'message' in value &&
    typeof value.message === 'string' &&
    (value.kind === 'unspecified' || value.kind === 'boundsMismatch')
  )
}

/**
 * Offset, in nanoseconds, to add before flooring so that day- and hour-scale
 * buckets break at local wall-clock boundaries rather than UTC ones.
 *
 * Flooring against the epoch aligns to UTC, so in UTC+10 a "1 day" column runs
 * 10:00-10:00 local -- which is wrong wherever the boundary is being read as a
 * date. Sub-hour widths are unaffected in whole-hour zones, but half-hour and
 * 45-minute zones exist (India, Nepal, Chatham), so the offset is applied at
 * every width rather than only the large ones.
 *
 * The offset is resolved per timestamp, so a bucket either side of a DST
 * transition uses the offset in force at that moment instead of one snapshot
 * applied to the whole range.
 */
/** The viewer's UTC offset at a moment, in nanoseconds. Exported so a caller
 *  can hand the same alignment to the store, which buckets on epoch boundaries
 *  unless told otherwise. */
export function localOffsetNs(timestampNs: bigint): bigint {
  const ms = Number(timestampNs / 1_000_000n)
  // getTimezoneOffset is minutes *behind* UTC, so negate it.
  return BigInt(-new Date(ms).getTimezoneOffset()) * 60n * 1_000_000_000n
}

/** Keep slices whose series are visible. Empty means no visible series. */
function filterVisibleSlices(
  slices: HistogramSlicePoint[],
  visibleKeys: Set<string>
): HistogramSlicePoint[] {
  return slices.filter(s => visibleKeys.has(s.seriesRef))
}

/** Store bucket target for histogram views. Rendering handles cell sizing. */
export const HEATMAP_BUCKET_TARGET = 100

export const DEFAULT_HISTOGRAM_QUANTILES = [0.5, 0.95, 0.99] as const

export const QUANTILE_SERIES_KEY_SEP = '\0q:'

export const QUANTILE_LABELS: { key: string; label: string }[] = [
  { key: '0.5', label: 'p50' },
  { key: '0.95', label: 'p95' },
  { key: '0.99', label: 'p99' },
]

export function quantileKeyFromValue(q: number): string {
  return String(q)
}

/** Default quantile overlay when opening the Quantiles tab (p50). */
export const DEFAULT_ACTIVE_HISTOGRAM_QUANTILE_KEY = quantileKeyFromValue(0.5)

export function quantileSeriesKey(
  seriesKey: string,
  quantileKey: string
): string {
  return `${seriesKey}${QUANTILE_SERIES_KEY_SEP}${quantileKey}`
}

export function parseQuantileSeriesKey(
  key: string
): { seriesKey: string; quantileKey: string } | null {
  const idx = key.indexOf(QUANTILE_SERIES_KEY_SEP)
  if (idx === -1) return null
  return {
    seriesKey: key.slice(0, idx),
    quantileKey: key.slice(idx + QUANTILE_SERIES_KEY_SEP.length),
  }
}

export function quantileLabelForKey(quantileKey: string): string {
  return QUANTILE_LABELS.find(q => q.key === quantileKey)?.label ?? quantileKey
}

/** Looks up the store-computed quantile for this bucket. */
export function sliceQuantileValue(
  slice: HistogramSlicePoint,
  quantile: number
): number | null {
  return slice.quantiles?.[quantileKeyFromValue(quantile)] ?? null
}

function quantilePointsFromMergedSlices(
  slices: HistogramSlicePoint[],
  quantile: number
): ChartPoint[] {
  const points: ChartPoint[] = []
  for (const slice of slices) {
    const value = sliceQuantileValue(slice, quantile)
    if (value === null || !Number.isFinite(value)) continue
    points.push({
      date: new Date(Number(slice.timestamp / 1_000_000n)),
      value,
      timestampNs: slice.timestamp,
      sourceDatapointID: slice.sourceDatapointID,
    })
  }
  points.sort((a, b) => {
    const millisecondOrder = a.date.getTime() - b.date.getTime()
    if (millisecondOrder !== 0) return millisecondOrder
    if (a.timestampNs === undefined || b.timestampNs === undefined) return 0
    return a.timestampNs < b.timestampNs
      ? -1
      : a.timestampNs > b.timestampNs
        ? 1
        : 0
  })
  return points
}

/** Per-visible-series quantile lines for each active percentile overlay. */
export function buildVisibleSeriesQuantileChartTimeseries(
  perAttributeSlices: HistogramSlicePoint[],
  quantiles: readonly number[],
  visibleKeys: Set<string>,
  /** Series id -> how a reader names it; see buildPerSeriesQuantileSeries. */
  labelByKey?: ReadonlyMap<string, string>
): ChartTimeseries[] {
  const out: ChartTimeseries[] = []
  for (const q of quantiles) {
    const quantileKey = quantileKeyFromValue(q)
    const pill = quantileLabelForKey(quantileKey)
    for (const line of buildPerSeriesQuantileSeries(
      perAttributeSlices,
      q,
      visibleKeys,
      labelByKey
    )) {
      out.push({
        key: quantileSeriesKey(line.key, quantileKey),
        label: `${line.label} · ${pill}`,
        points: line.points,
      })
    }
  }
  out.sort((a, b) => a.key.localeCompare(b.key))
  return out
}

export function seriesBucketsToSlices(
  timeseries: MetricSeriesViewData[]
): HistogramSlicePoint[] {
  const out: HistogramSlicePoint[] = []
  for (const ts of timeseries) {
    for (const dp of ts.datapoints) {
      if (
        dp.metricType !== 'Histogram' &&
        dp.metricType !== 'ExponentialHistogram'
      ) {
        continue
      }
      // Histogram charts operate in the numeric display domain. Keep the
      // received datapoint exact and approximate only the projected slice.
      const totals = {
        count: Number(dp.count),
        sum: dp.sum ?? undefined,
        min: dp.min ?? undefined,
        max: dp.max ?? undefined,
      }
      if (dp.metricType === 'Histogram') {
        out.push({
          kind: 'histogram',
          timestamp: dp.timestamp,
          sourceDatapointID: dp.id,
          seriesRef: ts.seriesRef,
          bounds: dp.explicitBounds ?? [],
          counts: dp.bucketCounts.map(Number),
          totals,
          quantiles: dp.quantiles ?? null,
        })
        continue
      }
      out.push({
        kind: 'expHistogram',
        timestamp: dp.timestamp,
        sourceDatapointID: dp.id,
        seriesRef: ts.seriesRef,
        scale: dp.scale ?? 0,
        zeroThreshold: dp.zeroThreshold ?? 0,
        zeroCount: Number(dp.zeroCount),
        positiveOffset: dp.positiveBucketOffset ?? 0,
        positiveCounts: dp.positiveBucketCounts.map(Number),
        negativeOffset: dp.negativeBucketOffset ?? 0,
        negativeCounts: dp.negativeBucketCounts.map(Number),
        totals,
        quantiles: dp.quantiles ?? null,
      })
    }
  }
  return out
}

export function buildPerSeriesQuantileSeries(
  perAttributeSlices: HistogramSlicePoint[],
  quantile: number,
  visibleKeys: Set<string>,
  /** Series id -> how a reader names it. Ids are opaque, so without
   *  this a legend entry reads as a uuid. */
  labelByKey?: ReadonlyMap<string, string>
): ChartTimeseries[] {
  const visible = filterVisibleSlices(perAttributeSlices, visibleKeys)
  const byKey = new Map<string, HistogramSlicePoint[]>()
  for (const slice of visible) {
    const list = byKey.get(slice.seriesRef)
    if (list) list.push(slice)
    else byKey.set(slice.seriesRef, [slice])
  }

  const out: ChartTimeseries[] = []
  for (const [key, slices] of byKey) {
    const points = quantilePointsFromMergedSlices(slices, quantile)
    if (points.length === 0) continue
    out.push({ key, label: labelByKey?.get(key) ?? key, points })
  }
  out.sort((a, b) => a.key.localeCompare(b.key))
  return out
}

export function histogramDatapointToChartDatapoint(
  datapoint: HistogramDataPoint | ExponentialHistogramDataPoint
): HistogramChartDataPoint {
  if (datapoint.metricType === 'Histogram') {
    return {
      ...datapoint,
      count: Number(datapoint.count),
      bucketCounts: datapoint.bucketCounts.map(Number),
      sum: datapoint.sum ?? undefined,
      min: datapoint.min ?? undefined,
      max: datapoint.max ?? undefined,
    }
  }
  return {
    ...datapoint,
    count: Number(datapoint.count),
    zeroCount: Number(datapoint.zeroCount),
    positiveBucketCounts: datapoint.positiveBucketCounts.map(Number),
    negativeBucketCounts: datapoint.negativeBucketCounts.map(Number),
    sum: datapoint.sum ?? undefined,
    min: datapoint.min ?? undefined,
    max: datapoint.max ?? undefined,
  }
}

export function histogramSliceToChartDatapoint(
  slice: HistogramSlicePoint,
  id: string,
  temporality: string,
  temporalityCode: number
): HistogramChartDataPoint {
  const normalized = slice
  const base = {
    id,
    timestamp: normalized.timestamp,
    timestampMs: Number(normalized.timestamp / 1_000_000n),
    startTime: normalized.timestamp,
    flags: 0,
    // A merged datapoint is built from bucket vectors, which carry no
    // exemplars -- so it holds none, and none were withheld.
    exemplars: [],
    count: normalized.totals.count,
    sum: normalized.totals.sum,
    min: normalized.totals.min,
    max: normalized.totals.max,
    aggregationTemporalityCode: temporalityCode,
    aggregationTemporality: temporality,
  }
  if (normalized.kind === 'histogram') {
    return {
      ...base,
      metricType: 'Histogram',
      explicitBounds: normalized.bounds,
      bucketCounts: normalized.counts,
    }
  }
  return {
    ...base,
    metricType: 'ExponentialHistogram',
    scale: normalized.scale,
    zeroCount: normalized.zeroCount,
    zeroThreshold: normalized.zeroThreshold,
    positiveBucketOffset: normalized.positiveOffset,
    positiveBucketCounts: normalized.positiveCounts,
    negativeBucketOffset: normalized.negativeOffset,
    negativeBucketCounts: normalized.negativeCounts,
  }
}
