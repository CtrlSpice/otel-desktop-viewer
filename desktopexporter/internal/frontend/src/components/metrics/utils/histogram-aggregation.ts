import type {
  ExponentialHistogramDataPoint,
  HistogramDataPoint,
  MetricTimeseries,
} from '@/types/api-types'
import type { ChartPoint, ChartTimeseries } from '@/types/metric-chart-types'
import {
  expHistQuantileRecord,
  histQuantileRecord,
  bucketExtents,
  expBuckets,
  histBuckets,
} from '@/components/metrics/utils/histogram-quantile'
import {
  HistogramBoundsMismatchError,
  mergeExplicitHistogramVectors,
  mergeExpHistogramStreams,
  rollupHistogramTotals,
  sumBucketVectors,
  type ExpHistogramWire,
  type HistogramTotals,
} from '@/components/metrics/utils/histogram-merge'

const MIN_BUCKET_NS = 1_000_000n // 1 ms

export type HistogramSlicePoint =
  | {
      kind: 'histogram'
      timestamp: bigint
      attributesKey: string
      bounds: number[]
      counts: number[]
      totals: HistogramTotals
    }
  | {
      kind: 'expHistogram'
      timestamp: bigint
      attributesKey: string
      scale: number
      zeroThreshold: number
      zeroCount: number
      positiveOffset: number
      positiveCounts: number[]
      negativeOffset: number
      negativeCounts: number[]
      totals: HistogramTotals
    }

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
    ((value as HistogramAggregationError).kind === 'unspecified' ||
      (value as HistogramAggregationError).kind === 'boundsMismatch')
  )
}

const NS_PER_SEC = 1_000_000_000n
const NS_PER_MIN = 60n * NS_PER_SEC
const NS_PER_HOUR = 60n * NS_PER_MIN
const NS_PER_DAY = 24n * NS_PER_HOUR

/**
 * Bucket widths we are willing to choose, smallest first.
 *
 * Every entry divides evenly into a day, and the epoch is a whole number
 * of seconds, so flooring a timestamp against any of them lands on the
 * same wall-clock boundaries regardless of the query window. That is what
 * keeps columns still while the user pans or zooms -- an arbitrary width
 * like span/100 moves every boundary on the smallest range change, and
 * the whole heatmap reshuffles.
 *
 * Widths are also nameable: "5 minute buckets" rather than "1h41m".
 *
 * Caveat: day-scale alignment is against UTC midnight, not local. Fine
 * for now; revisit if buckets are ever labelled by calendar date.
 */
const NS_PER_MS = 1_000_000n

const BUCKET_LADDER: readonly bigint[] = [
  // Sub-second rungs matter for burst captures: a replay that emits a
  // whole session in a couple of seconds has nothing but sub-second
  // structure, and a 1s floor would flatten it to one or two columns.
  // These divide a second evenly, so they keep the stable-boundary
  // property the rest of the ladder has.
  NS_PER_MS,
  10n * NS_PER_MS,
  100n * NS_PER_MS,
  250n * NS_PER_MS,
  500n * NS_PER_MS,
  NS_PER_SEC,
  5n * NS_PER_SEC,
  10n * NS_PER_SEC,
  30n * NS_PER_SEC,
  NS_PER_MIN,
  5n * NS_PER_MIN,
  10n * NS_PER_MIN,
  15n * NS_PER_MIN,
  30n * NS_PER_MIN,
  NS_PER_HOUR,
  3n * NS_PER_HOUR,
  6n * NS_PER_HOUR,
  12n * NS_PER_HOUR,
  NS_PER_DAY,
]

/**
 * Median gap between consecutive timestamps, or null below two distinct
 * ones.
 *
 * Median rather than mean: a capture containing a pause (or one old burst
 * plus a recent one) has a mean gap far larger than its real reporting
 * interval, which would floor the bucket width to something uselessly
 * coarse.
 */
function medianIntervalNs(timestamps: readonly bigint[]): bigint | null {
  if (timestamps.length < 2) return null
  const sorted = [...timestamps].sort((a, b) => (a < b ? -1 : a > b ? 1 : 0))
  const gaps: bigint[] = []
  for (let i = 1; i < sorted.length; i++) {
    const gap = sorted[i]! - sorted[i - 1]!
    if (gap > 0n) gaps.push(gap)
  }
  if (gaps.length === 0) return null
  gaps.sort((a, b) => (a < b ? -1 : a > b ? 1 : 0))
  return gaps[Math.floor(gaps.length / 2)]!
}

/**
 * Choose a bucket width for the histogram heatmap.
 *
 * Spans the *data*, not the query window. A window ending at "now" over
 * data that is hours or days old would otherwise have its width set by
 * the empty tail: the race capture is 1.8s of datapoints viewed 11 days
 * later, and dividing that 11-day span by 100 put every datapoint into a
 * single column -- worse the further you zoom out, which is backwards.
 * The gauge/sum path already spans first-to-last datapoint (`bucketize`);
 * this brings histograms into line.
 *
 * Then snap up to the smallest ladder width yielding at most `maxPoints`
 * buckets, never finer than the data's own cadence -- bucketing below the
 * reporting interval only manufactures empty columns between real ones.
 */
export function histogramBucketNs(
  startTsNs: bigint,
  endTsNs: bigint,
  minDataTsNs: bigint | null,
  maxPoints: number,
  maxDataTsNs: bigint | null = null,
  dataTimestampsNs: readonly bigint[] = []
): bigint {
  const effectiveStart =
    minDataTsNs !== null && minDataTsNs > startTsNs ? minDataTsNs : startTsNs
  const effectiveEnd =
    maxDataTsNs !== null && maxDataTsNs < endTsNs ? maxDataTsNs : endTsNs

  const span = effectiveEnd - effectiveStart
  if (span <= 0n || maxPoints < 1) return MIN_BUCKET_NS

  const cadence = medianIntervalNs(dataTimestampsNs)
  const floorNs =
    cadence !== null && cadence > MIN_BUCKET_NS ? cadence : MIN_BUCKET_NS

  for (const width of BUCKET_LADDER) {
    if (width < floorNs) continue
    if (span / width <= BigInt(maxPoints)) return width
  }

  // Span outruns the ladder (beyond ~100 days at maxPoints=100): whole
  // days, rounded up so the bucket count stays under target.
  return (span / NS_PER_DAY / BigInt(maxPoints) + 1n) * NS_PER_DAY
}

export function histogramBucketStart(
  timestampNs: bigint,
  bucketNs: bigint
): bigint {
  return (timestampNs / bucketNs) * bucketNs
}

function isHistogramDp(
  dp: HistogramDataPoint | ExponentialHistogramDataPoint
): dp is HistogramDataPoint {
  return dp.metricType === 'Histogram'
}

function totalsFromDp(
  dp: HistogramDataPoint | ExponentialHistogramDataPoint
): HistogramTotals {
  return {
    count: dp.count,
    sum: dp.sum,
    min: dp.min,
    max: dp.max,
  }
}

/** Min/max from populated bucket bounds; count/sum stay on OTLP summary fields. */
export function histogramSliceBucketExtents(
  slice: HistogramSlicePoint
): { min: number; max: number } | null {
  if (slice.kind === 'histogram') {
    return bucketExtents(histBuckets(slice.bounds, slice.counts))
  }
  return bucketExtents(
    expBuckets(
      slice.scale,
      slice.negativeOffset,
      slice.negativeCounts,
      slice.zeroCount,
      slice.positiveOffset,
      slice.positiveCounts
    )
  )
}

function withBucketDerivedMinMax(
  slice: HistogramSlicePoint
): HistogramSlicePoint {
  const extents = histogramSliceBucketExtents(slice)
  if (!extents) return slice
  return {
    ...slice,
    totals: {
      ...slice.totals,
      min: extents.min,
      max: extents.max,
    },
  }
}

function mergeHistogramSliceDelta(
  dps: (HistogramDataPoint | ExponentialHistogramDataPoint)[]
): HistogramSlicePoint | null {
  if (dps.length === 0) return null
  const first = dps[0]!
  const timestamp = first.timestamp
  const attributesKey = '' // filled by caller

  if (isHistogramDp(first)) {
    const bounds = first.explicitBounds
    const vectors = dps.map(dp => (dp as HistogramDataPoint).bucketCounts)
    const counts = sumBucketVectors(vectors) ?? []
    return withBucketDerivedMinMax({
      kind: 'histogram',
      timestamp,
      attributesKey: '',
      bounds,
      counts,
      totals: rollupHistogramTotals(dps.map(totalsFromDp)),
    })
  }

  const expDps = dps as ExponentialHistogramDataPoint[]
  const posVectors = expDps.map(dp => dp.positiveBucketCounts)
  const negVectors = expDps.map(dp => dp.negativeBucketCounts)
  return withBucketDerivedMinMax({
    kind: 'expHistogram',
    timestamp,
    attributesKey: '',
    scale: expDps[0]!.scale,
    zeroThreshold: Math.max(...expDps.map(dp => dp.zeroThreshold)),
    zeroCount: expDps.reduce((n, dp) => n + dp.zeroCount, 0),
    positiveOffset: expDps[0]!.positiveBucketOffset,
    positiveCounts: sumBucketVectors(posVectors) ?? [],
    negativeOffset: expDps[0]!.negativeBucketOffset,
    negativeCounts: sumBucketVectors(negVectors) ?? [],
    totals: rollupHistogramTotals(dps.map(totalsFromDp)),
  })
}

/**
 * Convert a bucket's worth of Cumulative datapoints into the activity
 * *within* that bucket.
 *
 * Cumulative counts run since the start of the stream, so a bucket's own
 * contribution is last-minus-first, not last. Returning the latest point
 * (as this did) makes every column show the running total to date: the
 * heatmap climbs monotonically instead of showing where the activity
 * was, and it worsens with window length as more points are discarded
 * per bucket.
 *
 * A single datapoint in the bucket has no earlier point to difference
 * against, so it passes through unchanged -- the first bucket of a stream
 * therefore carries the stream's history, which is correct: that activity
 * did happen, we just cannot attribute it more precisely.
 *
 * A decrease means the counter reset (process restart). The post-reset
 * point is then already a delta from zero, so it is taken as-is rather
 * than differenced into negative counts.
 */
function mergeHistogramSliceCumulative(
  dps: (HistogramDataPoint | ExponentialHistogramDataPoint)[]
): HistogramSlicePoint | null {
  if (dps.length === 0) return null

  const ordered = [...dps].sort((a, b) =>
    a.timestamp < b.timestamp ? -1 : a.timestamp > b.timestamp ? 1 : 0
  )
  const first = ordered[0]!
  const last = ordered[ordered.length - 1]!
  if (first === last) return mergeHistogramSliceDelta([last])

  const lastSlice = mergeHistogramSliceDelta([last])
  const firstSlice = mergeHistogramSliceDelta([first])
  if (!lastSlice || !firstSlice) return lastSlice

  return subtractHistogramSlices(lastSlice, firstSlice)
}

/** Element-wise `a - b`, clamped at zero. A negative result anywhere means
 *  the counter reset inside the bucket, in which case `a` already counts
 *  from zero and is returned unchanged. */
function subtractHistogramSlices(
  a: HistogramSlicePoint,
  b: HistogramSlicePoint
): HistogramSlicePoint {
  if (a.kind !== b.kind) return a

  const diffCounts = (xs: number[], ys: number[]): number[] | null => {
    if (xs.length !== ys.length) return null
    const out: number[] = []
    for (let i = 0; i < xs.length; i++) {
      const d = xs[i]! - ys[i]!
      if (d < 0) return null
      out.push(d)
    }
    return out
  }

  const totals = {
    count: a.totals.count - b.totals.count,
    sum: a.totals.sum - b.totals.sum,
    min: a.totals.min,
    max: a.totals.max,
  }
  if (totals.count < 0) return a

  if (a.kind === 'histogram' && b.kind === 'histogram') {
    if (a.bounds.length !== b.bounds.length) return a
    const counts = diffCounts(a.counts, b.counts)
    if (counts === null) return a
    return { ...a, counts, totals }
  }

  if (a.kind === 'expHistogram' && b.kind === 'expHistogram') {
    if (a.scale !== b.scale) return a
    if (
      a.positiveOffset !== b.positiveOffset ||
      a.negativeOffset !== b.negativeOffset
    ) {
      return a
    }
    const positiveCounts = diffCounts(a.positiveCounts, b.positiveCounts)
    const negativeCounts = diffCounts(a.negativeCounts, b.negativeCounts)
    if (positiveCounts === null || negativeCounts === null) return a
    const zeroCount = a.zeroCount - b.zeroCount
    if (zeroCount < 0) return a
    return { ...a, positiveCounts, negativeCounts, zeroCount, totals }
  }

  return a
}

function mergeSliceGroup(
  dps: (HistogramDataPoint | ExponentialHistogramDataPoint)[],
  temporality: string,
  attributesKey: string
): HistogramSlicePoint | null {
  const merged =
    temporality === 'Cumulative'
      ? mergeHistogramSliceCumulative(dps)
      : mergeHistogramSliceDelta(dps)
  if (!merged) return null
  return { ...merged, attributesKey }
}

/** Per-(time bucket, attributesKey) slices after within-slice temporality merge. */
export function buildHistogramTimeMergedSeries(
  timeseries: MetricTimeseries[],
  startTsNs: bigint,
  endTsNs: bigint,
  maxPoints: number,
  temporality: string
): HistogramSlicePoint[] | HistogramAggregationError {
  if (temporality !== 'Delta' && temporality !== 'Cumulative') {
    return {
      kind: 'unspecified',
      message: `Aggregation temporality is ${temporality || 'Unspecified'}`,
    }
  }

  let minDataTs: bigint | null = null
  let maxDataTs: bigint | null = null
  const dataTimestamps: bigint[] = []
  const allDps: (HistogramDataPoint | ExponentialHistogramDataPoint)[] = []
  for (const ts of timeseries) {
    for (const dp of ts.datapoints) {
      if (
        dp.metricType !== 'Histogram' &&
        dp.metricType !== 'ExponentialHistogram'
      ) {
        continue
      }
      const hdp = dp as HistogramDataPoint | ExponentialHistogramDataPoint
      if (hdp.timestamp < startTsNs || hdp.timestamp >= endTsNs) continue
      allDps.push(hdp)
      dataTimestamps.push(hdp.timestamp)
      if (minDataTs === null || hdp.timestamp < minDataTs) {
        minDataTs = hdp.timestamp
      }
      if (maxDataTs === null || hdp.timestamp > maxDataTs) {
        maxDataTs = hdp.timestamp
      }
    }
  }

  const bucketNs = histogramBucketNs(
    startTsNs,
    endTsNs,
    minDataTs,
    maxPoints,
    maxDataTs,
    dataTimestamps
  )
  const groups = new Map<
    string,
    (HistogramDataPoint | ExponentialHistogramDataPoint)[]
  >()

  for (const ts of timeseries) {
    for (const dp of ts.datapoints) {
      if (
        dp.metricType !== 'Histogram' &&
        dp.metricType !== 'ExponentialHistogram'
      ) {
        continue
      }
      const hdp = dp as HistogramDataPoint | ExponentialHistogramDataPoint
      if (hdp.timestamp < startTsNs || hdp.timestamp >= endTsNs) continue
      const bucketStart = histogramBucketStart(hdp.timestamp, bucketNs)
      const key = `${bucketStart.toString()}\0${ts.attributesKey}`
      const list = groups.get(key)
      if (list) list.push(hdp)
      else groups.set(key, [hdp])
    }
  }

  const out: HistogramSlicePoint[] = []
  for (const [key, dps] of groups) {
    const sep = key.indexOf('\0')
    const bucketStart = BigInt(key.slice(0, sep))
    const attributesKey = key.slice(sep + 1)
    const slice = mergeSliceGroup(dps, temporality, attributesKey)
    if (slice) out.push({ ...slice, timestamp: bucketStart })
  }

  out.sort((a, b) => {
    if (a.timestamp !== b.timestamp) {
      return a.timestamp < b.timestamp ? -1 : 1
    }
    return a.attributesKey.localeCompare(b.attributesKey)
  })
  return out
}

function filterVisibleSlices(
  slices: HistogramSlicePoint[],
  visibleKeys: Set<string> | null
): HistogramSlicePoint[] {
  if (!visibleKeys) return slices
  return slices.filter(s => visibleKeys.has(s.attributesKey))
}

function mergeSlicesAtTimestamp(
  slices: HistogramSlicePoint[]
): HistogramSlicePoint {
  if (slices.length === 1) return withBucketDerivedMinMax(slices[0]!)
  const timestamp = slices[0]!.timestamp
  const first = slices[0]!

  if (first.kind === 'histogram') {
    const histSlices = slices as Extract<
      HistogramSlicePoint,
      { kind: 'histogram' }
    >[]
    try {
      const merged = mergeExplicitHistogramVectors(
        histSlices.map(s => ({ bounds: s.bounds, counts: s.counts }))
      )
      return withBucketDerivedMinMax({
        kind: 'histogram',
        timestamp,
        attributesKey: '',
        bounds: merged.bounds,
        counts: merged.counts,
        totals: rollupHistogramTotals(histSlices.map(s => s.totals)),
      })
    } catch (e) {
      if (e instanceof HistogramBoundsMismatchError) throw e
      throw e
    }
  }

  const expSlices = slices as Extract<
    HistogramSlicePoint,
    { kind: 'expHistogram' }
  >[]
  const wires: ExpHistogramWire[] = expSlices.map(s => ({
    scale: s.scale,
    zeroCount: s.zeroCount,
    zeroThreshold: s.zeroThreshold,
    positiveBucketOffset: s.positiveOffset,
    positiveBucketCounts: s.positiveCounts,
    negativeBucketOffset: s.negativeOffset,
    negativeBucketCounts: s.negativeCounts,
  }))
  const merged = mergeExpHistogramStreams(
    wires,
    rollupHistogramTotals(expSlices.map(s => s.totals))
  )
  return withBucketDerivedMinMax({
    kind: 'expHistogram',
    timestamp,
    attributesKey: '',
    scale: merged.scale,
    zeroThreshold: merged.zeroThreshold,
    zeroCount: merged.zeroCount,
    positiveOffset: merged.positiveBucketOffset,
    positiveCounts: merged.positiveBucketCounts,
    negativeOffset: merged.negativeBucketOffset,
    negativeCounts: merged.negativeBucketCounts,
    totals: {
      count: merged.count,
      sum: merged.sum,
      min: merged.min,
      max: merged.max,
    },
  })
}

/** Merge visible per-attribute slices per timestamp (heatmap column). */
export function mergeHistogramSlicesAcrossTime(
  slices: HistogramSlicePoint[],
  visibleKeys: Set<string> | null
): HistogramSlicePoint[] | HistogramAggregationError {
  const visible = filterVisibleSlices(slices, visibleKeys)
  const byTime = new Map<string, HistogramSlicePoint[]>()
  for (const s of visible) {
    const key = s.timestamp.toString()
    const list = byTime.get(key)
    if (list) list.push(s)
    else byTime.set(key, [s])
  }
  const out: HistogramSlicePoint[] = []
  try {
    for (const group of byTime.values()) {
      out.push(mergeSlicesAtTimestamp(group))
    }
  } catch (e) {
    if (e instanceof HistogramBoundsMismatchError) {
      return { kind: 'boundsMismatch', message: e.message }
    }
    throw e
  }
  out.sort((a, b) => (a.timestamp < b.timestamp ? -1 : 1))
  return out
}

/** Full-window merge of visible per-attribute slices (Summary tab). */
export function mergeHistogramWindowSummary(
  perAttributeSlices: HistogramSlicePoint[],
  visibleKeys: Set<string> | null,
  temporality: string
): HistogramSlicePoint | null | HistogramAggregationError {
  const visible = filterVisibleSlices(perAttributeSlices, visibleKeys)
  if (visible.length === 0) return null

  if (temporality === 'Cumulative') {
    // Latest slice per attributesKey, then merge across series.
    const latestByKey = new Map<string, HistogramSlicePoint>()
    for (const s of visible) {
      const prev = latestByKey.get(s.attributesKey)
      if (!prev || s.timestamp > prev.timestamp) {
        latestByKey.set(s.attributesKey, s)
      }
    }
    try {
      return mergeSlicesAtTimestamp([...latestByKey.values()])
    } catch (e) {
      if (e instanceof HistogramBoundsMismatchError) {
        return { kind: 'boundsMismatch', message: e.message }
      }
      throw e
    }
  }

  // Delta: merge all slices (each time bucket) into one distribution.
  if (visible[0]!.kind === 'histogram') {
    try {
      const merged = mergeExplicitHistogramVectors(
        visible.map(s => ({
          bounds: (s as Extract<HistogramSlicePoint, { kind: 'histogram' }>)
            .bounds,
          counts: (s as Extract<HistogramSlicePoint, { kind: 'histogram' }>)
            .counts,
        }))
      )
      return withBucketDerivedMinMax({
        kind: 'histogram',
        timestamp: visible[visible.length - 1]!.timestamp,
        attributesKey: '',
        bounds: merged.bounds,
        counts: merged.counts,
        totals: rollupHistogramTotals(visible.map(s => s.totals)),
      })
    } catch (e) {
      if (e instanceof HistogramBoundsMismatchError) {
        return { kind: 'boundsMismatch', message: e.message }
      }
      throw e
    }
  }

  const expVisible = visible as Extract<
    HistogramSlicePoint,
    { kind: 'expHistogram' }
  >[]
  const wires: ExpHistogramWire[] = expVisible.map(s => ({
    scale: s.scale,
    zeroCount: s.zeroCount,
    zeroThreshold: s.zeroThreshold,
    positiveBucketOffset: s.positiveOffset,
    positiveBucketCounts: s.positiveCounts,
    negativeBucketOffset: s.negativeOffset,
    negativeBucketCounts: s.negativeCounts,
  }))
  const merged = mergeExpHistogramStreams(
    wires,
    rollupHistogramTotals(expVisible.map(s => s.totals))
  )
  return withBucketDerivedMinMax({
    kind: 'expHistogram',
    timestamp: visible[visible.length - 1]!.timestamp,
    attributesKey: '',
    scale: merged.scale,
    zeroThreshold: merged.zeroThreshold,
    zeroCount: merged.zeroCount,
    positiveOffset: merged.positiveBucketOffset,
    positiveCounts: merged.positiveBucketCounts,
    negativeOffset: merged.negativeBucketOffset,
    negativeCounts: merged.negativeBucketCounts,
    totals: {
      count: merged.count,
      sum: merged.sum,
      min: merged.min,
      max: merged.max,
    },
  })
}

/** Slice at a heatmap column timestamp (visible series merged). */
export function histogramSliceAtTimestamp(
  perAttributeSlices: HistogramSlicePoint[],
  timestampNs: bigint,
  visibleKeys: Set<string> | null
): HistogramSlicePoint | null | HistogramAggregationError {
  const merged = mergeHistogramSlicesAcrossTime(perAttributeSlices, visibleKeys)
  if ('kind' in merged && merged.kind === 'boundsMismatch') return merged
  if ('kind' in merged && merged.kind === 'unspecified') return merged
  const list = merged as HistogramSlicePoint[]
  return list.find(s => s.timestamp === timestampNs) ?? null
}

export function histogramBucketWidthMs(
  startTsNs: bigint,
  endTsNs: bigint,
  minDataTsNs: bigint | null,
  maxPoints: number
): number {
  const ns = histogramBucketNs(startTsNs, endTsNs, minDataTsNs, maxPoints)
  return Number(ns / 1_000_000n)
}

export function minHistogramTimestampInWindow(
  timeseries: MetricTimeseries[],
  startTsNs: bigint,
  endTsNs: bigint
): bigint | null {
  let min: bigint | null = null
  for (const ts of timeseries) {
    for (const dp of ts.datapoints) {
      if (
        dp.metricType !== 'Histogram' &&
        dp.metricType !== 'ExponentialHistogram'
      ) {
        continue
      }
      if (dp.timestamp < startTsNs || dp.timestamp >= endTsNs) continue
      if (min === null || dp.timestamp < min) min = dp.timestamp
    }
  }
  return min
}

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

export function sliceQuantileValue(
  slice: HistogramSlicePoint,
  quantile: number
): number | null {
  const dp = histogramSliceToDatapoint(slice, 'quantile', 'Delta')
  const record = histogramQuantilesForDatapoint(dp, [quantile])
  return record[quantileKeyFromValue(quantile)] ?? null
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
    })
  }
  points.sort((a, b) => a.date.getTime() - b.date.getTime())
  return points
}

/** Per-visible-series quantile lines for each active percentile overlay. */
export function buildVisibleSeriesQuantileChartTimeseries(
  perAttributeSlices: HistogramSlicePoint[],
  quantiles: readonly number[],
  visibleKeys: Set<string> | null
): ChartTimeseries[] {
  const out: ChartTimeseries[] = []
  for (const q of quantiles) {
    const quantileKey = quantileKeyFromValue(q)
    const pill = quantileLabelForKey(quantileKey)
    for (const line of buildPerSeriesQuantileSeries(
      perAttributeSlices,
      q,
      visibleKeys
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

/** Per-visible-series quantile line for one percentile. */
export function buildPerSeriesQuantileSeries(
  perAttributeSlices: HistogramSlicePoint[],
  quantile: number,
  visibleKeys: Set<string> | null
): ChartTimeseries[] {
  const visible = filterVisibleSlices(perAttributeSlices, visibleKeys)
  const byKey = new Map<string, HistogramSlicePoint[]>()
  for (const slice of visible) {
    const list = byKey.get(slice.attributesKey)
    if (list) list.push(slice)
    else byKey.set(slice.attributesKey, [slice])
  }

  const out: ChartTimeseries[] = []
  for (const [key, slices] of byKey) {
    const points = quantilePointsFromMergedSlices(slices, quantile)
    if (points.length === 0) continue
    out.push({ key, label: key, points })
  }
  out.sort((a, b) => a.key.localeCompare(b.key))
  return out
}

export function histogramSliceToDatapoint(
  slice: HistogramSlicePoint,
  id: string,
  temporality: string
): HistogramDataPoint | ExponentialHistogramDataPoint {
  const normalized = withBucketDerivedMinMax(slice)
  const base = {
    id,
    timestamp: normalized.timestamp,
    startTime: normalized.timestamp,
    flags: 0,
    exemplars: [],
    count: normalized.totals.count,
    sum: normalized.totals.sum,
    min: normalized.totals.min,
    max: normalized.totals.max,
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

export function histogramQuantilesForDatapoint(
  dp: HistogramDataPoint | ExponentialHistogramDataPoint,
  quantiles: readonly number[] = DEFAULT_HISTOGRAM_QUANTILES
): Record<string, number | null> {
  if (dp.metricType === 'Histogram') {
    return histQuantileRecord(dp.explicitBounds, dp.bucketCounts, [
      ...quantiles,
    ])
  }
  return expHistQuantileRecord(
    dp.scale,
    dp.negativeBucketOffset,
    dp.negativeBucketCounts,
    dp.zeroCount,
    dp.positiveBucketOffset,
    dp.positiveBucketCounts,
    [...quantiles]
  )
}
