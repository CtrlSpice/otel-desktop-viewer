import type {
  ExponentialHistogramDataPoint,
  HistogramDataPoint,
  MetricTimeseries,
} from '@/types/api-types'
import type { ChartPoint, ChartTimeseries } from '@/types/metric-chart-types'
import {
  expHistQuantileRecord,
  histQuantileRecord,
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

export function histogramBucketNs(
  startTsNs: bigint,
  endTsNs: bigint,
  minDataTsNs: bigint | null,
  maxPoints: number
): bigint {
  const effectiveStart =
    minDataTsNs !== null && minDataTsNs > startTsNs ? minDataTsNs : startTsNs
  const span = endTsNs - effectiveStart
  if (span <= 0n || maxPoints < 1) return MIN_BUCKET_NS
  const raw = span / BigInt(maxPoints)
  return raw < MIN_BUCKET_NS ? MIN_BUCKET_NS : raw
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
    return {
      kind: 'histogram',
      timestamp,
      attributesKey: '',
      bounds,
      counts,
      totals: rollupHistogramTotals(dps.map(totalsFromDp)),
    }
  }

  const expDps = dps as ExponentialHistogramDataPoint[]
  const posVectors = expDps.map(dp => dp.positiveBucketCounts)
  const negVectors = expDps.map(dp => dp.negativeBucketCounts)
  return {
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
  }
}

function mergeHistogramSliceCumulative(
  dps: (HistogramDataPoint | ExponentialHistogramDataPoint)[]
): HistogramSlicePoint | null {
  if (dps.length === 0) return null
  let latest = dps[0]!
  for (const dp of dps) {
    if (dp.timestamp > latest.timestamp) latest = dp
  }
  const slice = mergeHistogramSliceDelta([latest])
  return slice
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
      if (minDataTs === null || hdp.timestamp < minDataTs) {
        minDataTs = hdp.timestamp
      }
    }
  }

  const bucketNs = histogramBucketNs(startTsNs, endTsNs, minDataTs, maxPoints)
  const groups = new Map<string, (HistogramDataPoint | ExponentialHistogramDataPoint)[]>()

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
  if (slices.length === 1) return slices[0]!
  const timestamp = slices[0]!.timestamp
  const first = slices[0]!

  if (first.kind === 'histogram') {
    const histSlices = slices as Extract<HistogramSlicePoint, { kind: 'histogram' }>[]
    try {
      const merged = mergeExplicitHistogramVectors(
        histSlices.map(s => ({ bounds: s.bounds, counts: s.counts }))
      )
      return {
        kind: 'histogram',
        timestamp,
        attributesKey: '',
        bounds: merged.bounds,
        counts: merged.counts,
        totals: rollupHistogramTotals(histSlices.map(s => s.totals)),
      }
    } catch (e) {
      if (e instanceof HistogramBoundsMismatchError) throw e
      throw e
    }
  }

  const expSlices = slices as Extract<HistogramSlicePoint, { kind: 'expHistogram' }>[]
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
  return {
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
  }
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
          bounds: (s as Extract<HistogramSlicePoint, { kind: 'histogram' }>).bounds,
          counts: (s as Extract<HistogramSlicePoint, { kind: 'histogram' }>).counts,
        }))
      )
      return {
        kind: 'histogram',
        timestamp: visible[visible.length - 1]!.timestamp,
        attributesKey: '',
        bounds: merged.bounds,
        counts: merged.counts,
        totals: rollupHistogramTotals(visible.map(s => s.totals)),
      }
    } catch (e) {
      if (e instanceof HistogramBoundsMismatchError) {
        return { kind: 'boundsMismatch', message: e.message }
      }
      throw e
    }
  }

  const expVisible = visible as Extract<HistogramSlicePoint, { kind: 'expHistogram' }>[]
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
  return {
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
  }
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

export const QUANTILE_LINE_KEY_SELECTED = '__quantile:selected:'
export const QUANTILE_LINE_KEY_ALL = '__quantile:all:'

export const QUANTILE_COLORS: Record<string, string> = {
  '0.5': 'var(--color-info)',
  '0.95': 'var(--color-warning)',
  '0.99': 'var(--color-error)',
}

export const QUANTILE_LABELS: { key: string; label: string }[] = [
  { key: '0.5', label: 'p50' },
  { key: '0.95', label: 'p95' },
  { key: '0.99', label: 'p99' },
]

export function quantileKeyFromValue(q: number): string {
  return String(q)
}

export function quantileLineKey(
  scope: 'selected' | 'all',
  quantileKey: string
): string {
  return scope === 'selected'
    ? `${QUANTILE_LINE_KEY_SELECTED}${quantileKey}`
    : `${QUANTILE_LINE_KEY_ALL}${quantileKey}`
}

export function isQuantileLineKey(key: string): boolean {
  return (
    key.startsWith(QUANTILE_LINE_KEY_SELECTED) ||
    key.startsWith(QUANTILE_LINE_KEY_ALL)
  )
}

export function parseQuantileLineKey(
  key: string
): { scope: 'selected' | 'all'; quantileKey: string } | null {
  if (key.startsWith(QUANTILE_LINE_KEY_SELECTED)) {
    return {
      scope: 'selected',
      quantileKey: key.slice(QUANTILE_LINE_KEY_SELECTED.length),
    }
  }
  if (key.startsWith(QUANTILE_LINE_KEY_ALL)) {
    return {
      scope: 'all',
      quantileKey: key.slice(QUANTILE_LINE_KEY_ALL.length),
    }
  }
  return null
}

export function quantileLineLabel(
  quantileKey: string,
  scope: 'selected' | 'all'
): string {
  const pill = QUANTILE_LABELS.find(q => q.key === quantileKey)?.label ?? quantileKey
  return scope === 'selected' ? `${pill} · checked` : `${pill} · all`
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

/** Merged quantile lines over adaptive time buckets (selected or all series). */
export function buildMergedQuantileSeries(
  perAttributeSlices: HistogramSlicePoint[],
  quantiles: readonly number[],
  scope: 'selected' | 'all',
  visibleKeys: Set<string> | null
): ChartTimeseries[] | HistogramAggregationError {
  const merged = mergeHistogramSlicesAcrossTime(
    perAttributeSlices,
    scope === 'all' ? null : visibleKeys
  )
  if (isHistogramAggregationError(merged)) return merged

  const slices = merged as HistogramSlicePoint[]
  return quantiles.map(q => {
    const quantileKey = quantileKeyFromValue(q)
    return {
      key: quantileLineKey(scope, quantileKey),
      label: quantileLineLabel(quantileKey, scope),
      points: quantilePointsFromMergedSlices(slices, q),
    }
  })
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
  const base = {
    id,
    timestamp: slice.timestamp,
    startTime: slice.timestamp,
    flags: 0,
    exemplars: [],
    count: slice.totals.count,
    sum: slice.totals.sum,
    min: slice.totals.min,
    max: slice.totals.max,
    aggregationTemporality: temporality,
  }
  if (slice.kind === 'histogram') {
    return {
      ...base,
      metricType: 'Histogram',
      explicitBounds: slice.bounds,
      bucketCounts: slice.counts,
    }
  }
  return {
    ...base,
    metricType: 'ExponentialHistogram',
    scale: slice.scale,
    zeroCount: slice.zeroCount,
    zeroThreshold: slice.zeroThreshold,
    positiveBucketOffset: slice.positiveOffset,
    positiveBucketCounts: slice.positiveCounts,
    negativeBucketOffset: slice.negativeOffset,
    negativeBucketCounts: slice.negativeCounts,
  }
}

export function histogramQuantilesForDatapoint(
  dp: HistogramDataPoint | ExponentialHistogramDataPoint,
  quantiles: readonly number[] = DEFAULT_HISTOGRAM_QUANTILES
): Record<string, number | null> {
  if (dp.metricType === 'Histogram') {
    return histQuantileRecord(
      dp.explicitBounds,
      dp.bucketCounts,
      [...quantiles]
    )
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

