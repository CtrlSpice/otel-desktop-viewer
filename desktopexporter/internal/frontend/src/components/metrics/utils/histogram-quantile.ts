// Quantile interpolation for explicit and exponential histograms.
// Ports DuckDB macros from desktopexporter/internal/store/schema/schema.go.

export type HistBucket = { lo: number; hi: number; cnt: number }

export function interpLinear(
  lo: number,
  hi: number,
  accPrev: number,
  cnt: number,
  target: number
): number {
  if (cnt === 0) return lo
  return lo + ((hi - lo) * (target - accPrev)) / cnt
}

export function interpLoglin(
  lo: number,
  hi: number,
  accPrev: number,
  cnt: number,
  target: number
): number {
  if (lo === 0 || hi === 0 || Math.sign(lo) !== Math.sign(hi)) {
    return interpLinear(lo, hi, accPrev, cnt, target)
  }
  return lo * Math.pow(hi / lo, (target - accPrev) / cnt)
}

/** Explicit-bound histogram buckets in CDF order. counts.length = bounds.length + 1. */
export function histBuckets(bounds: number[], counts: number[]): HistBucket[] {
  if (bounds.length === 0 || counts.length === 0) return []
  const out: HistBucket[] = []
  for (let i = 0; i < counts.length; i++) {
    let lo: number
    let hi: number
    if (i === 0) {
      lo = bounds[0]!
      hi = bounds[0]!
    } else if (i === counts.length - 1) {
      lo = bounds[bounds.length - 1]!
      hi = bounds[bounds.length - 1]!
    } else {
      lo = bounds[i - 1]!
      hi = bounds[i]!
    }
    out.push({ lo, hi, cnt: counts[i]! })
  }
  return out
}

function expBase(scale: number): number {
  return Math.pow(2, Math.pow(2, -scale))
}

/** Positive exp-histogram region buckets in CDF order. */
export function expPosBuckets(
  scale: number,
  offset: number,
  counts: number[]
): HistBucket[] {
  const base = expBase(scale)
  return counts.map((cnt, i) => {
    const idx = offset + i
    return {
      lo: Math.pow(base, idx),
      hi: Math.pow(base, idx + 1),
      cnt,
    }
  })
}

/** Negative exp-histogram region buckets in CDF order (most negative first). */
export function expNegBuckets(
  scale: number,
  offset: number,
  counts: number[]
): HistBucket[] {
  const base = expBase(scale)
  const out: HistBucket[] = []
  for (let j = counts.length - 1; j >= 0; j--) {
    const idx = offset + j
    out.push({
      lo: -Math.pow(base, idx + 1),
      hi: -Math.pow(base, idx),
      cnt: counts[j]!,
    })
  }
  return out
}

export function expZeroBucket(zeroCount: number): HistBucket[] {
  return [{ lo: 0, hi: 0, cnt: zeroCount }]
}

export function expBuckets(
  scale: number,
  negOffset: number,
  negCounts: number[],
  zeroCount: number,
  posOffset: number,
  posCounts: number[]
): HistBucket[] {
  return [
    ...expNegBuckets(scale, negOffset, negCounts),
    ...expZeroBucket(zeroCount),
    ...expPosBuckets(scale, posOffset, posCounts),
  ]
}

/** Lower bound of a populated bucket for min/max display. */
function bucketLoForExtent(b: HistBucket): number {
  if (Number.isFinite(b.lo)) return b.lo
  // (-∞, hi] underflow — finite lower bound unknown; use 0 for non-negative metrics.
  return 0
}

/** Upper bound of a populated bucket for min/max display. */
function bucketHiForExtent(b: HistBucket): number {
  if (Number.isFinite(b.hi)) return b.hi
  // [lo, +∞) overflow
  return b.lo
}

/** Min/max over populated bucket bounds (ignores OTLP summary fields). */
export function bucketExtents(
  buckets: HistBucket[]
): { min: number; max: number } | null {
  let min = Infinity
  let max = -Infinity
  let found = false
  for (const b of buckets) {
    if (b.cnt <= 0) continue
    found = true
    const lo = bucketLoForExtent(b)
    const hi = bucketHiForExtent(b)
    if (lo < min) min = lo
    if (hi > max) max = hi
  }
  if (!found || !Number.isFinite(min) || !Number.isFinite(max)) return null
  return { min, max }
}

function bucketTotal(buckets: HistBucket[]): number {
  let total = 0
  for (const b of buckets) total += b.cnt
  return total
}

function bucketQuantile(
  buckets: HistBucket[],
  q: number,
  interp: typeof interpLinear
): number | null {
  if (buckets.length === 0) return null
  const total = bucketTotal(buckets)
  if (total <= 0) return null
  const target = q * total
  let accPrev = 0
  for (const b of buckets) {
    const acc = accPrev + b.cnt
    if (acc >= target) {
      return interp(b.lo, b.hi, accPrev, b.cnt, target)
    }
    accPrev = acc
  }
  return null
}

export function bucketQuantileLinear(
  buckets: HistBucket[],
  q: number
): number | null {
  return bucketQuantile(buckets, q, interpLinear)
}

export function bucketQuantileLoglin(
  buckets: HistBucket[],
  q: number
): number | null {
  return bucketQuantile(buckets, q, interpLoglin)
}

export function histQuantile(
  bounds: number[] | null | undefined,
  counts: number[] | null | undefined,
  q: number
): number | null {
  if (!bounds || !counts || bounds.length === 0 || counts.length === 0) {
    return null
  }
  return bucketQuantileLinear(histBuckets(bounds, counts), q)
}

export function expHistQuantile(
  scale: number,
  negOffset: number,
  negCounts: number[],
  zeroCount: number,
  posOffset: number,
  posCounts: number[],
  q: number
): number | null {
  return bucketQuantileLoglin(
    expBuckets(scale, negOffset, negCounts, zeroCount, posOffset, posCounts),
    q
  )
}

/** Keys match Go strconv.FormatFloat(q, 'f', -1, 64). */
export function quantileRecord(
  quantiles: number[],
  values: (number | null)[]
): Record<string, number | null> {
  const out: Record<string, number | null> = {}
  for (let i = 0; i < quantiles.length; i++) {
    const key = String(quantiles[i])
    out[key] = values[i] ?? null
  }
  return out
}

export function histQuantileRecord(
  bounds: number[],
  counts: number[],
  quantiles: number[]
): Record<string, number | null> {
  return quantileRecord(
    quantiles,
    quantiles.map(q => histQuantile(bounds, counts, q))
  )
}

export function expHistQuantileRecord(
  scale: number,
  negOffset: number,
  negCounts: number[],
  zeroCount: number,
  posOffset: number,
  posCounts: number[],
  quantiles: number[]
): Record<string, number | null> {
  return quantileRecord(
    quantiles,
    quantiles.map(q =>
      expHistQuantile(
        scale,
        negOffset,
        negCounts,
        zeroCount,
        posOffset,
        posCounts,
        q
      )
    )
  )
}
