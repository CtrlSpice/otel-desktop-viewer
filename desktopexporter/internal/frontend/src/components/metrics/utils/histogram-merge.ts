// Histogram bucket-vector merge helpers.
// Ports DuckDB macros from desktopexporter/internal/store/schema/schema.go.

export class HistogramBoundsMismatchError extends Error {
  readonly kind = 'boundsMismatch' as const
  constructor(message = 'Histogram bounds disagree across series') {
    super(message)
    this.name = 'HistogramBoundsMismatchError'
  }
}

/** Floor division toward negative infinity (matches DuckDB floor_div). */
export function floorDiv(a: number, b: number): number {
  return Math.floor(a / b)
}

/** Element-wise sum of equal-length numeric lists. */
export function sumBucketVectors(vectors: number[][]): number[] | null {
  if (vectors.length === 0) return null
  let acc = vectors[0]!.slice()
  for (let v = 1; v < vectors.length; v++) {
    const next = vectors[v]!
    const len = Math.max(acc.length, next.length)
    const merged = new Array<number>(len)
    for (let i = 0; i < len; i++) {
      merged[i] = (acc[i] ?? 0) + (next[i] ?? 0)
    }
    acc = merged
  }
  return acc
}

export type DownscaledBuckets = { offset: number; counts: number[] }

export function downscaleExpBuckets(
  counts: number[] | null | undefined,
  offset: number,
  levels: number
): DownscaledBuckets {
  if (!counts || counts.length === 0 || levels <= 0) {
    return { offset, counts: counts ?? [] }
  }
  const factor = Math.pow(2, levels)
  const newOffset = floorDiv(offset, factor)
  const lastOriginal = offset + counts.length - 1
  const lastK = floorDiv(lastOriginal, factor)
  const outLen = lastK - newOffset + 1
  const out = new Array<number>(outLen).fill(0)
  for (let i = 0; i < counts.length; i++) {
    const originalIdx = offset + i
    const k = floorDiv(originalIdx, factor) - newOffset
    out[k]! += counts[i]!
  }
  return { offset: newOffset, counts: out }
}

export type FoldBelowCutoffResult = {
  counts: number[]
  offset: number
  folded: number
}

export function foldBelowCutoff(
  counts: number[] | null | undefined,
  offset: number,
  cutoff: number | null
): FoldBelowCutoffResult {
  if (!counts || counts.length === 0 || cutoff === null || cutoff < offset) {
    return { counts: counts ?? [], offset, folded: 0 }
  }
  const dropN = Math.min(cutoff - offset + 1, counts.length)
  let folded = 0
  for (let i = 0; i < dropN; i++) folded += counts[i]!
  return {
    counts: counts.slice(dropN),
    offset: offset + dropN,
    folded,
  }
}

export function padLeftToOffset(
  counts: number[] | null | undefined,
  currentOffset: number,
  targetOffset: number
): number[] | null {
  if (!counts || currentOffset <= targetOffset) return counts ?? null
  const padLen = currentOffset - targetOffset
  return [...new Array<number>(padLen).fill(0), ...counts]
}

export function boundsKey(bounds: number[]): string {
  return bounds.join('\0')
}

/** Merge explicit histogram bucket vectors; throws on bounds mismatch. */
export function mergeExplicitHistogramVectors(
  entries: { bounds: number[]; counts: number[] }[]
): { bounds: number[]; counts: number[] } {
  if (entries.length === 0) {
    return { bounds: [], counts: [] }
  }
  const firstKey = boundsKey(entries[0]!.bounds)
  for (let i = 1; i < entries.length; i++) {
    if (boundsKey(entries[i]!.bounds) !== firstKey) {
      throw new HistogramBoundsMismatchError()
    }
  }
  const merged = sumBucketVectors(entries.map(e => e.counts))
  return { bounds: entries[0]!.bounds, counts: merged ?? [] }
}

export type ExpHistogramWire = {
  scale: number
  zeroCount: number
  zeroThreshold: number
  positiveBucketOffset: number
  positiveBucketCounts: number[]
  negativeBucketOffset: number
  negativeBucketCounts: number[]
}

export type MergedExpHistogramWire = ExpHistogramWire & {
  count: number
  sum: number
  min: number
  max: number
}

function expPositiveCutoff(
  targetZeroThreshold: number,
  targetScale: number
): number | null {
  if (targetZeroThreshold <= 0) return null
  return (
    Math.floor(Math.log2(targetZeroThreshold) * Math.pow(2, targetScale)) - 1
  )
}

/** Cross-series exponential histogram merge (ports merged exp SQL pipeline). */
export function mergeExpHistogramStreams(
  entries: ExpHistogramWire[],
  totals: { count: number; sum: number; min: number; max: number }
): MergedExpHistogramWire {
  if (entries.length === 0) {
    return {
      scale: 0,
      zeroCount: 0,
      zeroThreshold: 0,
      positiveBucketOffset: 0,
      positiveBucketCounts: [],
      negativeBucketOffset: 0,
      negativeBucketCounts: [],
      ...totals,
    }
  }

  const targetScale = Math.min(...entries.map(e => e.scale))
  const targetZeroThreshold = Math.max(...entries.map(e => e.zeroThreshold))
  let baseZeroCount = 0
  for (const e of entries) baseZeroCount += e.zeroCount

  const downscaled = entries.map(e => ({
    pos: downscaleExpBuckets(
      e.positiveBucketCounts,
      e.positiveBucketOffset,
      e.scale - targetScale
    ),
    neg: downscaleExpBuckets(
      e.negativeBucketCounts,
      e.negativeBucketOffset,
      e.scale - targetScale
    ),
  }))

  const posTargetOffset = Math.min(...downscaled.map(d => d.pos.offset))
  const negTargetOffset = Math.min(...downscaled.map(d => d.neg.offset))

  const posPadded = downscaled.map(d =>
    padLeftToOffset(d.pos.counts, d.pos.offset, posTargetOffset)
  )
  const negPadded = downscaled.map(d =>
    padLeftToOffset(d.neg.counts, d.neg.offset, negTargetOffset)
  )

  const posSummed =
    sumBucketVectors(posPadded.filter((v): v is number[] => v != null)) ?? []
  const negSummed =
    sumBucketVectors(negPadded.filter((v): v is number[] => v != null)) ?? []

  const posCutoff = expPositiveCutoff(targetZeroThreshold, targetScale)
  const negCutoff = expPositiveCutoff(targetZeroThreshold, targetScale)

  const posFold = foldBelowCutoff(posSummed, posTargetOffset, posCutoff)
  const negFold = foldBelowCutoff(negSummed, negTargetOffset, negCutoff)

  return {
    scale: targetScale,
    zeroThreshold: targetZeroThreshold,
    zeroCount: baseZeroCount + posFold.folded + negFold.folded,
    positiveBucketOffset: posFold.offset,
    positiveBucketCounts: posFold.counts,
    negativeBucketOffset: negFold.offset,
    negativeBucketCounts: negFold.counts,
    ...totals,
  }
}

export type HistogramTotals = {
  count: number
  sum: number
  min: number
  max: number
}

export function rollupHistogramTotals(
  entries: HistogramTotals[]
): HistogramTotals {
  if (entries.length === 0) {
    return { count: 0, sum: 0, min: 0, max: 0 }
  }
  let count = 0
  let sum = 0
  let min = Infinity
  let max = -Infinity
  for (const e of entries) {
    count += e.count
    sum += e.sum
    if (e.min < min) min = e.min
    if (e.max > max) max = e.max
  }
  if (!Number.isFinite(min)) min = 0
  if (!Number.isFinite(max)) max = 0
  return { count, sum, min, max }
}
