// Quantile interpolation for explicit and exponential histograms.
// Ports DuckDB macros from desktopexporter/internal/store/schema/schema.go.

export type HistBucket = { lo: number; hi: number; cnt: number }

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
